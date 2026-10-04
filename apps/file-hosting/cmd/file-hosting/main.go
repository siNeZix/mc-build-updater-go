package main

import (
	"flag"
	"net/http"
	"os"

	"github.com/sinezix/mc-build-updater-go/file-hosting/internal/console"
	"github.com/sinezix/mc-build-updater-go/file-hosting/internal/httpserver"
)

func main() {
	addr := flag.String("addr", valueFromEnv("ADDR", ":1447"), "HTTP listen address")
	filesPath := flag.String("files-path", valueFromEnv("FILES_PATH", "files"), "каталог публикуемых файлов")
	flag.Parse()

	service, err := httpserver.New(*filesPath, os.Getenv("FILE_HOSTING_TOKEN"))
	if err != nil {
		console.Error("не удалось инициализировать карту файлов: %v", err)
		return
	}

	console.Success("file-hosting запущен: %s; файлы: %s", *addr, service.Root())
	if err := http.ListenAndServe(*addr, service.Handler()); err != nil {
		console.Error("сервер остановлен с ошибкой: %v", err)
	}
}

func valueFromEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
