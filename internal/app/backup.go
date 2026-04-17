package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	backupInterval   = 24 * time.Hour
	backupRetainDays = 14 // keep the last 14 daily backups; prune older
	backupDirName    = "backups"
	backupPrefix     = "messages-"
	backupSuffix     = ".db"
)

// StartBackupScheduler spawns a background goroutine that snapshots
// messages.db to <DataDir>/backups/messages-YYYY-MM-DD.db once per day.
// If no backup exists for today, the first snapshot runs immediately.
// Older backups beyond backupRetainDays are pruned best-effort.
//
// Safe to call once per process. Returns immediately — the ticker runs
// under the given context; cancel it to stop.
func (a *App) StartBackupScheduler(ctx context.Context) {
	go a.runBackupLoop(ctx)
}

func (a *App) runBackupLoop(ctx context.Context) {
	// Attempt an immediate backup so newly-installed apps have coverage
	// on day one without waiting 24h.
	a.RunDailyBackup()

	ticker := time.NewTicker(backupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.RunDailyBackup()
		}
	}
}

// RunDailyBackup produces today's backup if absent, then prunes old ones.
// Exported so tests and manual triggers can call it without the ticker.
// Errors are logged — no return value because the scheduler can't act on them.
func (a *App) RunDailyBackup() {
	if a.DataDir == "" || a.Store == nil {
		return
	}
	backupDir := filepath.Join(a.DataDir, backupDirName)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		a.Logger.Error().Err(err).Str("dir", backupDir).Msg("backup: mkdir failed")
		return
	}
	todayPath := filepath.Join(backupDir, backupPrefix+time.Now().UTC().Format("2006-01-02")+backupSuffix)
	if _, err := os.Stat(todayPath); err == nil {
		// Already have today's snapshot; just prune.
		a.pruneOldBackups(backupDir)
		return
	} else if !os.IsNotExist(err) {
		a.Logger.Error().Err(err).Str("path", todayPath).Msg("backup: stat failed")
		return
	}
	if err := a.Store.BackupTo(todayPath); err != nil {
		// VACUUM INTO can fail if the target exists (race with another
		// process) — surface the error and let the next tick retry.
		a.Logger.Error().Err(err).Str("path", todayPath).Msg("backup: VACUUM INTO failed")
		// Clean up any partial artefact so the next run's existence
		// check doesn't false-positive.
		_ = os.Remove(todayPath)
		return
	}
	a.Logger.Info().Str("path", todayPath).Msg("backup: snapshot written")
	a.pruneOldBackups(backupDir)
}

// pruneOldBackups keeps the most recent backupRetainDays entries and
// deletes the rest. Unrecognised files in the backup dir are ignored.
func (a *App) pruneOldBackups(backupDir string) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		a.Logger.Error().Err(err).Str("dir", backupDir).Msg("backup: read dir failed")
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, backupPrefix) || !strings.HasSuffix(n, backupSuffix) {
			continue
		}
		names = append(names, n)
	}
	if len(names) <= backupRetainDays {
		return
	}
	// Names encode the date (YYYY-MM-DD) so lexicographic sort == chronological.
	sort.Strings(names)
	for _, old := range names[:len(names)-backupRetainDays] {
		p := filepath.Join(backupDir, old)
		if err := os.Remove(p); err != nil {
			a.Logger.Warn().Err(err).Str("path", p).Msg("backup: prune failed")
			continue
		}
		a.Logger.Info().Str("path", p).Msg("backup: pruned")
	}
}

// backupPathForDate returns the canonical backup path for a given day.
// Exposed for tests; production code constructs the path inline.
func backupPathForDate(dir string, day time.Time) string {
	return filepath.Join(dir, fmt.Sprintf("%s%s%s", backupPrefix, day.UTC().Format("2006-01-02"), backupSuffix))
}
