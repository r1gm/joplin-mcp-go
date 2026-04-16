// Package joplin provides a client for the Joplin Data API.
// It communicates with the Joplin desktop app's REST API on localhost.
package joplin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the Joplin Data API running on localhost.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient creates a Joplin API client.
// port is the Joplin clipper port (default 41184).
// token is the authorization token from Joplin > Options > Web Clipper.
func NewClient(port int, token string) *Client {
	return &Client{
		BaseURL: fmt.Sprintf("http://localhost:%d", port),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// PaginatedResponse is the envelope returned by list endpoints.
type PaginatedResponse struct {
	Items   json.RawMessage `json:"items"`
	HasMore bool            `json:"has_more"`
}

// Ping checks if the Joplin service is available.
func (c *Client) Ping() (string, error) {
	body, err := c.doRequest("GET", "/ping", nil, nil)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ---- Notes ----

// GetNotes returns all notes with optional query parameters.
func (c *Client) GetNotes(params url.Values) (json.RawMessage, error) {
	return c.doRequest("GET", "/notes", params, nil)
}

// GetNote returns a single note by ID.
func (c *Client) GetNote(id string, params url.Values) (json.RawMessage, error) {
	return c.doRequest("GET", "/notes/"+id, params, nil)
}

// GetNoteTags returns all tags attached to a note.
func (c *Client) GetNoteTags(noteID string, params url.Values) (json.RawMessage, error) {
	return c.doRequest("GET", "/notes/"+noteID+"/tags", params, nil)
}

// GetNoteResources returns all resources attached to a note.
func (c *Client) GetNoteResources(noteID string, params url.Values) (json.RawMessage, error) {
	return c.doRequest("GET", "/notes/"+noteID+"/resources", params, nil)
}

// CreateNote creates a new note.
func (c *Client) CreateNote(data map[string]interface{}) (json.RawMessage, error) {
	return c.doRequest("POST", "/notes", nil, data)
}

// UpdateNote updates properties of a note.
func (c *Client) UpdateNote(id string, data map[string]interface{}) (json.RawMessage, error) {
	return c.doRequest("PUT", "/notes/"+id, nil, data)
}

// DeleteNote deletes a note (moves to trash by default).
func (c *Client) DeleteNote(id string, permanent bool) error {
	params := url.Values{}
	if permanent {
		params.Set("permanent", "1")
	}
	_, err := c.doRequest("DELETE", "/notes/"+id, params, nil)
	return err
}

// ---- Folders (Notebooks) ----

// GetFolders returns all folders as a tree.
func (c *Client) GetFolders(params url.Values) (json.RawMessage, error) {
	return c.doRequest("GET", "/folders", params, nil)
}

// GetFolder returns a single folder by ID.
func (c *Client) GetFolder(id string, params url.Values) (json.RawMessage, error) {
	return c.doRequest("GET", "/folders/"+id, params, nil)
}

// GetFolderNotes returns all notes in a folder.
func (c *Client) GetFolderNotes(folderID string, params url.Values) (json.RawMessage, error) {
	return c.doRequest("GET", "/folders/"+folderID+"/notes", params, nil)
}

// CreateFolder creates a new folder.
func (c *Client) CreateFolder(data map[string]interface{}) (json.RawMessage, error) {
	return c.doRequest("POST", "/folders", nil, data)
}

// UpdateFolder updates properties of a folder.
func (c *Client) UpdateFolder(id string, data map[string]interface{}) (json.RawMessage, error) {
	return c.doRequest("PUT", "/folders/"+id, nil, data)
}

// DeleteFolder deletes a folder.
func (c *Client) DeleteFolder(id string, permanent bool) error {
	params := url.Values{}
	if permanent {
		params.Set("permanent", "1")
	}
	_, err := c.doRequest("DELETE", "/folders/"+id, params, nil)
	return err
}

// ---- Tags ----

// GetTags returns all tags.
func (c *Client) GetTags(params url.Values) (json.RawMessage, error) {
	return c.doRequest("GET", "/tags", params, nil)
}

// GetTag returns a single tag by ID.
func (c *Client) GetTag(id string, params url.Values) (json.RawMessage, error) {
	return c.doRequest("GET", "/tags/"+id, params, nil)
}

// GetTagNotes returns all notes with a given tag.
func (c *Client) GetTagNotes(tagID string, params url.Values) (json.RawMessage, error) {
	return c.doRequest("GET", "/tags/"+tagID+"/notes", params, nil)
}

// CreateTag creates a new tag.
func (c *Client) CreateTag(data map[string]interface{}) (json.RawMessage, error) {
	return c.doRequest("POST", "/tags", nil, data)
}

// AddTagToNote adds a tag to a note.
func (c *Client) AddTagToNote(tagID string, noteID string) (json.RawMessage, error) {
	data := map[string]interface{}{"id": noteID}
	return c.doRequest("POST", "/tags/"+tagID+"/notes", nil, data)
}

// UpdateTag updates properties of a tag.
func (c *Client) UpdateTag(id string, data map[string]interface{}) (json.RawMessage, error) {
	return c.doRequest("PUT", "/tags/"+id, nil, data)
}

// DeleteTag deletes a tag.
func (c *Client) DeleteTag(id string) error {
	_, err := c.doRequest("DELETE", "/tags/"+id, nil, nil)
	return err
}

// RemoveTagFromNote removes a tag from a note.
func (c *Client) RemoveTagFromNote(tagID string, noteID string) error {
	_, err := c.doRequest("DELETE", "/tags/"+tagID+"/notes/"+noteID, nil, nil)
	return err
}

// ---- Search ----

// Search searches for notes or other items.
//
// Query syntax supported by Joplin:
//   - "exact phrase"
//   - title:word, body:word
//   - -word (exclude)
//   - word1 OR word2
//   - tag:tagname, notebook:"Name"
//   - type:note, type:todo
//   - iscompleted:0, iscompleted:1
//   - Wildcard "*" matches everything.
func (c *Client) Search(query string, params url.Values) (json.RawMessage, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("query", query)
	return c.doRequest("GET", "/search", params, nil)
}

// ---- Internal HTTP ----

func (c *Client) doRequest(method, path string, params url.Values, body interface{}) (json.RawMessage, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("token", c.Token)

	reqURL := c.BaseURL + path + "?" + params.Encode()

	var reqBody io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequest(method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s failed: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Try to extract Joplin error message
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("joplin API error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return nil, fmt.Errorf("joplin API error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return json.RawMessage(respBody), nil
}
