// Package output formats resource payloads for human or machine consumption.
package output

import (
	"encoding/json"
	"io"
)

// JSON writes v as indented JSON to w.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
