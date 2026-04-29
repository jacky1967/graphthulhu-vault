package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skridlevsky/graphthulhu/types"
	"github.com/skridlevsky/graphthulhu/vault"
)

func newTestCompat(t *testing.T, files map[string]string) (*VaultObsidianCompat, *Navigate, *vault.Client, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		fp := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	c := vault.New(dir)
	if err := c.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	c.BuildBacklinks()
	nav := NewNavigate(c)
	compat := NewVaultObsidianCompat(c, nav, c)
	return compat, nav, c, dir
}

func compatResultText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if r == nil || len(r.Content) == 0 {
		return ""
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is not TextContent")
	}
	return tc.Text
}

// --- get_outgoing_links ----------------------------------------------------

func TestGetOutgoingLinks_Forward(t *testing.T) {
	compat, _, _, _ := newTestCompat(t, map[string]string{
		"hub.md":  "# Hub\n\n- Voir [[a]] et [[b]].\n",
		"a.md":    "# A\n\n- voir [[c]]\n",
		"b.md":    "# B\n",
		"c.md":    "# C\n",
	})
	res, _, err := compat.GetOutgoingLinks(context.Background(), nil, types.GetOutgoingLinksInput{Name: "hub"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("err=%v body=%s", err, compatResultText(t, res))
	}
	body := compatResultText(t, res)
	if !strings.Contains(body, `"outgoingLinks"`) {
		t.Errorf("expected outgoingLinks key, got %s", body)
	}
	if strings.Contains(body, `"backlinks"`) {
		t.Errorf("get_outgoing_links must NOT include backlinks (direction=forward), got %s", body)
	}
	if !strings.Contains(body, `"a"`) || !strings.Contains(body, `"b"`) {
		t.Errorf("expected links to a and b, got %s", body)
	}
}

// --- get_backlinks ---------------------------------------------------------

func TestGetBacklinks_Backward(t *testing.T) {
	compat, _, _, _ := newTestCompat(t, map[string]string{
		"target.md": "# Target\n",
		"linker1.md": "# Linker1\n\n- pointe vers [[target]]\n",
		"linker2.md": "# Linker2\n\n- aussi [[target]] cite ici\n",
		"unrelated.md": "# Unrelated\n",
	})
	res, _, err := compat.GetBacklinks(context.Background(), nil, types.GetBacklinksInput{Name: "target"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("err=%v body=%s", err, compatResultText(t, res))
	}
	body := compatResultText(t, res)
	if !strings.Contains(body, `"backlinks"`) {
		t.Errorf("expected backlinks key, got %s", body)
	}
	if strings.Contains(body, `"outgoingLinks"`) {
		t.Errorf("get_backlinks must NOT include outgoingLinks (direction=backward), got %s", body)
	}
	if !strings.Contains(body, "Linker1") || !strings.Contains(body, "Linker2") {
		t.Errorf("expected both linkers as backlinks, got %s", body)
	}
}

// --- get_neighbors ---------------------------------------------------------

func TestGetNeighbors_Depth1Both(t *testing.T) {
	compat, _, _, _ := newTestCompat(t, map[string]string{
		"center.md": "# Center\n\n- vers [[forward1]] et [[forward2]]\n",
		"forward1.md": "# F1\n",
		"forward2.md": "# F2\n",
		"backward1.md": "# B1\n\n- vers [[center]]\n",
		"backward2.md": "# B2\n\n- citation [[center]]\n",
		"unrelated.md": "# U\n",
	})
	res, _, err := compat.GetNeighbors(context.Background(), nil, types.GetNeighborsInput{Name: "center", Depth: 1, Direction: "both"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("err=%v body=%s", err, compatResultText(t, res))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(compatResultText(t, res)), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	nodes, _ := payload["nodes"].([]any)
	names := map[string]bool{}
	for _, n := range nodes {
		nm, _ := n.(map[string]any)
		names[strings.ToLower(nm["name"].(string))] = true
	}
	for _, expected := range []string{"forward1", "forward2", "backward1", "backward2"} {
		if !names[expected] {
			t.Errorf("expected neighbor %s in nodes %v", expected, names)
		}
	}
	if names["unrelated"] {
		t.Errorf("unrelated should NOT be a neighbor")
	}
	if names["center"] {
		t.Errorf("origin (center) should not appear in neighbors list")
	}
}

func TestGetNeighbors_DirectionForwardOnly(t *testing.T) {
	compat, _, _, _ := newTestCompat(t, map[string]string{
		"center.md": "# Center\n\n- [[forward1]]\n",
		"forward1.md": "# F1\n",
		"backward1.md": "# B1\n\n- [[center]]\n",
	})
	res, _, _ := compat.GetNeighbors(context.Background(), nil, types.GetNeighborsInput{Name: "center", Depth: 1, Direction: "forward"})
	body := compatResultText(t, res)
	if !strings.Contains(body, `"forward1"`) {
		t.Errorf("expected forward1 in nodes, got %s", body)
	}
	if strings.Contains(body, `"backward1"`) {
		t.Errorf("backward1 must NOT appear with direction=forward, got %s", body)
	}
}

func TestGetNeighbors_DepthClampedTo3(t *testing.T) {
	compat, _, _, _ := newTestCompat(t, map[string]string{
		"a.md": "# A\n\n- [[b]]\n",
		"b.md": "# B\n",
	})
	// depth=10 doit etre clampe à 3 (sans erreur).
	res, _, err := compat.GetNeighbors(context.Background(), nil, types.GetNeighborsInput{Name: "a", Depth: 10})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("err=%v body=%s", err, compatResultText(t, res))
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(compatResultText(t, res)), &payload)
	if d, _ := payload["depth"].(float64); d != 3 {
		t.Errorf("expected depth clamped to 3, got %v", payload["depth"])
	}
}

func TestGetNeighbors_InvalidDirection(t *testing.T) {
	compat, _, _, _ := newTestCompat(t, map[string]string{"a.md": "# A\n"})
	res, _, _ := compat.GetNeighbors(context.Background(), nil, types.GetNeighborsInput{Name: "a", Direction: "sideways"})
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError for invalid direction, got %v", res)
	}
}

// --- read_note_metadata ----------------------------------------------------

func TestReadNoteMetadata_FrontmatterAndTags(t *testing.T) {
	body := `---
type: project
status: in_progress
memory_type: episodic
---

# Mon projet

- objectif :: livraison
- #important #projet/alpha
`
	compat, _, _, _ := newTestCompat(t, map[string]string{"projet.md": body})
	res, _, err := compat.ReadNoteMetadata(context.Background(), nil, types.ReadNoteMetadataInput{Name: "projet"})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("err=%v body=%s", err, compatResultText(t, res))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(compatResultText(t, res)), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	props, _ := payload["properties"].(map[string]any)
	if props["type"] != "project" || props["status"] != "in_progress" || props["memory_type"] != "episodic" {
		t.Errorf("frontmatter not exposed correctly: %v", props)
	}
	tags, _ := payload["tags"].([]any)
	tagSet := map[string]bool{}
	for _, t := range tags {
		tagSet[strings.ToLower(t.(string))] = true
	}
	if !tagSet["important"] {
		t.Errorf("expected #important in tags, got %v", tags)
	}
	// Ne doit PAS exposer 'blocks' ni 'outgoingLinks' (lightweight).
	if _, has := payload["blocks"]; has {
		t.Errorf("read_note_metadata must NOT expose blocks (lightweight contract)")
	}
}

func TestReadNoteMetadata_NotFound(t *testing.T) {
	compat, _, _, _ := newTestCompat(t, map[string]string{"a.md": "# A\n"})
	res, _, _ := compat.ReadNoteMetadata(context.Background(), nil, types.ReadNoteMetadataInput{Name: "absent"})
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError for missing page, got %v", res)
	}
}

// --- create_note -----------------------------------------------------------

func TestCreateNote_WritesRawMarkdown(t *testing.T) {
	compat, _, _, dir := newTestCompat(t, map[string]string{"existing.md": "# Existing\n"})
	content := "---\ntype: note\n---\n\n# Hello\n\nbody"
	res, _, err := compat.CreateNote(context.Background(), nil, types.CreateNoteInput{
		Name:    "00_Inbox/test_note",
		Content: content,
	})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("err=%v body=%s", err, compatResultText(t, res))
	}
	written, err := os.ReadFile(filepath.Join(dir, "00_Inbox", "test_note.md"))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(written) != content {
		t.Errorf("content mismatch:\nwant=%q\ngot =%q", content, string(written))
	}
}

func TestCreateNote_RequiresName(t *testing.T) {
	compat, _, _, _ := newTestCompat(t, map[string]string{})
	res, _, _ := compat.CreateNote(context.Background(), nil, types.CreateNoteInput{Name: "", Content: "x"})
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError for empty name")
	}
}
