package manifest

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sinezix/mc-build-updater-go/file-hosting/internal/filemap"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("открыть базу манифеста: %w", err)
	}
	store := &Store{db: db}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS files (
			path TEXT PRIMARY KEY,
			hash TEXT NOT NULL,
			name TEXT NOT NULL,
			dir TEXT NOT NULL,
			size INTEGER NOT NULL
		)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("создать схему манифеста: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("создать схему метаданных манифеста: %w", err)
	}
	return store, nil
}

func (s *Store) Version() (string, error) {
	var version string
	err := s.db.QueryRow("SELECT value FROM metadata WHERE key = 'version'").Scan(&version)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("прочитать версию манифеста: %w", err)
	}
	return version, nil
}

func (s *Store) SetVersion(version string) error {
	_, err := s.db.Exec(`INSERT INTO metadata(key, value) VALUES ('version', ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, version)
	if err != nil {
		return fmt.Errorf("сохранить версию манифеста: %w", err)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Sync(entries []filemap.Entry) (bool, error) {
	transaction, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("начать синхронизацию манифеста: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck

	changed := false
	rows, err := transaction.Query("SELECT path, hash, name, dir, size FROM files")
	if err != nil {
		return false, fmt.Errorf("прочитать файлы манифеста: %w", err)
	}
	existing := map[string]filemap.Entry{}
	for rows.Next() {
		var entry filemap.Entry
		if err := rows.Scan(&entry.Path, &entry.Hash, &entry.Name, &entry.Dir, &entry.Size); err != nil {
			rows.Close()
			return false, fmt.Errorf("прочитать запись манифеста: %w", err)
		}
		existing[entry.Path] = entry
	}
	if err := rows.Close(); err != nil {
		return false, fmt.Errorf("закрыть записи манифеста: %w", err)
	}
	for _, entry := range entries {
		old, exists := existing[entry.Path]
		if exists && old.Hash == entry.Hash && old.Name == entry.Name && old.Dir == entry.Dir && old.Size == entry.Size {
			delete(existing, entry.Path)
			continue
		}
		if _, err := transaction.Exec(`INSERT INTO files(path, hash, name, dir, size) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(path) DO UPDATE SET hash=excluded.hash, name=excluded.name, dir=excluded.dir, size=excluded.size`, entry.Path, entry.Hash, entry.Name, entry.Dir, entry.Size); err != nil {
			return false, fmt.Errorf("обновить файл манифеста: %w", err)
		}
		delete(existing, entry.Path)
		changed = true
	}
	for filePath := range existing {
		if _, err := transaction.Exec("DELETE FROM files WHERE path = ?", filePath); err != nil {
			return false, fmt.Errorf("удалить устаревший файл манифеста: %w", err)
		}
		changed = true
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("зафиксировать синхронизацию манифеста: %w", err)
	}
	return changed, nil
}

func (s *Store) Entries() ([]filemap.Entry, error) {
	rows, err := s.db.Query("SELECT hash, path, name, dir, size FROM files ORDER BY path")
	if err != nil {
		return nil, fmt.Errorf("прочитать манифест: %w", err)
	}
	defer rows.Close()
	var entries []filemap.Entry
	for rows.Next() {
		var entry filemap.Entry
		if err := rows.Scan(&entry.Hash, &entry.Path, &entry.Name, &entry.Dir, &entry.Size); err != nil {
			return nil, fmt.Errorf("прочитать запись манифеста: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func DatabaseFiles(path string) func(string) bool {
	base := filepath.Base(path)
	return func(candidate string) bool {
		name := filepath.Base(candidate)
		return name == base || name == base+"-wal" || name == base+"-shm"
	}
}

func IsDatabaseFile(path, databasePath string) bool {
	return strings.EqualFold(filepath.Clean(path), filepath.Clean(databasePath)) ||
		strings.EqualFold(filepath.Clean(path), filepath.Clean(databasePath+"-wal")) ||
		strings.EqualFold(filepath.Clean(path), filepath.Clean(databasePath+"-shm"))
}
