package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skridlevsky/graphthulhu/types"
	"github.com/skridlevsky/graphthulhu/vault"
)

func newTestAppendSection(t *testing.T, body string) (*AppendSection, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c := vault.New(dir)
	if err := c.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	c.BuildBacklinks()
	return NewAppendSection(c), dir
}

const docBody = `# Title

intro

## Logs

old entry
`

func appendSectionResultText(t *testing.T, r *mcp.CallToolResult) string {
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

func TestAppendSection_HappyPath(t *testing.T) {
	a, dir := newTestAppendSection(t, docBody)
	res, _, err := a.Run(context.Background(), nil, types.AppendToSectionInput{
		Page:          "doc",
		Content:       "fresh entry",
		TargetHeading: "Logs",
	})
	if err != nil || res.IsError {
		t.Fatalf("err=%v isError=%v body=%s", err, res.IsError, appendSectionResultText(t, res))
	}
	text := appendSectionResultText(t, res)
	if !strings.Contains(text, `"skipped": false`) {
		t.Errorf("missing skipped=false: %s", text)
	}
	if !strings.Contains(text, `"uuid"`) {
		t.Errorf("missing uuid in response: %s", text)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "doc.md"))
	if !strings.Contains(string(got), "fresh entry") {
		t.Errorf("entry not written: %s", got)
	}
}

func TestAppendSection_EmptyPage(t *testing.T) {
	a, _ := newTestAppendSection(t, docBody)
	res, _, _ := a.Run(context.Background(), nil, types.AppendToSectionInput{
		Page: "", Content: "x", TargetHeading: "Logs",
	})
	if !res.IsError {
		t.Error("expected error on empty page")
	}
}

func TestAppendSection_EmptyHeading(t *testing.T) {
	a, _ := newTestAppendSection(t, docBody)
	res, _, _ := a.Run(context.Background(), nil, types.AppendToSectionInput{
		Page: "doc", Content: "x", TargetHeading: "",
	})
	if !res.IsError {
		t.Error("expected error on empty targetHeading")
	}
}

func TestAppendSection_EmptyContent(t *testing.T) {
	a, _ := newTestAppendSection(t, docBody)
	res, _, _ := a.Run(context.Background(), nil, types.AppendToSectionInput{
		Page: "doc", Content: "", TargetHeading: "Logs",
	})
	if !res.IsError {
		t.Error("expected error on empty content")
	}
}

func TestAppendSection_HeadingNotFound(t *testing.T) {
	a, _ := newTestAppendSection(t, docBody)
	res, _, _ := a.Run(context.Background(), nil, types.AppendToSectionInput{
		Page: "doc", Content: "x", TargetHeading: "Nonexistent",
	})
	if !res.IsError {
		t.Error("expected error when heading missing")
	}
	if !strings.Contains(appendSectionResultText(t, res), "heading not found") {
		t.Errorf("error message should mention 'heading not found': %s", appendSectionResultText(t, res))
	}
}

func TestAppendSection_PageNotFound(t *testing.T) {
	a, _ := newTestAppendSection(t, docBody)
	res, _, _ := a.Run(context.Background(), nil, types.AppendToSectionInput{
		Page: "missing", Content: "x", TargetHeading: "Logs",
	})
	if !res.IsError {
		t.Error("expected error when page missing")
	}
}

func TestAppendSection_SkipIfPresent(t *testing.T) {
	a, dir := newTestAppendSection(t, docBody)

	first, _, _ := a.Run(context.Background(), nil, types.AppendToSectionInput{
		Page: "doc", Content: "log marker UNIQ123", TargetHeading: "Logs", SkipIfPresent: true,
	})
	if first.IsError {
		t.Fatalf("first failed: %s", appendSectionResultText(t, first))
	}

	second, _, _ := a.Run(context.Background(), nil, types.AppendToSectionInput{
		Page: "doc", Content: "log marker UNIQ123", TargetHeading: "Logs", SkipIfPresent: true,
	})
	text := appendSectionResultText(t, second)
	if !strings.Contains(text, `"skipped": true`) {
		t.Errorf("expected skipped=true on duplicate: %s", text)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "doc.md"))
	if count := strings.Count(string(got), "UNIQ123"); count != 1 {
		t.Errorf("expected 1 occurrence of UNIQ123, got %d", count)
	}
}
