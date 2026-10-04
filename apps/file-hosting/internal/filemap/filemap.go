package filemap

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is public response shape retained for mc-build-updater clients.
// Path is intentionally absolute, as it was in the original service.
type Entry struct {
	Hash string `json:"hash"`
	Path string `json:"path"`
	Name string `json:"name"`
	Dir  string `json:"dir"`
	Size int64  `json:"size"`
}

// Build recursively scans root and returns a stable, deterministic map.
func Build(root string, skip func(string) bool) ([]Entry, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve files root: %w", err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat files root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("files root %q is not a directory", absRoot)
	}

	var entries []Entry
	err = filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if skip != nil && skip(path) {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}

		hash, err := SHA1(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(absRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		directory := filepath.ToSlash(relative)
		if directory == "." {
			directory = ""
		}
		entries = append(entries, Entry{
			Hash: hash,
			Path: path,
			Name: entry.Name(),
			Dir:  directory,
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan files root: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.Compare(entries[i].Path, entries[j].Path) < 0
	})
	return entries, nil
}

func SHA1(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha1.New() // #nosec G401 -- SHA-1 is protocol compatibility, not security.
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
