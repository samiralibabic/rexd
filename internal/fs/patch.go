package fs

import (
	"errors"
	"fmt"
	"strings"
)

const (
	patchBeginMarker = "*** Begin Patch"
	patchEndMarker   = "*** End Patch"
)

type PatchChunk struct {
	OldLines      []string
	NewLines      []string
	ChangeContext string
	IsEndOfFile   bool
}

type PatchHunk struct {
	Type     string
	Path     string
	MovePath string
	Contents string
	Chunks   []PatchChunk
}

type patchReplacement struct {
	Start   int
	OldLen  int
	NewLine []string
}

func ParsePatch(patchText string) ([]PatchHunk, error) {
	normalized := strings.ReplaceAll(patchText, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")

	beginIndex := -1
	endIndex := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if beginIndex == -1 && trimmed == patchBeginMarker {
			beginIndex = i
			continue
		}
		if beginIndex != -1 && trimmed == patchEndMarker {
			endIndex = i
			break
		}
	}

	if beginIndex == -1 || endIndex == -1 || beginIndex >= endIndex {
		return nil, errors.New("invalid patch format: missing Begin/End markers")
	}

	hunks := make([]PatchHunk, 0)
	for i := beginIndex + 1; i < endIndex; {
		line := lines[i]

		switch {
		case strings.HasPrefix(line, "*** Add File:"):
			filePath := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File:"))
			if filePath == "" {
				return nil, fmt.Errorf("invalid add file header at line %d", i+1)
			}
			i++
			contentLines := make([]string, 0)
			for i < endIndex && !strings.HasPrefix(lines[i], "***") {
				if strings.HasPrefix(lines[i], "+") {
					contentLines = append(contentLines, lines[i][1:])
				}
				i++
			}
			hunks = append(hunks, PatchHunk{
				Type:     "add",
				Path:     filePath,
				Contents: strings.Join(contentLines, "\n"),
			})

		case strings.HasPrefix(line, "*** Delete File:"):
			filePath := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File:"))
			if filePath == "" {
				return nil, fmt.Errorf("invalid delete file header at line %d", i+1)
			}
			hunks = append(hunks, PatchHunk{
				Type: "delete",
				Path: filePath,
			})
			i++

		case strings.HasPrefix(line, "*** Update File:"):
			filePath := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File:"))
			if filePath == "" {
				return nil, fmt.Errorf("invalid update file header at line %d", i+1)
			}
			i++

			movePath := ""
			if i < endIndex && strings.HasPrefix(lines[i], "*** Move to:") {
				movePath = strings.TrimSpace(strings.TrimPrefix(lines[i], "*** Move to:"))
				if movePath == "" {
					return nil, fmt.Errorf("invalid move target at line %d", i+1)
				}
				i++
			}

			chunks := make([]PatchChunk, 0)
			for i < endIndex && !strings.HasPrefix(lines[i], "***") {
				if !strings.HasPrefix(lines[i], "@@") {
					i++
					continue
				}

				context := strings.TrimSpace(strings.TrimPrefix(lines[i], "@@"))
				i++
				oldLines := make([]string, 0)
				newLines := make([]string, 0)
				isEndOfFile := false

				for i < endIndex && !strings.HasPrefix(lines[i], "@@") && !strings.HasPrefix(lines[i], "***") {
					changeLine := lines[i]
					if changeLine == "*** End of File" {
						isEndOfFile = true
						i++
						break
					}
					if changeLine == "" {
						i++
						continue
					}

					prefix := changeLine[0]
					content := ""
					if len(changeLine) > 1 {
						content = changeLine[1:]
					}

					switch prefix {
					case ' ':
						oldLines = append(oldLines, content)
						newLines = append(newLines, content)
					case '-':
						oldLines = append(oldLines, content)
					case '+':
						newLines = append(newLines, content)
					}
					i++
				}

				chunks = append(chunks, PatchChunk{
					OldLines:      oldLines,
					NewLines:      newLines,
					ChangeContext: context,
					IsEndOfFile:   isEndOfFile,
				})
			}

			hunks = append(hunks, PatchHunk{
				Type:     "update",
				Path:     filePath,
				MovePath: movePath,
				Chunks:   chunks,
			})

		default:
			i++
		}
	}

	if len(hunks) == 0 {
		return nil, errors.New("patch does not contain any hunks")
	}

	return hunks, nil
}

func DerivePatchedContent(original string, chunks []PatchChunk) (string, error) {
	originalLines := splitPatchLines(original)
	replacements, err := computePatchReplacements(originalLines, chunks)
	if err != nil {
		return "", err
	}

	result := append([]string(nil), originalLines...)
	for i := len(replacements) - 1; i >= 0; i-- {
		replacement := replacements[i]
		if replacement.Start < 0 || replacement.Start > len(result) {
			return "", fmt.Errorf("patch replacement index out of range")
		}

		replacementEnd := replacement.Start + replacement.OldLen
		if replacementEnd > len(result) {
			return "", fmt.Errorf("patch replacement length out of range")
		}

		head := append([]string(nil), result[:replacement.Start]...)
		head = append(head, replacement.NewLine...)
		result = append(head, result[replacementEnd:]...)
	}

	if len(result) == 0 || result[len(result)-1] != "" {
		result = append(result, "")
	}

	return strings.Join(result, "\n"), nil
}

func splitPatchLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func computePatchReplacements(originalLines []string, chunks []PatchChunk) ([]patchReplacement, error) {
	replacements := make([]patchReplacement, 0, len(chunks))
	lineIndex := 0

	for _, chunk := range chunks {
		if chunk.ChangeContext != "" {
			contextIndex := seekSequence(originalLines, []string{chunk.ChangeContext}, lineIndex, false)
			if contextIndex == -1 {
				return nil, fmt.Errorf("failed to find patch context %q", chunk.ChangeContext)
			}
			lineIndex = contextIndex + 1
		}

		if len(chunk.OldLines) == 0 {
			replacements = append(replacements, patchReplacement{
				Start:   len(originalLines),
				OldLen:  0,
				NewLine: chunk.NewLines,
			})
			continue
		}

		pattern := append([]string(nil), chunk.OldLines...)
		newLines := append([]string(nil), chunk.NewLines...)

		found := seekSequence(originalLines, pattern, lineIndex, chunk.IsEndOfFile)
		if found == -1 && len(pattern) > 0 && pattern[len(pattern)-1] == "" {
			pattern = pattern[:len(pattern)-1]
			if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
				newLines = newLines[:len(newLines)-1]
			}
			found = seekSequence(originalLines, pattern, lineIndex, chunk.IsEndOfFile)
		}

		if found == -1 {
			return nil, fmt.Errorf("failed to find expected patch lines")
		}

		replacements = append(replacements, patchReplacement{
			Start:   found,
			OldLen:  len(pattern),
			NewLine: newLines,
		})
		lineIndex = found + len(pattern)
	}

	return replacements, nil
}

func seekSequence(lines, pattern []string, startIndex int, endOfFile bool) int {
	if len(pattern) == 0 {
		return -1
	}

	if exact := tryMatch(lines, pattern, startIndex, endOfFile, func(a, b string) bool {
		return a == b
	}); exact != -1 {
		return exact
	}

	if rstrip := tryMatch(lines, pattern, startIndex, endOfFile, func(a, b string) bool {
		return strings.TrimRight(a, " \t") == strings.TrimRight(b, " \t")
	}); rstrip != -1 {
		return rstrip
	}

	if trimmed := tryMatch(lines, pattern, startIndex, endOfFile, func(a, b string) bool {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}); trimmed != -1 {
		return trimmed
	}

	return -1
}

func tryMatch(lines, pattern []string, startIndex int, endOfFile bool, compare func(a, b string) bool) int {
	if len(pattern) == 0 || len(lines) < len(pattern) {
		return -1
	}

	if endOfFile {
		fromEnd := len(lines) - len(pattern)
		if fromEnd >= startIndex && sequenceMatches(lines, pattern, fromEnd, compare) {
			return fromEnd
		}
	}

	maxStart := len(lines) - len(pattern)
	for i := startIndex; i <= maxStart; i++ {
		if sequenceMatches(lines, pattern, i, compare) {
			return i
		}
	}

	return -1
}

func sequenceMatches(lines, pattern []string, start int, compare func(a, b string) bool) bool {
	for i := 0; i < len(pattern); i++ {
		if !compare(lines[start+i], pattern[i]) {
			return false
		}
	}
	return true
}
