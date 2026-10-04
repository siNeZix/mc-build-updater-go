package console

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteWithoutColor(t *testing.T) {
	previous := colorEnabled
	t.Cleanup(func() { colorEnabled = previous })
	colorEnabled = false

	var output bytes.Buffer
	write(&output, green, "✓", "Готово: %d", 3)

	if got, want := output.String(), "✓ Готово: 3\n"; got != want {
		t.Fatalf("вывод = %q, ожидается %q", got, want)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatal("в выводе без цвета найдены ANSI-последовательности")
	}
}

func TestWriteWithColor(t *testing.T) {
	previous := colorEnabled
	t.Cleanup(func() { colorEnabled = previous })
	colorEnabled = true

	var output bytes.Buffer
	write(&output, green, "✓", "Готово")

	if got, want := output.String(), green+"✓ "+reset+"Готово\n"; got != want {
		t.Fatalf("вывод = %q, ожидается %q", got, want)
	}
}
