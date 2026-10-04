package httpserver

import (
	"crypto/md5" // #nosec G501 -- preserved map version protocol, not a security use.
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sinezix/mc-build-updater-go/file-hosting/internal/filemap"
)

type Service struct {
	root string

	mu      sync.RWMutex
	entries []filemap.Entry
	version string
}

func New(root string) (*Service, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	service := &Service{root: absRoot}
	if err := service.Refresh(); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) Root() string { return s.root }

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/map/update", s.updateMap)
	mux.HandleFunc("/map/version", s.mapVersion)
	mux.HandleFunc("/map", s.mapResponse)
	mux.HandleFunc("/", s.download)
	return mux
}

func (s *Service) Refresh() error {
	entries, err := filemap.Build(s.root)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal file map: %w", err)
	}
	digest := md5.Sum(payload) // #nosec G401 -- protocol compatibility with prior version route.
	version := hex.EncodeToString(digest[:])[:6] + ":" + strconv.FormatInt(time.Now().UnixMilli(), 10)

	s.mu.Lock()
	s.entries = entries
	s.version = version
	s.mu.Unlock()
	log.Printf("files map updated: %d file(s)", len(entries))
	return nil
}

func (s *Service) updateMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if err := s.Refresh(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Service) mapVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	s.mu.RLock()
	version := s.version
	s.mu.RUnlock()
	writeJSON(w, version)
}

func (s *Service) mapResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	s.mu.RLock()
	entries := append([]filemap.Entry(nil), s.entries...)
	s.mu.RUnlock()
	writeJSON(w, entries)
}

func (s *Service) download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if requested == "" || requested == "." {
		http.NotFound(w, r)
		return
	}
	directory, target := path.Split(requested)
	directory = strings.TrimSuffix(directory, "/")
	if target == "" || strings.Contains(target, "/") {
		http.NotFound(w, r)
		return
	}

	s.mu.RLock()
	var found *filemap.Entry
	for index := range s.entries {
		entry := &s.entries[index]
		if entry.Dir == directory && (entry.Hash == target || entry.Name == target) {
			copy := *entry
			found = &copy
			break
		}
	}
	s.mu.RUnlock()
	if found == nil {
		http.NotFound(w, r)
		return
	}

	file, err := os.Open(found.Path)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "open served file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.Error(w, "stat served file", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", mimeAttachment(found.Name))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	http.ServeContent(w, r, found.Name, info.ModTime(), file)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func mimeAttachment(fileName string) string {
	return "attachment; filename=" + strconv.Quote(strings.ReplaceAll(fileName, "\r", ""))
}
