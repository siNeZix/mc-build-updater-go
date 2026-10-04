package modsync

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/remote"
)

type LocalMod struct {
	Hash string `json:"hash"`
	Path string `json:"path"`
}

type Result struct {
	Downloaded int
	Deleted    int
}

type Synchronizer struct {
	remote      *remote.Client
	modsPath    string
	maxParallel int
}

func New(client *remote.Client, modsPath string, maxParallel int) Synchronizer {
	if maxParallel < 1 {
		maxParallel = 1
	}
	return Synchronizer{remote: client, modsPath: modsPath, maxParallel: maxParallel}
}

func (s Synchronizer) Sync(branch string) (Result, error) {
	local, err := LocalMap(s.modsPath)
	if err != nil {
		return Result{}, err
	}
	downloadMap, err := s.remote.BranchModsMap(branch)
	if err != nil {
		return Result{}, err
	}

	remoteHashes := make(map[string]struct{}, len(downloadMap))
	for _, file := range downloadMap {
		remoteHashes[file.Hash] = struct{}{}
	}
	localHashes := make(map[string]struct{}, len(local))
	for _, mod := range local {
		localHashes[mod.Hash] = struct{}{}
	}

	result := Result{}
	for _, mod := range local {
		if _, exists := remoteHashes[mod.Hash]; !exists {
			fmt.Printf("DELETE %s\n", filepath.Base(mod.Path))
			if err := os.Remove(mod.Path); err != nil && !os.IsNotExist(err) {
				return Result{}, fmt.Errorf("delete obsolete mod %s: %w", mod.Path, err)
			}
			result.Deleted++
		}
	}

	var missing []remote.FileMap
	for _, file := range downloadMap {
		if _, exists := localHashes[file.Hash]; !exists {
			missing = append(missing, file)
		}
	}
	if len(missing) == 0 {
		return result, nil
	}

	fmt.Printf("Starting download of %d mod(s)\n", len(missing))
	jobs := make(chan downloadJob)
	errors := make(chan error, len(missing))
	var workers sync.WaitGroup
	for worker := 0; worker < min(s.maxParallel, len(missing)); worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				if err := s.remote.DownloadVerified("mods/"+job.file.Hash, filepath.Join(s.modsPath, job.file.Name), job.label, job.file.Hash, job.file.Size); err != nil {
					errors <- err
				}
			}
		}()
	}

	for index, file := range missing {
		jobs <- downloadJob{file: file, label: fmt.Sprintf("%d/%d", index+1, len(missing))}
	}
	close(jobs)
	workers.Wait()
	close(errors)
	downloadErrors := make([]error, 0)
	for err := range errors {
		if err != nil {
			downloadErrors = append(downloadErrors, err)
		}
	}
	if len(downloadErrors) > 0 {
		return Result{}, downloadErrors[0]
	}
	result.Downloaded = len(missing)
	return result, nil
}

type downloadJob struct {
	file  remote.FileMap
	label string
}

func LocalMap(modsPath string) ([]LocalMod, error) {
	entries, err := os.ReadDir(modsPath)
	if os.IsNotExist(err) {
		return []LocalMod{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read mods directory: %w", err)
	}

	mods := make([]LocalMod, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		path := filepath.Join(modsPath, entry.Name())
		hash, err := SHA1(path)
		if err != nil {
			return nil, fmt.Errorf("checksum %s: %w", path, err)
		}
		mods = append(mods, LocalMod{Hash: hash, Path: path})
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Path < mods[j].Path })
	return mods, nil
}

func WriteJSON(path string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func SHA1(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha1.New() // #nosec G401 -- legacy wire protocol uses SHA-1 checksums.
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func min(first, second int) int {
	if first < second {
		return first
	}
	return second
}
