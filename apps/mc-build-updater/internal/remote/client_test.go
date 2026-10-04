package remote

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadVerifiedDoesNotBlockWhenAllRangeWorkersFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Range") == "" {
			t.Error("ожидается Range-запрос")
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
			t.Fatal("DownloadVerified должен вернуть ошибку Range-загрузки")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DownloadVerified завис при ошибках всех Range-работников")
	}
}
