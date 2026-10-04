package uploader

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/console"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/remote"
)

type Config struct {
	BaseURL       string
	Timeout       time.Duration
	Token         string
	LocalModsPath string
	Workers       int
}

type fileInfo struct {
	name string
	path string
	hash string
}

func UploadMissing(configuration Config) error {
	if configuration.Token == "" {
		return fmt.Errorf("переменная окружения FILE_HOSTING_TOKEN не задана")
	}
	if configuration.Workers < 1 {
		configuration.Workers = 1
	}
	if configuration.Timeout <= 0 {
		configuration.Timeout = 15 * time.Minute
	}
	client, err := remote.NewWithTimeout(configuration.BaseURL, configuration.Timeout)
	if err != nil {
		return err
	}
	files, err := listLocal(configuration.LocalModsPath)
	if err != nil {
		return err
	}
	mapEntries, err := client.FileMap()
	if err != nil {
		return err
	}
	remoteHashes := make(map[string]string)
	for _, entry := range mapEntries {
		if entry.Dir == "mods" {
			remoteHashes[entry.Name] = entry.Hash
		}
	}
	var pending []fileInfo
	for _, file := range files {
		if remoteHashes[file.name] != file.hash {
			pending = append(pending, file)
		}
	}
	if len(pending) == 0 {
		console.Success("Нет новых или изменённых модов.")
		return nil
	}
	console.Info("Загрузка модов: %d; потоков: %d", len(pending), min(configuration.Workers, len(pending)))
	jobs := make(chan fileInfo)
	errors := make(chan error, len(pending))
	var workers sync.WaitGroup
	for range min(configuration.Workers, len(pending)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for file := range jobs {
				if _, err := client.Upload("mods/"+file.name, file.path, configuration.Token); err != nil {
					errors <- err
					continue
				}
				console.Action("Загрузка: %s", file.name)
			}
		}()
	}
	for _, file := range pending {
		jobs <- file
	}
	close(jobs)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

func listLocal(root string) ([]fileInfo, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []fileInfo{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("прочитать каталог модов: %w", err)
	}
	files := make([]fileInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		filePath := filepath.Join(root, entry.Name())
		hash, err := sha1File(filePath)
		if err != nil {
			return nil, err
		}
		files = append(files, fileInfo{name: entry.Name(), path: filePath, hash: hash})
	}
	return files, nil
}

func sha1File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("открыть %s: %w", path, err)
	}
	defer file.Close()
	digest := sha1.New() // #nosec G401 -- file-hosting protocol requires SHA-1.
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("вычислить хеш %s: %w", path, err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
