package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/r1gm/joplin-mcp-go/joplin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Input types ----

// GrepNoteInput is the input for the grep_note tool.
// It searches inside a single note's body and returns matching lines
// with line numbers and optional surrounding context.
//
// Two matching modes are supported:
//   - Plain text (default): pattern is matched literally via strings.Contains.
//   - Regex (regex=true): pattern is compiled as a Go regular expression.
//     Syntax reference: https://pkg.go.dev/regexp/syntax
//     Common patterns: ^\s*##\s (markdown H2 headings) / \d{4}-\d{2}-\d{2} (dates) /
//     (?i)todo (case-insensitive via regex flag — alternative to ignore_case).
type GrepNoteInput struct {
	ID           string `json:"id" jsonschema:"The 32-hex-character note ID to search inside"`
	Pattern      string `json:"pattern" jsonschema:"Text or regex pattern to search for inside the note body"`
	Regex        bool   `json:"regex,omitempty" jsonschema:"If true treat pattern as a Go regular expression. Default false (plain text match)."`
	ContextLines *int   `json:"context_lines,omitempty" jsonschema:"Number of lines to show before and after each match (default 2). Set to 0 for matching lines only."`
	IgnoreCase   bool   `json:"ignore_case,omitempty" jsonschema:"If true perform case-insensitive matching. For regex mode you can also use (?i) in the pattern."`
}

// GrepMatch represents a single match occurrence with its surrounding context.
type GrepMatch struct {
	LineNumber int      `json:"line_number"`
	Line       string   `json:"line"`
	Before     []string `json:"before,omitempty"`
	After      []string `json:"after,omitempty"`
}

// GrepResult is the structured output of grep_note.
type GrepResult struct {
	NoteID     string      `json:"note_id"`
	NoteTitle  string      `json:"note_title"`
	Pattern    string      `json:"pattern"`
	IsRegex    bool        `json:"is_regex"`
	TotalLines int         `json:"total_lines"`
	MatchCount int         `json:"match_count"`
	Matches    []GrepMatch `json:"matches"`
}

// ---- Tool registration ----

func registerGrepTool(server *mcp.Server, client *joplin.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "grep_note",
		Description: "Search inside a single note's body and return matching lines with line numbers " +
			"and surrounding context. Useful for locating text before using patch_note — the match " +
			"lines give you exact anchor text for replace / insert_before / insert_after operations " +
			"and line numbers for line-based operations.\n\n" +
			"Two matching modes:\n" +
			"  Plain text (default) — pattern is matched literally\n" +
			"  Regex (regex=true) — pattern is a Go regular expression. Useful for matching headings " +
			"(^## ) / dates (\\d{4}-\\d{2}-\\d{2}) / flexible patterns. Syntax: https://pkg.go.dev/regexp/syntax\n\n" +
			"Returns a structured result with note_id / note_title / total_lines / match_count / and " +
			"each match with its line_number plus context_lines of surrounding lines (default 2). " +
			"Set context_lines=0 for matching lines only. Set ignore_case=true for case-insensitive " +
			"matching (or use (?i) flag in regex).\n\n" +
			"For searching ACROSS notes by content use find_notes instead. This tool searches WITHIN " +
			"a single note.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GrepNoteInput) (*mcp.CallToolResult, TextResult, error) {
		if input.Pattern == "" {
			return nil, TextResult{}, fmt.Errorf("pattern must not be empty")
		}

		// 1. Build the match function
		var matchFn func(line string) bool

		if input.Regex {
			// Regex mode: compile the pattern
			p := input.Pattern
			if input.IgnoreCase && !strings.HasPrefix(p, "(?i)") {
				p = "(?i)" + p
			}
			re, err := regexp.Compile(p)
			if err != nil {
				return nil, TextResult{}, fmt.Errorf("invalid regex pattern: %w", err)
			}
			matchFn = func(line string) bool {
				return re.MatchString(line)
			}
		} else {
			// Plain text mode
			if input.IgnoreCase {
				lowerPattern := strings.ToLower(input.Pattern)
				matchFn = func(line string) bool {
					return strings.Contains(strings.ToLower(line), lowerPattern)
				}
			} else {
				pattern := input.Pattern
				matchFn = func(line string) bool {
					return strings.Contains(line, pattern)
				}
			}
		}

		// 2. Fetch the note body
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

		// 3. Determine context lines (default 2; nil = omitted = 2)
		ctxLines := 2
		if input.ContextLines != nil {
			ctxLines = *input.ContextLines
			if ctxLines < 0 {
				ctxLines = 0
			}
		}

		// 4. Search through lines
		lines := strings.Split(note.Body, "\n")
		var matches []GrepMatch

		for i, line := range lines {
			if !matchFn(line) {
				continue
			}

			m := GrepMatch{
				LineNumber: i + 1, // 1-based
				Line:       line,
			}

			// Collect context before
			if ctxLines > 0 {
				start := i - ctxLines
				if start < 0 {
					start = 0
				}
				if start < i {
					m.Before = lines[start:i]
				}

				// Collect context after
				end := i + 1 + ctxLines
				if end > len(lines) {
					end = len(lines)
				}
				if i+1 < end {
					m.After = lines[i+1 : end]
				}
			}

			matches = append(matches, m)
		}

		// 5. Build result
		result := GrepResult{
			NoteID:     note.ID,
			NoteTitle:  note.Title,
			Pattern:    input.Pattern,
			IsRegex:    input.Regex,
			TotalLines: len(lines),
			MatchCount: len(matches),
			Matches:    matches,
		}

		resultJSON, err := json.Marshal(result)
		if err != nil {
			return nil, TextResult{}, fmt.Errorf("failed to marshal result: %w", err)
		}

		return nil, TextResult{Result: string(resultJSON)}, nil
	})
}
