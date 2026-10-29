// Package config reads configuration overlays without depending on CLI frameworks.
package config

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Values applies user then project config. Environment and flags are applied by the adapter.
func Values(userDir, projectDir string) (map[string]string, error) {
	out := map[string]string{}
	for _, path := range []string{filepath.Join(userDir, "tracefull", "config.yaml"), filepath.Join(projectDir, "tracefull.yaml")} {
		b, e := os.ReadFile(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return nil, fmt.Errorf("config %s: %w", path, e)
		}
		values, e := Decode(b)
		if e != nil {
			return nil, fmt.Errorf("config %s: %w", path, e)
		}
		for k, v := range values {
			out[k] = v
		}
	}
	return out, nil
}
func Decode(b []byte) (map[string]string, error) {
	var m map[string]yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(b))
	if e := d.Decode(&m); e != nil {
		return nil, e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return nil, fmt.Errorf("config must contain exactly one document")
	}
	out := map[string]string{}
	for k, v := range m {
		key := strings.ReplaceAll(k, "_", "-")
		if _, ok := out[key]; ok {
			return nil, fmt.Errorf("duplicate config key %s", key)
		}
		switch v.Kind {
		case yaml.ScalarNode:
			if v.Tag == "!!null" {
				return nil, fmt.Errorf("null config value %s", k)
			}
			out[key] = v.Value
		case yaml.SequenceNode:
			var buf bytes.Buffer
			w := csv.NewWriter(&buf)
			fields := []string{}
			for _, n := range v.Content {
				if n.Kind != yaml.ScalarNode {
					return nil, fmt.Errorf("invalid list %s", k)
				}
				fields = append(fields, n.Value)
			}
			if e := w.Write(fields); e != nil {
				return nil, e
			}
			w.Flush()
			out[key] = strings.TrimSuffix(buf.String(), "\n")
		default:
			return nil, fmt.Errorf("config %s must be scalar or list", k)
		}
	}
	return Normalize(out)
}
func UserDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return d
	}
	h, e := os.UserHomeDir()
	if e != nil {
		return ".config"
	}
	return filepath.Join(h, ".config")
}

// Normalize resolves aliases within one precedence layer before layers are merged.
func Normalize(values map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range values {
		out[k] = v
	}
	selected := ""
	for _, key := range []string{"json", "markdown"} {
		if raw, ok := out[key]; ok {
			enabled, e := strconv.ParseBool(raw)
			if e != nil {
				return nil, fmt.Errorf("config %s: %w", key, e)
			}
			delete(out, key)
			if enabled {
				if selected != "" {
					return nil, fmt.Errorf("json and markdown conflict within configuration layer")
				}
				selected = key
			}
		}
	}
	if selected != "" {
		if format, ok := out["format"]; ok && format != selected {
			return nil, fmt.Errorf("format and shorthand conflict within configuration layer")
		}
		out["format"] = selected
	}
	if raw, ok := out["no-rdns"]; ok {
		disabled, e := strconv.ParseBool(raw)
		if e != nil {
			return nil, e
		}
		delete(out, "no-rdns")
		if disabled {
			if raw, ok := out["rdns"]; ok {
				enabled, e := strconv.ParseBool(raw)
				if e != nil {
					return nil, e
				}
				if enabled {
					return nil, fmt.Errorf("rdns and no-rdns conflict within configuration layer")
				}
			}
			out["rdns"] = "false"
		}
	}
	return out, nil
}
