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
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/modsync"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/remote"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/selfupdate"
)

const version = "alpha.4"

func main() {
	development := flag.Bool("dev", false, "use local file-hosting and skip self-update")
	updated := flag.Bool("updated", false, "cleanup update artifacts before starting")
	createModsMap := flag.Bool("mm", false, "write local mods checksum map to mm.json")
	applyUpdate := flag.Bool("apply-update", false, "apply a previously downloaded self-update")
	fileHostingURL := flag.String("file-hosting-url", "", "file-hosting base URL")
	flag.Parse()

	configureLog()
	workingDirectory, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if *applyUpdate {
		if err := selfupdate.Apply(workingDirectory); err != nil {
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
	if *updated {
		if err := selfupdate.Clean(workingDirectory); err != nil {
			log.Printf("clean update artifacts: %v", err)
		}
	}

	baseURL := selectFileHostingURL(*development, *fileHostingURL)
	client, err := remote.New(baseURL)
	if err != nil {
		log.Fatal(err)
	}

	if !*development {
		result, err := selfupdate.Run(selfupdate.Options{
			WorkingDirectory: workingDirectory,
			CurrentVersion:   version,
			Remote:           client,
			RuntimeOS:        runtime.GOOS,
		})
		if err != nil {
			log.Fatal(err)
		}
		if result.StartedReplacement {
			log.Println("mc-build updater started")
			return
		}
	}

	if err := runSetup(workingDirectory, client); err != nil {
		log.Fatal(err)
	}
}

func runSetup(workingDirectory string, client *remote.Client) error {
	localConfig, err := config.LoadOrCreate(workingDirectory)
	if err != nil {
		return err
	}

	fmt.Printf("Minecraft Mods Updater [%s]\n\n", version)
	branch.Print(localConfig.Branch)
	time.Sleep(3 * time.Second)

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
	return selfupdate.Clean(workingDirectory)
}

func selectFileHostingURL(development bool, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if development {
		return "http://localhost:1447/"
	}
	return "http://mc.sinezix.ru:1447/"
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
