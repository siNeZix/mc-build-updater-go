package main

import "testing"

func TestDevelopmentSkipsStartupDelay(t *testing.T) {
	// runSetup не должен ждать перед синхронизацией в --dev.
	if startupDelay(true) != 0 {
		t.Fatalf("startupDelay(true) = %s, ожидается 0", startupDelay(true))
	}
}

func TestProductionKeepsStartupDelay(t *testing.T) {
	if got := startupDelay(false).Seconds(); got != 3 {
		t.Fatalf("startupDelay(false) = %gs, ожидается 3s", got)
	}
}
