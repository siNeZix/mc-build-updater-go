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
		switch {
		case r.URL.Path == "/map" && r.URL.Query().Get("dir") == "mods":
			_, _ = w.Write([]byte(`[{"dir":"mods","name":"new.jar","hash":"` + hash + `","size":7}]`))
		case r.URL.Path == "/map" && (r.URL.Query().Get("dir") == "resourcepacks" || r.URL.Query().Get("dir") == "shaderpacks"):
			_, _ = w.Write([]byte("[]"))
		case r.URL.Path == "/mods/"+hash:
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

func TestSynchronizerSyncDownloadsPacksAndPreservesLocalFiles(t *testing.T) {
	root := t.TempDir()
	modsPath := filepath.Join(root, "mods")
	resourcePacksPath := filepath.Join(root, "resourcepacks")
	shaderPacksPath := filepath.Join(root, "shaderpacks")
	if err := os.MkdirAll(resourcePacksPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(shaderPacksPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resourcePacksPath, "local.zip"), []byte("local pack"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shaderPacksPath, "local.zip"), []byte("local shader"), 0o644); err != nil {
		t.Fatal(err)
	}

	resourcePack := []byte("remote pack")
	shaderPack := []byte("remote shader")
	resourcePackHash := checksum(resourcePack)
	shaderPackHash := checksum(shaderPack)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/map" && r.URL.Query().Get("dir") == "mods":
			_, _ = w.Write([]byte("[]"))
		case r.URL.Path == "/map" && r.URL.Query().Get("dir") == "resourcepacks":
			_, _ = w.Write([]byte(`[{"dir":"resourcepacks","name":"remote.zip","hash":"` + resourcePackHash + `","size":11}]`))
		case r.URL.Path == "/map" && r.URL.Query().Get("dir") == "shaderpacks":
			_, _ = w.Write([]byte(`[{"dir":"shaderpacks","name":"remote.zip","hash":"` + shaderPackHash + `","size":13}]`))
		case r.URL.Path == "/resourcepacks/"+resourcePackHash:
			_, _ = w.Write(resourcePack)
		case r.URL.Path == "/shaderpacks/"+shaderPackHash:
			_, _ = w.Write(shaderPack)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := remote.NewWithTimeout(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := New(client, modsPath, 2).Sync("branch")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Downloaded != 2 || result.Deleted != 0 {
		t.Fatalf("результат = %#v", result)
	}
	for path, want := range map[string]string{
		filepath.Join(resourcePacksPath, "local.zip"):  "local pack",
		filepath.Join(resourcePacksPath, "remote.zip"): "remote pack",
		filepath.Join(shaderPacksPath, "local.zip"):    "local shader",
		filepath.Join(shaderPacksPath, "remote.zip"):   "remote shader",
	} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != want {
			t.Fatalf("файл %s = %q, ошибка = %v", path, contents, err)
		}
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
