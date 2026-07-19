package openrouter

// These tests pin the exact HTTP shape of the provisioning calls. They exist
// because a wrong URL shape is invisible in code review but fatal in
// production: the endpoint was originally written with a trailing slash, and
// OpenRouter answers POST ".../keys/" with a 404, so every activation failed
// at mint time (found in Phase 3.6, against the live API).

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient points a client at a stub server, mirroring how NewClient
// wires the real endpoint.
func newTestClient(srv *httptest.Server) *Client {
	c := NewClient("test-provisioning-key")
	c.baseURL = srv.URL + "/api/v1/keys"
	return c
}

func TestMintRequestShape(t *testing.T) {
	var gotPath, gotMethod, gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotAuth = r.URL.Path, r.Method, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"key":"sk-or-v1-real","data":{"hash":"abc123"}}`))
	}))
	defer srv.Close()

	key, hash, err := newTestClient(srv).Mint(context.Background(), "tester@example.com", 3)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	// The regression this file exists for: no trailing slash.
	if gotPath != "/api/v1/keys" {
		t.Errorf("path = %q, want %q (a trailing slash 404s on the real API)", gotPath, "/api/v1/keys")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotAuth != "Bearer test-provisioning-key" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotBody["name"] != "tester@example.com" {
		t.Errorf("body name = %v", gotBody["name"])
	}
	if gotBody["limit"] != float64(3) {
		t.Errorf("body limit = %v, want 3 (the per-tester USD cap)", gotBody["limit"])
	}
	if key != "sk-or-v1-real" || hash != "abc123" {
		t.Errorf("Mint returned key=%q hash=%q", key, hash)
	}
}

func TestDeleteRequestShape(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := newTestClient(srv).Delete(context.Background(), "abc123"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotPath != "/api/v1/keys/abc123" {
		t.Errorf("path = %q, want /api/v1/keys/abc123 (hash appended after a separator)", gotPath)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
}

func TestMintSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"Not Found","code":404}}`))
	}))
	defer srv.Close()

	if _, _, err := newTestClient(srv).Mint(context.Background(), "x@y.com", 3); err == nil {
		t.Fatal("Mint returned no error on a 404")
	}
}
