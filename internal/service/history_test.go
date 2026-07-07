package service

import (
	"encoding/json"
	"strings"
	"testing"

	"mogi/internal/models"
)

// historyWith builds a History service over fakes; activeID defaults to idle.
func historyWith(st *fakeStore, aiClient AI, activeID string) *History {
	p := NewProviders()
	p.ai = aiClient
	return NewHistory(st, p, func() string { return activeID })
}

// TestDeleteGuardsActiveSession verifies the running session is protected and
// everything else deletes normally.
func TestDeleteGuardsActiveSession(t *testing.T) {
	deleted := ""
	st := &fakeStore{deleteSession: func(id string) error {
		deleted = id
		return nil
	}}
	h := historyWith(st, nil, "live-id")

	if err := h.Delete("live-id"); err == nil {
		t.Error("deleting the active session should error")
	}
	if deleted != "" {
		t.Error("the store must not be touched when the guard fires")
	}
	if err := h.Delete("old-id"); err != nil || deleted != "old-id" {
		t.Errorf("Delete(old-id) err=%v deleted=%q, want store delete", err, deleted)
	}
}

// TestDebriefRequiresAI verifies the no-provider guard.
func TestDebriefRequiresAI(t *testing.T) {
	if _, err := historyWith(&fakeStore{}, nil, "").Debrief("sid"); err == nil {
		t.Error("Debrief() without an AI provider should error")
	}
}

// TestDebriefCacheHit returns the stored scorecard without any AI call.
func TestDebriefCacheHit(t *testing.T) {
	cached, _ := json.Marshal(models.Debrief{Verdict: "Hire", Summary: "solid"})
	st := &fakeStore{getSessionDebrief: func(string) (string, error) { return string(cached), nil }}
	aiCalled := false
	aiClient := &fakeAI{generateDebrief: func(string, string, string) (models.Debrief, error) {
		aiCalled = true
		return models.Debrief{}, nil
	}}

	d, err := historyWith(st, aiClient, "").Debrief("sid")
	if err != nil {
		t.Fatalf("Debrief() error: %v", err)
	}
	if d.Verdict != "Hire" || d.Summary != "solid" {
		t.Errorf("Debrief() = %+v, want the cached scorecard", d)
	}
	if aiCalled {
		t.Error("a cache hit must spend zero tokens")
	}
}

// TestDebriefGeneratesAndCaches walks the full generation path: corrupt cache
// falls through, the transcript + final code reach the AI with the session's
// own model, and the result is persisted.
func TestDebriefGeneratesAndCaches(t *testing.T) {
	saved := ""
	st := &fakeStore{
		getSessionDebrief: func(string) (string, error) { return "{not json", nil }, // corrupt → regenerate
		getSession: func(id string) (models.Session, error) {
			return models.Session{ID: id, Model: "session-model"}, nil
		},
		getMessages: func(string) ([]models.Message, error) {
			return []models.Message{{Role: "user", Content: "q"}, {Role: "assistant", Content: "a"}}, nil
		},
		getSessionFinalCode: func(string) (string, error) { return "def solve(): pass", nil },
		saveSessionDebrief:  func(_, debrief string) error { saved = debrief; return nil },
	}
	var gotModel, gotTranscript, gotCode string
	aiClient := &fakeAI{generateDebrief: func(model, transcript, finalCode string) (models.Debrief, error) {
		gotModel, gotTranscript, gotCode = model, transcript, finalCode
		return models.Debrief{Verdict: "Lean hire"}, nil
	}}

	d, err := historyWith(st, aiClient, "").Debrief("sid")
	if err != nil {
		t.Fatalf("Debrief() error: %v", err)
	}
	if d.Verdict != "Lean hire" {
		t.Errorf("Debrief() = %+v, want the generated scorecard", d)
	}
	if gotModel != "session-model" {
		t.Errorf("generated with model %q, want the session's own model", gotModel)
	}
	if !strings.Contains(gotTranscript, "Candidate: q") || !strings.Contains(gotTranscript, "Interviewer: a") {
		t.Errorf("transcript %q missing speaker-labelled turns", gotTranscript)
	}
	if gotCode != "def solve(): pass" {
		t.Errorf("final code %q not passed through", gotCode)
	}
	var persisted models.Debrief
	if err := json.Unmarshal([]byte(saved), &persisted); err != nil || persisted.Verdict != "Lean hire" {
		t.Errorf("cached debrief %q should round-trip the scorecard", saved)
	}
}

// TestDebriefTooShort refuses sessions with fewer than two turns.
func TestDebriefTooShort(t *testing.T) {
	st := &fakeStore{getMessages: func(string) ([]models.Message, error) {
		return []models.Message{{Role: "user", Content: "q"}}, nil
	}}
	if _, err := historyWith(st, &fakeAI{}, "").Debrief("sid"); err == nil || !strings.Contains(err.Error(), "too short") {
		t.Errorf("Debrief() = %v, want too-short error", err)
	}
}
