package httpserver

import (
	"crypto/md5" // #nosec G501 -- preserved map version protocol, not a security use.
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sinezix/mc-build-updater-go/file-hosting/internal/console"
	"github.com/sinezix/mc-build-updater-go/file-hosting/internal/filemap"
	"github.com/sinezix/mc-build-updater-go/file-hosting/internal/manifest"
)

const databaseName = ".file-hosting.sqlite"

type Service struct {
	root     string
	database string
	token    string
	store    *manifest.Store

	mu      sync.RWMutex
	entries []filemap.Entry
	version string
}

func New(root, token string) (*Service, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	database := filepath.Join(absRoot, databaseName)
	store, err := manifest.Open(database)
	if err != nil {
		return nil, err
	}
	service := &Service{root: absRoot, database: database, token: token, store: store}
	if err := service.Refresh(); err != nil {
		store.Close() //nolint:errcheck
		return nil, err
	}
	return service, nil
}

func (s *Service) Root() string { return s.root }

func (s *Service) Close() error { return s.store.Close() }

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/files/", s.filesAPI)
	mux.HandleFunc("/map/update", s.updateMap)
	mux.HandleFunc("/map/version", s.mapVersion)
	mux.HandleFunc("/map", s.mapResponse)
	mux.HandleFunc("/", s.download)
	return mux
}

func (s *Service) Refresh() error {
	entries, err := filemap.Build(s.root, manifest.DatabaseFiles(s.database))
	if err != nil {
		return err
	}
	changed, err := s.store.Sync(entries)
	if err != nil {
		return err
	}
	version, err := s.store.Version()
	if err != nil {
		return err
	}
	if changed || version == "" {
		payload, err := json.Marshal(entries)
		if err != nil {
			return fmt.Errorf("сериализовать карту файлов: %w", err)
		}
		digest := md5.Sum(payload) // #nosec G401 -- protocol compatibility with prior version route.
		version = hex.EncodeToString(digest[:])[:6] + ":" + strconv.FormatInt(time.Now().UnixMilli(), 10)
		if err := s.store.SetVersion(version); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.entries = entries
	s.version = version
	s.mu.Unlock()
	console.Success("Карта файлов обновлена: %d", len(entries))
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
	filtered, status, err := s.filterMap(entries, r.URL.Query().Get("dir"), r.URL.Query().Get("branch"))
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, filtered)
}

func (s *Service) filterMap(entries []filemap.Entry, directory, branch string) ([]filemap.Entry, int, error) {
	if directory != "" && !validRelativePath(directory) {
		return nil, http.StatusBadRequest, fmt.Errorf("некорректный параметр dir")
	}
	allowedHashes := map[string]struct{}(nil)
	if branch != "" {
		if strings.ContainsAny(branch, `/\\`) || branch == "." || branch == ".." {
			return nil, http.StatusBadRequest, fmt.Errorf("некорректный параметр branch")
		}
		contents, err := os.ReadFile(filepath.Join(s.root, "MM", branch+".json"))
		if os.IsNotExist(err) {
			return nil, http.StatusNotFound, fmt.Errorf("манифест ветки не найден")
		}
		if err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("прочитать манифест ветки: %w", err)
		}
		var mods []struct {
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal(contents, &mods); err != nil {
			return nil, http.StatusConflict, fmt.Errorf("разобрать манифест ветки: %w", err)
		}
		allowedHashes = make(map[string]struct{}, len(mods))
		for _, mod := range mods {
			allowedHashes[mod.Hash] = struct{}{}
		}
		present := make(map[string]struct{}, len(entries))
		for _, entry := range entries {
			present[entry.Hash] = struct{}{}
		}
		for hash := range allowedHashes {
			if _, exists := present[hash]; !exists {
				return nil, http.StatusConflict, fmt.Errorf("манифест ветки ссылается на недоступный файл %s", hash)
			}
		}
	}
	filtered := make([]filemap.Entry, 0, len(entries))
	for _, entry := range entries {
		if directory != "" && entry.Dir != directory && !strings.HasPrefix(entry.Dir, directory+"/") {
			continue
		}
		if allowedHashes != nil {
			if _, exists := allowedHashes[entry.Hash]; !exists {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return filtered, http.StatusOK, nil
}

func (s *Service) download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	relative := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if relative == "" || !validRelativePath(relative) {
		http.NotFound(w, r)
		return
	}
	directory, target := path.Split(relative)
	directory = strings.TrimSuffix(directory, "/")
	s.mu.RLock()
	var selected *filemap.Entry
	for index := range s.entries {
		entry := &s.entries[index]
		if entry.Dir == directory && (entry.Name == target || entry.Hash == target) {
			selected = entry
			break
		}
	}
	s.mu.RUnlock()
	if selected == nil {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(selected.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", selected.Name))
	http.ServeContent(w, r, selected.Name, info.ModTime(), file)
}

func (s *Service) filesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		http.NotFound(w, r)
		return
	}
	if s.token == "" {
		http.Error(w, "file write API is not configured", http.StatusServiceUnavailable)
		return
	}
	if !authorized(r, s.token) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	relative := strings.TrimPrefix(r.URL.Path, "/api/files/")
	if !validRelativePath(relative) {
		http.Error(w, "invalid file path", http.StatusBadRequest)
		return
	}
	target := filepath.Join(s.root, filepath.FromSlash(relative))
	if manifest.IsDatabaseFile(target, s.database) {
		http.Error(w, "reserved file path", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodDelete {
		s.deleteFile(w, target)
		return
	}
	s.putFile(w, r, target)
}

func (s *Service) putFile(w http.ResponseWriter, r *http.Request, target string) {
	_, statErr := os.Stat(target)
	existed := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		http.Error(w, statErr.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath) //nolint:errcheck
	if _, err := io.Copy(temporary, r.Body); err != nil {
		temporary.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := temporary.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	identical := false
	if oldHash, err := filemap.SHA1(target); err == nil {
		newHash, hashErr := filemap.SHA1(temporaryPath)
		identical = hashErr == nil && oldHash == newHash
	}
	if !identical {
		if err := os.Rename(temporaryPath, target); err != nil {
			http.Error(w, fmt.Sprintf("replace file: %v", err), http.StatusInternalServerError)
			return
		}
	}
	if err := s.Refresh(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if identical || existed {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

func (s *Service) deleteFile(w http.ResponseWriter, target string) {
	if err := os.Remove(target); os.IsNotExist(err) {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Refresh(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func authorized(r *http.Request, token string) bool {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func validRelativePath(value string) bool {
	return value != "" && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && path.Clean(value) == value && value != "." && !strings.HasPrefix(value, "../")
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		console.Warning("не удалось записать JSON-ответ: %v", err)
	}
}
