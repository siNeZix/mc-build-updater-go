package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/filehosting"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/uploader"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "upload-mods" {
		fmt.Fprintln(os.Stderr, "использование: mc-bu-utils upload-mods [--dev] [--file-hosting-url URL] [--workers N]")
		os.Exit(2)
	}
	flags := flag.NewFlagSet("upload-mods", flag.ExitOnError)
	development := flags.Bool("dev", false, "использовать только локальный file-hosting")
	baseURL := flags.String("file-hosting-url", "", "базовый URL file-hosting")
	workers := flags.Int("workers", 8, "число параллельных загрузок")
	_ = flags.Parse(os.Args[2:])
	if *workers < 1 {
		log.Fatal("workers должен быть положительным числом")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if err := uploader.UploadMissing(uploader.Config{BaseURL: filehosting.URL(*development, *baseURL), Timeout: filehosting.Timeout(*development), Token: os.Getenv("FILE_HOSTING_TOKEN"), LocalModsPath: filepath.Join(workingDirectory, "mods"), Workers: *workers}); err != nil {
		log.Fatal(err)
	}
}
