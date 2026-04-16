package joplin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// TagRef represents a resolved tag.
type TagRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// AmbiguousTagError is returned when a tag name matches multiple tags (rare but possible).
type AmbiguousTagError struct {
	Name       string
	Candidates []TagRef
}

func (e *AmbiguousTagError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "multiple tags match %q. Use tag_id instead. Candidates:\n", e.Name)
	for _, c := range e.Candidates {
		fmt.Fprintf(&sb, "  - id=%s  title=%q\n", c.ID, c.Title)
	}
	return sb.String()
}

// TagNotFoundError is returned when no tag matches.
type TagNotFoundError struct {
	Name string
}

func (e *TagNotFoundError) Error() string {
	return fmt.Sprintf("no tag found matching %q", e.Name)
}

// ListAllTags returns every tag in the Joplin instance, paginating through the API.
func (c *Client) ListAllTags() ([]TagRef, error) {
	var all []TagRef
	page := 1
	for {
		params := url.Values{}
		params.Set("fields", "id,title")
		params.Set("limit", "100")
		params.Set("page", fmt.Sprintf("%d", page))

		body, err := c.GetTags(params)
		if err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}

		var env struct {
			Items   []TagRef `json:"items"`
			HasMore bool     `json:"has_more"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, fmt.Errorf("parse tags: %w", err)
		}
		all = append(all, env.Items...)
		if !env.HasMore {
			break
		}
		page++
		if page > 100 { // safety guard
			break
		}
	}
	return all, nil
}

// ResolveTag resolves a tag name or ID to a single tag.
//
// Matching rules (tried in order):
//  1. If nameOrID is a 32-char hex string, it's treated as an ID and verified.
//  2. Exact title match (case-sensitive). Joplin normalizes tag titles to lowercase,
//     but a case-sensitive pass is kept for consistency with the notebook resolver.
//  3. Case-insensitive title match.
//
// Returns TagNotFoundError if nothing matches, or AmbiguousTagError if multiple match
// (rare, but can happen if duplicate tags ever exist).
func (c *Client) ResolveTag(nameOrID string) (*TagRef, error) {
	nameOrID = strings.TrimSpace(nameOrID)
	if nameOrID == "" {
		return nil, fmt.Errorf("tag name is empty")
	}

	if isJoplinID(nameOrID) {
		params := url.Values{}
		params.Set("fields", "id,title")
		body, err := c.GetTag(nameOrID, params)
		if err != nil {
			return nil, fmt.Errorf("tag id %s not found: %w", nameOrID, err)
		}
		var t TagRef
		if err := json.Unmarshal(body, &t); err != nil {
			return nil, fmt.Errorf("parse tag: %w", err)
		}
		return &t, nil
	}

	tags, err := c.ListAllTags()
	if err != nil {
		return nil, err
	}

	var exact []TagRef
	for _, t := range tags {
		if t.Title == nameOrID {
			exact = append(exact, t)
		}
	}
	if len(exact) == 1 {
		return &exact[0], nil
	}
	if len(exact) > 1 {
		return nil, &AmbiguousTagError{Name: nameOrID, Candidates: exact}
	}

	lower := strings.ToLower(nameOrID)
	var ci []TagRef
	for _, t := range tags {
		if strings.ToLower(t.Title) == lower {
			ci = append(ci, t)
		}
	}
	if len(ci) == 1 {
		return &ci[0], nil
	}
	if len(ci) > 1 {
		return nil, &AmbiguousTagError{Name: nameOrID, Candidates: ci}
	}

	return nil, &TagNotFoundError{Name: nameOrID}
}
