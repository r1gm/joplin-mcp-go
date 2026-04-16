// Package tools registers all MCP tools for the Joplin MCP server.
package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/r1gm/joplin-mcp-go/joplin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TextResult is the structured output type shared by all tools.
// It wraps a JSON or plain-text payload returned from the Joplin API.
type TextResult struct {
	Result string `json:"result"`
}

// RegisterAll registers all Joplin tools on the given MCP server.
func RegisterAll(server *mcp.Server, client *joplin.Client) {
	registerNoteTools(server, client)
	registerFolderTools(server, client)
	registerTagTools(server, client)
	registerSearchTools(server, client)
}

// resolveNotebookArg resolves a notebook from either an ID or a name/path.
// At least one of notebookID or notebookName must be non-empty.
// If both are provided, notebookID wins.
// Returns a user-friendly error that an LLM can act on (includes candidates on ambiguity).
func resolveNotebookArg(client *joplin.Client, notebookID, notebookName string) (string, error) {
	if notebookID != "" {
		return notebookID, nil
	}
	if notebookName == "" {
		return "", fmt.Errorf("must provide either notebook_id or notebook_name")
	}
	ref, err := client.ResolveNotebook(notebookName)
	if err != nil {
		var ambig *joplin.AmbiguousNotebookError
		if errors.As(err, &ambig) {
			return "", err
		}
		var nf *joplin.NotebookNotFoundError
		if errors.As(err, &nf) {
			return "", err
		}
		return "", fmt.Errorf("resolve notebook %q: %w", notebookName, err)
	}
	return ref.ID, nil
}

// resolveTagArg resolves a tag from either an ID or a name.
// At least one of tagID or tagName must be non-empty.
// If both are provided, tagID wins.
func resolveTagArg(client *joplin.Client, tagID, tagName string) (string, error) {
	if tagID != "" {
		return tagID, nil
	}
	if tagName == "" {
		return "", fmt.Errorf("must provide either tag_id or tag_name")
	}
	ref, err := client.ResolveTag(tagName)
	if err != nil {
		var ambig *joplin.AmbiguousTagError
		if errors.As(err, &ambig) {
			return "", err
		}
		var nf *joplin.TagNotFoundError
		if errors.As(err, &nf) {
			return "", err
		}
		return "", fmt.Errorf("resolve tag %q: %w", tagName, err)
	}
	return ref.ID, nil
}

// todoFilter builds a Joplin search query fragment for task and completion filtering.
// task: "any" (default / empty), "todo", "note"
// completed: nil = don't filter, true = only completed, false = only uncompleted
// Returns fragments that can be appended to a query like "my search " + fragment.
func todoFilter(task string, completed *bool) string {
	var parts []string
	switch strings.ToLower(task) {
	case "todo":
		parts = append(parts, "type:todo")
	case "note":
		parts = append(parts, "type:note")
	}
	if completed != nil {
		if *completed {
			parts = append(parts, "iscompleted:1")
		} else {
			parts = append(parts, "iscompleted:0")
		}
	}
	return strings.Join(parts, " ")
}
