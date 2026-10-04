package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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

	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
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
