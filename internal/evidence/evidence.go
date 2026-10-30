// Package evidence persists exclusive private files with content attribution.
package evidence

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

func Create(path, prefix string) (string, error) {
	var err error
	if path == "" {
		path, err = os.MkdirTemp(".", prefix)
	} else {
		err = os.Mkdir(path, 0700)
	}
	if err != nil {
		return "", fmt.Errorf("create new evidence directory: %w", err)
	}
	return filepath.Abs(path)
}
func Write(dir, name string, data []byte) (*File, error) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return nil, fmt.Errorf("evidence name must be a filename")
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("create evidence: %w", err)
	}
	_, writeErr := f.Write(data)
	if err = errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return nil, fmt.Errorf("persist evidence: %w", err)
	}
	return &File{Path: name, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Bytes: len(data)}, nil
}
func JSON(dir, name string, value any) (*File, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return Write(dir, name, append(data, '\n'))
}
