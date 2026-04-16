package tools

import "encoding/json"

// jsonUnmarshal is a thin wrapper to keep folders.go tidy.
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
