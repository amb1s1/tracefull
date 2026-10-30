package evidence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateExclusiveEvidence(t *testing.T) {
	dir, err := Create(filepath.Join(t.TempDir(), "evidence"), "unused")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Write(dir, "out", []byte("saved")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"out", "../escape", "", ".", "..", "a/b"} {
		if _, err = Write(dir, name, []byte("replace")); err == nil {
			t.Fatal(name)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil || string(data) != "saved" {
		t.Fatal(err, string(data))
	}
	if _, err = JSON(dir, "invalid.json", make(chan int)); err == nil {
		t.Fatal("unsupported JSON")
	}
}
