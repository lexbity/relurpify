package security

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

func testDecode(path string, data []byte, out any) (any, error) {
	_ = path
	parts := bytes.SplitN(data, []byte("\n"), 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("missing document body")
	}
	dec := yaml.NewDecoder(bytes.NewReader(parts[1]))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return nil, err
	}
	return out, nil
}
