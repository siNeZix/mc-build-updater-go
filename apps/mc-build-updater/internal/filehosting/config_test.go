package filehosting

import (
	"testing"
	"time"
)

func TestURLUsesLocalhostInDevelopment(t *testing.T) {
	if got := URL(true, "http://mc.sinezix.ru:1447/"); got != LocalURL {
		t.Fatalf("URL(true, explicit) = %q, ожидается %q", got, LocalURL)
	}
}

func TestURLUsesExplicitValueOutsideDevelopment(t *testing.T) {
	const explicit = "http://example.test:1447/"
	if got := URL(false, explicit); got != explicit {
		t.Fatalf("URL(false, explicit) = %q, ожидается %q", got, explicit)
	}
}

func TestURLUsesProductionDefaultOutsideDevelopment(t *testing.T) {
	if got := URL(false, ""); got != ProductionURL {
		t.Fatalf("URL(false, empty) = %q, ожидается %q", got, ProductionURL)
	}
}

func TestTimeoutIsShortInDevelopment(t *testing.T) {
	if got := Timeout(true); got != 15*time.Second {
		t.Fatalf("Timeout(true) = %s, ожидается 15s", got)
	}
}

func TestTimeoutIsLongInProduction(t *testing.T) {
	if got := Timeout(false); got != 15*time.Minute {
		t.Fatalf("Timeout(false) = %s, ожидается 15m", got)
	}
}
