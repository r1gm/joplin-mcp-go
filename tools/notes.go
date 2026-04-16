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

type FindNotesInput struct {
	Query     string `json:"query" jsonschema:"description=Search text or '*' for all notes. Supports Joplin syntax: exact phrase (\"foo bar\"), title:word, body:word, -exclude, word1 OR word2, tag:name, notebook:\"Name\"."`
	Task      string `json:"task,omitempty" jsonschema:"description=Filter by item type: 'todo' for todos only, 'note' for regular notes only. Empty for both."`
	Completed *bool  `json:"completed,omitempty" jsonschema:"description=Filter todos by completion: true=completed only, false=uncompleted only. Omit to include both. Only relevant when task=todo."`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=Max results per page (1-100, default 20),minimum=1,maximum=100"`
	Page      int    `json:"page,omitempty" jsonschema:"description=Page number for pagination starting at 1 (default 1),minimum=1"`
	OrderBy   string `json:"order_by,omitempty" jsonschema:"description=Sort field: title, created_time, or updated_time (default: updated_time for '*', relevance for text queries)"`
	OrderDir  string `json:"order_dir,omitempty" jsonschema:"description=Sort direction: ASC or DESC"`
	Fields    string `json:"fields,omitempty" jsonschema:"description=Comma-separated fields to return (default: id, parent_id, title, is_todo, todo_completed, updated_time)"`
}

type GetNoteInput struct {
	ID     string `json:"id" jsonschema:"required,description=The 32-hex-character note ID"`
	Fields string `json:"fields,omitempty" jsonschema:"description=Comma-separated fields to return. Omit 'body' to get just metadata. Available fields: id, parent_id, title, body, is_todo, todo_due, todo_completed, created_time, updated_time, source_url, author, latitude, longitude, altitude, markup_language. Default includes body, which may be large."`
}

type CreateNoteInput struct {
	Title         string `json:"title" jsonschema:"required,description=The note title"`
	Body          string `json:"body,omitempty" jsonschema:"description=The note body in Markdown"`
	ParentID      string `json:"parent_id,omitempty" jsonschema:"description=ID of the notebook to create the note in. Takes precedence over notebook_name. If neither is set, note goes to the default notebook."`
	NotebookName  string `json:"notebook_name,omitempty" jsonschema:"description=Notebook title or path (e.g. 'Work' or 'Projects/Work'). If ambiguous an error lists all candidates with their IDs."`
	IsTodo        int    `json:"is_todo,omitempty" jsonschema:"description=Set to 1 to create as a todo item (checkbox). Default 0 (regular note)."`
	TodoDue       int64  `json:"todo_due,omitempty" jsonschema:"description=Todo due date as Unix timestamp in milliseconds. Triggers an alarm on that date. Only relevant when is_todo=1."`
}

type UpdateNoteInput struct {
	ID            string `json:"id" jsonschema:"required,description=The 32-hex-character note ID to update"`
	Title         string `json:"title,omitempty" jsonschema:"description=New title"`
	Body          string `json:"body,omitempty" jsonschema:"description=New body in Markdown. Replaces the entire body."`
	ParentID      string `json:"parent_id,omitempty" jsonschema:"description=Move note to this notebook ID. Takes precedence over notebook_name."`
	NotebookName  string `json:"notebook_name,omitempty" jsonschema:"description=Move note to this notebook by title or path (e.g. 'Projects/Work')"`
	IsTodo        *int   `json:"is_todo,omitempty" jsonschema:"description=1 to convert to a todo, 0 to convert back to a regular note"`
	TodoDue       *int64 `json:"todo_due,omitempty" jsonschema:"description=Due date as Unix timestamp in ms. Use 0 to clear."`
	TodoCompleted *int64 `json:"todo_completed,omitempty" jsonschema:"description=Completion timestamp in ms (e.g. time.Now().UnixMilli()). Use 0 to mark uncompleted."`
}

type DeleteNoteInput struct {
	ID        string `json:"id" jsonschema:"required,description=The 32-hex-character note ID to delete"`
	Permanent bool   `json:"permanent,omitempty" jsonschema:"description=If true, permanently delete. Default false (moves to trash, recoverable from Joplin)."`
}

type GetTagsByNoteInput struct {
	NoteID string `json:"note_id" jsonschema:"required,description=The 32-hex-character note ID"`
	Fields string `json:"fields,omitempty" jsonschema:"description=Comma-separated fields to return (default: id, title)"`
}

// ---- Handlers ----

func registerNoteTools(server *mcp.Server, client *joplin.Client) {

	mcp.AddTool(server, &mcp.Tool{
		Name: "find_notes",
		Description: "Full-text search for notes, or pass query='*' to list all notes. " +
			"Use task and completed filters to narrow results to todos or by completion state. " +
			"For filtering by notebook, prefer get_notebook_notes; by tag, prefer get_tag_notes. " +
			"Returns a paginated list with has_more flag.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input FindNotesInput) (*mcp.CallToolResult, TextResult, error) {
		query := input.Query
		if filter := todoFilter(input.Task, input.Completed); filter != "" {
			if query == "" || query == "*" {
				query = filter
			} else {
				query = query + " " + filter
			}
		}
		if query == "" {
			query = "*"
		}

		params := url.Values{}
		if input.Fields != "" {
			params.Set("fields", input.Fields)
		} else {
			params.Set("fields", "id,parent_id,title,is_todo,todo_completed,updated_time")
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		params.Set("limit", fmt.Sprintf("%d", limit))
		if input.Page > 0 {
			params.Set("page", fmt.Sprintf("%d", input.Page))
		}
		if input.OrderBy != "" {
			params.Set("order_by", input.OrderBy)
		}
		if input.OrderDir != "" {
			params.Set("order_dir", strings.ToUpper(input.OrderDir))
		}
		result, err := client.Search(query, params)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(result)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_note",
		Description: "Get a single note by ID. The default fields include the full body, which may be " +
			"large — pass fields='id,title,parent_id,updated_time' (omitting body) to get metadata only. " +
			"Available fields: id, parent_id, title, body, is_todo, todo_due, todo_completed, " +
			"created_time, updated_time, source_url, author, latitude, longitude, altitude, markup_language.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetNoteInput) (*mcp.CallToolResult, TextResult, error) {
		params := url.Values{}
		if input.Fields != "" {
			params.Set("fields", input.Fields)
		} else {
			params.Set("fields", "id,parent_id,title,body,is_todo,todo_due,todo_completed,updated_time")
		}
		result, err := client.GetNote(input.ID, params)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(result)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "create_note",
		Description: "Create a new note with the given title and (optional) Markdown body. " +
			"Target notebook with either parent_id (32-hex Joplin ID) or notebook_name " +
			"(title or path like 'Projects/Work'). If notebook_name is ambiguous, the error response " +
			"lists all candidate notebooks with their IDs so you can retry with parent_id. " +
			"Set is_todo=1 to create as a checkbox todo. Returns the created note's id and title.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateNoteInput) (*mcp.CallToolResult, TextResult, error) {
		data := map[string]interface{}{"title": input.Title}
		if input.Body != "" {
			data["body"] = input.Body
		}
		if input.ParentID != "" || input.NotebookName != "" {
			parentID, err := resolveNotebookArg(client, input.ParentID, input.NotebookName)
			if err != nil {
				return nil, TextResult{}, err
			}
			data["parent_id"] = parentID
		}
		if input.IsTodo != 0 {
			data["is_todo"] = input.IsTodo
		}
		if input.TodoDue != 0 {
			data["todo_due"] = input.TodoDue
		}
		result, err := client.CreateNote(data)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(result)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_note",
		Description: "Update an existing note. Only supplied fields are changed; others remain untouched. " +
			"Note: supplying body replaces the entire body. To move the note to another notebook pass " +
			"either parent_id or notebook_name. To mark a todo as completed set todo_completed to the " +
			"current Unix time in milliseconds; use 0 to mark it uncompleted.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, TextResult, error) {
		data := map[string]interface{}{}
		if input.Title != "" {
			data["title"] = input.Title
		}
		if input.Body != "" {
			data["body"] = input.Body
		}
		if input.ParentID != "" || input.NotebookName != "" {
			parentID, err := resolveNotebookArg(client, input.ParentID, input.NotebookName)
			if err != nil {
				return nil, TextResult{}, err
			}
			data["parent_id"] = parentID
		}
		if input.IsTodo != nil {
			data["is_todo"] = *input.IsTodo
		}
		if input.TodoDue != nil {
			data["todo_due"] = *input.TodoDue
		}
		if input.TodoCompleted != nil {
			data["todo_completed"] = *input.TodoCompleted
		}
		if len(data) == 0 {
			return nil, TextResult{}, fmt.Errorf("no fields to update")
		}
		result, err := client.UpdateNote(input.ID, data)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(result)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "delete_note",
		Description: "Delete a note by ID. By default moves the note to trash where it can be " +
			"restored from Joplin. Set permanent=true to bypass trash and delete permanently.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteNoteInput) (*mcp.CallToolResult, TextResult, error) {
		err := client.DeleteNote(input.ID, input.Permanent)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: fmt.Sprintf("Note %s deleted successfully", input.ID)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_tags_by_note",
		Description: "Get all tags attached to a specific note. Returns id and title for each tag.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetTagsByNoteInput) (*mcp.CallToolResult, TextResult, error) {
		params := url.Values{}
		if input.Fields != "" {
			params.Set("fields", input.Fields)
		} else {
			params.Set("fields", "id,title")
		}
		result, err := client.GetNoteTags(input.NoteID, params)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(result)}, nil
	})
}
