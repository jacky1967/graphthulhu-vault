package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skridlevsky/graphthulhu/types"
	"github.com/skridlevsky/graphthulhu/vault"
)

// RawPage implements the write_raw_page / read_raw_page MCP tools (Obsidian-only).
type RawPage struct {
	client *vault.Client
}

// NewRawPage creates a new RawPage tool handler.
func NewRawPage(c *vault.Client) *RawPage {
	return &RawPage{client: c}
}

// Write writes a markdown page verbatim (upsert).
func (r *RawPage) Write(ctx context.Context, req *mcp.CallToolRequest, input types.WriteRawPageInput) (*mcp.CallToolResult, any, error) {
	if input.Name == "" {
		return errorResult("name is required"), nil, nil
	}
	result, err := r.client.WriteRawPage(ctx, input.Name, input.Content)
	if err != nil {
		return errorResult(fmt.Sprintf("write_raw_page failed: %v", err)), nil, nil
	}
	res, err := jsonTextResult(result)
	return res, nil, err
}

// Read returns the verbatim markdown of a page.
func (r *RawPage) Read(ctx context.Context, req *mcp.CallToolRequest, input types.ReadRawPageInput) (*mcp.CallToolResult, any, error) {
	if input.Name == "" {
		return errorResult("name is required"), nil, nil
	}
	content, err := r.client.ReadRawPage(ctx, input.Name)
	if err != nil {
		return errorResult(fmt.Sprintf("read_raw_page failed: %v", err)), nil, nil
	}
	res, err := jsonTextResult(map[string]any{
		"name":    input.Name,
		"content": content,
	})
	return res, nil, err
}
