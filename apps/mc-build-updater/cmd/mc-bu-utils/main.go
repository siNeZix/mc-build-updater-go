package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/console"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/envfile"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/filehosting"
	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/uploader"
)

func main() {
	if err := envfile.LoadNextToExecutable(); err != nil {
		console.Warning("не удалось загрузить .env: %v", err)
	}
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	command := os.Args[1]
	flags := flag.NewFlagSet(command, flag.ExitOnError)
	development := flags.Bool("dev", false, "использовать только локальный file-hosting")
	baseURL := flags.String("file-hosting-url", "", "базовый URL file-hosting")
	workers := flags.Int("workers", 8, "число параллельных загрузок")
	arguments := os.Args[2:]
	branchArgument := ""
	if (command == "upload" || command == "upload-manifest") && len(arguments) > 0 && arguments[0][0] != '-' {
		branchArgument = arguments[0]
		arguments = arguments[1:]
	}
	_ = flags.Parse(arguments)
	if *workers < 1 {
		console.Error("число потоков должно быть положительным")
		return
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		console.Error("не удалось определить рабочий каталог: %v", err)
		return
	}
	configuration := uploader.Config{
		BaseURL:                filehosting.URL(*development, *baseURL),
		Timeout:                filehosting.Timeout(*development),
		Token:                  os.Getenv("FILE_HOSTING_TOKEN"),
		LocalModsPath:          filepath.Join(workingDirectory, "mods"),
		LocalResourcePacksPath: filepath.Join(workingDirectory, "resourcepacks"),
		LocalShaderPacksPath:   filepath.Join(workingDirectory, "shaderpacks"),
		Workers:                *workers,
	}
	switch command {
	case "upload-mods":
		if flags.NArg() != 0 {
			console.Error("использование: mc-bu-utils upload-mods [--dev] [--file-hosting-url URL] [--workers N]")
			return
		}
		if err := uploader.UploadMissing(configuration); err != nil {
			console.Error("не удалось загрузить моды: %v", err)
		}
	case "upload":
		branch, valid := positionalBranch(branchArgument, flags)
		if !valid {
			console.Error("использование: mc-bu-utils upload [<branch>] [--dev] [--file-hosting-url URL] [--workers N]")
			return
		}
		if branch == "" {
			branches, err := uploader.ListBranches(configuration)
			if err != nil {
				console.Error("не удалось получить список веток: %v", err)
				return
			}
			if len(branches) == 0 {
				console.Info("Опубликованных веток нет.")
				return
			}
			for _, branch := range branches {
				fmt.Println(branch)
			}
			return
		}
		if err := uploader.UploadBranch(configuration, branch); err != nil {
			console.Error("не удалось обновить ветку: %v", err)
		}
	case "upload-manifest":
		branch, valid := positionalBranch(branchArgument, flags)
		if !valid || branch == "" {
			console.Error("использование: mc-bu-utils upload-manifest <branch> [--dev] [--file-hosting-url URL] [--workers N]")
			return
		}
		if err := uploader.UploadManifest(configuration, branch); err != nil {
			console.Error("не удалось обновить манифест: %v", err)
		}
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "использование:")
	fmt.Fprintln(os.Stderr, "  mc-bu-utils upload [--dev] [--file-hosting-url URL]")
	fmt.Fprintln(os.Stderr, "  mc-bu-utils upload-mods [--dev] [--file-hosting-url URL] [--workers N]")
	fmt.Fprintln(os.Stderr, "  mc-bu-utils upload-manifest <branch> [--dev] [--file-hosting-url URL] [--workers N]")
}

func positionalBranch(first string, flags *flag.FlagSet) (string, bool) {
	if first != "" {
		return first, flags.NArg() == 0
	}
	if flags.NArg() == 0 {
		return "", true
	}
	if flags.NArg() == 1 {
		return flags.Arg(0), true
	}
	return "", false
}
