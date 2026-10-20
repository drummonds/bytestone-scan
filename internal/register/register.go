// Package register reads life-weaver's register of people, the records
// behind i.bytestone.uk, to check that a batch's home person exists.
package register

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Register is a directory of person records, <id>.yaml.
type Register struct{ Dir string }

// Person is the name of the person with id.
func (r Register) Person(id pid.ID) (string, error) {
	path := filepath.Join(r.Dir, id.String()+".yaml")
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("person %s is not in the register %s", id, r.Dir)
	}
	if err != nil {
		return "", err
	}
	var rec struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(b, &rec); err != nil {
		return "", fmt.Errorf("person %s: %w", id, err)
	}
	if rec.Name == "" {
		return "", fmt.Errorf("person %s: record has no name", id)
	}
	return rec.Name, nil
}
