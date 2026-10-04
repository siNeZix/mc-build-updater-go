package httpserver

import (
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
