package selfupdate

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestReleaseUsesGitHubFirst(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/github":
			fmt.Fprint(response, `{"tag_name":"v1.2.3","assets":[{"name":"mc-build-updater.exe","browser_download_url":"https://example.test/updater.exe"},{"name":"checksums.txt","browser_download_url":"https://example.test/checksums.txt"}]}`)
		case "/gitlab":
			t.Errorf("GitLab не должен запрашиваться при корректном GitHub release")
			http.Error(response, "unexpected", http.StatusInternalServerError)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	release, err := latestRelease(Options{HTTPClient: server.Client(), GitHubURL: server.URL + "/github", GitLabURL: server.URL + "/gitlab"})
	if err != nil {
		t.Fatalf("latestRelease: %v", err)
	}
	if release.ProviderName != "GitHub" || release.Version.String() != "v1.2.3" {
		t.Fatalf("получен release %+v, ожидается GitHub v1.2.3", release)
	}
}

func TestLatestReleaseFallsBackToGitLab(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/github":
			http.Error(response, "unavailable", http.StatusServiceUnavailable)
		case "/gitlab":
			fmt.Fprint(response, `[{"tag_name":"v2.0.0","assets":{"links":[{"name":"mc-build-updater.exe","url":"https://example.test/updater.exe"},{"name":"checksums.txt","url":"https://example.test/checksums.txt"}]}}]`)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	release, err := latestRelease(Options{HTTPClient: server.Client(), GitHubURL: server.URL + "/github", GitLabURL: server.URL + "/gitlab"})
	if err != nil {
		t.Fatalf("latestRelease: %v", err)
	}
	if release.ProviderName != "GitLab" || release.Version.String() != "v2.0.0" {
		t.Fatalf("получен release %+v, ожидается GitLab v2.0.0", release)
	}
}

func TestDownloadAndVerify(t *testing.T) {
	contents := []byte("новый Windows-клиент")
	checksum := fmt.Sprintf("%x", sha256.Sum256(contents))
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/checksums.txt":
			fmt.Fprintf(response, "%s  mc-build-updater.exe\n", checksum)
		case "/mc-build-updater.exe":
			_, _ = response.Write(contents)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	destination := t.TempDir() + "/mc-build-updater.exe.new"
	if err := downloadAndVerify(server.Client(), server.URL+"/mc-build-updater.exe", server.URL+"/checksums.txt", destination); err != nil {
		t.Fatalf("downloadAndVerify: %v", err)
	}
}

func TestParseVersion(t *testing.T) {
	for _, test := range []struct {
		input string
		valid bool
	}{
		{input: "v0.0.1", valid: true},
		{input: "v1.20.4", valid: true},
		{input: "1.2.3", valid: false},
		{input: "v1.2.3-beta.1", valid: false},
		{input: "dev", valid: false},
	} {
		_, valid := parseVersion(test.input)
		if valid != test.valid {
			t.Errorf("parseVersion(%q) = %v, ожидается %v", test.input, valid, test.valid)
		}
	}
}

func TestUpdaterPathKeepsExecutableExtension(t *testing.T) {
	path := updaterPath(`C:\games\mc-build-updater.exe`)
	want := `C:\games\mc-build-updater.updater.exe`
	if path != want {
		t.Fatalf("updaterPath() = %q, ожидается %q", path, want)
	}
}

func TestMakeReleaseRequiresHTTPAssets(t *testing.T) {
	_, err := makeRelease("v1.2.3", []asset{
		{Name: executableName, URL: "file:///mc-build-updater.exe"},
		{Name: checksumsName, URL: "https://example.test/checksums.txt"},
	}, "тест")
	if err == nil {
		t.Fatal("release с file:// URL должен быть отклонён")
	}
}

func TestUpdateWorkingDirectory(t *testing.T) {
	if got := updateWorkingDirectory(`C:\minecraft`, `C:\programs\mc-build-updater.exe`); got != `C:\minecraft` {
		t.Fatalf("указанный рабочий каталог: %q", got)
	}
	if got := updateWorkingDirectory("", `C:\programs\mc-build-updater.exe`); got != `C:\programs` {
		t.Fatalf("рабочий каталог по умолчанию: %q", got)
	}
}
