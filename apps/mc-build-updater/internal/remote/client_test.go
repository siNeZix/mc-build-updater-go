package remote

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadVerifiedDoesNotBlockWhenAllRangeWorkersFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Range") == "" {
			t.Error("?????????????????? Range-????????????")
		}
		http.Error(response, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := NewWithTimeout(server.URL, time.Second)
	if err != nil {
		t.Fatalf("NewWithTimeout: %v", err)
	}

	completed := make(chan error, 1)
	go func() {
		completed <- client.DownloadVerified("mods/example.jar", filepath.Join(t.TempDir(), "example.jar"), "", strings.Repeat("0", 40), 5*rangeChunkSize)
	}()

	select {
	case err := <-completed:
		if err == nil {
			t.Fatal("DownloadVerified ???????????? ?????????????? ???????????? Range-????????????????")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DownloadVerified ?????????? ?????? ?????????????? ???????? Range-????????????????????")
	}
}


func TestNewWithTimeoutValidatesAndNormalizesURL(t *testing.T) {
	for _, rawURL := range []string{"", "ftp://example.test", "http:///missing-host"} {
		if _, err := NewWithTimeout(rawURL, time.Second); err == nil {
			t.Fatalf("NewWithTimeout(%q) ???????????? ?????????????? ????????????", rawURL)
		}
	}
	client, err := NewWithTimeout("https://example.test/base", 0)
	if err != nil {
		t.Fatalf("NewWithTimeout: %v", err)
	}
	if got := client.resolve("map"); got != "https://example.test/base/map" {
		t.Fatalf("resolve = %q", got)
	}
}

func TestMapsAndDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/MM/branch.json":
			_, _ = w.Write([]byte(`[{"hash":"abc","path":"C:/mods/a.jar"}]`))
		case "/map":
			_, _ = w.Write([]byte(`[{"hash":"abc","name":"a.jar","dir":"mods","size":3}]`))
		case "/mods/a.jar":
			_, _ = w.Write([]byte("abc"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewWithTimeout(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mods, err := client.ModsMap("branch")
	if err != nil || len(mods) != 1 || mods[0].Path != "C:/mods/a.jar" {
		t.Fatalf("ModsMap = %#v, ???????????? = %v", mods, err)
	}
	entries, err := client.FileMap()
	if err != nil || len(entries) != 1 || entries[0].Name != "a.jar" {
		t.Fatalf("FileMap = %#v, ???????????? = %v", entries, err)
	}
	destination := filepath.Join(t.TempDir(), "nested", "a.jar")
	if err := client.Download("mods/a.jar", destination, "1/1"); err != nil {
		t.Fatalf("Download: %v", err)
	}
	contents, err := os.ReadFile(destination)
	if err != nil || string(contents) != "abc" {
		t.Fatalf("?????????????????? ???????? = %q, ???????????? = %v", contents, err)
	}
}

func TestDownloadVerifiedRangeAndChecksum(t *testing.T) {
	contents := []byte(strings.Repeat("a", int(rangeChunkSize)) + "tail")
	checksum := sha1Hex(contents)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			t.Fatal("?????????????????? Range")
		}
		var start, end int
		if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
			t.Fatalf("Range = %q: %v", rangeHeader, err)
		}
		if end >= len(contents) {
			end = len(contents) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(contents)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(contents[start : end+1])
	}))
	defer server.Close()
	client, err := NewWithTimeout(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "range.jar")
	if err := client.DownloadVerified("mods/range.jar", destination, "", checksum, int64(len(contents))); err != nil {
		t.Fatalf("DownloadVerified: %v", err)
	}
	got, _ := os.ReadFile(destination)
	if string(got) != string(contents) {
		t.Fatal("Range-???????????????? ???????????????? ???????????????? ????????????????????")
	}
	if err := client.DownloadVerified("mods/range.jar", filepath.Join(t.TempDir(), "bad.jar"), "", strings.Repeat("0", 40), int64(len(contents))); err == nil {
		t.Fatal("???????????????? SHA-1 ???????????? ?????????????? ????????????")
	}
}

func TestUploadSendsTokenAndReportsHTTPError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mod.jar")
	if err := os.WriteFile(file, []byte("mod"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/files/mods/mod.jar" {
			t.Fatalf("???????????? = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "mod" {
			t.Fatalf("???????? = %q", body)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	client, _ := NewWithTimeout(server.URL, time.Second)
	status, err := client.Upload("mods/mod.jar", file, "token")
	if err != nil || status != http.StatusCreated {
		t.Fatalf("Upload = (%d, %v)", status, err)
	}
}

func sha1Hex(contents []byte) string {
	sum := sha1.Sum(contents)
	return hex.EncodeToString(sum[:])
}

