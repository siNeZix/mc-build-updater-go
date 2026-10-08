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
	BaseURL                string
	Timeout                time.Duration
	Token                  string
	LocalModsPath          string
	LocalResourcePacksPath string
	LocalShaderPacksPath   string
	Workers                int
}

type fileInfo struct {
	directory string
	name      string
	path      string
	hash      string
}

func UploadManifest(configuration Config, branch string) error {
	if configuration.Token == "" {
		return fmt.Errorf("переменная окружения FILE_HOSTING_TOKEN не задана")
	}
	if err := validateBranch(branch); err != nil {
		return err
	}
	if configuration.Timeout <= 0 {
		configuration.Timeout = 15 * time.Minute
	}
	client, err := remote.NewWithTimeout(configuration.BaseURL, configuration.Timeout)
	if err != nil {
		return err
	}
	files, err := listContent(configuration)
	if err != nil {
		return err
	}
	manifest := make([]remote.ManifestEntry, 0, len(files))
	for _, file := range files {
		manifest = append(manifest, remote.ManifestEntry{Hash: file.hash, Path: filepath.ToSlash(filepath.Join(file.directory, file.name))})
	}
	if _, err := client.UploadManifest(branch, manifest, configuration.Token); err != nil {
		return err
	}
	console.Success("Манифест ветки %s обновлён: %d файлов.", branch, len(manifest))
	return nil
}

func UploadBranch(configuration Config, branch string) error {
	if configuration.Token == "" {
		return fmt.Errorf("переменная окружения FILE_HOSTING_TOKEN не задана")
	}
	if err := validateBranch(branch); err != nil {
		return err
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
	files, err := listContent(configuration)
	if err != nil {
		return err
	}
	if err := uploadFiles(client, files, configuration); err != nil {
		return err
	}
	return UploadManifest(configuration, branch)
}

func uploadFiles(client *remote.Client, files []fileInfo, configuration Config) error {
	if len(files) == 0 {
		return nil
	}
	console.Info("Загрузка модов: %d; потоков: %d", len(files), min(configuration.Workers, len(files)))
	jobs := make(chan fileInfo)
	errors := make(chan error, len(files))
	var workers sync.WaitGroup
	for range min(configuration.Workers, len(files)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for file := range jobs {
				if _, err := client.Upload(filepath.ToSlash(filepath.Join(file.directory, file.name)), file.path, configuration.Token); err != nil {
					errors <- err
					continue
				}
				console.Action("Загружен: %s", filepath.ToSlash(filepath.Join(file.directory, file.name)))
			}
		}()
	}
	for _, file := range files {
		jobs <- file
	}
	close(jobs)
	workers.Wait()
	close(errors)
	var failures []error
	for err := range errors {
		failures = append(failures, err)
	}
	if len(failures) > 0 {
		return fmt.Errorf("не удалось загрузить моды: %w", failures[0])
	}
	return nil
}

func ListBranches(configuration Config) ([]string, error) {
	if configuration.Timeout <= 0 {
		configuration.Timeout = 15 * time.Minute
	}
	client, err := remote.NewWithTimeout(configuration.BaseURL, configuration.Timeout)
	if err != nil {
		return nil, err
	}
	return client.ListBranches()
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

func listContent(configuration Config) ([]fileInfo, error) {
	paths := []struct {
		directory string
		path      string
	}{
		{directory: "mods", path: configuration.LocalModsPath},
		{directory: "resourcepacks", path: resourcePacksPath(configuration)},
		{directory: "shaderpacks", path: shaderPacksPath(configuration)},
	}

	var files []fileInfo
	for _, content := range paths {
		entries, err := listLocal(content.path)
		if err != nil {
			return nil, err
		}
		for index := range entries {
			entries[index].directory = content.directory
		}
		files = append(files, entries...)
	}
	return files, nil
}

func resourcePacksPath(configuration Config) string {
	if configuration.LocalResourcePacksPath != "" {
		return configuration.LocalResourcePacksPath
	}
	return filepath.Join(filepath.Dir(configuration.LocalModsPath), "resourcepacks")
}

func shaderPacksPath(configuration Config) string {
	if configuration.LocalShaderPacksPath != "" {
		return configuration.LocalShaderPacksPath
	}
	return filepath.Join(filepath.Dir(configuration.LocalModsPath), "shaderpacks")
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

func validateBranch(branch string) error {
	if branch == "" || branch == "." || branch == ".." || filepath.Base(branch) != branch || filepath.Ext(branch) != "" {
		return fmt.Errorf("некорректное имя ветки: %q", branch)
	}
	return nil
}
