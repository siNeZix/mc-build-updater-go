package filemap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildScansRegularFilesAndSortsByPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "z.jar"), "z")
	writeFile(t, filepath.Join(root, "mods", "a.jar"), "a")
	writeFile(t, filepath.Join(root, "mods", "nested", "b.jar"), "b")
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries, err := Build(root, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("число записей = %d, ожидается 3", len(entries))
	}

	want := []struct {
		name string
		dir  string
		size int64
		hash string
	}{
		{"a.jar", "mods", 1, "86f7e437faa5a7fce15d1ddcb9eaeaea377667b8"},
		{"b.jar", "mods/nested", 1, "e9d71f5ee7c92d6dc9e92ffdad17b8bd49418f98"},
		{"z.jar", "", 1, "395df8f7c51f007019cb30201c49e884b46b92fa"},
	}
	for index, entry := range entries {
		if entry.Name != want[index].name || entry.Dir != want[index].dir || entry.Size != want[index].size || entry.Hash != want[index].hash {
			t.Fatalf("запись %d = %#v, ожидается %+v", index, entry, want[index])
		}
		if !filepath.IsAbs(entry.Path) {
			t.Fatalf("путь должен быть абсолютным: %q", entry.Path)
		}
	}
}

func TestBuildSkipsSelectedFiles(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "keep.jar")
	skip := filepath.Join(root, ".file-hosting.sqlite")
	writeFile(t, keep, "keep")
	writeFile(t, skip, "database")

	entries, err := Build(root, func(path string) bool { return path == skip })
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != keep {
		t.Fatalf("карта = %#v, ожидается только %q", entries, keep)
	}
}

func TestBuildRejectsMissingPathAndFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := Build(missing, nil); err == nil {
		t.Fatal("Build для отсутствующего пути должен вернуть ошибку")
	}

	file := filepath.Join(t.TempDir(), "not-a-directory")
	writeFile(t, file, "content")
	if _, err := Build(file, nil); err == nil {
		t.Fatal("Build для файла вместо каталога должен вернуть ошибку")
	}
}

func TestSHA1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mod.jar")
	writeFile(t, path, "abc")

	got, err := SHA1(path)
	if err != nil {
		t.Fatalf("SHA1: %v", err)
	}
	if want := "a9993e364706816aba3e25717850c26c9cd0d89d"; got != want {
		t.Fatalf("SHA1 = %q, ожидается %q", got, want)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
