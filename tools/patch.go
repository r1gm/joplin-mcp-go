package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/r1gm/joplin-mcp-go/joplin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Input types ----

// PatchOperation represents a single edit operation on a note's body.
// Exactly one operation type should be specified per entry.
//
// Text-based operations (anchor text must appear exactly once by default;
// set count to allow multiple):
//   - replace:       find old text and replace with new text
//   - delete:        remove target text
//   - insert_before: insert content before anchor text
//   - insert_after:  insert content after anchor text
//
// The count field controls how many occurrences to act on:
//   - 0 (default):  exactly 1 (text must be unique; error if multiple matches)
//   - N > 1:        act on the first N occurrences (error if fewer than N exist)
//   - -1:           act on ALL occurrences
//
// Position-based operations:
//   - prepend: add content to the beginning of the body
//   - append:  add content to the end of the body
//
// Line-based operations (1-based inclusive line numbers):
//   - replace_lines:  replace a range of lines with new content
//   - insert_at_line: insert content before a specific line
//   - delete_lines:   delete a range of lines
type PatchOperation struct {
	Op      string `json:"op" jsonschema:"Operation type: replace / delete / prepend / append / insert_before / insert_after / replace_lines / insert_at_line / delete_lines"`
	Old     string `json:"old,omitempty" jsonschema:"For replace: the exact text to find"`
	New     string `json:"new,omitempty" jsonschema:"For replace: the replacement text"`
	Target  string `json:"target,omitempty" jsonschema:"For delete: the exact text to remove"`
	Anchor  string `json:"anchor,omitempty" jsonschema:"For insert_before / insert_after: the text to locate"`
	Content string `json:"content,omitempty" jsonschema:"For prepend / append / insert_before / insert_after / replace_lines / insert_at_line: the text to insert"`
	Count   int    `json:"count,omitempty" jsonschema:"For text-based ops: max occurrences to act on. Default 1 (must be unique). Use -1 for all occurrences."`
	Start   int    `json:"start,omitempty" jsonschema:"For replace_lines / delete_lines: start line number (1-based inclusive)"`
	End     int    `json:"end,omitempty" jsonschema:"For replace_lines / delete_lines: end line number (1-based inclusive)"`
	Line    int    `json:"line,omitempty" jsonschema:"For insert_at_line: insert content before this line number (1-based). Use 1 for top; a value beyond the last line appends."`
}

// PatchNoteInput is the input for the patch_note tool.
type PatchNoteInput struct {
	ID         string           `json:"id" jsonschema:"The 32-hex-character note ID to patch"`
	Operations []PatchOperation `json:"operations" jsonschema:"Ordered list of patch operations to apply sequentially. Each operation sees the body as modified by the previous one."`
}

// ---- Patch engine ----

// effectiveCount returns the max occurrences to act on for text-based operations.
// Default is 1 (require unique match). -1 means all occurrences.
func effectiveCount(op PatchOperation) int {
	if op.Count == 0 {
		return 1
	}
	return op.Count
}

// validateTextMatch checks that the needle appears in the body and that the
// occurrence count is compatible with the requested max count.
// Returns the actual number of occurrences found.
func validateTextMatch(body, needle, opName string, maxCount int) (int, error) {
	found := strings.Count(body, needle)
	if found == 0 {
		return 0, fmt.Errorf("%s: text not found in note body", opName)
	}
	if maxCount == 1 && found > 1 {
		return found, fmt.Errorf("%s: text appears %d times in note body; it must be unique (set count to allow multiple). Include more surrounding context to disambiguate", opName, found)
	}
	if maxCount > 1 && found < maxCount {
		return found, fmt.Errorf("%s: text appears %d times but count=%d was requested; only %d available", opName, found, maxCount, found)
	}
	return found, nil
}

// replaceN replaces up to n occurrences of old with new in s.
// If n == -1, replaces all occurrences.
func replaceN(s, old, new string, n int) string {
	if n == -1 {
		return strings.ReplaceAll(s, old, new)
	}
	return strings.Replace(s, old, new, n)
}

// applyOperation applies a single PatchOperation to the body and returns the modified body.
func applyOperation(body string, op PatchOperation) (string, error) {
	switch strings.ToLower(op.Op) {

	case "replace":
		if op.Old == "" {
			return "", fmt.Errorf("replace: 'old' must not be empty")
		}
		maxCount := effectiveCount(op)
		if _, err := validateTextMatch(body, op.Old, "replace", maxCount); err != nil {
			return "", err
		}
		return replaceN(body, op.Old, op.New, maxCount), nil

	case "delete":
		if op.Target == "" {
			return "", fmt.Errorf("delete: 'target' must not be empty")
		}
		maxCount := effectiveCount(op)
		if _, err := validateTextMatch(body, op.Target, "delete", maxCount); err != nil {
			return "", err
		}
		return replaceN(body, op.Target, "", maxCount), nil

	case "prepend":
		if op.Content == "" {
			return "", fmt.Errorf("prepend: 'content' must not be empty")
		}
		if body == "" {
			return op.Content, nil
		}
		return op.Content + "\n" + body, nil

	case "append":
		if op.Content == "" {
			return "", fmt.Errorf("append: 'content' must not be empty")
		}
		if body == "" {
			return op.Content, nil
		}
		return body + "\n" + op.Content, nil

	case "insert_before":
		if op.Anchor == "" {
			return "", fmt.Errorf("insert_before: 'anchor' must not be empty")
		}
		if op.Content == "" {
			return "", fmt.Errorf("insert_before: 'content' must not be empty")
		}
		maxCount := effectiveCount(op)
		if _, err := validateTextMatch(body, op.Anchor, "insert_before", maxCount); err != nil {
			return "", err
		}
		return replaceN(body, op.Anchor, op.Content+op.Anchor, maxCount), nil

	case "insert_after":
		if op.Anchor == "" {
			return "", fmt.Errorf("insert_after: 'anchor' must not be empty")
		}
		if op.Content == "" {
			return "", fmt.Errorf("insert_after: 'content' must not be empty")
		}
		maxCount := effectiveCount(op)
		if _, err := validateTextMatch(body, op.Anchor, "insert_after", maxCount); err != nil {
			return "", err
		}
		return replaceN(body, op.Anchor, op.Anchor+op.Content, maxCount), nil

	case "replace_lines":
		if op.Start < 1 {
			return "", fmt.Errorf("replace_lines: 'start' must be >= 1")
		}
		if op.End < op.Start {
			return "", fmt.Errorf("replace_lines: 'end' (%d) must be >= 'start' (%d)", op.End, op.Start)
		}
		lines := strings.Split(body, "\n")
		total := len(lines)

		// Clamp to valid range
		start := op.Start - 1 // convert to 0-based
		end := op.End         // exclusive end in 0-based
		if start >= total {
			start = total
		}
		if end > total {
			end = total
		}

		var result []string
		result = append(result, lines[:start]...)
		if op.Content != "" {
			result = append(result, strings.Split(op.Content, "\n")...)
		}
		result = append(result, lines[end:]...)
		return strings.Join(result, "\n"), nil

	case "insert_at_line":
		if op.Line < 1 {
			return "", fmt.Errorf("insert_at_line: 'line' must be >= 1")
		}
		if op.Content == "" {
			return "", fmt.Errorf("insert_at_line: 'content' must not be empty")
		}
		lines := strings.Split(body, "\n")
		total := len(lines)

		insertIdx := op.Line - 1 // convert to 0-based
		if insertIdx > total {
			insertIdx = total
		}

		newLines := strings.Split(op.Content, "\n")
		var result []string
		result = append(result, lines[:insertIdx]...)
		result = append(result, newLines...)
		result = append(result, lines[insertIdx:]...)
		return strings.Join(result, "\n"), nil

	case "delete_lines":
		if op.Start < 1 {
			return "", fmt.Errorf("delete_lines: 'start' must be >= 1")
		}
		if op.End < op.Start {
			return "", fmt.Errorf("delete_lines: 'end' (%d) must be >= 'start' (%d)", op.End, op.Start)
		}
		lines := strings.Split(body, "\n")
		total := len(lines)

		start := op.Start - 1
		end := op.End
		if start >= total {
			start = total
		}
		if end > total {
			end = total
		}

		var result []string
		result = append(result, lines[:start]...)
		result = append(result, lines[end:]...)
		return strings.Join(result, "\n"), nil

	default:
		return "", fmt.Errorf("unknown operation %q. Supported: replace / delete / prepend / append / insert_before / insert_after / replace_lines / insert_at_line / delete_lines", op.Op)
	}
}

// ---- Tool registration ----

func registerPatchTool(server *mcp.Server, client *joplin.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "patch_note",
		Description: "Apply partial edits to a note's body without rewriting the entire content. " +
			"The server fetches the current body / applies your operations in order / then saves the result. " +
			"This is much cheaper than update_note for small edits on large notes. " +
			"Three families of operations are available:\n\n" +
			"Text-based (by default anchor must appear exactly once; set count for multiple):\n" +
			"  replace  — find 'old' text and swap it with 'new'\n" +
			"  delete   — remove 'target' text\n" +
			"  insert_before — insert 'content' immediately before 'anchor'\n" +
			"  insert_after  — insert 'content' immediately after 'anchor'\n\n" +
			"  The 'count' field controls how many occurrences to act on:\n" +
			"    omit or 0 → default 1 (text must be unique; error if multiple matches)\n" +
			"    N > 1     → act on the first N occurrences (error if fewer than N exist)\n" +
			"    -1        → act on ALL occurrences\n\n" +
			"Position-based:\n" +
			"  prepend — add 'content' at the top of the body\n" +
			"  append  — add 'content' at the bottom of the body\n\n" +
			"Line-based (1-based inclusive line numbers):\n" +
			"  replace_lines  — replace lines 'start' through 'end' with 'content'\n" +
			"  insert_at_line — insert 'content' before line 'line'\n" +
			"  delete_lines   — delete lines 'start' through 'end'\n\n" +
			"Operations are applied sequentially; each sees the body as modified by the previous one.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PatchNoteInput) (*mcp.CallToolResult, TextResult, error) {
		if len(input.Operations) == 0 {
			return nil, TextResult{}, fmt.Errorf("no operations provided")
		}

		// 1. Fetch the current note body
		params := url.Values{}
		params.Set("fields", "id,title,body")
		noteRaw, err := client.GetNote(input.ID, params)
		if err != nil {
			return nil, TextResult{}, fmt.Errorf("failed to fetch note %s: %w", input.ID, err)
		}

		var note struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		if err := json.Unmarshal(noteRaw, &note); err != nil {
			return nil, TextResult{}, fmt.Errorf("failed to parse note: %w", err)
		}

		// 2. Apply operations sequentially
		body := note.Body
		for i, op := range input.Operations {
			body, err = applyOperation(body, op)
			if err != nil {
				return nil, TextResult{}, fmt.Errorf("operation %d (%s) failed: %w", i+1, op.Op, err)
			}
		}

		// 3. Save the patched body back
		result, err := client.UpdateNote(input.ID, map[string]interface{}{
			"body": body,
		})
		if err != nil {
			return nil, TextResult{}, fmt.Errorf("failed to save patched note: %w", err)
		}

		return nil, TextResult{Result: string(result)}, nil
	})
}
