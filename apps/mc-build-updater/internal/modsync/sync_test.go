package modsync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalMapIncludesOnlyDirectRegularFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one.jar"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "two.jar"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

	mods, err := LocalMap(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 {
		t.Fatalf("got %d mods, want 1", len(mods))
	}
	if mods[0].Hash != "fe05bcdcdc4928012781a5f1a2a77cbb5398e106" {
		t.Fatalf("unexpected checksum: %s", mods[0].Hash)
	}
}
