package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"

	"github.com/maxghenis/openmessage/internal/client"
	"github.com/maxghenis/openmessage/internal/db"
)

// BackfillPhase represents the current phase of a deep backfill.
type BackfillPhase string

const (
	BackfillPhaseIdle     BackfillPhase = ""
	BackfillPhaseFolders  BackfillPhase = "folders"
	BackfillPhaseMessages BackfillPhase = "messages"
	BackfillPhaseContacts BackfillPhase = "contacts"
	BackfillPhaseDone     BackfillPhase = "done"
)

const maxErrorDetails = 100

// BackfillProgress tracks the current state of a deep backfill operation.
type BackfillProgress struct {
	mu                 sync.Mutex
	Running            bool          `json:"running"`
	Phase              BackfillPhase `json:"phase"`
	FoldersScanned     int           `json:"folders_scanned"`
	ConversationsFound int           `json:"conversations_found"`
	MessagesFound      int           `json:"messages_found"`
	ContactsChecked    int           `json:"contacts_checked"`
	Errors             int           `json:"errors"`
	ErrorDetails       []string      `json:"error_details,omitempty"`
}

// reset clears all fields for a fresh backfill run.
func (p *BackfillProgress) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Running = true
	p.Phase = BackfillPhaseFolders
	p.FoldersScanned = 0
	p.ConversationsFound = 0
	p.MessagesFound = 0
	p.ContactsChecked = 0
	p.Errors = 0
	p.ErrorDetails = nil
}

// setPhase updates the current phase.
func (p *BackfillProgress) setPhase(phase BackfillPhase) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Phase = phase
}

// finish marks the backfill as complete.
func (p *BackfillProgress) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Running = false
	p.Phase = BackfillPhaseDone
}

// addError increments the error count and optionally records a detail string.
func (p *BackfillProgress) addError(detail string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Errors++
	if detail != "" && len(p.ErrorDetails) < maxErrorDetails {
		p.ErrorDetails = append(p.ErrorDetails, detail)
	}
}

// add increments the given counters atomically.
func (p *BackfillProgress) add(conversations, messages, contacts, folders int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ConversationsFound += conversations
	p.MessagesFound += messages
	p.ContactsChecked += contacts
	p.FoldersScanned += folders
}

func (p *BackfillProgress) snapshot() BackfillProgress {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := *p
	if len(p.ErrorDetails) > 0 {
		cp.ErrorDetails = append([]string(nil), p.ErrorDetails...)
	}
	return cp
}

type App struct {
	Client       *client.Client
	Store        *db.Store
	EventHandler *client.EventHandler
	Logger       zerolog.Logger
	DataDir      string
	SessionPath  string
	Connected    atomic.Bool

	// gmClient is used by backfill methods. If nil, it's derived from Client.GM.
	// Set this field directly in tests to inject a mock.
	gmClient         GMClient
	BackfillProgress BackfillProgress
}

func DefaultDataDir() string {
	if dir := os.Getenv("OPENMESSAGES_DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "openmessage")
}

func New(logger zerolog.Logger) (*App, error) {
	dataDir := DefaultDataDir()
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// In demo mode, use a temp DB so we never touch real data
	dbPath := filepath.Join(dataDir, "messages.db")
	if os.Getenv("OPENMESSAGES_DEMO") != "" {
		tmpDir, err := os.MkdirTemp("", "openmessage-demo-*")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
		dbPath = filepath.Join(tmpDir, "demo.db")
	}

	store, err := db.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Seed demo data
	if os.Getenv("OPENMESSAGES_DEMO") != "" {
		if err := store.SeedDemo(); err != nil {
			store.Close()
			return nil, fmt.Errorf("seed demo data: %w", err)
		}
		logger.Info().Str("db", dbPath).Msg("Demo mode — seeded fake data")
	}

	sessionPath := filepath.Join(dataDir, "session.json")

	app := &App{
		Store:       store,
		Logger:      logger,
		DataDir:     dataDir,
		SessionPath: sessionPath,
	}
	return app, nil
}

func (a *App) LoadAndConnect() error {
	sessionData, err := client.LoadSession(a.SessionPath)
	if err != nil {
		return fmt.Errorf("load session (run 'gmessages-mcp pair' first): %w", err)
	}

	cli, err := client.NewFromSession(sessionData, a.Logger)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	a.Client = cli

	a.EventHandler = &client.EventHandler{
		Store:       a.Store,
		Logger:      a.Logger,
		SessionPath: a.SessionPath,
		Client:      cli,
		OnDisconnect: func() {
			a.Connected.Store(false)
			a.Logger.Warn().Msg("Disconnected from Google Messages")
		},
	}
	cli.GM.SetEventHandler(a.EventHandler.Handle)

	if err := cli.GM.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	a.Connected.Store(true)
	a.Logger.Info().Msg("Connected to Google Messages")
	return nil
}

// Unpair deletes the session file so the app can re-pair.
func (a *App) Unpair() error {
	a.Connected.Store(false)
	if a.Client != nil {
		a.Client.GM.Disconnect()
		a.Client = nil
	}
	if err := os.Remove(a.SessionPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove session: %w", err)
	}
	a.Logger.Info().Msg("Unpaired — session deleted")
	return nil
}

// getGMClient returns the GMClient for backfill operations.
// Uses the injected mock if set, otherwise wraps the real libgm client.
func (a *App) getGMClient() GMClient {
	if a.gmClient != nil {
		return a.gmClient
	}
	if a.Client != nil {
		return newRealGMClient(a.Client.GM)
	}
	return nil
}

// GetBackfillProgress returns a snapshot of the current backfill progress.
func (a *App) GetBackfillProgress() BackfillProgress {
	return a.BackfillProgress.snapshot()
}

func (a *App) Close() {
	if a.Client != nil {
		a.Client.GM.Disconnect()
	}
	if a.Store != nil {
		a.Store.Close()
	}
}
