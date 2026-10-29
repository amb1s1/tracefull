package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrecedence(t *testing.T) {
	user, project := t.TempDir(), t.TempDir()
	if e := os.MkdirAll(filepath.Join(user, "tracefull"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(user, "tracefull", "config.yaml"), []byte("format: json\nrdns: false\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(project, "tracefull.yaml"), []byte("format: markdown\nmap: [192.0.2.1=leaf]\n"), 0600); e != nil {
		t.Fatal(e)
	}
	v, e := Values(user, project)
	if e != nil || v["format"] != "markdown" || v["rdns"] != "false" || v["map"] != "192.0.2.1=leaf" {
		t.Fatalf("%v %v", v, e)
	}
}
func TestInvalid(t *testing.T) {
	for _, s := range []string{"format: [", "format: null", "format: {x: y}", "format: json\nformat: text", "rdns_timeout: 1s\nrdns-timeout: 2s", "map: [{x: y}]", "format: json\n---\nformat: text"} {
		if _, e := Decode([]byte(s)); e == nil {
			t.Fatal(s)
		}
	}
}

func TestAliasLayers(t *testing.T) {
	user, project := t.TempDir(), t.TempDir()
	if e := os.MkdirAll(filepath.Join(user, "tracefull"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(user, "tracefull", "config.yaml"), []byte("json: true\nno_rdns: true\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(project, "tracefull.yaml"), []byte("format: markdown\nrdns: true\n"), 0600); e != nil {
		t.Fatal(e)
	}
	v, e := Values(user, project)
	if e != nil || v["format"] != "markdown" || v["rdns"] != "true" {
		t.Fatalf("alias leaked across layers: %v %v", v, e)
	}
	for _, s := range []string{"json: true\nmarkdown: true", "json: true\nformat: text", "rdns: true\nno_rdns: true", "json: nope"} {
		if _, e := Decode([]byte(s)); e == nil {
			t.Fatal(s)
		}
	}
}
