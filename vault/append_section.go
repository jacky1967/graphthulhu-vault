package vault

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/skridlevsky/graphthulhu/types"
)

// AppendSectionResult is returned by AppendBlockToSection.
type AppendSectionResult struct {
	Skipped bool                `json:"skipped"`
	Block   *types.BlockEntity  `json:"block,omitempty"`
}

// findHeadingSection returns (start, end, level) for the first occurrence of a
// markdown heading whose text (after stripping leading #s and whitespace) equals
// targetHeading exactly (case-sensitive). start is the line index AFTER the
// heading; end is the line index of the next heading at the same or shallower
// level, or len(lines) if none exists. Code-fenced blocks are skipped while
// scanning for headings. Returns -1, -1, -1 when not found.
func findHeadingSection(lines []string, targetHeading string) (start, end, level int) {
	target := strings.TrimSpace(targetHeading)
	if target == "" {
		return -1, -1, -1
	}

	inCode := false
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}

		hashes, text, ok := parseHeading(trimmed)
		if !ok {
			continue
		}
		if text != target {
			continue
		}

		level = hashes
		start = i + 1

		// Find the next heading of level <= level.
		inCode2 := false
		for j := start; j < len(lines); j++ {
			tj := strings.TrimLeft(lines[j], " \t")
			if strings.HasPrefix(tj, "```") || strings.HasPrefix(tj, "~~~") {
				inCode2 = !inCode2
				continue
			}
			if inCode2 {
				continue
			}
			h, _, ok := parseHeading(tj)
			if !ok {
				continue
			}
			if h <= level {
				return start, j, level
			}
		}
		return start, len(lines), level
	}
	return -1, -1, -1
}

// parseHeading returns the heading level (1..6), the trimmed heading text, and
// ok=true if the line is a markdown ATX heading. The line should already have
// its leading whitespace removed.
func parseHeading(line string) (level int, text string, ok bool) {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return 0, "", false
	}
	if n >= len(line) || (line[n] != ' ' && line[n] != '\t') {
		return 0, "", false
	}
	rawText := strings.TrimSpace(line[n+1:])
	// ATX headings allow optional trailing #s; strip them per CommonMark.
	rawText = strings.TrimRight(rawText, " \t")
	rawText = strings.TrimRight(rawText, "#")
	rawText = strings.TrimRight(rawText, " \t")
	return n, rawText, true
}

// AppendBlockToSection appends content as a new block under the named heading.
// Returns Skipped=true (without writing) when skipIfPresent is true and the
// trimmed content already appears as a paragraph block in the target section.
// The heading is never created — an error is returned if it doesn't exist.
func (c *Client) AppendBlockToSection(_ context.Context, page, content, targetHeading string, skipIfPresent bool) (*AppendSectionResult, error) {
	if strings.TrimSpace(targetHeading) == "" {
		return nil, fmt.Errorf("targetHeading is required")
	}
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("content is empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	lowerName := strings.ToLower(page)
	cached, exists := c.pages[lowerName]
	if !exists {
		return nil, fmt.Errorf("page not found: %s", page)
	}

	relPath := cached.filePath
	absPath, err := c.safePath(relPath)
	if err != nil {
		return nil, err
	}

	existing, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	lines := strings.Split(string(existing), "\n")
	start, end, _ := findHeadingSection(lines, targetHeading)
	if start < 0 {
		return nil, fmt.Errorf("heading not found: %s", targetHeading)
	}

	if skipIfPresent {
		sectionLines := lines[start:end]
		if sectionContainsBlock(sectionLines, content) {
			return &AppendSectionResult{Skipped: true}, nil
		}
	}

	// Embed (or reuse) UUID just like AppendBlockInPage does.
	blockUUID, cleanContent := extractUUID(content)
	if blockUUID == "" {
		blockUUID = generateRandomUUID()
		content = embedUUID(cleanContent, blockUUID)
	} else {
		content = cleanContent
	}

	// Splice the new content at the end of the section.
	// We trim trailing blank lines from the section and ensure exactly one blank
	// line separates the new block from existing content.
	trimEnd := end
	for trimEnd > start && strings.TrimSpace(lines[trimEnd-1]) == "" {
		trimEnd--
	}

	insert := []string{}
	if trimEnd > start {
		insert = append(insert, "")
	}
	insert = append(insert, strings.Split(content, "\n")...)
	if end < len(lines) {
		insert = append(insert, "")
	}

	newLines := make([]string, 0, len(lines)+len(insert))
	newLines = append(newLines, lines[:trimEnd]...)
	newLines = append(newLines, insert...)
	newLines = append(newLines, lines[end:]...)

	newContent := strings.Join(newLines, "\n")

	if err := atomicWrite(absPath, newContent); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	info, _ := os.Stat(absPath)
	c.indexFileCore(relPath, newContent, info)
	c.rebuildLinksLocked()

	return &AppendSectionResult{
		Skipped: false,
		Block: &types.BlockEntity{
			UUID:    blockUUID,
			Content: cleanContent,
		},
	}, nil
}

// sectionContainsBlock returns true when content (whitespace-trimmed) matches a
// paragraph block within sectionLines (paragraphs separated by blank lines),
// using whitespace-trimmed comparison.
func sectionContainsBlock(sectionLines []string, content string) bool {
	target := strings.TrimSpace(content)
	if target == "" {
		return false
	}

	var paragraph []string
	flush := func() bool {
		if len(paragraph) == 0 {
			return false
		}
		got := strings.TrimSpace(strings.Join(paragraph, "\n"))
		// Strip embedded UUID so a re-append of the same logical content still matches.
		if _, clean := extractUUID(got); clean != "" {
			got = strings.TrimSpace(clean)
		}
		paragraph = paragraph[:0]
		return got == target
	}

	for _, line := range sectionLines {
		if strings.TrimSpace(line) == "" {
			if flush() {
				return true
			}
			continue
		}
		paragraph = append(paragraph, line)
	}
	return flush()
}
