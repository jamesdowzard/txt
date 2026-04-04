package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestLinkPreviewServiceFetchParsesMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head>
    <title>Ignored Title</title>
    <meta property="og:title" content="Preview Title">
    <meta property="og:description" content="Preview Description">
    <meta property="og:image" content="/card.png">
    <meta property="og:site_name" content="Preview Site">
  </head>
  <body>Hello</body>
</html>`))
	}))
	defer srv.Close()

	service := NewLinkPreviewService(zerolog.Nop())
	service.allowPrivateHosts = true

	preview, err := service.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Title != "Preview Title" {
		t.Fatalf("got title %q", preview.Title)
	}
	if preview.Description != "Preview Description" {
		t.Fatalf("got description %q", preview.Description)
	}
	if preview.SiteName != "Preview Site" {
		t.Fatalf("got site name %q", preview.SiteName)
	}
	if preview.ImageURL != srv.URL+"/card.png" {
		t.Fatalf("got image URL %q", preview.ImageURL)
	}
}

func TestLinkPreviewServiceBlocksPrivateHostsByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	service := NewLinkPreviewService(zerolog.Nop())
	_, err := service.Fetch(context.Background(), srv.URL)
	if !errors.Is(err, ErrBlockedLinkPreviewURL) {
		t.Fatalf("got err %v, want ErrBlockedLinkPreviewURL", err)
	}
}
