package ui

import (
	"encoding/csv"
	"fmt"
	"github.com/amb1s1/tracefull/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"os"
	"sort"
	"strings"
)

// Overlay applies defaults < user config < project config < environment < flags.
func Overlay(c *cobra.Command, user, project string) error {
	values, e := config.Values(user, project)
	if e != nil {
		return e
	}
	flags := c.Flags()
	env := map[string]string{}
	flags.VisitAll(func(f *pflag.Flag) {
		key := "TRACEFULL_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if v, ok := os.LookupEnv(key); ok {
			env[f.Name] = v
		}
	})
	normalized, e := config.Normalize(env)
	if e != nil {
		return e
	}
	for k, v := range normalized {
		values[k] = v
	}
	keys := []string{}
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f := flags.Lookup(k)
		if f == nil || k == "help" {
			return fmt.Errorf("unknown configuration key %q", k)
		}
		if !f.Changed {
			if e := setOverlay(f, values[k]); e != nil {
				return fmt.Errorf("config %s: %w", k, e)
			}
		}
	}
	return nil
}

func setOverlay(f *pflag.Flag, value string) error {
	if slice, ok := f.Value.(pflag.SliceValue); ok {
		values := []string{}
		if value != "" {
			var err error
			values, err = csv.NewReader(strings.NewReader(value)).Read()
			if err != nil {
				return err
			}
		}
		return slice.Replace(values)
	}
	return f.Value.Set(value)
}
