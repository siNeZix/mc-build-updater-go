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

func TestLoadOrCreateReadsExistingConfiguration(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, FileName)
	if err := os.WriteFile(path, []byte("Branch: neko-land\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	configuration, err := LoadOrCreate(directory)
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if configuration.Branch != "neko-land" {
		t.Fatalf("ветка = %q, ожидается neko-land", configuration.Branch)
	}
}

func TestLoadOrCreateRejectsMissingBranchAndInvalidYAML(t *testing.T) {
	for name, contents := range map[string]string{
		"без ветки":     "Other: value\n",
		"неверный YAML": "Branch: [\n",
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, FileName), []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadOrCreate(directory); err == nil {
				t.Fatal("LoadOrCreate должен вернуть ошибку")
			}
		})
	}
}
