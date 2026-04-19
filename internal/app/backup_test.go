package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/jamesdowzard/txt/internal/db"
)

func newBackupApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := db.New(filepath.Join(dir, "messages.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	app := &App{
		Store:   store,
		DataDir: dir,
		Logger:  zerolog.Nop(),
	}
	return app, dir
}

func TestRunDailyBackupProducesSnapshot(t *testing.T) {
	app, dir := newBackupApp(t)

	app.RunDailyBackup()

	backupDir := filepath.Join(dir, backupDirName)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	var count int
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, backupPrefix) && strings.HasSuffix(n, backupSuffix) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("want 1 backup, got %d", count)
	}

	// Second call on the same day is a no-op — still one file.
	app.RunDailyBackup()
	entries, _ = os.ReadDir(backupDir)
	count = 0
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, backupPrefix) && strings.HasSuffix(n, backupSuffix) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("same-day retry expected 1 file, got %d", count)
	}
}

func TestPruneOldBackupsKeepsRetainedDays(t *testing.T) {
	app, dir := newBackupApp(t)
	backupDir := filepath.Join(dir, backupDirName)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed backupRetainDays+5 synthetic backups across consecutive days.
	seeded := backupRetainDays + 5
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < seeded; i++ {
		name := backupPathForDate(backupDir, base.AddDate(0, 0, i))
		if err := os.WriteFile(name, []byte("fake"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	app.pruneOldBackups(backupDir)

	entries, _ := os.ReadDir(backupDir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != backupRetainDays {
		t.Fatalf("want %d backups after prune, got %d: %v", backupRetainDays, len(names), names)
	}
	// Oldest seeded day should be gone.
	oldest := backupPathForDate(backupDir, base)
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Fatalf("oldest backup %s should have been pruned", oldest)
	}
	// Newest seeded day should remain.
	newest := backupPathForDate(backupDir, base.AddDate(0, 0, seeded-1))
	if _, err := os.Stat(newest); err != nil {
		t.Fatalf("newest backup %s should remain: %v", newest, err)
	}
}
