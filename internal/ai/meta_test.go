package ai

import "testing"

// TestParseSessionMeta covers the end-of-session label+code extraction: clean
// JSON, code-fenced and prose-wrapped replies, difficulty normalisation, and
// garbage input.
func TestParseSessionMeta(t *testing.T) {
	t.Run("clean json with code", func(t *testing.T) {
		got, err := parseSessionMeta(`{"title":"Two Sum","difficulty":"easy","code":"def f(): pass"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "Two Sum" || got.Difficulty != "Easy" || got.Code != "def f(): pass" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("code-fenced and prose-wrapped", func(t *testing.T) {
		raw := "Here you go:\n```json\n{\"title\":\"LRU Cache\",\"difficulty\":\"Medium\",\"code\":\"x=1\"}\n```"
		got, err := parseSessionMeta(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "LRU Cache" || got.Difficulty != "Medium" || got.Code != "x=1" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("unknown difficulty and empty code", func(t *testing.T) {
		got, err := parseSessionMeta(`{"title":"Mystery","difficulty":"insane","code":""}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Difficulty != "" {
			t.Errorf("difficulty = %q, want empty", got.Difficulty)
		}
		if got.Code != "" {
			t.Errorf("code = %q, want empty", got.Code)
		}
	})

	t.Run("garbage errors", func(t *testing.T) {
		if _, err := parseSessionMeta("not json at all"); err == nil {
			t.Error("expected error, got nil")
		}
	})
}
