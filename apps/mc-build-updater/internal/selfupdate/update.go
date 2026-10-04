package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	githubLatestReleaseURL = "https://api.github.com/repos/siNeZix/mc-build-updater-go/releases/latest"
	gitlabReleasesURL      = "https://gitlab.com/api/v4/projects/siNeZix%2Fmc-build-updater-go/releases?order_by=released_at&sort=desc&per_page=100"

	executableName = "mc-build-updater.exe"
	checksumsName  = "checksums.txt"
)

var stableVersionPattern = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)

type Options struct {
	CurrentVersion   string
	RuntimeOS        string
	WorkingDirectory string
	ExecutablePath   string
	HTTPClient       *http.Client
	GitHubURL        string
	GitLabURL        string
}

type Result struct {
	StartedReplacement bool
	CheckError         error
}

type release struct {
	Version      version
	DownloadURL  string
	ChecksumURL  string
	ProviderName string
}

type version struct {
	Major int
	Minor int
	Patch int
}

// Run проверяет GitHub, затем GitLab, если GitHub не отдал корректный release.
// Ошибка проверки не блокирует установленный клиент. Найденное обновление обязательно.
func Run(options Options) (Result, error) {
	if options.RuntimeOS != "windows" {
		return Result{}, nil
	}
	if options.CurrentVersion == "dev" {
		return Result{}, nil
	}

	current, currentIsStable := parseVersion(options.CurrentVersion)
	release, err := latestRelease(options)
	if err != nil {
		return Result{CheckError: err}, nil
	}
	if currentIsStable && release.Version.compare(current) <= 0 {
		return Result{}, nil
	}

	executablePath := options.ExecutablePath
	if executablePath == "" {
		executablePath, err = os.Executable()
		if err != nil {
			return Result{}, fmt.Errorf("определить путь текущего исполняемого файла: %w", err)
		}
	}
	executablePath, err = filepath.Abs(executablePath)
	if err != nil {
		return Result{}, fmt.Errorf("получить абсолютный путь исполняемого файла: %w", err)
	}

	replacementPath := executablePath + ".new"
	if err := downloadAndVerify(options.httpClient(), release.DownloadURL, release.ChecksumURL, replacementPath); err != nil {
		if release.ProviderName != "GitHub" {
			return Result{CheckError: fmt.Errorf("скачать обновление %s из %s: %w", release.Version, release.ProviderName, err)}, nil
		}

		gitlabRelease, gitlabErr := gitlabLatest(options.httpClient(), gitLabURL(options))
		if gitlabErr != nil {
			return Result{CheckError: fmt.Errorf("скачать обновление %s из GitHub: %v; GitLab: %w", release.Version, err, gitlabErr)}, nil
		}
		if currentIsStable && gitlabRelease.Version.compare(current) <= 0 {
			return Result{CheckError: fmt.Errorf("скачать обновление %s из GitHub: %v; GitLab содержит не более новую версию %s", release.Version, err, gitlabRelease.Version)}, nil
		}
		if gitlabErr := downloadAndVerify(options.httpClient(), gitlabRelease.DownloadURL, gitlabRelease.ChecksumURL, replacementPath); gitlabErr != nil {
			return Result{CheckError: fmt.Errorf("скачать обновление %s из GitHub: %v; из GitLab: %w", release.Version, err, gitlabErr)}, nil
		}
		release = gitlabRelease
	}

	launcherPath := updaterPath(executablePath)
	if err := copyFile(executablePath, launcherPath); err != nil {
		_ = os.Remove(replacementPath)
		return Result{}, fmt.Errorf("создать процесс замены обновления: %w", err)
	}

	command := exec.Command(launcherPath,
		"--apply-update",
		"--update-target", executablePath,
		"--update-replacement", replacementPath,
		"--update-working-directory", options.WorkingDirectory,
	)
	command.Dir = updateWorkingDirectory(options.WorkingDirectory, executablePath)
	if err := command.Start(); err != nil {
		_ = os.Remove(launcherPath)
		_ = os.Remove(replacementPath)
		return Result{}, fmt.Errorf("запустить процесс замены обновления: %w", err)
	}
	return Result{StartedReplacement: true}, nil
}

// Apply заменяет цель после завершения исходного клиента и запускает новую версию.
func Apply(target, replacement, workingDirectory string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("замена обновления поддерживается только в Windows")
	}
	if target == "" || replacement == "" {
		return fmt.Errorf("для замены обновления не указаны целевой и новый исполняемые файлы")
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("получить путь целевого исполняемого файла: %w", err)
	}
	replacement, err = filepath.Abs(replacement)
	if err != nil {
		return fmt.Errorf("получить путь нового исполняемого файла: %w", err)
	}
	if filepath.Dir(target) != filepath.Dir(replacement) {
		return fmt.Errorf("новый исполняемый файл должен находиться рядом с целевым")
	}
	if err := replaceFile(replacement, target); err != nil {
		return fmt.Errorf("заменить исполняемый файл: %w", err)
	}

	command := exec.Command(target, "--updated")
	command.Dir = updateWorkingDirectory(workingDirectory, target)
	if err := command.Start(); err != nil {
		return fmt.Errorf("запустить обновлённый клиент: %w", err)
	}
	return nil
}

// Clean удаляет временные файлы успешного или прерванного обновления.
func Clean(executablePath string) error {
	if executablePath == "" {
		return nil
	}
	for _, path := range []string{executablePath + ".new", updaterPath(executablePath)} {
		deadline := time.Now().Add(30 * time.Second)
		for {
			err := os.Remove(path)
			if err == nil || errors.Is(err, os.ErrNotExist) {
				break
			}
			if time.Now().After(deadline) {
				return err
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	return nil
}

func latestRelease(options Options) (release, error) {
	githubURL := options.GitHubURL
	if githubURL == "" {
		githubURL = githubLatestReleaseURL
	}
	githubRelease, githubErr := githubLatest(options.httpClient(), githubURL)
	if githubErr == nil {
		return githubRelease, nil
	}

	gitlabRelease, gitlabErr := gitlabLatest(options.httpClient(), gitLabURL(options))
	if gitlabErr == nil {
		return gitlabRelease, nil
	}
	return release{}, fmt.Errorf("проверка обновлений: GitHub: %v; GitLab: %w", githubErr, gitlabErr)
}

func gitLabURL(options Options) string {
	if options.GitLabURL != "" {
		return options.GitLabURL
	}
	return gitlabReleasesURL
}

func githubLatest(client *http.Client, endpoint string) (release, error) {
	var response struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := getJSON(client, endpoint, &response); err != nil {
		return release{}, err
	}
	if response.Draft || response.Prerelease {
		return release{}, fmt.Errorf("последний release не является стабильным")
	}
	return makeRelease(response.TagName, githubAssets(response.Assets), "GitHub")
}

func gitlabLatest(client *http.Client, endpoint string) (release, error) {
	var responses []struct {
		TagName string `json:"tag_name"`
		Assets  struct {
			Links []struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"links"`
		} `json:"assets"`
	}
	if err := getJSON(client, endpoint, &responses); err != nil {
		return release{}, err
	}
	for _, response := range responses {
		if _, ok := parseVersion(response.TagName); !ok {
			continue
		}
		return makeRelease(response.TagName, gitlabAssets(response.Assets.Links), "GitLab")
	}
	return release{}, fmt.Errorf("стабильный release с %s и %s не найден", executableName, checksumsName)
}

type asset struct {
	Name string
	URL  string
}

func githubAssets(input []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) []asset {
	assets := make([]asset, 0, len(input))
	for _, item := range input {
		assets = append(assets, asset{Name: item.Name, URL: item.BrowserDownloadURL})
	}
	return assets
}

func gitlabAssets(input []struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}) []asset {
	assets := make([]asset, 0, len(input))
	for _, item := range input {
		assets = append(assets, asset{Name: item.Name, URL: item.URL})
	}
	return assets
}

func makeRelease(tag string, assets []asset, providerName string) (release, error) {
	parsedVersion, ok := parseVersion(tag)
	if !ok {
		return release{}, fmt.Errorf("тег %q не соответствует стабильной версии vX.Y.Z", tag)
	}
	var executableURL, checksumURL string
	for _, asset := range assets {
		switch asset.Name {
		case executableName:
			executableURL = asset.URL
		case checksumsName:
			checksumURL = asset.URL
		}
	}
	if executableURL == "" || checksumURL == "" {
		return release{}, fmt.Errorf("релиз %s не содержит %s и %s", tag, executableName, checksumsName)
	}
	if _, err := url.ParseRequestURI(executableURL); err != nil {
		return release{}, fmt.Errorf("некорректная ссылка на %s: %w", executableName, err)
	}
	if _, err := url.ParseRequestURI(checksumURL); err != nil {
		return release{}, fmt.Errorf("некорректная ссылка на %s: %w", checksumsName, err)
	}
	if !isHTTPURL(executableURL) || !isHTTPURL(checksumURL) {
		return release{}, fmt.Errorf("ссылки release должны использовать HTTP(S)")
	}
	return release{Version: parsedVersion, DownloadURL: executableURL, ChecksumURL: checksumURL, ProviderName: providerName}, nil
}

func getJSON(client *http.Client, endpoint string, destination any) error {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "mc-build-updater")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP %s", response.Status)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 5<<20)).Decode(destination); err != nil {
		return fmt.Errorf("декодировать JSON: %w", err)
	}
	return nil
}

func downloadAndVerify(client *http.Client, executableURL, checksumURL, destination string) error {
	expectedChecksum, err := downloadChecksum(client, checksumURL)
	if err != nil {
		return err
	}

	response, err := get(client, executableURL)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(file, hash), response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return closeErr
	}
	actualChecksum := hex.EncodeToString(hash.Sum(nil))
	if actualChecksum != expectedChecksum {
		_ = os.Remove(destination)
		return fmt.Errorf("SHA-256 не совпадает: ожидался %s, получен %s", expectedChecksum, actualChecksum)
	}
	return nil
}

func downloadChecksum(client *http.Client, checksumURL string) (string, error) {
	response, err := get(client, checksumURL)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == executableName {
			checksum := strings.ToLower(fields[0])
			if len(checksum) != sha256.Size*2 {
				break
			}
			if _, err := hex.DecodeString(checksum); err == nil {
				return checksum, nil
			}
			break
		}
	}
	return "", fmt.Errorf("в %s нет корректной SHA-256 для %s", checksumsName, executableName)
}

func get(client *http.Client, endpoint string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "mc-build-updater")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		response.Body.Close()
		return nil, fmt.Errorf("HTTP %s", response.Status)
	}
	return response, nil
}

func replaceFile(source, target string) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			if time.Now().After(deadline) {
				return err
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err := os.Rename(source, target); err == nil {
			return nil
		} else if time.Now().After(deadline) {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func updaterPath(executablePath string) string {
	extension := filepath.Ext(executablePath)
	return strings.TrimSuffix(executablePath, extension) + ".updater" + extension
}

func updateWorkingDirectory(workingDirectory, executablePath string) string {
	if workingDirectory != "" {
		return workingDirectory
	}
	return filepath.Dir(executablePath)
}

func isHTTPURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func parseVersion(input string) (version, bool) {
	matches := stableVersionPattern.FindStringSubmatch(input)
	if matches == nil {
		return version{}, false
	}
	var parsed version
	if _, err := fmt.Sscanf(input, "v%d.%d.%d", &parsed.Major, &parsed.Minor, &parsed.Patch); err != nil {
		return version{}, false
	}
	return parsed, true
}

func (v version) compare(other version) int {
	if v.Major != other.Major {
		return compareInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return compareInt(v.Minor, other.Minor)
	}
	return compareInt(v.Patch, other.Patch)
}

func (v version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func compareInt(left, right int) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func (options Options) httpClient() *http.Client {
	if options.HTTPClient != nil {
		return options.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Minute}
}
