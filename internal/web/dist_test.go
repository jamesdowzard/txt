package web

import (
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing"
)

// TestEmbedServesDistIndex verifies that the embedded static/dist/index.html
// is present and contains the Preact mount point. This is the contract
// between the Vite build (web/) and the Go server.
func TestEmbedServesDistIndex(t *testing.T) {
	dist, err := fs.Sub(staticFS, "static/dist")
	if err != nil {
		t.Fatalf("sub-fs static/dist: %v", err)
	}
	f, err := dist.Open("index.html")
	if err != nil {
		t.Fatalf("open dist/index.html: %v", err)
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read dist/index.html: %v", err)
	}
	if !strings.Contains(string(body), `id="app"`) {
		t.Fatalf("dist/index.html missing #app root, got:\n%s", body)
	}
}

// TestEmbedServesFavicon verifies Vite copied public/favicon.svg into dist/.
func TestEmbedServesFavicon(t *testing.T) {
	dist, err := fs.Sub(staticFS, "static/dist")
	if err != nil {
		t.Fatalf("sub-fs static/dist: %v", err)
	}
	f, err := dist.Open("favicon.svg")
	if err != nil {
		t.Fatalf("open dist/favicon.svg: %v", err)
	}
	f.Close()
}

// TestRootServesDistIndex asserts the HTTP mux serves dist/index.html at "/".
// This proves the embed directive + fs.Sub wiring round-trips through the
// handler, not just the embedded FS.
func TestRootServesDistIndex(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.server.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `id="app"`) {
		t.Fatalf("GET / returned wrong body (missing #app):\n%s", body)
	}
}

// TestRootServesFavicon asserts /favicon.svg resolves via dist/.
func TestRootServesFavicon(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.server.URL + "/favicon.svg")
	if err != nil {
		t.Fatalf("GET /favicon.svg: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
