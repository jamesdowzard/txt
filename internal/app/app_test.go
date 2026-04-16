package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewDemoUsesIsolatedTempDataDir(t *testing.T) {
	realDataDir := filepath.Join(t.TempDir(), "real-data")
	if err := os.MkdirAll(realDataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}

	t.Setenv("OPENMESSAGES_DATA_DIR", realDataDir)
	t.Setenv("OPENMESSAGES_DEMO", "1")

	a, err := New(zerolog.Nop())
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	if a.DataDir == realDataDir {
		t.Fatalf("DataDir = %q, want isolated temp dir", a.DataDir)
	}
	if a.tempDataDir == "" {
		t.Fatal("expected tempDataDir to be set in demo mode")
	}
	if filepath.Dir(a.SessionPath) != a.DataDir {
		t.Fatalf("SessionPath dir = %q, want %q", filepath.Dir(a.SessionPath), a.DataDir)
	}
	if filepath.Dir(a.WhatsAppSessionPath) != a.DataDir {
		t.Fatalf("WhatsAppSessionPath dir = %q, want %q", filepath.Dir(a.WhatsAppSessionPath), a.DataDir)
	}
	if _, err := os.Stat(filepath.Join(a.DataDir, "messages.db")); err != nil {
		t.Fatalf("expected demo db to exist: %v", err)
	}
	if count, err := a.Store.ConversationCount(""); err != nil {
		t.Fatalf("ConversationCount(): %v", err)
	} else if count == 0 {
		t.Fatal("expected seeded demo conversations")
	}
	if entries, err := os.ReadDir(realDataDir); err != nil {
		t.Fatalf("ReadDir(realDataDir): %v", err)
	} else if len(entries) != 0 {
		t.Fatalf("real data dir should stay untouched, found %d entries", len(entries))
	}

	demoDir := a.DataDir
	a.Close()

	if _, err := os.Stat(demoDir); !os.IsNotExist(err) {
		t.Fatalf("expected demo dir cleanup, stat err = %v", err)
	}
}

func TestNewMigratesLegacySessionFile(t *testing.T) {
	root := t.TempDir()
	activeDir := filepath.Join(root, "active")
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("MkdirAll legacy: %v", err)
	}
	want := []byte(`{"token":"legacy"}`)
	if err := os.WriteFile(filepath.Join(legacyDir, "session.json"), want, 0o600); err != nil {
		t.Fatalf("write legacy session: %v", err)
	}

	t.Setenv("OPENMESSAGES_DATA_DIR", activeDir)
	t.Setenv("OPENMESSAGES_LEGACY_DATA_DIR", legacyDir)
	t.Setenv("OPENMESSAGES_DEMO", "")

	a, err := New(zerolog.Nop())
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	defer a.Close()

	got, err := os.ReadFile(filepath.Join(activeDir, "session.json"))
	if err != nil {
		t.Fatalf("read active session: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("session.json contents = %q, want %q", got, want)
	}
}

func TestNewKeepsExistingSessionFile(t *testing.T) {
	root := t.TempDir()
	activeDir := filepath.Join(root, "active")
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(activeDir, 0o700); err != nil {
		t.Fatalf("MkdirAll active: %v", err)
	}
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("MkdirAll legacy: %v", err)
	}
	keep := []byte(`{"token":"active"}`)
	if err := os.WriteFile(filepath.Join(activeDir, "session.json"), keep, 0o600); err != nil {
		t.Fatalf("write active session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "session.json"), []byte(`{"token":"legacy"}`), 0o600); err != nil {
		t.Fatalf("write legacy session: %v", err)
	}

	t.Setenv("OPENMESSAGES_DATA_DIR", activeDir)
	t.Setenv("OPENMESSAGES_LEGACY_DATA_DIR", legacyDir)
	t.Setenv("OPENMESSAGES_DEMO", "")

	a, err := New(zerolog.Nop())
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	defer a.Close()

	got, err := os.ReadFile(filepath.Join(activeDir, "session.json"))
	if err != nil {
		t.Fatalf("read active session: %v", err)
	}
	if string(got) != string(keep) {
		t.Fatalf("active session.json was overwritten; got %q, want %q", got, keep)
	}
}

func TestNewMigrationNoOpWithoutLegacySession(t *testing.T) {
	root := t.TempDir()
	activeDir := filepath.Join(root, "active")
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("MkdirAll legacy: %v", err)
	}

	t.Setenv("OPENMESSAGES_DATA_DIR", activeDir)
	t.Setenv("OPENMESSAGES_LEGACY_DATA_DIR", legacyDir)
	t.Setenv("OPENMESSAGES_DEMO", "")

	a, err := New(zerolog.Nop())
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	defer a.Close()

	if _, err := os.Stat(filepath.Join(activeDir, "session.json")); !os.IsNotExist(err) {
		t.Fatalf("did not expect a session.json to be created; err = %v", err)
	}
}

func TestDemoModeEnvParsing(t *testing.T) {
	t.Run("disabled when empty", func(t *testing.T) {
		t.Setenv("OPENMESSAGES_DEMO", "")
		if DemoMode() {
			t.Fatal("expected demo mode off")
		}
	})

	t.Run("enabled for truthy values", func(t *testing.T) {
		for _, value := range []string{"1", "true", "yes", "demo"} {
			t.Run(value, func(t *testing.T) {
				t.Setenv("OPENMESSAGES_DEMO", value)
				if !DemoMode() {
					t.Fatalf("expected demo mode on for %q", value)
				}
			})
		}
	})

	t.Run("disabled for explicit false values", func(t *testing.T) {
		for _, value := range []string{"0", "false", "off", "no"} {
			t.Run(value, func(t *testing.T) {
				t.Setenv("OPENMESSAGES_DEMO", value)
				if DemoMode() {
					t.Fatalf("expected demo mode off for %q", value)
				}
			})
		}
	})
}
