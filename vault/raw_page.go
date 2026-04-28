package vault

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RawPageResult is returned by WriteRawPage.
type RawPageResult struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Created bool   `json:"created"` // true if the page did not exist beforehand
}

// WriteRawPage writes a page file with the given markdown content verbatim.
// It is an upsert: a missing page is created (parent dirs auto-created),
// an existing page is replaced atomically.
//
// The name MUST NOT carry a .md suffix and MUST NOT escape the vault root.
// After the write, the in-memory index is refreshed and backlinks rebuilt
// so callers see consistent state immediately.
func (c *Client) WriteRawPage(_ context.Context, name, content string) (*RawPageResult, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("page name is required")
	}
	if strings.HasSuffix(strings.ToLower(name), ".md") {
		return nil, fmt.Errorf("name must not include .md suffix (got %q)", name)
	}
	if strings.ContainsAny(name, "\x00") {
		return nil, fmt.Errorf("invalid page name: contains null bytes")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	relPath := name + ".md"
	absPath, err := c.safePath(relPath)
	if err != nil {
		return nil, err
	}

	created := true
	if _, err := os.Stat(absPath); err == nil {
		created = false
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	if err := atomicWrite(absPath, content); err != nil {
		return nil, fmt.Errorf("write page: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat page: %w", err)
	}

	c.indexFileCore(relPath, content, info)
	c.rebuildLinksLocked()

	return &RawPageResult{
		Name:    name,
		Path:    filepath.ToSlash(relPath),
		Size:    info.Size(),
		Created: created,
	}, nil
}

// ReadRawPage returns the raw markdown content of a page (no parsing,
// no JSON wrapping). Useful when callers want to do their own light
// parsing (e.g. extracting a single frontmatter field) without paying
// the get_page block-tree machinery.
func (c *Client) ReadRawPage(_ context.Context, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("page name is required")
	}
	if strings.HasSuffix(strings.ToLower(name), ".md") {
		return "", fmt.Errorf("name must not include .md suffix (got %q)", name)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Resolve via the page index when present (handles aliases / case folding),
	// otherwise fall back to the literal name.md.
	lower := strings.ToLower(name)
	relPath := name + ".md"
	if cached, ok := c.pages[lower]; ok && cached.filePath != "" {
		relPath = cached.filePath
	}

	absPath, err := c.safePath(relPath)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("page not found: %s", name)
		}
		return "", fmt.Errorf("read page: %w", err)
	}
	return string(data), nil
}
