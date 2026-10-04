package uploader

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestUploadMissingUploadsOnlyNewOrChangedMods(t *testing.T) {
	root := t.TempDir()
	writeLocal(t, filepath.Join(root, "same.jar"), "same")
	writeLocal(t, filepath.Join(root, "changed.jar"), "changed")
	writeLocal(t, filepath.Join(root, "new.jar"), "new")
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	uploaded := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/map":
			_, _ = w.Write([]byte(`[{"dir":"mods","name":"same.jar","hash":"` + hash("same") + `"},{"dir":"mods","name":"changed.jar","hash":"old"}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/files/mods/changed.jar":
			assertToken(t, r)
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			uploaded["changed.jar"] = string(body)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && r.URL.Path == "/api/files/mods/new.jar":
			assertToken(t, r)
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			uploaded["new.jar"] = string(body)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	err := UploadMissing(Config{BaseURL: server.URL, Timeout: time.Second, Token: "secret", LocalModsPath: root, Workers: 2})
	if err != nil {
		t.Fatalf("UploadMissing: %v", err)
	}
	if len(uploaded) != 2 || uploaded["changed.jar"] != "changed" || uploaded["new.jar"] != "new" {
		t.Fatalf("загруженные файлы = %#v", uploaded)
	}
}

func TestUploadMissingRequiresTokenAndReturnsUploadError(t *testing.T) {
	if err := UploadMissing(Config{}); err == nil {
		t.Fatal("UploadMissing без токена должен вернуть ошибку")
	}

	root := t.TempDir()
	writeLocal(t, filepath.Join(root, "new.jar"), "new")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("[]"))
			return
		}
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer server.Close()
	if err := UploadMissing(Config{BaseURL: server.URL, Token: "secret", LocalModsPath: root}); err == nil {
		t.Fatal("ошибочный upload должен вернуть ошибку")
	}
}

func TestListLocalIgnoresDirectoriesAndHashesFiles(t *testing.T) {
	root := t.TempDir()
	writeLocal(t, filepath.Join(root, "one.jar"), "one")
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := listLocal(root)
	if err != nil {
		t.Fatalf("listLocal: %v", err)
	}
	if len(files) != 1 || files[0].name != "one.jar" || files[0].hash != hash("one") {
		t.Fatalf("файлы = %#v", files)
	}
}

func assertToken(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer secret" {
		t.Errorf("Authorization = %q, ожидается Bearer secret", got)
	}
}

func writeLocal(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hash(contents string) string {
	sum := sha1.Sum([]byte(contents))
	return hex.EncodeToString(sum[:])
}
