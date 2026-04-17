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

type ListNotebooksInput struct {
	Fields string `json:"fields,omitempty" jsonschema:"Comma-separated fields to return. Default: id title parent_id."`
}

type ResolveNotebookInput struct {
	Name string `json:"name" jsonschema:"Notebook title or path (e.g. 'Work' or 'Projects/Work'). Returns id / title / path; or an error listing all candidates if ambiguous."`
}

type GetNotebookInput struct {
	ID           string `json:"id,omitempty" jsonschema:"The 32-hex-character notebook ID. Takes precedence over notebook_name."`
	NotebookName string `json:"notebook_name,omitempty" jsonschema:"Notebook title or path as alternative to ID (e.g. 'Projects/Work')"`
	Fields       string `json:"fields,omitempty" jsonschema:"Comma-separated fields to return"`
}

type GetNotebookNotesInput struct {
	NotebookID   string `json:"notebook_id,omitempty" jsonschema:"The 32-hex-character notebook ID. Takes precedence over notebook_name."`
	NotebookName string `json:"notebook_name,omitempty" jsonschema:"Notebook title or path as alternative to ID (e.g. 'Projects/Work'). Ambiguous names return an error with all candidates."`
	Task         string `json:"task,omitempty" jsonschema:"Filter by type: 'todo' for todos only / 'note' for regular notes only / empty for both."`
	Completed    *bool  `json:"completed,omitempty" jsonschema:"Filter todos by completion: true=completed only / false=uncompleted only. Only relevant when task=todo."`
	Fields       string `json:"fields,omitempty" jsonschema:"Comma-separated fields to return. Default: id parent_id title is_todo todo_completed updated_time."`
	Limit        int    `json:"limit,omitempty" jsonschema:"Max results per page (1-100; default 20)"`
	Page         int    `json:"page,omitempty" jsonschema:"Page number starting at 1 (default 1)"`
	OrderBy      string `json:"order_by,omitempty" jsonschema:"Sort field: title / created_time / updated_time. Default updated_time."`
	OrderDir     string `json:"order_dir,omitempty" jsonschema:"Sort direction: ASC or DESC"`
}

type CreateNotebookInput struct {
	Title              string `json:"title" jsonschema:"The notebook title"`
	ParentID           string `json:"parent_id,omitempty" jsonschema:"Parent notebook ID. Takes precedence over parent_notebook_name. Omit both for a top-level notebook."`
	ParentNotebookName string `json:"parent_notebook_name,omitempty" jsonschema:"Parent notebook title or path (e.g. 'Projects') for creating a sub-notebook"`
}

type UpdateNotebookInput struct {
	ID                 string `json:"id,omitempty" jsonschema:"The notebook ID to update. Takes precedence over notebook_name."`
	NotebookName       string `json:"notebook_name,omitempty" jsonschema:"Notebook title or path as alternative to ID"`
	Title              string `json:"title,omitempty" jsonschema:"New title"`
	ParentID           string `json:"parent_id,omitempty" jsonschema:"Move under this parent notebook ID. Takes precedence over parent_notebook_name."`
	ParentNotebookName string `json:"parent_notebook_name,omitempty" jsonschema:"Move under this parent notebook by title or path"`
}

type DeleteNotebookInput struct {
	ID           string `json:"id,omitempty" jsonschema:"The notebook ID to delete. Takes precedence over notebook_name."`
	NotebookName string `json:"notebook_name,omitempty" jsonschema:"Notebook title or path as alternative to ID"`
	Permanent    bool   `json:"permanent,omitempty" jsonschema:"If true permanently delete including all contained notes. Default false (moves to trash; recoverable from Joplin)."`
}

// ---- Handlers ----

func registerFolderTools(server *mcp.Server, client *joplin.Client) {

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_notebooks",
		Description: "List all notebooks (folders) in the Joplin instance as a tree. Sub-notebooks " +
			"appear under each parent's 'children' array. Useful for discovering notebook IDs and " +
			"paths before using other tools.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListNotebooksInput) (*mcp.CallToolResult, TextResult, error) {
		params := url.Values{}
		if input.Fields != "" {
			params.Set("fields", input.Fields)
		}
		result, err := client.GetFolders(params)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(result)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "resolve_notebook",
		Description: "Resolve a notebook title or path to its ID and full path. Use this to check " +
			"whether a name is unique before calling tools that accept notebook_name. Returns an error " +
			"listing all candidates with their IDs and paths if the name is ambiguous. Input can be a " +
			"bare title ('Work') or a slash-separated path ('Projects/Work') or a 32-hex ID (round-trip " +
			"verification).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResolveNotebookInput) (*mcp.CallToolResult, TextResult, error) {
		ref, err := client.ResolveNotebook(input.Name)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: fmt.Sprintf(`{"id":%q,"title":%q,"path":%q}`, ref.ID, ref.Title, ref.Path)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_notebook",
		Description: "Get a single notebook's metadata by id or notebook_name. At least one of id or " +
			"notebook_name must be provided.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetNotebookInput) (*mcp.CallToolResult, TextResult, error) {
		id, err := resolveNotebookArg(client, input.ID, input.NotebookName)
		if err != nil {
			return nil, TextResult{}, err
		}
		params := url.Values{}
		if input.Fields != "" {
			params.Set("fields", input.Fields)
		}
		result, err := client.GetFolder(id, params)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(result)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_notebook_notes",
		Description: "List all notes in a specific notebook paginated. Target by notebook_id (32-hex) or " +
			"notebook_name (title or path like 'Projects/Work'). If notebook_name is ambiguous the error " +
			"lists all candidates. Use task and completed filters to narrow to todos or by completion.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetNotebookNotesInput) (*mcp.CallToolResult, TextResult, error) {
		id, err := resolveNotebookArg(client, input.NotebookID, input.NotebookName)
		if err != nil {
			return nil, TextResult{}, err
		}

		filter := todoFilter(input.Task, input.Completed)

		params := url.Values{}
		if input.Fields != "" {
			params.Set("fields", input.Fields)
		} else {
			params.Set("fields", "id,parent_id,title,is_todo,todo_completed,updated_time")
		}
		limit := input.Limit
		if limit <= 0 || limit > 100 {
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

		var result []byte
		if filter != "" {
			titleParams := url.Values{}
			titleParams.Set("fields", "title")
			titleBody, err := client.GetFolder(id, titleParams)
			if err != nil {
				return nil, TextResult{}, err
			}
			var meta struct {
				Title string `json:"title"`
			}
			if err := jsonUnmarshal(titleBody, &meta); err != nil {
				return nil, TextResult{}, err
			}
			query := fmt.Sprintf(`notebook:"%s" %s`, meta.Title, filter)
			r, err := client.Search(query, params)
			if err != nil {
				return nil, TextResult{}, err
			}
			result = r
		} else {
			r, err := client.GetFolderNotes(id, params)
			if err != nil {
				return nil, TextResult{}, err
			}
			result = r
		}
		return nil, TextResult{Result: string(result)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "create_notebook",
		Description: "Create a new notebook. Omit parent to create at the top level. For a sub-notebook " +
			"pass either parent_id or parent_notebook_name.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateNotebookInput) (*mcp.CallToolResult, TextResult, error) {
		data := map[string]interface{}{"title": input.Title}
		if input.ParentID != "" || input.ParentNotebookName != "" {
			parentID, err := resolveNotebookArg(client, input.ParentID, input.ParentNotebookName)
			if err != nil {
				return nil, TextResult{}, err
			}
			data["parent_id"] = parentID
		}
		result, err := client.CreateFolder(data)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(result)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_notebook",
		Description: "Rename a notebook and/or move it under a different parent. Target by id or " +
			"notebook_name; new parent by parent_id or parent_notebook_name.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateNotebookInput) (*mcp.CallToolResult, TextResult, error) {
		id, err := resolveNotebookArg(client, input.ID, input.NotebookName)
		if err != nil {
			return nil, TextResult{}, err
		}
		data := map[string]interface{}{}
		if input.Title != "" {
			data["title"] = input.Title
		}
		if input.ParentID != "" || input.ParentNotebookName != "" {
			parentID, err := resolveNotebookArg(client, input.ParentID, input.ParentNotebookName)
			if err != nil {
				return nil, TextResult{}, err
			}
			data["parent_id"] = parentID
		}
		if len(data) == 0 {
			return nil, TextResult{}, fmt.Errorf("no fields to update")
		}
		result, err := client.UpdateFolder(id, data)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: string(result)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "delete_notebook",
		Description: "Delete a notebook and all notes it contains. Target by id or notebook_name. " +
			"By default moves to trash; set permanent=true to delete permanently.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteNotebookInput) (*mcp.CallToolResult, TextResult, error) {
		id, err := resolveNotebookArg(client, input.ID, input.NotebookName)
		if err != nil {
			return nil, TextResult{}, err
		}
		err = client.DeleteFolder(id, input.Permanent)
		if err != nil {
			return nil, TextResult{}, err
		}
		return nil, TextResult{Result: fmt.Sprintf("Notebook %s deleted successfully", id)}, nil
	})
}
