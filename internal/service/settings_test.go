package service

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"ai-interviewer/internal/hotkey"
	"ai-interviewer/internal/models"
)

// settingsWith builds a Settings service over fakes, returning the fakes for
// assertions.
func settingsWith(st *fakeStore) (*Settings, *Providers, *fakeScreen, *fakeHotkey) {
	p := NewProviders()
	screen := &fakeScreen{}
	hk := &fakeHotkey{}
	return NewSettings(st, p, screen, hk), p, screen, hk
}

// TestUpdatePropagates verifies the business invariant: saving preferences
// retargets the capturer and re-applies the hotkey in one call.
func TestUpdatePropagates(t *testing.T) {
	var saved models.Preferences
	st := &fakeStore{
		savePreferences: func(p models.Preferences) error { saved = p; return nil },
		// ApplyHotkey re-reads the store, so echo back what was saved.
		getPreferences: func() (models.Preferences, error) { return saved, nil },
	}
	s, _, screen, hk := settingsWith(st)

	prefs := models.Preferences{
		CaptureDisplay: 1, RegionX: 0.1, RegionY: 0.2, RegionW: 0.3, RegionH: 0.4,
		PushToTalkEnabled: true, PushToTalkKey: "F8",
	}
	if err := s.Update(context.Background(), prefs); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if saved.CaptureDisplay != 1 {
		t.Error("preferences were not saved")
	}
	if len(screen.regions) != 1 || screen.regions[0] != [5]float64{1, 0.1, 0.2, 0.3, 0.4} {
		t.Errorf("capturer regions = %v, want the new region applied", screen.regions)
	}
	if len(hk.applies) != 1 || !hk.applies[0].enabled || hk.applies[0].spec.String() != "F8" {
		t.Errorf("hotkey applies = %+v, want enabled with the saved key", hk.applies)
	}
}

// TestApplyHotkeyFallsBackToDefault verifies an unparseable saved key degrades
// to the default spec instead of leaving the hook unbound.
func TestApplyHotkeyFallsBackToDefault(t *testing.T) {
	st := &fakeStore{getPreferences: func() (models.Preferences, error) {
		return models.Preferences{PushToTalkEnabled: true, PushToTalkKey: "NotAKey+Nope"}, nil
	}}
	s, _, _, hk := settingsWith(st)

	s.ApplyHotkey(context.Background())
	if len(hk.applies) != 1 || hk.applies[0].spec.String() != hotkey.DefaultSpec {
		t.Errorf("hotkey applies = %+v, want the default spec fallback", hk.applies)
	}
}

// TestAPIKeysFlipRegistry verifies key writes activate/deactivate the live
// client slots, and that a failed store write leaves the registry untouched.
func TestAPIKeysFlipRegistry(t *testing.T) {
	s, p, _, _ := settingsWith(&fakeStore{})

	if err := s.SetAPIKey("openrouter", "k"); err != nil || p.AI() == nil {
		t.Errorf("SetAPIKey: err=%v, want a live AI client", err)
	}
	if err := s.DeleteAPIKey("openrouter"); err != nil || p.AI() != nil {
		t.Errorf("DeleteAPIKey: err=%v, want the AI slot cleared", err)
	}

	failing := &fakeStore{setAPIKey: func(string, string) error { return errors.New("disk full") }}
	s2, p2, _, _ := settingsWith(failing)
	if err := s2.SetAPIKey("openrouter", "k"); err == nil {
		t.Error("SetAPIKey should surface the store error")
	}
	if p2.AI() != nil {
		t.Error("a failed store write must not activate the client")
	}
}

// TestAuthStatus maps the three stored keys onto the status struct.
func TestAuthStatus(t *testing.T) {
	st := &fakeStore{getAPIKey: func(provider string) (string, error) {
		if provider == "google" {
			return "gkey", nil
		}
		return "", nil
	}}
	s, _, _, _ := settingsWith(st)

	status := s.AuthStatus()
	if status.OpenRouterConfigured || status.ElevenLabsConfigured || !status.GoogleConfigured {
		t.Errorf("AuthStatus() = %+v, want only Google configured", status)
	}
}

// TestSetCaptureRegion verifies the read-modify-write: the region fields change,
// the rest of the preferences survive, and the capturer is retargeted.
func TestSetCaptureRegion(t *testing.T) {
	var saved models.Preferences
	st := &fakeStore{
		getPreferences: func() (models.Preferences, error) {
			return models.Preferences{Model: "keep-me", SessionLimitMinutes: 30}, nil
		},
		savePreferences: func(p models.Preferences) error { saved = p; return nil },
	}
	s, _, screen, _ := settingsWith(st)

	if err := s.SetCaptureRegion(2, 0.1, 0.2, 0.3, 0.4); err != nil {
		t.Fatalf("SetCaptureRegion() error: %v", err)
	}
	if saved.Model != "keep-me" || saved.SessionLimitMinutes != 30 {
		t.Errorf("unrelated preferences were clobbered: %+v", saved)
	}
	if saved.CaptureDisplay != 2 || saved.RegionX != 0.1 || saved.RegionH != 0.4 {
		t.Errorf("region not persisted: %+v", saved)
	}
	if len(screen.regions) != 1 || screen.regions[0] != [5]float64{2, 0.1, 0.2, 0.3, 0.4} {
		t.Errorf("capturer regions = %v, want the new region", screen.regions)
	}
}

// TestApplySavedRegion verifies startup region restoration is best-effort.
func TestApplySavedRegion(t *testing.T) {
	st := &fakeStore{getPreferences: func() (models.Preferences, error) {
		return models.Preferences{CaptureDisplay: 3, RegionW: 0.5}, nil
	}}
	s, _, screen, _ := settingsWith(st)
	s.ApplySavedRegion()
	if len(screen.regions) != 1 || screen.regions[0] != [5]float64{3, 0, 0, 0.5, 0} {
		t.Errorf("regions = %v, want the saved region", screen.regions)
	}

	failing := &fakeStore{getPreferences: func() (models.Preferences, error) {
		return models.Preferences{}, errors.New("no prefs yet")
	}}
	s2, _, screen2, _ := settingsWith(failing)
	s2.ApplySavedRegion() // must not panic
	if len(screen2.regions) != 0 {
		t.Error("a failed read must not touch the capturer")
	}
}

// TestCompanyStarring verifies the star/unstar round-trip and the empty-slug
// guard.
func TestCompanyStarring(t *testing.T) {
	starred := map[string]bool{}
	st := &fakeStore{
		setCompanyStarred: func(slug string, on bool) error {
			if on {
				starred[slug] = true
			} else {
				delete(starred, slug)
			}
			return nil
		},
		listStarredCompanies: func() ([]string, error) {
			slugs := make([]string, 0, len(starred))
			for s := range starred {
				slugs = append(slugs, s)
			}
			sort.Strings(slugs)
			return slugs, nil
		},
	}
	s, _, _, _ := settingsWith(st)

	if err := s.SetCompanyStarred("meta", true); err != nil {
		t.Fatalf("star meta: %v", err)
	}
	if err := s.SetCompanyStarred("google", true); err != nil {
		t.Fatalf("star google: %v", err)
	}
	got, err := s.StarredCompanies()
	if err != nil || !reflect.DeepEqual(got, []string{"google", "meta"}) {
		t.Errorf("StarredCompanies() = %v, %v; want [google meta]", got, err)
	}

	if err := s.SetCompanyStarred("google", false); err != nil {
		t.Fatalf("unstar google: %v", err)
	}
	if got, _ := s.StarredCompanies(); !reflect.DeepEqual(got, []string{"meta"}) {
		t.Errorf("after unstar, StarredCompanies() = %v, want [meta]", got)
	}

	if err := s.SetCompanyStarred("", true); err == nil {
		t.Error("SetCompanyStarred(\"\") should reject the empty slug")
	}
	if len(starred) != 1 {
		t.Errorf("empty-slug call must not touch the store; starred = %v", starred)
	}
}
