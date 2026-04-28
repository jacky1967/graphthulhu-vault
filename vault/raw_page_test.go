package vault

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rwRawVault(t *testing.T) *Client {
	t.Helper()
	dir := t.TempDir()
	c := New(dir)
	if err := c.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	return c
}

func TestWriteRawPage_HappyPath_Creates(t *testing.T) {
	c := rwRawVault(t)

	content := `---
type: report
tags: [scout]
---

# Rapport Scout — 2026-04-28

intro

## Section A

- item 1
- item 2

## Section B

paragraph
`
	res, err := c.WriteRawPage(context.Background(), "00_Inbox/Rapport_Scout_2026-04-28", content)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !res.Created {
		t.Error("Created should be true on first write")
	}
	if res.Name != "00_Inbox/Rapport_Scout_2026-04-28" {
		t.Errorf("name = %q", res.Name)
	}
	if res.Path != "00_Inbox/Rapport_Scout_2026-04-28.md" {
		t.Errorf("path = %q", res.Path)
	}

	got, err := os.ReadFile(filepath.Join(c.vaultPath, "00_Inbox", "Rapport_Scout_2026-04-28.md"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != content {
		t.Errorf("content roundtrip failed")
	}
}

func TestWriteRawPage_OverwritesIdempotently(t *testing.T) {
	c := rwRawVault(t)

	if _, err := c.WriteRawPage(context.Background(), "page", "first"); err != nil {
		t.Fatalf("first: %v", err)
	}
	res2, err := c.WriteRawPage(context.Background(), "page", "second-longer-content")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if res2.Created {
		t.Error("Created should be false on overwrite")
	}

	got, _ := os.ReadFile(filepath.Join(c.vaultPath, "page.md"))
	if string(got) != "second-longer-content" {
		t.Errorf("content = %q, want second-longer-content", got)
	}
}

func TestWriteRawPage_AutoCreatesParent(t *testing.T) {
	c := rwRawVault(t)
	_, err := c.WriteRawPage(context.Background(), "deep/nested/path/page", "content")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.vaultPath, "deep", "nested", "path", "page.md")); err != nil {
		t.Fatalf("page or parents not created: %v", err)
	}
}

func TestWriteRawPage_RejectsMdSuffix(t *testing.T) {
	c := rwRawVault(t)
	_, err := c.WriteRawPage(context.Background(), "page.md", "x")
	if err == nil || !strings.Contains(err.Error(), ".md") {
		t.Errorf("err = %v, want '.md' rejection", err)
	}
}

func TestWriteRawPage_RejectsTraversal(t *testing.T) {
	c := rwRawVault(t)
	_, err := c.WriteRawPage(context.Background(), "../escape", "x")
	if err == nil {
		t.Errorf("expected traversal rejection")
	}
}

func TestWriteRawPage_RejectsEmptyName(t *testing.T) {
	c := rwRawVault(t)
	_, err := c.WriteRawPage(context.Background(), "", "x")
	if err == nil {
		t.Errorf("expected error on empty name")
	}
}

func TestWriteRawPage_RefreshesIndex(t *testing.T) {
	// After WriteRawPage, GetPage must see the new content.
	c := rwRawVault(t)
	if _, err := c.WriteRawPage(context.Background(), "indexed", "# Indexed\n\nbody [[link-target]]\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	page, err := c.GetPage(context.Background(), "indexed")
	if err != nil {
		t.Fatalf("GetPage after write: %v", err)
	}
	if page == nil {
		t.Fatal("page not found in index after write")
	}
}

func TestReadRawPage_HappyPath(t *testing.T) {
	c := rwRawVault(t)
	const body = "# Hello\n\nworld\n"
	if _, err := c.WriteRawPage(context.Background(), "hello", body); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := c.ReadRawPage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != body {
		t.Errorf("content = %q, want %q", got, body)
	}
}

func TestReadRawPage_NotFound(t *testing.T) {
	c := rwRawVault(t)
	_, err := c.ReadRawPage(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "page not found") {
		t.Errorf("err = %v, want 'page not found'", err)
	}
}

func TestReadRawPage_RejectsMdSuffix(t *testing.T) {
	c := rwRawVault(t)
	_, err := c.ReadRawPage(context.Background(), "page.md")
	if err == nil || !strings.Contains(err.Error(), ".md") {
		t.Errorf("err = %v, want '.md' rejection", err)
	}
}

func TestReadRawPage_RejectsEmptyName(t *testing.T) {
	c := rwRawVault(t)
	_, err := c.ReadRawPage(context.Background(), "")
	if err == nil {
		t.Errorf("expected error on empty name")
	}
}

func TestReadRawPage_NestedPath(t *testing.T) {
	c := rwRawVault(t)
	if _, err := c.WriteRawPage(context.Background(), "00_Inbox/Archive_Rapports/Rapport_X", "statut: ok\nbody\n"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := c.ReadRawPage(context.Background(), "00_Inbox/Archive_Rapports/Rapport_X")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(got, "statut: ok") {
		t.Errorf("content lost: %q", got)
	}
}
