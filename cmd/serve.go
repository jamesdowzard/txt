package cmd

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog"

	"github.com/maxghenis/openmessage/internal/app"
	"github.com/maxghenis/openmessage/internal/importer"
	"github.com/maxghenis/openmessage/internal/tools"
	"github.com/maxghenis/openmessage/internal/web"
)

func RunServe(logger zerolog.Logger) error {
	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	// Connect to Google Messages (skip in demo mode)
	if os.Getenv("OPENMESSAGES_DEMO") == "" {
		if err := a.LoadAndConnect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}

		// Backfill existing conversations and messages
		go func() {
			if err := a.Backfill(); err != nil {
				logger.Warn().Err(err).Msg("Backfill failed")
			}
		}()
	} else {
		logger.Info().Msg("Demo mode — skipping phone connection")
	}

	// Sync WhatsApp and iMessage periodically (every 30s, incremental)
	syncLocalPlatforms := func() {
		wa := &importer.WhatsAppNative{MyName: "Max"}
		if result, err := wa.ImportFromDB(a.Store); err != nil {
			logger.Warn().Err(err).Msg("WhatsApp sync failed")
		} else if result.MessagesImported > 0 {
			logger.Info().
				Int("messages", result.MessagesImported).
				Int("conversations", result.ConversationsCreated).
				Msg("WhatsApp sync complete")
		}

		im := &importer.IMessage{MyName: "Max"}
		if result, err := im.ImportFromDB(a.Store); err != nil {
			logger.Warn().Err(err).Msg("iMessage sync failed")
		} else if result.MessagesImported > 0 {
			logger.Info().
				Int("messages", result.MessagesImported).
				Int("conversations", result.ConversationsCreated).
				Msg("iMessage sync complete")
		}
	}

	// Run once immediately, then every 30 seconds
	go func() {
		syncLocalPlatforms()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			syncLocalPlatforms()
		}
	}()

	// Start web server
	port := os.Getenv("OPENMESSAGES_PORT")
	if port == "" {
		port = "7007"
	}

	// Create MCP server
	mcpSrv := mcpserver.NewMCPServer(
		"openmessage",
		"0.1.0",
		mcpserver.WithToolCapabilities(true),
	)
	tools.Register(mcpSrv, a)

	// Create SSE transport for MCP, mounted at /mcp/
	sseSrv := mcpserver.NewSSEServer(mcpSrv,
		mcpserver.WithBaseURL(fmt.Sprintf("http://localhost:%s", port)),
		mcpserver.WithStaticBasePath("/mcp"),
	)

	httpHandler := web.APIHandlerWithOptions(a.Store, a.Client, logger, sseSrv, web.APIOptions{
		IsConnected:    func() bool { return a.Connected.Load() },
		Unpair:         a.Unpair,
		OnDeepBackfill: a.DeepBackfill,
		BackfillStatus: func() any { return a.GetBackfillProgress() },
		BackfillPhone:  a.BackfillConversationByPhone,
	})
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", port, err)
	}
	go func() {
		logger.Info().Str("port", port).Msg("Web UI available at http://localhost:" + port)
		logger.Info().Str("port", port).Msg("MCP SSE available at http://localhost:" + port + "/mcp/sse")
		if err := http.Serve(ln, httpHandler); err != nil {
			logger.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Block until signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Info().Msg("Shutting down")
	return nil
}

// LogLevel returns the zerolog level based on OPENMESSAGES_LOG_LEVEL env var.
func LogLevel() zerolog.Level {
	switch os.Getenv("OPENMESSAGES_LOG_LEVEL") {
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "trace":
		return zerolog.TraceLevel
	default:
		return zerolog.InfoLevel
	}
}
