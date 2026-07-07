package service

import (
	"context"
	"fmt"

	"mogi/internal/hotkey"
	"mogi/internal/models"
)

// SettingsStore is the slice of the data layer the settings service needs.
// *store.DB satisfies it.
type SettingsStore interface {
	GetAPIKey(provider string) (string, error)
	SetAPIKey(provider, value string) error
	DeleteAPIKey(provider string) error
	GetPreferences() (models.Preferences, error)
	SavePreferences(p models.Preferences) error
	ListStarredCompanies() ([]string, error)
	SetCompanyStarred(slug string, starred bool) error
}

// HotkeyApplier is the control surface of the global push-to-talk hook.
// *hotkey.Listener satisfies it; tests use a fake so no OS keyboard hook is
// ever installed.
type HotkeyApplier interface {
	Apply(ctx context.Context, enabled bool, spec hotkey.Spec)
}

// Settings owns API keys and preferences — including their propagation into
// running infrastructure. "Saving prefs must retarget the capturer and
// re-apply the hotkey" is a business invariant, so it lives here once rather
// than in each caller.
type Settings struct {
	store     SettingsStore
	providers *Providers
	screen    Screen
	hotkey    HotkeyApplier
}

// NewSettings wires the settings service to the key/preference store, the live
// client registry, and the infrastructure it keeps in sync.
func NewSettings(store SettingsStore, providers *Providers, screen Screen, hk HotkeyApplier) *Settings {
	return &Settings{store: store, providers: providers, screen: screen, hotkey: hk}
}

// SetAPIKey stores an API key for the given provider ("openrouter",
// "elevenlabs", or "google") and activates it immediately. No restart required.
func (s *Settings) SetAPIKey(provider, key string) error {
	if err := s.store.SetAPIKey(provider, key); err != nil {
		return err
	}
	s.providers.SetKey(provider, key)
	return nil
}

// DeleteAPIKey removes the stored key for the given provider and deactivates
// its client immediately, so STT/TTS provider resolution falls back to
// whatever remains configured. No restart needed.
func (s *Settings) DeleteAPIKey(provider string) error {
	if err := s.store.DeleteAPIKey(provider); err != nil {
		return err
	}
	s.providers.SetKey(provider, "") // empty key deactivates the slot
	return nil
}

// AuthStatus reports which API providers currently have keys configured. It
// reads the key store — the source of truth — rather than the live registry.
func (s *Settings) AuthStatus() models.AuthStatus {
	orKey, _ := s.store.GetAPIKey("openrouter")
	elKey, _ := s.store.GetAPIKey("elevenlabs")
	googleKey, _ := s.store.GetAPIKey("google")
	return models.AuthStatus{
		OpenRouterConfigured: orKey != "",
		ElevenLabsConfigured: elKey != "",
		GoogleConfigured:     googleKey != "",
	}
}

// Preferences returns the user's settings.
func (s *Settings) Preferences() (models.Preferences, error) {
	return s.store.GetPreferences()
}

// StarredCompanies returns the slugs of the companies the user starred in the
// Company Practice picker, alphabetically.
func (s *Settings) StarredCompanies() ([]string, error) {
	return s.store.ListStarredCompanies()
}

// SetCompanyStarred stars (true) or unstars (false) a company in the picker.
// Idempotent. Slugs are kept even if a later dataset refresh drops the company —
// the UI simply ignores slugs it can't resolve.
func (s *Settings) SetCompanyStarred(slug string, starred bool) error {
	if slug == "" {
		return fmt.Errorf("settings: star company: empty slug")
	}
	return s.store.SetCompanyStarred(slug, starred)
}

// Update persists updated settings and propagates them into the running
// infrastructure. ctx must be the Wails context — the hotkey listener retains
// it for emitting ptt events.
func (s *Settings) Update(ctx context.Context, prefs models.Preferences) error {
	if err := s.store.SavePreferences(prefs); err != nil {
		return err
	}
	// Keep the capturer in sync with any region/display change.
	s.screen.SetRegion(prefs.CaptureDisplay, prefs.RegionX, prefs.RegionY, prefs.RegionW, prefs.RegionH)
	// Enable/disable/re-key the global push-to-talk hook to match the new prefs.
	s.ApplyHotkey(ctx)
	return nil
}

// ApplyHotkey applies the saved push-to-talk preferences to the global hook.
// The hook starts on first enable and is never restarted — enabling, disabling,
// and rebinding all flow through Apply, which swaps guarded fields on the
// running hook. Best-effort — a bad/empty key falls back to the default.
func (s *Settings) ApplyHotkey(ctx context.Context) {
	prefs, err := s.store.GetPreferences()
	if err != nil {
		return
	}
	spec, perr := hotkey.ParseSpec(prefs.PushToTalkKey)
	if perr != nil {
		spec, _ = hotkey.ParseSpec(hotkey.DefaultSpec)
	}
	s.hotkey.Apply(ctx, prefs.PushToTalkEnabled, spec)
}

// ApplySavedRegion loads the persisted capture display/region and applies it to
// the capturer, so on-demand captures honour it before any session starts.
// Best-effort: falls back to the full primary display on any error.
func (s *Settings) ApplySavedRegion() {
	prefs, err := s.store.GetPreferences()
	if err != nil {
		return
	}
	s.screen.SetRegion(prefs.CaptureDisplay, prefs.RegionX, prefs.RegionY, prefs.RegionW, prefs.RegionH)
}

// SetCaptureRegion persists the chosen display and sub-region (fractions 0..1
// of the display; a zero width means full display) and applies it to the
// capturer.
func (s *Settings) SetCaptureRegion(displayIndex int, x, y, w, h float64) error {
	prefs, err := s.store.GetPreferences()
	if err != nil {
		return err
	}
	prefs.CaptureDisplay = displayIndex
	prefs.RegionX, prefs.RegionY, prefs.RegionW, prefs.RegionH = x, y, w, h
	if err := s.store.SavePreferences(prefs); err != nil {
		return err
	}
	s.screen.SetRegion(displayIndex, x, y, w, h)
	return nil
}

// ListModels returns the OpenRouter model catalog for the Settings picker.
// Saving a choice needs no service call — the picker writes the selected id to
// Preferences.Model through Update.
func (s *Settings) ListModels(ctx context.Context) ([]models.Model, error) {
	aiClient := s.providers.AI()
	if aiClient == nil {
		return nil, fmt.Errorf("set an OpenRouter API key first")
	}
	return aiClient.ListModels(ctx)
}
