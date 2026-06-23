// Package googletts wraps Google Cloud Text-to-Speech and Speech-to-Text. It
// mirrors internal/voice (ElevenLabs) so the two are interchangeable behind
// app.go's ttsProvider/sttProvider interfaces: it exposes Synthesize, ListVoices,
// and Transcribe, and keeps the API key in the Go backend. Google supports plain
// API-key auth, so no OAuth/service account is needed — the key is stored and
// passed exactly like the other providers'.
//
// Google TTS is ~10x cheaper than ElevenLabs (Neural2 $16/1M chars, with a free
// monthly tier), which is why it's the default voice. STT (Speech-to-Text) lets a
// Google-only user use the mic without an ElevenLabs key.
//
// (Despite the package name, it now wraps two Google APIs; a rename to
// internal/google is a possible future tidy-up.)
package googletts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"ai-interviewer/internal/models"
)

const (
	synthURL  = "https://texttospeech.googleapis.com/v1/text:synthesize"
	voicesURL = "https://texttospeech.googleapis.com/v1/voices"
	sttURL    = "https://speech.googleapis.com/v1/speech:recognize"

	// sttSampleRate matches the frontend's WAV capture (16 kHz mono LINEAR16).
	sttSampleRate = 16000

	// audioEncoding asks Google for MP3 so the bytes match the ElevenLabs path
	// (raw MP3 → base64 at the Wails boundary → played via Web Audio).
	audioEncoding = "MP3"

	httpTimeout = 60 * time.Second
	// voicesCacheTTL bounds how long ListVoices serves the cached catalog before
	// re-fetching (mirrors voice.Client — the list changes rarely).
	voicesCacheTTL = time.Hour
)

// Client calls the Google Cloud Text-to-Speech API.
type Client struct {
	apiKey     string
	httpClient *http.Client

	// Cached voice catalog, guarded by mu since Wails may call bound methods
	// from different goroutines.
	mu           sync.Mutex
	cachedVoices []models.Voice
	cachedAt     time.Time
}

// NewClient creates a Google TTS client with the given API key.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
	}
}

// Synthesize converts text to speech with the given voice and returns the raw
// MP3 bytes. voiceID is a Google voice name (e.g. "en-US-Neural2-F"); the
// language code is derived from it. The caller (app.go) base64-encodes the
// result for the Wails boundary.
func (c *Client) Synthesize(ctx context.Context, voiceID, text string) ([]byte, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("googletts: API key is not configured")
	}
	if voiceID == "" {
		return nil, fmt.Errorf("googletts: no voice selected")
	}

	payload := map[string]any{
		"input": map[string]any{"text": text},
		"voice": map[string]any{
			"languageCode": langCodeFromVoice(voiceID),
			"name":         voiceID,
		},
		"audioConfig": map[string]any{"audioEncoding": audioEncoding},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("googletts: marshal TTS request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.withKey(synthURL), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("googletts: build TTS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("googletts: TTS http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("googletts: read TTS response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("googletts: Google TTS returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Google returns JSON with base64-encoded audio (unlike ElevenLabs' raw MP3).
	var result struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("googletts: parse TTS response: %w", err)
	}
	audio, err := base64.StdEncoding.DecodeString(result.AudioContent)
	if err != nil {
		return nil, fmt.Errorf("googletts: decode audio: %w", err)
	}
	return audio, nil
}

// Transcribe sends recorded audio to Google Cloud Speech-to-Text and returns the
// transcribed text. The frontend sends 16 kHz mono 16-bit PCM WAV (LINEAR16), so
// the config is fixed to match. mimeType is accepted for parity with the
// ElevenLabs client (sttProvider interface) but isn't otherwise needed.
func (c *Client) Transcribe(ctx context.Context, audio []byte, mimeType string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("googletts: API key is not configured")
	}

	payload := map[string]any{
		"config": map[string]any{
			"encoding":        "LINEAR16",
			"sampleRateHertz": sttSampleRate,
			"languageCode":    "en-US",
			// latest_long is tuned for spontaneous, conversational speech (interview
			// answers, tradeoff discussions), not just brief commands; useEnhanced opts
			// into the higher-accuracy variant. Both improve correctness over the
			// generic default. (Sync recognize still caps at ~60s per clip.)
			"model":                      "latest_long",
			"useEnhanced":                true,
			"enableAutomaticPunctuation": true,
		},
		"audio": map[string]any{
			"content": base64.StdEncoding.EncodeToString(audio),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("googletts: marshal STT request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.withKey(sttURL), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("googletts: build STT request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("googletts: STT http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("googletts: read STT response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("googletts: Google STT returned %d: %s", resp.StatusCode, string(respBody))
	}

	return parseSTTResponse(respBody)
}

// parseSTTResponse extracts and joins the transcript fragments from a Google STT
// recognize response. Split out as a pure function so it can be unit-tested.
func parseSTTResponse(body []byte) (string, error) {
	var result struct {
		Results []struct {
			Alternatives []struct {
				Transcript string `json:"transcript"`
			} `json:"alternatives"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("googletts: parse STT response: %w", err)
	}

	var parts []string
	for _, r := range result.Results {
		if len(r.Alternatives) > 0 {
			if t := strings.TrimSpace(r.Alternatives[0].Transcript); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, " ")), nil
}

// ListVoices returns the account's English voices for the picker UI, filtered to
// the high-quality families and sorted by name. Results are cached for
// voicesCacheTTL. Google's catalog has no preview URLs, so Voice.PreviewURL is
// left empty — the picker synthesizes a sample on demand instead.
func (c *Client) ListVoices(ctx context.Context) ([]models.Voice, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("googletts: API key is not configured")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.cachedVoices) > 0 && time.Since(c.cachedAt) < voicesCacheTTL {
		return c.cachedVoices, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.withKey(voicesURL), nil)
	if err != nil {
		return nil, fmt.Errorf("googletts: build voices request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("googletts: voices http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("googletts: read voices response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("googletts: Google voices returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Voices []struct {
			Name           string   `json:"name"`
			SSMLGender     string   `json:"ssmlGender"`
			LanguageCodes  []string `json:"languageCodes"`
			NaturalSampleR int      `json:"naturalSampleRateHertz"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("googletts: parse voices response: %w", err)
	}

	out := make([]models.Voice, 0, len(result.Voices))
	for _, v := range result.Voices {
		if !isEnglish(v.LanguageCodes) {
			continue
		}
		family := voiceFamily(v.Name)
		if family == "" {
			continue // skip Standard/Polyglot/News and other lower-tier families
		}
		out = append(out, models.Voice{
			ID:       v.Name,
			Name:     v.Name,
			Category: family + " · " + prettyGender(v.SSMLGender),
			// PreviewURL intentionally empty — Google provides none.
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	c.cachedVoices = out
	c.cachedAt = time.Now()
	return out, nil
}

// withKey appends the API key as a query parameter (Google's API-key auth).
func (c *Client) withKey(endpoint string) string {
	return endpoint + "?key=" + url.QueryEscape(c.apiKey)
}

// langCodeFromVoice derives the BCP-47 language code from a Google voice name by
// taking its first two hyphen-separated segments ("en-US-Neural2-F" → "en-US").
// Falls back to "en-US" for unexpected shapes.
func langCodeFromVoice(name string) string {
	parts := strings.Split(name, "-")
	if len(parts) >= 2 {
		return parts[0] + "-" + parts[1]
	}
	return "en-US"
}

// voiceFamily classifies a Google voice name into a display family, or "" if it
// isn't one of the high-quality tiers we surface in the picker.
func voiceFamily(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "chirp3-hd"):
		return "Chirp3 HD"
	case strings.Contains(n, "chirp-hd"):
		return "Chirp HD"
	case strings.Contains(n, "neural2"):
		return "Neural2"
	case strings.Contains(n, "studio"):
		return "Studio"
	case strings.Contains(n, "wavenet"):
		return "WaveNet"
	default:
		return ""
	}
}

// isEnglish reports whether any of the voice's language codes is an English locale.
func isEnglish(codes []string) bool {
	for _, c := range codes {
		if strings.HasPrefix(strings.ToLower(c), "en-") {
			return true
		}
	}
	return false
}

// prettyGender title-cases Google's SSML gender for display.
func prettyGender(g string) string {
	switch strings.ToUpper(g) {
	case "FEMALE":
		return "Female"
	case "MALE":
		return "Male"
	default:
		return "Neutral"
	}
}
