package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skridlevsky/graphthulhu/backend"
	"github.com/skridlevsky/graphthulhu/parser"
	"github.com/skridlevsky/graphthulhu/types"
	"github.com/skridlevsky/graphthulhu/vault"
)

// VaultObsidianCompat regroupe les outils nommes selon la convention
// vault-obsidian v1.1.0 (TS, projet frere) que graphthulhu-vault remplace
// cote Hermes (bd-s5c Phase 6). Les renames triviaux (get_note, list_notes,
// search_by_tag) sont enregistres directement dans server.go en re-utilisant
// les handlers existants. Ce fichier hebergent les wrappers qui forcent un
// parametre (get_outgoing_links, get_backlinks) et les nouvelles
// implementations specifiques (get_neighbors, read_note_metadata, create_note).
type VaultObsidianCompat struct {
	nav    *Navigate
	vault  *vault.Client
	client backend.Backend
}

// NewVaultObsidianCompat cree un set d'outils compat vault-obsidian.
// vaultClient peut etre nil si on est sur un backend Logseq — dans ce cas
// create_note (qui s'appuie sur write_raw_page Obsidian-only) ne sera pas
// enregistre dans server.go.
func NewVaultObsidianCompat(c backend.Backend, nav *Navigate, vaultClient *vault.Client) *VaultObsidianCompat {
	return &VaultObsidianCompat{
		nav:    nav,
		vault:  vaultClient,
		client: c,
	}
}

// GetOutgoingLinks retourne uniquement les liens sortants d'une page.
// Wrapper sur GetLinks(direction=forward).
func (v *VaultObsidianCompat) GetOutgoingLinks(ctx context.Context, req *mcp.CallToolRequest, input types.GetOutgoingLinksInput) (*mcp.CallToolResult, any, error) {
	return v.nav.GetLinks(ctx, req, types.GetLinksInput{
		Name:      input.Name,
		Direction: "forward",
	})
}

// GetBacklinks retourne uniquement les notes qui pointent vers la cible.
// Wrapper sur GetLinks(direction=backward) avec truncate cote payload.
func (v *VaultObsidianCompat) GetBacklinks(ctx context.Context, req *mcp.CallToolRequest, input types.GetBacklinksInput) (*mcp.CallToolResult, any, error) {
	return v.nav.GetLinks(ctx, req, types.GetLinksInput{
		Name:      input.Name,
		Direction: "backward",
	})
}

// GetNeighbors retourne le voisinage d'une page via BFS sur le graphe de
// liens. Equivalent semantique de l'outil vault-obsidian v1.1.0 du meme nom :
// reponse a "qu'est-ce qui est lie a X" (vs "qu'est-ce qui ressemble a X"
// pour vault-rag-perso).
func (v *VaultObsidianCompat) GetNeighbors(ctx context.Context, req *mcp.CallToolRequest, input types.GetNeighborsInput) (*mcp.CallToolResult, any, error) {
	if input.Name == "" {
		return errorResult("name is required"), nil, nil
	}

	depth := input.Depth
	if depth <= 0 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	direction := input.Direction
	if direction == "" {
		direction = "both"
	}
	if direction != "forward" && direction != "backward" && direction != "both" {
		return errorResult(fmt.Sprintf("invalid direction: %s (expected forward, backward or both)", direction)), nil, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	type node struct {
		name     string
		distance int
	}

	type edge struct {
		from string
		to   string
	}

	originLower := strings.ToLower(input.Name)
	visited := map[string]int{originLower: 0} // nodeName -> distance
	originalNames := map[string]string{originLower: input.Name}
	queue := []node{{name: input.Name, distance: 0}}
	var edges []edge

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.distance >= depth {
			continue
		}

		// Forward neighbors (outgoing links from current).
		if direction == "forward" || direction == "both" {
			blocks, err := v.client.GetPageBlocksTree(ctx, current.name)
			if err == nil {
				for _, link := range collectAllLinks(blocks) {
					linkLower := strings.ToLower(link)
					edges = append(edges, edge{from: current.name, to: link})
					if _, seen := visited[linkLower]; !seen {
						visited[linkLower] = current.distance + 1
						originalNames[linkLower] = link
						queue = append(queue, node{name: link, distance: current.distance + 1})
					}
				}
			}
		}

		// Backward neighbors (pages linking to current).
		if direction == "backward" || direction == "both" {
			backlinks := v.nav.getBacklinks(ctx, current.name)
			for _, bl := range backlinks {
				blName := bl.PageName
				if blName == "" {
					continue
				}
				blLower := strings.ToLower(blName)
				edges = append(edges, edge{from: blName, to: current.name})
				if _, seen := visited[blLower]; !seen {
					visited[blLower] = current.distance + 1
					originalNames[blLower] = blName
					queue = append(queue, node{name: blName, distance: current.distance + 1})
				}
			}
		}
	}

	// Construire la liste de nodes (hors origine), tri par distance puis nom.
	type neighborNode struct {
		Name     string `json:"name"`
		Distance int    `json:"distance"`
	}
	var nodes []neighborNode
	for lower, dist := range visited {
		if lower == originLower {
			continue
		}
		nodes = append(nodes, neighborNode{Name: originalNames[lower], Distance: dist})
	}
	// Tri stable par (distance, name).
	for i := 1; i < len(nodes); i++ {
		for j := i; j > 0 && (nodes[j].Distance < nodes[j-1].Distance ||
			(nodes[j].Distance == nodes[j-1].Distance && strings.ToLower(nodes[j].Name) < strings.ToLower(nodes[j-1].Name))); j-- {
			nodes[j], nodes[j-1] = nodes[j-1], nodes[j]
		}
	}
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}

	// Dedup edges + format pour serialisation JSON.
	type edgeOut struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	seenEdges := map[string]bool{}
	var edgesOut []edgeOut
	for _, e := range edges {
		key := e.from + "->" + e.to
		if seenEdges[key] {
			continue
		}
		seenEdges[key] = true
		edgesOut = append(edgesOut, edgeOut{From: e.from, To: e.to})
	}

	result := map[string]any{
		"origin":    input.Name,
		"depth":     depth,
		"direction": direction,
		"nodes":     nodes,
		"edges":     edgesOut,
	}

	res, err := jsonTextResult(result)
	return res, nil, err
}

// ReadNoteMetadata retourne uniquement le frontmatter / properties d'une page,
// sans le block tree. Utile en pattern de reconciliation post-write des skills
// (verifier qu'un champ frontmatter a bien ete ecrit) sans payer le cout du
// get_page complet.
func (v *VaultObsidianCompat) ReadNoteMetadata(ctx context.Context, req *mcp.CallToolRequest, input types.ReadNoteMetadataInput) (*mcp.CallToolResult, any, error) {
	if input.Name == "" {
		return errorResult("name is required"), nil, nil
	}
	page, err := v.client.GetPage(ctx, input.Name)
	if err != nil {
		return errorResult(fmt.Sprintf("page not found: %s — %v", input.Name, err)), nil, nil
	}
	if page == nil {
		return errorResult(fmt.Sprintf("page not found: %s", input.Name)), nil, nil
	}

	// Tags = collecte sur tous les blocs (cohérent avec le critère pageHasTag).
	tags := collectPageTags(ctx, v.client, input.Name)

	result := map[string]any{
		"name":         page.OriginalName,
		"properties":   page.Properties,
		"tags":         tags,
		"journal":      page.Journal,
	}
	if page.OriginalName == "" {
		result["name"] = page.Name
	}
	if page.UpdatedAt > 0 {
		result["updatedAt"] = page.UpdatedAt
	}
	if page.CreatedAt > 0 {
		result["createdAt"] = page.CreatedAt
	}

	res, err := jsonTextResult(result)
	return res, nil, err
}

// CreateNote ecrit une note markdown verbatim (frontmatter + body) via la
// primitive write_raw_page. Wrapper preferentiel pour les skills d'ecriture
// qui cherchent un equivalent au create_note de vault-obsidian (signature
// {name, content}, pas {name, properties, blocks}).
func (v *VaultObsidianCompat) CreateNote(ctx context.Context, req *mcp.CallToolRequest, input types.CreateNoteInput) (*mcp.CallToolResult, any, error) {
	if v.vault == nil {
		return errorResult("create_note requires Obsidian backend (vault.Client)"), nil, nil
	}
	if input.Name == "" {
		return errorResult("name is required"), nil, nil
	}
	result, err := v.vault.WriteRawPage(ctx, input.Name, input.Content)
	if err != nil {
		return errorResult(fmt.Sprintf("create_note failed: %v", err)), nil, nil
	}
	res, err := jsonTextResult(result)
	return res, nil, err
}

// collectPageTags walks the block tree and collects unique tags.
func collectPageTags(ctx context.Context, b backend.Backend, name string) []string {
	blocks, err := b.GetPageBlocksTree(ctx, name)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var tags []string
	var walk func([]types.BlockEntity)
	walk = func(bs []types.BlockEntity) {
		for _, blk := range bs {
			parsed := parser.Parse(blk.Content)
			for _, t := range parsed.Tags {
				if !seen[t] {
					seen[t] = true
					tags = append(tags, t)
				}
			}
			if len(blk.Children) > 0 {
				walk(blk.Children)
			}
		}
	}
	walk(blocks)
	return tags
}
