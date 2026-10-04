package modsync

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/remote"
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

func TestSynchronizerSyncDeletesOldAndDownloadsMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "old.jar"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	contents := []byte("new mod")
	hash := checksum(contents)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/map":
			_, _ = w.Write([]byte(`[{"dir":"mods","name":"new.jar","hash":"` + hash + `","size":7}]`))
		case "/mods/" + hash:
			_, _ = w.Write(contents)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := remote.NewWithTimeout(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := New(client, root, 2).Sync("branch")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Deleted != 1 || result.Downloaded != 1 {
		t.Fatalf("результат = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "old.jar")); !os.IsNotExist(err) {
		t.Fatalf("old.jar должен быть удалён: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "new.jar"))
	if err != nil || string(got) != string(contents) {
		t.Fatalf("new.jar = %q, ошибка = %v", got, err)
	}
}

func TestWriteJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "map.json")
	if err := WriteJSON(path, []LocalMod{{Hash: "abc", Path: "mod.jar"}}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "[{\"hash\":\"abc\",\"path\":\"mod.jar\"}]" {
		t.Fatalf("JSON = %q", contents)
	}
}

func checksum(contents []byte) string {
	sum := sha1.Sum(contents)
	return hex.EncodeToString(sum[:])
}
