package joplin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// NotebookRef represents a notebook with its full path resolved.
type NotebookRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"` // e.g. "Projects/Work"
}

// AmbiguousNotebookError is returned when a notebook name matches multiple notebooks.
type AmbiguousNotebookError struct {
	Name       string
	Candidates []NotebookRef
}

func (e *AmbiguousNotebookError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "multiple notebooks match %q. Use a full path or notebook_id. Candidates:\n", e.Name)
	for _, c := range e.Candidates {
		fmt.Fprintf(&sb, "  - id=%s  path=%q\n", c.ID, c.Path)
	}
	return sb.String()
}

// NotebookNotFoundError is returned when no notebook matches.
type NotebookNotFoundError struct {
	Name string
}

func (e *NotebookNotFoundError) Error() string {
	return fmt.Sprintf("no notebook found matching %q", e.Name)
}

// flatFolder is used internally to walk the folder tree.
type flatFolder struct {
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	ParentID string       `json:"parent_id"`
	Children []flatFolder `json:"children"`
}

// ListAllNotebooks returns all notebooks with their full paths, flattened from Joplin's tree.
func (c *Client) ListAllNotebooks() ([]NotebookRef, error) {
	params := url.Values{}
	params.Set("fields", "id,title,parent_id")

	body, err := c.doRequest("GET", "/folders", params, nil)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}

	// Joplin returns folders as a tree when no pagination is used.
	// It may be either a tree (array of root folders with children)
	// or a paginated envelope. Handle both.
	var roots []flatFolder
	if err := json.Unmarshal(body, &roots); err != nil {
		// Try paginated envelope
		var env struct {
			Items []flatFolder `json:"items"`
		}
		if err2 := json.Unmarshal(body, &env); err2 != nil {
			return nil, fmt.Errorf("parse folders: %w", err)
		}
		roots = env.Items
	}

	var refs []NotebookRef
	walkFolders(roots, "", &refs)
	return refs, nil
}

func walkFolders(folders []flatFolder, parentPath string, out *[]NotebookRef) {
	for _, f := range folders {
		path := f.Title
		if parentPath != "" {
			path = parentPath + "/" + f.Title
		}
		*out = append(*out, NotebookRef{
			ID:    f.ID,
			Title: f.Title,
			Path:  path,
		})
		if len(f.Children) > 0 {
			walkFolders(f.Children, path, out)
		}
	}
}

// ResolveNotebook resolves a notebook name or path to a single notebook ID.
//
// Matching rules (tried in order):
//  1. If nameOrPath is a 32-char hex string, it's treated as an ID and returned as-is
//     (after verifying the notebook exists).
//  2. Exact path match: "Projects/Work" matches only that notebook, case-sensitive.
//  3. Exact title match: "Work" matches any notebook with that title.
//     If multiple match, returns AmbiguousNotebookError with candidates.
//  4. Case-insensitive title match as fallback.
//
// Returns NotebookNotFoundError if nothing matches.
func (c *Client) ResolveNotebook(nameOrPath string) (*NotebookRef, error) {
	nameOrPath = strings.TrimSpace(nameOrPath)
	if nameOrPath == "" {
		return nil, fmt.Errorf("notebook name is empty")
	}

	// Case 1: looks like a Joplin ID (32 hex chars)
	if isJoplinID(nameOrPath) {
		params := url.Values{}
		params.Set("fields", "id,title,parent_id")
		body, err := c.GetFolder(nameOrPath, params)
		if err != nil {
			return nil, fmt.Errorf("notebook id %s not found: %w", nameOrPath, err)
		}
		var f flatFolder
		if err := json.Unmarshal(body, &f); err != nil {
			return nil, fmt.Errorf("parse folder: %w", err)
		}
		return &NotebookRef{ID: f.ID, Title: f.Title, Path: f.Title}, nil
	}

	refs, err := c.ListAllNotebooks()
	if err != nil {
		return nil, err
	}

	// Case 2: exact path match (case-sensitive)
	var pathMatches []NotebookRef
	for _, r := range refs {
		if r.Path == nameOrPath {
			pathMatches = append(pathMatches, r)
		}
	}
	if len(pathMatches) == 1 {
		return &pathMatches[0], nil
	}
	if len(pathMatches) > 1 {
		// Extremely unusual, but possible if Joplin allows siblings with the same name
		return nil, &AmbiguousNotebookError{Name: nameOrPath, Candidates: pathMatches}
	}

	// Case 3: exact title match (case-sensitive)
	var titleMatches []NotebookRef
	for _, r := range refs {
		if r.Title == nameOrPath {
			titleMatches = append(titleMatches, r)
		}
	}
	if len(titleMatches) == 1 {
		return &titleMatches[0], nil
	}
	if len(titleMatches) > 1 {
		return nil, &AmbiguousNotebookError{Name: nameOrPath, Candidates: titleMatches}
	}

	// Case 4: case-insensitive title match as last resort
	lower := strings.ToLower(nameOrPath)
	var ciMatches []NotebookRef
	for _, r := range refs {
		if strings.ToLower(r.Title) == lower || strings.ToLower(r.Path) == lower {
			ciMatches = append(ciMatches, r)
		}
	}
	if len(ciMatches) == 1 {
		return &ciMatches[0], nil
	}
	if len(ciMatches) > 1 {
		return nil, &AmbiguousNotebookError{Name: nameOrPath, Candidates: ciMatches}
	}

	return nil, &NotebookNotFoundError{Name: nameOrPath}
}

// isJoplinID returns true if s looks like a 32-char hex Joplin ID.
func isJoplinID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}
