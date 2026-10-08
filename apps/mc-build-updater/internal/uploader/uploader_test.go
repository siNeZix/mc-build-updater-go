package uploader

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestUploadBranchUploadsContentBeforeManifest(t *testing.T) {
	root := t.TempDir()
	modsPath := filepath.Join(root, "mods")
	resourcePacksPath := filepath.Join(root, "resourcepacks")
	shaderPacksPath := filepath.Join(root, "shaderpacks")
	writeLocal(t, filepath.Join(modsPath, "example.jar"), "example")
	writeLocal(t, filepath.Join(resourcePacksPath, "pack.zip"), "pack")
	writeLocal(t, filepath.Join(shaderPacksPath, "shader.zip"), "shader")
	var manifest []struct {
		Hash string `json:"hash"`
		Path string `json:"path"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/map":
			_, _ = w.Write([]byte("[]"))
		case "/api/files/mods/example.jar":
			assertToken(t, r)
			w.WriteHeader(http.StatusCreated)
		case "/api/files/resourcepacks/pack.zip":
			assertToken(t, r)
			w.WriteHeader(http.StatusCreated)
		case "/api/files/shaderpacks/shader.zip":
			assertToken(t, r)
			w.WriteHeader(http.StatusCreated)
		case "/api/files/MM/test.json":
			assertToken(t, r)
			if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := UploadBranch(Config{
		BaseURL:                server.URL,
		Timeout:                time.Second,
		Token:                  "secret",
		LocalModsPath:          modsPath,
		LocalResourcePacksPath: resourcePacksPath,
		LocalShaderPacksPath:   shaderPacksPath,
		Workers:                2,
	}, "test"); err != nil {
		t.Fatalf("UploadBranch: %v", err)
	}
	if len(manifest) != 3 {
		t.Fatalf("манифест = %#v", manifest)
	}
	manifestPaths := map[string]string{}
	for _, entry := range manifest {
		manifestPaths[entry.Path] = entry.Hash
	}
	if manifestPaths["mods/example.jar"] != hash("example") ||
		manifestPaths["resourcepacks/pack.zip"] != hash("pack") ||
		manifestPaths["shaderpacks/shader.zip"] != hash("shader") {
		t.Fatalf("манифест = %#v", manifest)
	}
}

func TestUploadManifestRejectsUnsafeBranch(t *testing.T) {
	err := UploadManifest(Config{Token: "secret"}, "../unsafe")
	if err == nil {
		t.Fatal("UploadManifest должен отклонить небезопасное имя ветки")
	}
}

func TestUploadMissingUploadsOnlyNewOrChangedMods(t *testing.T) {
	root := t.TempDir()
	writeLocal(t, filepath.Join(root, "same.jar"), "same")
	writeLocal(t, filepath.Join(root, "changed.jar"), "changed")
	writeLocal(t, filepath.Join(root, "new.jar"), "new")
	writeLocal(t, filepath.Join(filepath.Dir(root), "resourcepacks", "ignored.zip"), "resource pack")
	writeLocal(t, filepath.Join(filepath.Dir(root), "shaderpacks", "ignored.zip"), "shader pack")
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

	err := UploadMissing(Config{
		BaseURL:                server.URL,
		Timeout:                time.Second,
		Token:                  "secret",
		LocalModsPath:          root,
		LocalResourcePacksPath: filepath.Join(filepath.Dir(root), "resourcepacks"),
		LocalShaderPacksPath:   filepath.Join(filepath.Dir(root), "shaderpacks"),
		Workers:                2,
	})
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hash(contents string) string {
	sum := sha1.Sum([]byte(contents))
	return hex.EncodeToString(sum[:])
}
