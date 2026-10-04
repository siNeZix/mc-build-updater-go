package filehosting

import "time"

const (
	LocalURL      = "http://localhost:1447/"
	ProductionURL = "http://mc.sinezix.ru:1447/"

	localTimeout      = 15 * time.Second
	productionTimeout = 15 * time.Minute
)

// URL возвращает localhost для --dev независимо от переданного production URL.
func URL(development bool, explicit string) string {
	if development {
		return LocalURL
	}
	if explicit != "" {
		return explicit
	}
	return ProductionURL
}

// Timeout ограничивает ожидание локального server, чтобы dev-запуск не зависал.
func Timeout(development bool) time.Duration {
	if development {
		return localTimeout
	}
	return productionTimeout
}
