package cmd

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestHTTPServerSurvivesIndependently(t *testing.T) {
	// Mirrors the RunServe architecture: HTTP server in a goroutine
	// stays alive even when the "main" blocking call returns.

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go http.Serve(ln, mux)

	// Simulate MCP stdio returning immediately (like EOF from /dev/null)
	done := make(chan struct{})
	go func() {
		// This returns instantly, simulating ServeStdio on closed stdin
		close(done)
	}()
	<-done

	// HTTP server should still be alive after "MCP" exits
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + ln.Addr().String() + "/healthz")
	if err != nil {
		t.Fatalf("HTTP server not responding after MCP exit: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("got %q, want %q", body, "ok")
	}
}

func TestMacOSNotificationsEnabled(t *testing.T) {
	originalGOOS := runtimeGOOS
	t.Cleanup(func() {
		runtimeGOOS = originalGOOS
	})

	t.Run("defaults on for interactive darwin", func(t *testing.T) {
		runtimeGOOS = func() string { return "darwin" }
		t.Setenv("OPENMESSAGES_MACOS_NOTIFICATIONS", "")
		if !macOSNotificationsEnabled(true) {
			t.Fatal("expected notifications enabled for interactive macOS serve")
		}
	})

	t.Run("defaults off for stdio launches", func(t *testing.T) {
		runtimeGOOS = func() string { return "darwin" }
		t.Setenv("OPENMESSAGES_MACOS_NOTIFICATIONS", "")
		if macOSNotificationsEnabled(false) {
			t.Fatal("expected notifications disabled for non-interactive launch")
		}
	})

	t.Run("env can force on outside darwin", func(t *testing.T) {
		runtimeGOOS = func() string { return "linux" }
		t.Setenv("OPENMESSAGES_MACOS_NOTIFICATIONS", "true")
		if !macOSNotificationsEnabled(false) {
			t.Fatal("expected env override to force notifications on")
		}
	})

	t.Run("env can force off on darwin", func(t *testing.T) {
		runtimeGOOS = func() string { return "darwin" }
		t.Setenv("OPENMESSAGES_MACOS_NOTIFICATIONS", "0")
		if macOSNotificationsEnabled(true) {
			t.Fatal("expected env override to disable notifications")
		}
	})
}
