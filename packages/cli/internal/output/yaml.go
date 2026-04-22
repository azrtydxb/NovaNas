package output

import (
	"io"

	"gopkg.in/yaml.v3"
)

// YAML writes v as YAML to w.
func YAML(w io.Writer, v any) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer enc.Close()
	return enc.Encode(v)
}
