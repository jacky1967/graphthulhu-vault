package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skridlevsky/graphthulhu/types"
	"github.com/skridlevsky/graphthulhu/vault"
)

// AppendSection implements the append_to_section MCP tool (Obsidian-only).
type AppendSection struct {
	client *vault.Client
}

// NewAppendSection creates a new AppendSection tool handler.
func NewAppendSection(c *vault.Client) *AppendSection {
	return &AppendSection{client: c}
}

// Run appends content under a target heading on a page, with optional
// idempotency via skipIfPresent.
func (a *AppendSection) Run(ctx context.Context, req *mcp.CallToolRequest, input types.AppendToSectionInput) (*mcp.CallToolResult, any, error) {
	if input.Page == "" {
		return errorResult("page is required"), nil, nil
	}
	if input.TargetHeading == "" {
		return errorResult("targetHeading is required"), nil, nil
	}
	if input.Content == "" {
		return errorResult("content is required"), nil, nil
	}

	res, err := a.client.AppendBlockToSection(ctx, input.Page, input.Content, input.TargetHeading, input.SkipIfPresent)
	if err != nil {
		return errorResult(fmt.Sprintf("append_to_section failed: %v", err)), nil, nil
	}

	out := map[string]any{
		"page":          input.Page,
		"targetHeading": input.TargetHeading,
		"skipped":       res.Skipped,
	}
	if res.Block != nil {
		out["uuid"] = res.Block.UUID
	}

	result, err := jsonTextResult(out)
	return result, nil, err
}
