package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skridlevsky/graphthulhu/types"
	"github.com/skridlevsky/graphthulhu/vault"
)

func newTestRawPage(t *testing.T) (*RawPage, string) {
	t.Helper()
	dir := t.TempDir()
	c := vault.New(dir)
	if err := c.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	return NewRawPage(c), dir
}

func TestRawPageWrite_HappyPath(t *testing.T) {
	r, dir := newTestRawPage(t)

	res, _, err := r.Write(context.Background(), nil, types.WriteRawPageInput{
		Name:    "report",
		Content: "# Report\n\nbody\n",
	})
	if err != nil || res.IsError {
		t.Fatalf("err=%v isError=%v", err, res.IsError)
	}
	if !strings.Contains(resultText(t, res), `"created": true`) {
		t.Errorf("missing created=true: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), `"path": "report.md"`) {
		t.Errorf("missing path: %s", resultText(t, res))
	}
	got, _ := os.ReadFile(filepath.Join(dir, "report.md"))
	if string(got) != "# Report\n\nbody\n" {
		t.Errorf("content lost on disk: %q", got)
	}
}

func TestRawPageWrite_EmptyName(t *testing.T) {
	r, _ := newTestRawPage(t)
	res, _, _ := r.Write(context.Background(), nil, types.WriteRawPageInput{Name: "", Content: "x"})
	if !res.IsError {
		t.Error("expected error on empty name")
	}
}

func TestRawPageWrite_RejectsMdSuffix(t *testing.T) {
	r, _ := newTestRawPage(t)
	res, _, _ := r.Write(context.Background(), nil, types.WriteRawPageInput{Name: "foo.md", Content: "x"})
	if !res.IsError {
		t.Error("expected error on .md suffix")
	}
}

func TestRawPageWrite_Overwrite(t *testing.T) {
	r, dir := newTestRawPage(t)

	if _, _, err := r.Write(context.Background(), nil, types.WriteRawPageInput{Name: "p", Content: "v1"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	res, _, _ := r.Write(context.Background(), nil, types.WriteRawPageInput{Name: "p", Content: "v2"})
	if res.IsError {
		t.Fatalf("second failed: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), `"created": false`) {
		t.Errorf("expected created=false on overwrite: %s", resultText(t, res))
	}
	got, _ := os.ReadFile(filepath.Join(dir, "p.md"))
	if string(got) != "v2" {
		t.Errorf("content = %q, want v2", got)
	}
}

func TestRawPageRead_HappyPath(t *testing.T) {
	r, dir := newTestRawPage(t)

	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Hi\n\nstuff"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, _, err := r.Read(context.Background(), nil, types.ReadRawPageInput{Name: "doc"})
	if err != nil || res.IsError {
		t.Fatalf("err=%v isError=%v body=%s", err, res.IsError, resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), `"# Hi\n\nstuff"`) {
		t.Errorf("missing content in response: %s", resultText(t, res))
	}
}

func TestRawPageRead_NotFound(t *testing.T) {
	r, _ := newTestRawPage(t)
	res, _, _ := r.Read(context.Background(), nil, types.ReadRawPageInput{Name: "missing"})
	if !res.IsError {
		t.Error("expected error on missing page")
	}
}

func TestRawPageRead_EmptyName(t *testing.T) {
	r, _ := newTestRawPage(t)
	res, _, _ := r.Read(context.Background(), nil, types.ReadRawPageInput{Name: ""})
	if !res.IsError {
		t.Error("expected error on empty name")
	}
}

func TestRawPageRead_RejectsMdSuffix(t *testing.T) {
	r, _ := newTestRawPage(t)
	res, _, _ := r.Read(context.Background(), nil, types.ReadRawPageInput{Name: "foo.md"})
	if !res.IsError {
		t.Error("expected error on .md suffix")
	}
}
