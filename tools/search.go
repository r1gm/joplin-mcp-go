package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/r1gm/joplin-mcp-go/joplin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchInput accepts a raw Joplin query string plus standard list-result options.
// Use this when find_notes / get_notebook_notes / get_tag_notes don't fit, or when
// you want to search across notebooks, tags, or use full Joplin query syntax directly.
type SearchInput struct {
	Query    string `json:"query" jsonschema:"required,description=Raw Joplin search query. Supports \"exact phrase\", title:word, body:word, -exclude, word1 OR word2, tag:name, notebook:\"Name\", type:todo, iscompleted:0. Use '*' to match everything."`
	Type     string `json:"type,omitempty" jsonschema:"description=Limit to an item type: 'note' (default), 'folder' (notebooks), or 'tag'. For folder/tag lookup the search becomes a simple case-insensitive title match and supports * wildcards."`
	Fields   string `json:"fields,omitempty" jsonschema:"description=Comma-separated fields to return"`
	Limit    int    `json:"limit,omitempty" jsonschema:"description=Max results per page (1-100, default 20),minimum=1,maximum=100"`
	Page     int    `json:"page,omitempty" jsonschema:"description=Page number starting at 1,minimum=1"`
	OrderBy  string `json:"order_by,omitempty" jsonschema:"description=Sort field: title, created_time, or updated_time"`
	OrderDir string `json:"order_dir,omitempty" jsonschema:"description=Sort direction: ASC or DESC"`
}

// PingInput is intentionally empty — the ping tool takes no arguments.
type PingInput struct{}

func registerSearchTools(server *mcp.Server, client *joplin.Client) {

	mcp.AddTool(server, &mcp.Tool{
		Name: "search",
		Description: "General-purpose Joplin search. For simple note-text searches prefer find_notes. " +
			"Use this when you need: (a) raw Joplin query syntax, (b) searching across notebooks/tags " +
			"by title (set type='folder' or type='tag'), or (c) complex filters like " +
			"'notebook:\"Work\" tag:urgent type:todo iscompleted:0'. When type is 'folder' or 'tag', " +
			"Joplin does a case-insensitive title match (supports '*' wildcards).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, TextResult, error) {
		p := url.Values{}
		if input.Fields != "" {
			p.Set("fields", input.Fields)
		} else {
			p.Set("fields", "id,parent_id,title,is_todo,todo_completed,updated_time")
		}
		if input.Type != "" {
			p.Set("type", input.Type)
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		p.Set("limit", fmt.Sprintf("%d", limit))
		if input.Page > 0 {
			p.Set("page", fmt.Sprintf("%d", input.Page))
		}
		if input.OrderBy != "" {
			p.Set("order_by", input.OrderBy)
		}
		if input.OrderDir != "" {
			p.Set("order_dir", strings.ToUpper(input.OrderDir))
		}
		r, err := client.Search(input.Query, p)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(r)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "ping",
		Description: "Check connectivity to Joplin. Returns 'Joplin is running' on success, or a " +
			"clear error if the Web Clipper service is not reachable at the configured host and port. " +
			"Use this for troubleshooting when other tools fail.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ PingInput) (*mcp.CallToolResult, TextResult, error) {
		r, err := client.Ping()
		if err != nil {
			return nil, TextResult{}, fmt.Errorf("Joplin is not reachable: %w", err)
		}
		return nil, TextResult{Result: "Joplin is running: " + r}, nil
	})
}
