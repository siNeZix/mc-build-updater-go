package remote

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/console"
)

type FileMap struct {
	Path string `json:"path"`
	Dir  string `json:"dir"`
	Name string `json:"name"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

type Client struct {
	baseURL *url.URL
	http    *http.Client
}

func New(rawBaseURL string) (*Client, error) {
	return NewWithTimeout(rawBaseURL, 15*time.Minute)
}

func NewWithTimeout(rawBaseURL string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("разобрать URL file-hosting: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("URL file-hosting должен использовать HTTP(S): %q", rawBaseURL)
	}
	if baseURL.Host == "" {
		return nil, fmt.Errorf("в URL file-hosting отсутствует хост: %q", rawBaseURL)
	}
	if !strings.HasSuffix(baseURL.Path, "/") {
		baseURL.Path += "/"
	}
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) ModsMap(branch string) ([]Mod, error) {
	var entries []Mod
	if err := c.getJSON("MM/"+url.PathEscape(branch)+".json", &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) FileMap() ([]FileMap, error) {
	return c.fileMap("")
}

func (c *Client) BranchModsMap(branch string) ([]FileMap, error) {
	return c.fileMap("map?dir=mods&branch=" + url.QueryEscape(branch))
}

func (c *Client) fileMap(relativeURL string) ([]FileMap, error) {
	var entries []FileMap
	if relativeURL == "" {
		relativeURL = "map"
	}
	if err := c.getJSON(relativeURL, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) Download(relativeURL, destination, prefix string) error {
	requestURL := c.resolve(relativeURL)
	if prefix != "" {
		prefix = "[" + prefix + "] "
	}
	console.Action("%s%s [%s]", prefix, requestURL, filepath.Base(destination))

	response, err := c.http.Get(requestURL)
	if err != nil {
		return fmt.Errorf("скачать %s: %w", requestURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("скачать %s: HTTP %d", requestURL, response.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("создать каталог для скачивания: %w", err)
	}

	temporaryFile, err := os.CreateTemp(filepath.Dir(destination), ".download-*")
	if err != nil {
		return fmt.Errorf("создать временный файл скачивания: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	if _, err := io.Copy(temporaryFile, response.Body); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("записать скачанный файл: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("закрыть скачанный файл: %w", err)
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("заменить целевой файл: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("переместить скачанный файл: %w", err)
	}
	return nil
}

const rangeChunkSize int64 = 2 * 1024 * 1024

// DownloadVerified uses four parallel 2 MiB ranges for files larger than 2 MiB.
// Servers without Range support automatically fall back to Download.
func (c *Client) DownloadVerified(relativeURL, destination, prefix, expectedHash string, size int64) error {
	if size <= rangeChunkSize {
		if err := c.Download(relativeURL, destination, prefix); err != nil {
			return err
		}
		return verifySHA1(destination, expectedHash)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("создать каталог для скачивания: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".download-*")
	if err != nil {
		return fmt.Errorf("создать временный файл скачивания: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath) //nolint:errcheck
	if err := temporary.Truncate(size); err != nil {
		temporary.Close()
		return fmt.Errorf("выделить место во временном файле: %w", err)
	}
	temporary.Close()

	type byteRange struct{ start, end int64 }
	var ranges []byteRange
	for start := int64(0); start < size; start += rangeChunkSize {
		end := start + rangeChunkSize - 1
		if end >= size {
			end = size - 1
		}
		ranges = append(ranges, byteRange{start, end})
	}
	jobs := make(chan byteRange)
	errors := make(chan error, len(ranges))
	var workers sync.WaitGroup
	for range min(4, len(ranges)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for block := range jobs {
				if err := c.downloadRange(relativeURL, temporaryPath, block.start, block.end, size); err != nil {
					errors <- err
				}
			}
		}()
	}
	for _, block := range ranges {
		jobs <- block
	}
	close(jobs)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			if strings.Contains(err.Error(), "range unsupported") {
				if downloadErr := c.Download(relativeURL, destination, prefix); downloadErr != nil {
					return downloadErr
				}
				return verifySHA1(destination, expectedHash)
			}
			return err
		}
	}
	if err := verifySHA1(temporaryPath, expectedHash); err != nil {
		return err
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("заменить целевой файл: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("переместить скачанный файл: %w", err)
	}
	return nil
}

func (c *Client) Upload(relativePath, filePath, token string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("открыть файл для загрузки: %w", err)
	}
	defer file.Close()
	requestURL := c.resolve("api/files/") + (&url.URL{Path: relativePath}).EscapedPath()
	request, err := http.NewRequest(http.MethodPut, requestURL, file)
	if err != nil {
		return 0, fmt.Errorf("создать запрос загрузки: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(request)
	if err != nil {
		return 0, fmt.Errorf("загрузить %s: %w", relativePath, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusNoContent {
		return 0, fmt.Errorf("загрузить %s: HTTP %d", relativePath, response.StatusCode)
	}
	return response.StatusCode, nil
}

func (c *Client) downloadRange(relativeURL, destination string, start, end, total int64) error {
	request, err := http.NewRequest(http.MethodGet, c.resolve(relativeURL), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("скачать диапазон: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return fmt.Errorf("сервер не поддерживает частичное скачивание")
	}
	if response.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("скачать диапазон: HTTP %d", response.StatusCode)
	}
	expectedRange := fmt.Sprintf("bytes %d-%d/%d", start, end, total)
	if response.Header.Get("Content-Range") != expectedRange {
		return fmt.Errorf("некорректный Content-Range %q", response.Header.Get("Content-Range"))
	}
	file, err := os.OpenFile(destination, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("открыть временный файл скачивания: %w", err)
	}
	defer file.Close()
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return fmt.Errorf("seek temporary download: %w", err)
	}
	if _, err := io.CopyN(file, response.Body, end-start+1); err != nil {
		return fmt.Errorf("write download range: %w", err)
	}
	return nil
}

func verifySHA1(filePath, expected string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha1.New() // #nosec G401 -- file-hosting protocol requires SHA-1.
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	if actual := hex.EncodeToString(digest.Sum(nil)); actual != expected {
		return fmt.Errorf("downloaded file SHA-1 mismatch: got %s, want %s", actual, expected)
	}
	return nil
}

func (c *Client) getJSON(relativeURL string, target any) error {
	requestURL := c.resolve(relativeURL)
	console.Action("Проверка доступности: %s", requestURL)
	response, err := c.http.Get(requestURL)
	if err != nil {
		return fmt.Errorf("выполнить запрос %s: %w", requestURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("выполнить запрос %s: HTTP %d", requestURL, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode %s response: %w", requestURL, err)
	}
	return nil
}

func (c *Client) resolve(relativeURL string) string {
	resolved := c.baseURL.ResolveReference(&url.URL{Path: relativeURL})
	return resolved.String()
}
