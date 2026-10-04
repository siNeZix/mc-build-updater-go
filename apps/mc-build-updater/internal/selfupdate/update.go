package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/remote"
)

const (
	archiveName  = "mc-build-updater.7z"
	launcherName = "update.exe"
	sevenZipName = "7z.exe"
)

type Options struct {
	WorkingDirectory string
	CurrentVersion   string
	Remote           *remote.Client
	RuntimeOS        string
}

type Result struct {
	StartedReplacement bool
}

// Run preserves two-stage Windows update flow from TypeScript updater.
func Run(options Options) (Result, error) {
	if options.Remote == nil {
		return Result{}, fmt.Errorf("remote client is required")
	}
	if err := ensureSevenZip(options.WorkingDirectory, options.Remote); err != nil {
		return Result{}, err
	}

	configuration, err := options.Remote.RemoteConfig()
	if err != nil {
		return Result{}, err
	}
	if configuration.LastVersion == options.CurrentVersion {
		return Result{}, nil
	}
	if options.RuntimeOS != "windows" {
		return Result{}, fmt.Errorf("an updater update is available (%s -> %s), but self-update requires Windows", options.CurrentVersion, configuration.LastVersion)
	}

	archivePath := filepath.Join(options.WorkingDirectory, archiveName)
	if err := options.Remote.Download("static/"+archiveName, archivePath, ""); err != nil {
		return Result{}, err
	}

	currentExecutable, err := os.Executable()
	if err != nil {
		return Result{}, fmt.Errorf("resolve current executable: %w", err)
	}
	launcherPath := filepath.Join(options.WorkingDirectory, launcherName)
	if err := copyFile(currentExecutable, launcherPath); err != nil {
		return Result{}, fmt.Errorf("copy update launcher: %w", err)
	}

	command := exec.Command(launcherPath, "--apply-update")
	command.Dir = options.WorkingDirectory
	if err := command.Start(); err != nil {
		return Result{}, fmt.Errorf("start update launcher: %w", err)
	}
	return Result{StartedReplacement: true}, nil
}

// Apply unpacks already-downloaded archive and starts fresh updater executable.
// It must be called by copied update.exe after original process exits.
func Apply(workingDirectory string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("apply update requires Windows")
	}
	time.Sleep(100 * time.Millisecond)

	archivePath := filepath.Join(workingDirectory, archiveName)
	command := exec.Command(filepath.Join(workingDirectory, sevenZipName), "x", archivePath, "-y", "-p123")
	command.Dir = workingDirectory
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("extract updater archive: %w", err)
	}

	freshExecutable := filepath.Join(workingDirectory, "mc-build-updater.exe")
	start := exec.Command(freshExecutable, "--updated")
	start.Dir = workingDirectory
	if err := start.Start(); err != nil {
		return fmt.Errorf("start updated updater: %w", err)
	}
	return nil
}

func Clean(workingDirectory string) error {
	for _, name := range []string{"mc-mods-updater.7z", "mods.7z", "update.bat", launcherName, archiveName} {
		path := filepath.Join(workingDirectory, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func ensureSevenZip(workingDirectory string, client *remote.Client) error {
	path := filepath.Join(workingDirectory, sevenZipName)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat 7-Zip executable: %w", err)
	}
	return client.Download("static/7z_x32.exe", path, "")
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	info, err := input.Stat()
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
	if err != nil {
		return err
	}
	if _, err := output.ReadFrom(input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}
