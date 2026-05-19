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
type PatchOperation struct {
	Op      string `json:"op" jsonschema:"Operation type: replace / delete / prepend / append / insert_before / insert_after / replace_lines / insert_at_line / delete_lines"`
	Old     string `json:"old,omitempty" jsonschema:"For replace: the exact text to find (must appear exactly once in the body)"`
	New     string `json:"new,omitempty" jsonschema:"For replace: the replacement text"`
	Target  string `json:"target,omitempty" jsonschema:"For delete: the exact text to remove (must appear exactly once)"`
	Anchor  string `json:"anchor,omitempty" jsonschema:"For insert_before / insert_after: the text to locate (must appear exactly once)"`
	Content string `json:"content,omitempty" jsonschema:"For prepend / append / insert_before / insert_after / replace_lines / insert_at_line: the text to insert"`
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

// applyOperation applies a single PatchOperation to the body and returns the modified body.
func applyOperation(body string, op PatchOperation) (string, error) {
	switch strings.ToLower(op.Op) {

	case "replace":
		if op.Old == "" {
			return "", fmt.Errorf("replace: 'old' must not be empty")
		}
		count := strings.Count(body, op.Old)
		if count == 0 {
			return "", fmt.Errorf("replace: 'old' text not found in note body")
		}
		if count > 1 {
			return "", fmt.Errorf("replace: 'old' text appears %d times in note body; it must be unique. Include more surrounding context to disambiguate", count)
		}
		return strings.Replace(body, op.Old, op.New, 1), nil

	case "delete":
		if op.Target == "" {
			return "", fmt.Errorf("delete: 'target' must not be empty")
		}
		count := strings.Count(body, op.Target)
		if count == 0 {
			return "", fmt.Errorf("delete: 'target' text not found in note body")
		}
		if count > 1 {
			return "", fmt.Errorf("delete: 'target' text appears %d times in note body; it must be unique. Include more surrounding context to disambiguate", count)
		}
		return strings.Replace(body, op.Target, "", 1), nil

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
		count := strings.Count(body, op.Anchor)
		if count == 0 {
			return "", fmt.Errorf("insert_before: 'anchor' text not found in note body")
		}
		if count > 1 {
			return "", fmt.Errorf("insert_before: 'anchor' text appears %d times in note body; it must be unique. Include more surrounding context to disambiguate", count)
		}
		return strings.Replace(body, op.Anchor, op.Content+op.Anchor, 1), nil

	case "insert_after":
		if op.Anchor == "" {
			return "", fmt.Errorf("insert_after: 'anchor' must not be empty")
		}
		if op.Content == "" {
			return "", fmt.Errorf("insert_after: 'content' must not be empty")
		}
		count := strings.Count(body, op.Anchor)
		if count == 0 {
			return "", fmt.Errorf("insert_after: 'anchor' text not found in note body")
		}
		if count > 1 {
			return "", fmt.Errorf("insert_after: 'anchor' text appears %d times in note body; it must be unique. Include more surrounding context to disambiguate", count)
		}
		return strings.Replace(body, op.Anchor, op.Anchor+op.Content, 1), nil

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
			"Text-based (anchor must appear exactly once):\n" +
			"  replace  — find 'old' text and swap it with 'new'\n" +
			"  delete   — remove 'target' text\n" +
			"  insert_before — insert 'content' immediately before 'anchor'\n" +
			"  insert_after  — insert 'content' immediately after 'anchor'\n\n" +
			"Position-based:\n" +
			"  prepend — add 'content' at the top of the body\n" +
			"  append  — add 'content' at the bottom of the body\n\n" +
			"Line-based (1-based inclusive line numbers):\n" +
			"  replace_lines  — replace lines 'start' through 'end' with 'content'\n" +
			"  insert_at_line — insert 'content' before line 'line'\n" +
			"  delete_lines   — delete lines 'start' through 'end'\n\n" +
			"Operations are applied sequentially; each sees the body as modified by the previous one. " +
			"For text-based ops the anchor text must be unique in the body or the operation fails with " +
			"the match count so you can add more context and retry.",
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
