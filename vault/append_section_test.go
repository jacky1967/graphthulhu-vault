package vault

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rwSectionVault returns a Client rooted at a fresh t.TempDir with one seeded
// page containing several headings, suitable for AppendBlockToSection tests.
func rwSectionVault(t *testing.T, body string) *Client {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("seed page: %v", err)
	}
	c := New(dir)
	if err := c.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	c.BuildBacklinks()
	return c
}

const sampleReport = `# Report

intro paragraph

## Section A

paragraph A1

paragraph A2

## Section B

paragraph B1

### Sub of B

deep paragraph

## Section C

paragraph C1
`

func TestFindHeadingSection_HappyPath(t *testing.T) {
	lines := strings.Split(sampleReport, "\n")
	start, end, level := findHeadingSection(lines, "Section B")
	if level != 2 {
		t.Errorf("level = %d, want 2", level)
	}
	if start < 0 {
		t.Fatalf("not found")
	}
	// section ends at "## Section C"
	if !strings.HasPrefix(lines[end], "## Section C") {
		t.Errorf("end line = %q, want '## Section C'", lines[end])
	}
	// start = line after "## Section B"
	if !strings.Contains(strings.Join(lines[start:end], "\n"), "paragraph B1") {
		t.Errorf("section content missing B1: %q", lines[start:end])
	}
	// section should INCLUDE the H3 sub-heading and its content
	if !strings.Contains(strings.Join(lines[start:end], "\n"), "deep paragraph") {
		t.Errorf("section should include deep paragraph (sub-heading is deeper)")
	}
}

func TestFindHeadingSection_NotFound(t *testing.T) {
	lines := strings.Split(sampleReport, "\n")
	start, _, _ := findHeadingSection(lines, "Nonexistent")
	if start != -1 {
		t.Errorf("expected -1 for missing heading, got %d", start)
	}
}

func TestFindHeadingSection_FirstMatchWins(t *testing.T) {
	body := `# A

stuff

## Same

first

# B

## Same

second
`
	lines := strings.Split(body, "\n")
	start, end, _ := findHeadingSection(lines, "Same")
	section := strings.Join(lines[start:end], "\n")
	if !strings.Contains(section, "first") {
		t.Errorf("expected first match to contain 'first': %q", section)
	}
	if strings.Contains(section, "second") {
		t.Errorf("first match should not contain 'second': %q", section)
	}
}

func TestFindHeadingSection_IgnoresCodeBlocks(t *testing.T) {
	body := "# A\n\n```\n## NotAHeading\n```\n\n## Real\n\ncontent\n"
	lines := strings.Split(body, "\n")
	start, _, _ := findHeadingSection(lines, "NotAHeading")
	if start != -1 {
		t.Errorf("heading inside code block should not match")
	}
	start, _, _ = findHeadingSection(lines, "Real")
	if start < 0 {
		t.Errorf("real heading should match")
	}
}

func TestFindHeadingSection_TrailingHashesStripped(t *testing.T) {
	body := "## Section ##\n\nbody\n"
	lines := strings.Split(body, "\n")
	start, _, _ := findHeadingSection(lines, "Section")
	if start < 0 {
		t.Errorf("trailing #s should be stripped")
	}
}

func TestAppendBlockToSection_HappyPath(t *testing.T) {
	c := rwSectionVault(t, sampleReport)

	res, err := c.AppendBlockToSection(context.Background(), "report", "new entry under A", "Section A", false)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if res.Skipped {
		t.Fatal("should not be skipped")
	}
	if res.Block == nil || res.Block.UUID == "" {
		t.Fatal("missing block/uuid")
	}

	got, err := os.ReadFile(filepath.Join(c.vaultPath, "report.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(got)
	if !strings.Contains(content, "new entry under A") {
		t.Errorf("missing inserted content: %q", content)
	}

	// Insertion must be inside Section A (before Section B).
	idxNew := strings.Index(content, "new entry under A")
	idxB := strings.Index(content, "## Section B")
	if idxNew < 0 || idxB < 0 || idxNew >= idxB {
		t.Errorf("content not inserted inside Section A (idxNew=%d, idxB=%d):\n%s", idxNew, idxB, content)
	}

	// Existing content must be preserved.
	if !strings.Contains(content, "paragraph A1") || !strings.Contains(content, "paragraph A2") {
		t.Errorf("preserved content missing: %s", content)
	}
}

func TestAppendBlockToSection_HeadingNotFound(t *testing.T) {
	c := rwSectionVault(t, sampleReport)

	_, err := c.AppendBlockToSection(context.Background(), "report", "x", "Nonexistent", false)
	if err == nil || !strings.Contains(err.Error(), "heading not found") {
		t.Errorf("err = %v, want 'heading not found'", err)
	}
}

func TestAppendBlockToSection_PageNotFound(t *testing.T) {
	c := rwSectionVault(t, sampleReport)
	_, err := c.AppendBlockToSection(context.Background(), "missing-page", "x", "Section A", false)
	if err == nil || !strings.Contains(err.Error(), "page not found") {
		t.Errorf("err = %v, want 'page not found'", err)
	}
}

func TestAppendBlockToSection_EmptyHeading(t *testing.T) {
	c := rwSectionVault(t, sampleReport)
	_, err := c.AppendBlockToSection(context.Background(), "report", "x", "", false)
	if err == nil {
		t.Fatal("expected error on empty targetHeading")
	}
}

func TestAppendBlockToSection_EmptyContent(t *testing.T) {
	c := rwSectionVault(t, sampleReport)
	_, err := c.AppendBlockToSection(context.Background(), "report", "   ", "Section A", false)
	if err == nil {
		t.Fatal("expected error on empty content")
	}
}

func TestAppendBlockToSection_SkipIfPresent_Match(t *testing.T) {
	c := rwSectionVault(t, sampleReport)

	// First append.
	if _, err := c.AppendBlockToSection(context.Background(), "report", "log entry XYZ", "Section A", true); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Second append with same content + skipIfPresent → must skip.
	res, err := c.AppendBlockToSection(context.Background(), "report", "log entry XYZ", "Section A", true)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected skipped=true on duplicate content")
	}
	// File should contain the entry exactly once.
	got, _ := os.ReadFile(filepath.Join(c.vaultPath, "report.md"))
	count := strings.Count(string(got), "log entry XYZ")
	if count != 1 {
		t.Errorf("expected 1 occurrence, got %d", count)
	}
}

func TestAppendBlockToSection_SkipIfPresent_NoMatch(t *testing.T) {
	c := rwSectionVault(t, sampleReport)

	if _, err := c.AppendBlockToSection(context.Background(), "report", "first entry", "Section A", true); err != nil {
		t.Fatalf("first: %v", err)
	}
	res, err := c.AppendBlockToSection(context.Background(), "report", "second entry", "Section A", true)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if res.Skipped {
		t.Error("should not skip — different content")
	}
	got, _ := os.ReadFile(filepath.Join(c.vaultPath, "report.md"))
	if !strings.Contains(string(got), "first entry") || !strings.Contains(string(got), "second entry") {
		t.Errorf("both entries should be present: %s", got)
	}
}

func TestAppendBlockToSection_SkipIfPresent_IgnoresEmbeddedUUID(t *testing.T) {
	// A previously-appended block carries an embedded UUID; a subsequent skipIfPresent
	// call with the same logical content (no UUID) must still be detected as a duplicate.
	c := rwSectionVault(t, sampleReport)

	if _, err := c.AppendBlockToSection(context.Background(), "report", "duplicate-detection-test", "Section A", false); err != nil {
		t.Fatalf("first: %v", err)
	}

	res, err := c.AppendBlockToSection(context.Background(), "report", "duplicate-detection-test", "Section A", true)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !res.Skipped {
		t.Error("expected skipped=true (embedded UUID should be normalized away for comparison)")
	}
}

func TestAppendBlockToSection_AppendsAtEndOfSection(t *testing.T) {
	body := `# A

## Section A

para1

para2

## Section B

paraB
`
	c := rwSectionVault(t, body)

	if _, err := c.AppendBlockToSection(context.Background(), "report", "newest", "Section A", false); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(c.vaultPath, "report.md"))
	content := string(got)

	// Sanity: para2 still before newest, newest still before Section B.
	idxPara2 := strings.Index(content, "para2")
	idxNewest := strings.Index(content, "newest")
	idxB := strings.Index(content, "## Section B")
	if !(idxPara2 < idxNewest && idxNewest < idxB) {
		t.Errorf("ordering wrong (para2=%d newest=%d B=%d):\n%s", idxPara2, idxNewest, idxB, content)
	}
}
