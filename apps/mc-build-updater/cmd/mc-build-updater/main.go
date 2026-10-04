package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/branch"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/config"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/filehosting"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/modsync"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/remote"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/selfupdate"
)

// version получает release-tag через -ldflags при release-сборке.
var version = "dev"

func main() {
	development := flag.Bool("dev", false, "использовать локальный file-hosting и пропустить self-update")
	updated := flag.Bool("updated", false, "очистить временные файлы обновления")
	createModsMap := flag.Bool("mm", false, "записать локальную карту checksum модов в mm.json")
	applyUpdate := flag.Bool("apply-update", false, "внутренний флаг замены обновления")
	updateTarget := flag.String("update-target", "", "внутренний путь заменяемого файла")
	updateReplacement := flag.String("update-replacement", "", "внутренний путь нового файла")
	updateWorkingDirectory := flag.String("update-working-directory", "", "внутренний рабочий каталог обновления")
	fileHostingURL := flag.String("file-hosting-url", "", "базовый URL file-hosting")
	flag.Parse()

	configureLog()
	workingDirectory, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if *applyUpdate {
		if err := selfupdate.Apply(*updateTarget, *updateReplacement, *updateWorkingDirectory); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *createModsMap {
		entries, err := modsync.LocalMap(filepath.Join(workingDirectory, "mods"))
		if err != nil {
			log.Fatal(err)
		}
		if err := modsync.WriteJSON(filepath.Join(workingDirectory, "mm.json"), entries); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *updated && !*development {
		executablePath, err := os.Executable()
		if err != nil {
			log.Printf("определить путь для очистки обновления: %v", err)
		} else if err := selfupdate.Clean(executablePath); err != nil {
			log.Printf("очистить временные файлы обновления: %v", err)
		}
	}

	baseURL := filehosting.URL(*development, *fileHostingURL)
	client, err := remote.NewWithTimeout(baseURL, filehosting.Timeout(*development))
	if err != nil {
		log.Fatal(err)
	}

	if !*development {
		result, err := selfupdate.Run(selfupdate.Options{
			CurrentVersion:   version,
			RuntimeOS:        runtime.GOOS,
			WorkingDirectory: workingDirectory,
		})
		if err != nil {
			log.Fatal(err)
		}
		if result.CheckError != nil {
			log.Printf("не удалось проверить обновления: %v; продолжается текущая версия", result.CheckError)
		}
		if result.StartedReplacement {
			log.Println("запущена замена клиента на новую версию")
			return
		}
	}

	if err := runSetup(workingDirectory, client, *development); err != nil {
		log.Fatal(err)
	}
}

func runSetup(workingDirectory string, client *remote.Client, development bool) error {
	localConfig, err := config.LoadOrCreate(workingDirectory)
	if err != nil {
		return err
	}

	fmt.Printf("Minecraft Mods Updater [%s]\n\n", version)
	branch.Print(localConfig.Branch)
	if delay := startupDelay(development); delay > 0 {
		time.Sleep(delay)
	}

	modsPath := filepath.Join(workingDirectory, "mods")
	if err := os.MkdirAll(modsPath, 0o755); err != nil {
		return fmt.Errorf("create mods directory: %w", err)
	}

	synchronizer := modsync.New(client, modsPath, 10)
	result, err := synchronizer.Sync(localConfig.Branch)
	if err != nil {
		return err
	}
	if result.Downloaded == 0 && result.Deleted == 0 {
		fmt.Println("All mods are already up to date.")
	} else {
		fmt.Printf("Done. Downloaded: %d, deleted: %d.\n", result.Downloaded, result.Deleted)
	}
	if !development {
		return selfupdate.Clean(workingDirectory)
	}
	return nil
}

func startupDelay(development bool) time.Duration {
	if development {
		return 0
	}
	return 3 * time.Second
}

func configureLog() {
	file, err := os.OpenFile("mc-mods-updater.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("open log file: %v", err)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, file))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix(strings.Repeat("=", 8) + " ")
}
