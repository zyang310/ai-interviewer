// Package openrouter provisions per-tester OpenRouter API keys. Each tester
// gets their own key with a small USD credit cap, so a leaked key costs at most
// that cap and revocation is per-tester (by the key's hash). Request/error
// idiom mirrors the app's internal/ai client.
package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// keysURL is OpenRouter's provisioning endpoint. It must NOT have a
	// trailing slash: POSTing to ".../keys/" returns 404 (GET 301-redirects,
	// which Go would silently downgrade to a GET and turn a mint into a key
	// list). Verified against the live API in Phase 3.6. DELETE appends
	// "/" + the key hash.
	keysURL = "https://openrouter.ai/api/v1/keys"
	// httpTimeout bounds a mint/delete call, which sits on the /verify path.
	httpTimeout = 30 * time.Second
)

// KeyMinter provisions a capped API key for a tester. *Client mints real keys;
// StubMinter fakes them for local development. The server depends only on this
// interface, so it is oblivious to which is wired.
type KeyMinter interface {
	// Mint creates a key named name with a limitUSD credit cap and returns the
	// usable key plus its management hash. OpenRouter returns the usable key
	// only here, at creation — callers must persist it.
	Mint(ctx context.Context, name string, limitUSD float64) (key, hash string, err error)
}

// Client calls the OpenRouter provisioning API with a management ("provisioning")
// key.
type Client struct {
	provisioningKey string
	// baseURL is the keys endpoint; a field only so tests can point it at an
	// httptest server and assert the exact request shape (the wrong shape cost
	// a 404 in 3.6). Production always uses keysURL.
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a provisioning client. An empty key makes every call error;
// main.go selects StubMinter instead when the key is unset.
func NewClient(provisioningKey string) *Client {
	return &Client{
		provisioningKey: provisioningKey,
		baseURL:         keysURL,
		httpClient:      &http.Client{Timeout: httpTimeout},
	}
}

var _ KeyMinter = (*Client)(nil)

// Mint provisions a new capped key. The raw key is returned once and never
// again (later reads mask it), so the caller must store it to re-serve on the
// app's launch refresh.
func (c *Client) Mint(ctx context.Context, name string, limitUSD float64) (string, string, error) {
	if c.provisioningKey == "" {
		return "", "", fmt.Errorf("openrouter: provisioning key is not configured")
	}

	reqBody, err := json.Marshal(map[string]any{"name": name, "limit": limitUSD})
	if err != nil {
		return "", "", fmt.Errorf("openrouter: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", "", fmt.Errorf("openrouter: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.provisioningKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("openrouter: http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("openrouter: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("openrouter: mint returned %d: %s", resp.StatusCode, string(body))
	}

	// The usable key is top-level "key"; "data.hash" is the management handle.
	var parsed struct {
		Key  string `json:"key"`
		Data struct {
			Hash string `json:"hash"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", "", fmt.Errorf("openrouter: decode response: %w", err)
	}
	if parsed.Key == "" {
		return "", "", fmt.Errorf("openrouter: mint response contained no key")
	}
	return parsed.Key, parsed.Data.Hash, nil
}

// Delete revokes a key by its hash. It is an operations/rotation hook (revoke a
// leaked tester's key, or rotate on test-phase end), not on any request path —
// see the ops notes in README.md.
func (c *Client) Delete(ctx context.Context, hash string) error {
	if c.provisioningKey == "" {
		return fmt.Errorf("openrouter: provisioning key is not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/"+hash, nil)
	if err != nil {
		return fmt.Errorf("openrouter: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.provisioningKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("openrouter: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openrouter: delete returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
