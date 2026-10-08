package httpserver

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sinezix/mc-build-updater-go/file-hosting/internal/filemap"
)

func TestFilesAPIReplacesExistingFile(t *testing.T) {
	root := t.TempDir()
	mods := filepath.Join(root, "mods")
	if err := os.Mkdir(mods, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(mods, "example.jar")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	service, err := New(root, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	server := httptest.NewServer(service.Handler())
	defer server.Close()

	request, err := http.NewRequest(http.MethodPut, server.URL+"/api/files/mods/example.jar", strings.NewReader("new"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer test-token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("?????? ?????? = %d", response.StatusCode)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new" {
		t.Fatalf("?????????? ????? = %q", contents)
	}
}

func TestMapAndDownloadRoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "mods", "example.jar")
	if err := os.WriteFile(filePath, []byte("mod content"), 0o644); err != nil {
		t.Fatal(err)
	}

	service, err := New(root, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	server := httptest.NewServer(service.Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/map")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var entries []filemap.Entry
	if err := json.NewDecoder(response.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Dir != "mods" || entries[0].Name != "example.jar" {
		t.Fatalf("unexpected map: %#v", entries)
	}

	for _, target := range []string{entries[0].Hash, entries[0].Name} {
		response, err := http.Get(server.URL + "/mods/" + target)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: got %d", target, response.StatusCode)
		}
		if response.Header.Get("Content-Disposition") != `attachment; filename="example.jar"` {
			t.Fatalf("unexpected content disposition: %q", response.Header.Get("Content-Disposition"))
		}
		response.Body.Close()
	}

	response, err = http.Get(server.URL + "/map/version")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var version string
	if err := json.NewDecoder(response.Body).Decode(&version); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[a-f0-9]{6}:\d+$`).MatchString(version) {
		t.Fatalf("unexpected version: %q", version)
	}
}

func TestMapBranchFiltersResourceAndShaderPacks(t *testing.T) {
	root := t.TempDir()
	for fileName, contents := range map[string]string{
		"resourcepacks/pack.zip": "resource pack",
		"shaderpacks/shader.zip": "shader pack",
	} {
		path := filepath.Join(root, filepath.FromSlash(fileName))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	resourceHash := sha1Hex([]byte("resource pack"))
	shaderHash := sha1Hex([]byte("shader pack"))
	manifestPath := filepath.Join(root, "MM", "branch.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `[{"hash":"` + resourceHash + `","path":"resourcepacks/pack.zip"},{"hash":"` + shaderHash + `","path":"shaderpacks/shader.zip"}]`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	service, err := New(root, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	server := httptest.NewServer(service.Handler())
	defer server.Close()

	for directory, expectedName := range map[string]string{
		"resourcepacks": "pack.zip",
		"shaderpacks":   "shader.zip",
	} {
		response, err := http.Get(server.URL + "/map?dir=" + directory + "&branch=branch")
		if err != nil {
			t.Fatal(err)
		}
		var entries []filemap.Entry
		err = json.NewDecoder(response.Body).Decode(&entries)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Dir != directory || entries[0].Name != expectedName {
			t.Fatalf("%s: записи = %#v", directory, entries)
		}
	}
}

func sha1Hex(contents []byte) string {
	digest := sha1.Sum(contents)
	return hex.EncodeToString(digest[:])
}

func TestFileWriteAPIAndRange(t *testing.T) {
	root := t.TempDir()
	service, err := New(root, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	server := httptest.NewServer(service.Handler())
	defer server.Close()

	request, err := http.NewRequest(http.MethodPut, server.URL+"/api/files/mods/example.jar", strings.NewReader("mod content"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer test-token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("PUT: got %d", response.StatusCode)
	}

	request, err = http.NewRequest(http.MethodGet, server.URL+"/mods/example.jar", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Range", "bytes=0-2")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("Range: got %d", response.StatusCode)
	}
	contents, _ := io.ReadAll(response.Body)
	if string(contents) != "mod" {
		t.Fatalf("range body: %q", contents)
	}

	request, err = http.NewRequest(http.MethodDelete, server.URL+"/api/files/mods/example.jar", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer test-token")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE: got %d", response.StatusCode)
	}
}

func TestMapFiltersAndRejectsUnsafeQueries(t *testing.T) {
	root := t.TempDir()
	writeServerFile(t, filepath.Join(root, "mods", "a.jar"), "a")
	writeServerFile(t, filepath.Join(root, "other", "b.jar"), "b")
	writeServerFile(t, filepath.Join(root, "MM", "branch.json"), `[{"hash":"86f7e437faa5a7fce15d1ddcb9eaeaea377667b8"}]`)
	service, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	server := httptest.NewServer(service.Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/map?dir=mods&branch=branch")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var entries []filemap.Entry
	if err := json.NewDecoder(response.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(entries) != 1 || entries[0].Name != "a.jar" {
		t.Fatalf("фильтр карты: статус %d, записи %#v", response.StatusCode, entries)
	}

	for _, query := range []string{"?dir=../mods", "?branch=../branch"} {
		response, err := http.Get(server.URL + "/map" + query)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("/map%s: статус %d, ожидается 400", query, response.StatusCode)
		}
	}

	response, err = http.Get(server.URL + "/map?branch=missing")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("отсутствующий manifest: статус %d", response.StatusCode)
	}
}

func TestFilesAPIRequiresTokenAndHandlesMissingFile(t *testing.T) {
	root := t.TempDir()
	service, err := New(root, "secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	server := httptest.NewServer(service.Handler())
	defer server.Close()

	request, err := http.NewRequest(http.MethodPut, server.URL+"/api/files/mods/new.jar", strings.NewReader("new"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("PUT без токена: статус %d", response.StatusCode)
	}

	request, err = http.NewRequest(http.MethodDelete, server.URL+"/api/files/mods/missing.jar", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("DELETE отсутствующего файла: статус %d", response.StatusCode)
	}

	response, err = http.Get(server.URL + "/mods/../secret")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("небезопасный download: статус %d", response.StatusCode)
	}
}

func writeServerFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
