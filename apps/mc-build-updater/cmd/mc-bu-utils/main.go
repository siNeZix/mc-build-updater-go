package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/sinezix/mc-build-updater-go/mc-build-updater/internal/uploader"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "upload-mods" {
		fmt.Fprintln(os.Stderr, "usage: mc-bu-utils upload-mods [workers]")
		os.Exit(2)
	}

	workers := 8
	if len(os.Args) == 3 {
		parsed, err := strconv.Atoi(os.Args[2])
		if err != nil || parsed < 1 {
			log.Fatal("workers must be a positive integer")
		}
		workers = parsed
	}
	if len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: mc-bu-utils upload-mods [workers]")
		os.Exit(2)
	}

	configuration, err := uploader.ConfigFromEnvironment()
	if err != nil {
		log.Fatal(err)
	}
	configuration.Workers = workers
	if err := uploader.UploadMissing(configuration); err != nil {
		log.Fatal(err)
	}
}
