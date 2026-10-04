package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateCreatesCompatibleDefault(t *testing.T) {
	directory := t.TempDir()
	configuration, err := LoadOrCreate(directory)
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Branch != "dead-inside-land" {
		t.Fatalf("unexpected default branch: %q", configuration.Branch)
	}

	contents, err := os.ReadFile(filepath.Join(directory, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "Branch: dead-inside-land\n" {
		t.Fatalf("unexpected configuration: %q", contents)
	}
}
