package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/r1gm/joplin-mcp-go/joplin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Input types ----

type ListTagsInput struct {
	Fields string `json:"fields,omitempty" jsonschema:"Comma-separated fields to return. Default: id title updated_time."`
}

type ResolveTagInput struct {
	Name string `json:"name" jsonschema:"Tag title. Returns id and title; or an error listing candidates if ambiguous (rare)."`
}

type GetTagNotesInput struct {
	TagID     string `json:"tag_id,omitempty" jsonschema:"The 32-hex-character tag ID. Takes precedence over tag_name."`
	TagName   string `json:"tag_name,omitempty" jsonschema:"Tag title as alternative to tag_id"`
	Task      string `json:"task,omitempty" jsonschema:"Filter by type: 'todo' for todos only / 'note' for regular notes only / empty for both."`
	Completed *bool  `json:"completed,omitempty" jsonschema:"Filter todos by completion: true=completed only / false=uncompleted only. Only relevant when task=todo."`
	Fields    string `json:"fields,omitempty" jsonschema:"Comma-separated fields to return. Default: id parent_id title is_todo todo_completed updated_time."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max results per page (1-100; default 20)"`
	Page      int    `json:"page,omitempty" jsonschema:"Page number starting at 1 (default 1)"`
	OrderBy   string `json:"order_by,omitempty" jsonschema:"Sort field: title / created_time / updated_time. Default updated_time."`
	OrderDir  string `json:"order_dir,omitempty" jsonschema:"Sort direction: ASC or DESC"`
}

type CreateTagInput struct {
	Title string `json:"title" jsonschema:"Tag title. Joplin normalizes tag titles to lowercase."`
}

type TagNoteInput struct {
	NoteID  string `json:"note_id" jsonschema:"The 32-hex-character note ID to tag"`
	TagID   string `json:"tag_id,omitempty" jsonschema:"The tag ID to add. Takes precedence over tag_name."`
	TagName string `json:"tag_name,omitempty" jsonschema:"Tag title as alternative to tag_id. Must already exist; use create_tag first if needed."`
}

type UntagNoteInput struct {
	NoteID  string `json:"note_id" jsonschema:"The 32-hex-character note ID"`
	TagID   string `json:"tag_id,omitempty" jsonschema:"The tag ID to remove. Takes precedence over tag_name."`
	TagName string `json:"tag_name,omitempty" jsonschema:"Tag title as alternative to tag_id"`
}

type DeleteTagInput struct {
	ID      string `json:"id,omitempty" jsonschema:"The tag ID to delete. Takes precedence over tag_name."`
	TagName string `json:"tag_name,omitempty" jsonschema:"Tag title as alternative to ID"`
}

// ---- Handlers (unchanged) ----

func registerTagTools(server *mcp.Server, client *joplin.Client) {

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tags",
		Description: "List all tags in the Joplin instance. Returns tag id / title / updated_time for each.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListTagsInput) (*mcp.CallToolResult, TextResult, error) {
		p := url.Values{}
		if input.Fields != "" {
			p.Set("fields", input.Fields)
		} else {
			p.Set("fields", "id,title,updated_time")
		}
		r, err := client.GetTags(p)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(r)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "resolve_tag",
		Description: "Resolve a tag title to its ID. Useful for checking whether a tag exists before " +
			"using it. Returns an error listing candidates if the name is ambiguous.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResolveTagInput) (*mcp.CallToolResult, TextResult, error) {
		ref, err := client.ResolveTag(input.Name)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: fmt.Sprintf(`{"id":%q,"title":%q}`, ref.ID, ref.Title)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_tag_notes",
		Description: "List all notes that carry a specific tag paginated. Target by tag_id (32-hex) or " +
			"tag_name (title). Use task and completed filters to narrow to todos or by completion.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetTagNotesInput) (*mcp.CallToolResult, TextResult, error) {
		tagID, err := resolveTagArg(client, input.TagID, input.TagName)
		if err != nil {
			return nil, TextResult{}, err
		}

		filter := todoFilter(input.Task, input.Completed)

		p := url.Values{}
		if input.Fields != "" {
			p.Set("fields", input.Fields)
		} else {
			p.Set("fields", "id,parent_id,title,is_todo,todo_completed,updated_time")
		}
		limit := input.Limit
		if limit <= 0 || limit > 100 {
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

		var result []byte
		if filter != "" {
			titleParams := url.Values{}
			titleParams.Set("fields", "title")
			titleBody, err := client.GetTag(tagID, titleParams)
			if err != nil {
				return nil, TextResult{}, err
			}
			var meta struct {
				Title string `json:"title"`
			}
			if err := jsonUnmarshal(titleBody, &meta); err != nil {
				return nil, TextResult{}, err
			}
			query := fmt.Sprintf(`tag:%s %s`, meta.Title, filter)
			r, err := client.Search(query, p)
			if err != nil {
				return nil, TextResult{}, err
			}
			result = r
		} else {
			r, err := client.GetTagNotes(tagID, p)
			if err != nil {
				return nil, TextResult{}, err
			}
			result = r
		}
		return nil, TextResult{Result: string(result)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "create_tag",
		Description: "Create a new tag with the given title. Tags can then be applied to notes via " +
			"tag_note. Joplin normalizes tag titles (typically to lowercase).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateTagInput) (*mcp.CallToolResult, TextResult, error) {
		r, err := client.CreateTag(map[string]interface{}{"title": input.Title})
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(r)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "tag_note",
		Description: "Attach a tag to a note. Target the tag by tag_id (32-hex) or tag_name (title). " +
			"Both the tag and the note must already exist; create the tag first with create_tag if " +
			"needed. A note may carry many tags.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TagNoteInput) (*mcp.CallToolResult, TextResult, error) {
		tagID, err := resolveTagArg(client, input.TagID, input.TagName)
		if err != nil {
			return nil, TextResult{}, err
		}
		r, err := client.AddTagToNote(tagID, input.NoteID)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(r)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "untag_note",
		Description: "Remove a tag from a note. The tag itself is not deleted; only the association is removed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UntagNoteInput) (*mcp.CallToolResult, TextResult, error) {
		tagID, err := resolveTagArg(client, input.TagID, input.TagName)
		if err != nil {
			return nil, TextResult{}, err
		}
		err = client.RemoveTagFromNote(tagID, input.NoteID)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: fmt.Sprintf("Tag %s removed from note %s", tagID, input.NoteID)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "delete_tag",
		Description: "Delete a tag from the Joplin instance. Only the tag is removed; notes that " +
			"carried it remain intact but lose that tag association. Target by id or tag_name.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteTagInput) (*mcp.CallToolResult, TextResult, error) {
		tagID, err := resolveTagArg(client, input.ID, input.TagName)
		if err != nil {
			return nil, TextResult{}, err
		}
		err = client.DeleteTag(tagID)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: fmt.Sprintf("Tag %s deleted", tagID)}, nil
	})
}
