package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

var quotedValue = regexp.MustCompile("`[^`]*`")

// decodeStrict reads a yaml file and refuses unknown keys. Error text never carries a value.
func decodeStrict(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("file %s is missing", path)
		}
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("file %s is empty", path)
		}
		return fmt.Errorf("invalid %s: %s", path, quotedValue.ReplaceAllString(err.Error(), "<value>"))
	}
	return nil
}
