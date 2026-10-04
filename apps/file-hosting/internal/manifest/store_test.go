package manifest

import (
	"path/filepath"
	"testing"

	"github.com/sinezix/mc-build-updater-go/file-hosting/internal/filemap"
)

func TestStoreSyncTracksAdditionsChangesAndRemovals(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "manifest.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	first := []filemap.Entry{{Path: `C:\files\a.jar`, Hash: "a", Name: "a.jar", Dir: "mods", Size: 1}}
	changed, err := store.Sync(first)
	if err != nil || !changed {
		t.Fatalf("первый Sync = (%t, %v), ожидается (true, nil)", changed, err)
	}

	changed, err = store.Sync(first)
	if err != nil || changed {
		t.Fatalf("повторный Sync = (%t, %v), ожидается (false, nil)", changed, err)
	}

	second := []filemap.Entry{{Path: `C:\files\a.jar`, Hash: "changed", Name: "a.jar", Dir: "mods", Size: 2}, {Path: `C:\files\b.jar`, Hash: "b", Name: "b.jar", Dir: "mods", Size: 3}}
	changed, err = store.Sync(second)
	if err != nil || !changed {
		t.Fatalf("Sync с изменениями = (%t, %v), ожидается (true, nil)", changed, err)
	}

	entries, err := store.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(entries) != 2 || entries[0].Hash != "changed" || entries[1].Name != "b.jar" {
		t.Fatalf("записи = %#v", entries)
	}

	changed, err = store.Sync(second[1:])
	if err != nil || !changed {
		t.Fatalf("Sync с удалением = (%t, %v), ожидается (true, nil)", changed, err)
	}
	entries, err = store.Entries()
	if err != nil || len(entries) != 1 || entries[0].Name != "b.jar" {
		t.Fatalf("записи после удаления = %#v, ошибка = %v", entries, err)
	}
}

func TestStoreVersionAndDatabaseFiles(t *testing.T) {
	database := filepath.Join(t.TempDir(), ".file-hosting.sqlite")
	store, err := Open(database)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if version, err := store.Version(); err != nil || version != "" {
		t.Fatalf("начальная версия = %q, ошибка = %v", version, err)
	}
	if err := store.SetVersion("abc123:2"); err != nil {
		t.Fatalf("SetVersion: %v", err)
	}
	if version, err := store.Version(); err != nil || version != "abc123:2" {
		t.Fatalf("версия = %q, ошибка = %v", version, err)
	}

	skip := DatabaseFiles(database)
	for _, path := range []string{database, database + "-wal", database + "-shm"} {
		if !skip(path) || !IsDatabaseFile(path, database) {
			t.Fatalf("файл БД должен исключаться: %q", path)
		}
	}
	if skip(filepath.Join(filepath.Dir(database), "mod.jar")) || IsDatabaseFile(filepath.Join(filepath.Dir(database), "mod.jar"), database) {
		t.Fatal("обычный файл не должен считаться файлом БД")
	}
}
