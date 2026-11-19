package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxConfigBytes bounds the configuration file size.
const maxConfigBytes = 1 << 20

// fileConfig mirrors the JSON file; names and action IDs are validated later.
type fileConfig struct {
	Theme string            `json:"theme"`
	Keys  map[string]KeyList `json:"keys"`
}

// KeyList is an action's binding: a string or a list of strings.
type KeyList []string

// UnmarshalJSON accepts a string or a string array and rejects null.
func (k *KeyList) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("la lista de teclas no admite null")
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*k = KeyList{single}

		return nil
	}

	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return errors.New("se esperaba una cadena o una lista de cadenas")
	}

	*k = KeyList(list)

	return nil
}

// keyOverrides returns the raw bindings, or nil when there are none.
func (f fileConfig) keyOverrides() map[string][]string {
	if len(f.Keys) == 0 {
		return nil
	}

	keys := make(map[string][]string, len(f.Keys))
	for action, list := range f.Keys {
		keys[action] = []string(list)
	}

	return keys
}

// resolveConfigPath prefers an explicit flag/env path over the XDG default.
func resolveConfigPath(fromFlag string) (path string, explicit bool) {
	if trimmed := strings.TrimSpace(fromFlag); trimmed != "" {
		return trimmed, true
	}

	return defaultConfigPath(), false
}

// defaultConfigPath returns the XDG config file, or an empty string.
func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return ""
	}

	return filepath.Join(dir, "tui-editor", "config.json")
}

// loadFile reads the file; a missing default file is ignored.
func loadFile(path string, explicit bool) (fileConfig, bool, error) {
	if strings.TrimSpace(path) == "" {
		return fileConfig{}, false, nil
	}

	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if explicit {
			return fileConfig{}, false, fmt.Errorf("%w: %s no existe", ErrConfigFile, path)
		}

		return fileConfig{}, false, nil
	case err != nil:
		return fileConfig{}, false, fmt.Errorf("%w: %s: %w", ErrConfigFile, path, err)
	case info.IsDir():
		return fileConfig{}, false, fmt.Errorf("%w: %s es un directorio", ErrConfigFile, path)
	case info.Size() > maxConfigBytes:
		return fileConfig{}, false, fmt.Errorf("%w: %s supera %d bytes", ErrConfigFile, path, maxConfigBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, false, fmt.Errorf("%w: %s: %w", ErrConfigFile, path, err)
	}

	cfg, err := decodeFile(data, path)
	if err != nil {
		return fileConfig{}, false, err
	}

	return cfg, true, nil
}

// decodeFile rejects unknown fields and trailing data.
func decodeFile(data []byte, path string) (fileConfig, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var cfg fileConfig
	if err := dec.Decode(&cfg); err != nil {
		return fileConfig{}, fmt.Errorf("%w: %s: %w", ErrConfigFormat, path, err)
	}

	if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return fileConfig{}, fmt.Errorf("%w: %s: datos extra tras el objeto JSON", ErrConfigFormat, path)
	}

	return cfg, nil
}
