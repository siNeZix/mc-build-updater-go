package remote

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileMap struct {
	Path string `json:"path"`
	Dir  string `json:"dir"`
	Name string `json:"name"`
	Hash string `json:"hash"`
}

type RemoteConfig struct {
	LastVersion string `json:"LastVersion"`
}

type Client struct {
	baseURL *url.URL
	http    *http.Client
}

func New(rawBaseURL string) (*Client, error) {
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse file-hosting URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("file-hosting URL requires HTTP(S) scheme: %q", rawBaseURL)
	}
	if baseURL.Host == "" {
		return nil, fmt.Errorf("file-hosting URL requires host: %q", rawBaseURL)
	}
	if !strings.HasSuffix(baseURL.Path, "/") {
		baseURL.Path += "/"
	}
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Minute},
	}, nil
}

func (c *Client) RemoteConfig() (RemoteConfig, error) {
	var configuration RemoteConfig
	if err := c.getJSON("config/remote.json", &configuration); err != nil {
		return RemoteConfig{}, err
	}
	return configuration, nil
}

func (c *Client) ModsMap(branch string) ([]Mod, error) {
	var entries []Mod
	if err := c.getJSON("MM/"+url.PathEscape(branch)+".json", &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) FileMap() ([]FileMap, error) {
	var entries []FileMap
	if err := c.getJSON("map", &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (c *Client) Download(relativeURL, destination, prefix string) error {
	requestURL := c.resolve(relativeURL)
	if prefix != "" {
		prefix = "[" + prefix + "] "
	}
	fmt.Printf("%s%s [%s]\n", prefix, requestURL, filepath.Base(destination))

	response, err := c.http.Get(requestURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", requestURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", requestURL, response.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create download directory: %w", err)
	}

	temporaryFile, err := os.CreateTemp(filepath.Dir(destination), ".download-*")
	if err != nil {
		return fmt.Errorf("create temporary download: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	if _, err := io.Copy(temporaryFile, response.Body); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("write downloaded file: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close downloaded file: %w", err)
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace download destination: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("move downloaded file into place: %w", err)
	}
	return nil
}

func (c *Client) getJSON(relativeURL string, target any) error {
	requestURL := c.resolve(relativeURL)
	fmt.Println(requestURL)
	response, err := c.http.Get(requestURL)
	if err != nil {
		return fmt.Errorf("request %s: %w", requestURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("request %s: HTTP %d", requestURL, response.StatusCode)
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
