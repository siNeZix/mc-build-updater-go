package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/sinezix/mc-build-updater-go/file-hosting/internal/httpserver"
)

func main() {
	addr := flag.String("addr", valueFromEnv("ADDR", ":1447"), "HTTP listen address")
	filesPath := flag.String("files-path", valueFromEnv("FILES_PATH", "files"), "directory containing served files")
	flag.Parse()

	service, err := httpserver.New(*filesPath)
	if err != nil {
		log.Fatalf("initialize files map: %v", err)
	}

	log.Printf("file-hosting listening on %s, serving %s", *addr, service.Root())
	if err := http.ListenAndServe(*addr, service.Handler()); err != nil {
		log.Fatal(err)
	}
}

func valueFromEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
