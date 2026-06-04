package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"ai-interviewer/internal/ai"
	"ai-interviewer/internal/capture"
	"ai-interviewer/internal/models"
	"ai-interviewer/internal/store"

	"github.com/google/uuid"
)

// activeSession holds the in-memory state for a running interview.
// Not exported — lives only in the Go process while a session is active.
type activeSession struct {
	session models.Session
	problem models.Problem
	history []ai.ChatMessage
}

// App is the main application struct. Its exported methods are bound to the
// frontend via Wails and callable as async TypeScript functions.
type App struct {
	ctx      context.Context
	db       *store.DB
	capturer *capture.Capturer
	aiClient *ai.Client
	active   *activeSession
}

// NewApp initialises the application: opens the database, creates the screen
// capturer, and restores the AI client from a persisted API key (if any).
func NewApp() (*App, error) {
	db, err := store.Open()
	if err != nil {
		return nil, fmt.Errorf("app: open database: %w", err)
	}

	app := &App{
		db:       db,
		capturer: capture.NewCapturer(),
	}

	// Restore AI client from persisted key.
	key, err := db.GetAPIKey("openrouter")
	if err != nil {
		log.Printf("warning: could not read OpenRouter key: %v", err)
	} else if key != "" {
		app.aiClient = ai.NewClient(key)
	}

	return app, nil
}

// startup is called by Wails when the application is ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown is called by Wails when the application is closing.
func (a *App) shutdown(ctx context.Context) {
	a.capturer.Stop()
	if err := a.db.Close(); err != nil {
		log.Printf("warning: closing database: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

// SetAPIKey stores an API key for the given provider ("openrouter" or
// "elevenlabs") and activates it immediately. No restart required.
func (a *App) SetAPIKey(provider, key string) error {
	if err := a.db.SetAPIKey(provider, key); err != nil {
		return err
	}
	if provider == "openrouter" {
		a.aiClient = ai.NewClient(key)
	}
	return nil
}

// GetAuthStatus reports which API providers currently have keys configured.
func (a *App) GetAuthStatus() models.AuthStatus {
	orKey, _ := a.db.GetAPIKey("openrouter")
	elKey, _ := a.db.GetAPIKey("elevenlabs")
	return models.AuthStatus{
		OpenRouterConfigured: orKey != "",
		ElevenLabsConfigured: elKey != "",
	}
}

// ---------------------------------------------------------------------------
// Interview session
// ---------------------------------------------------------------------------

// StartSession creates a new interview session, initialises the conversation
// history with the system prompt, and starts screen capture.
func (a *App) StartSession(problemID string, model string) (models.Session, error) {
	if a.active != nil {
		return models.Session{}, fmt.Errorf("a session is already active — end it first")
	}

	problem, err := a.GetProblem(problemID)
	if err != nil {
		return models.Session{}, err
	}

	if model == "" {
		prefs, _ := a.db.GetPreferences()
		model = prefs.Model
	}

	id := uuid.New().String()
	session, err := a.db.CreateSession(id, problemID, model)
	if err != nil {
		return models.Session{}, err
	}

	systemPrompt := ai.BuildSystemPrompt(problem)
	a.active = &activeSession{
		session: session,
		problem: problem,
		history: []ai.ChatMessage{
			{Role: "system", Content: systemPrompt},
		},
	}

	// Auto-start screen capture.
	prefs, _ := a.db.GetPreferences()
	a.capturer.Start(a.ctx, prefs.CaptureIntervalMs)

	return session, nil
}

// EndSession stops the current interview and persists the end timestamp.
func (a *App) EndSession(sessionID string) error {
	a.capturer.Stop()

	if err := a.db.EndSession(sessionID); err != nil {
		return err
	}
	a.active = nil
	return nil
}

// SendMessage is the core interview loop. It captures a screenshot, sends the
// user's text plus the screenshot to OpenRouter, persists both turns, and
// returns the interviewer's response.
func (a *App) SendMessage(text string) (string, error) {
	if a.active == nil {
		return "", fmt.Errorf("no active session — start an interview first")
	}
	if a.aiClient == nil {
		return "", fmt.Errorf("OpenRouter API key not configured — add it in Settings")
	}

	// 1. Grab the latest screenshot (may be empty on first call).
	screenshot := a.capturer.Latest()

	// 2. Build and record the user message.
	userMsg := ai.BuildUserMessage(text, screenshot)
	a.active.history = append(a.active.history, userMsg)

	now := time.Now().UTC()
	if err := a.db.AddMessage(models.Message{
		ID:        uuid.New().String(),
		SessionID: a.active.session.ID,
		Role:      "user",
		Content:   text,
		HasImage:  screenshot != "",
		CreatedAt: now,
	}); err != nil {
		return "", fmt.Errorf("save user message: %w", err)
	}

	// 3. Call OpenRouter.
	response, err := a.aiClient.Complete(a.ctx, a.active.session.Model, a.active.history)
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}

	// 4. Record the assistant message.
	assistantMsg := ai.ChatMessage{Role: "assistant", Content: response}
	a.active.history = append(a.active.history, assistantMsg)

	if err := a.db.AddMessage(models.Message{
		ID:        uuid.New().String(),
		SessionID: a.active.session.ID,
		Role:      "assistant",
		Content:   response,
		HasImage:  false,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		return "", fmt.Errorf("save assistant message: %w", err)
	}

	return response, nil
}

// ---------------------------------------------------------------------------
// Screen capture
// ---------------------------------------------------------------------------

// StartCapture begins periodic screen capture at the given interval (ms).
func (a *App) StartCapture(intervalMs int) error {
	if intervalMs <= 0 {
		intervalMs = 3000
	}
	a.capturer.Start(a.ctx, intervalMs)
	return nil
}

// StopCapture halts periodic screen capture.
func (a *App) StopCapture() error {
	a.capturer.Stop()
	return nil
}

// GetLatestScreenshot returns the most recent screenshot as a base64 PNG.
func (a *App) GetLatestScreenshot() (string, error) {
	s := a.capturer.Latest()
	if s == "" {
		return "", fmt.Errorf("no screenshot captured yet")
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Problems
// ---------------------------------------------------------------------------

// ListProblems returns all available interview problems.
// Phase 1: returns a single hardcoded problem.
func (a *App) ListProblems() []models.Problem {
	return []models.Problem{getHardcodedProblem()}
}

// GetProblem returns a problem by ID.
func (a *App) GetProblem(id string) (models.Problem, error) {
	p := getHardcodedProblem()
	if id == p.ID {
		return p, nil
	}
	return models.Problem{}, fmt.Errorf("problem %q not found", id)
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

// ListSessions returns summaries of all past sessions.
func (a *App) ListSessions() ([]models.SessionSummary, error) {
	return a.db.ListSessions()
}

// GetSessionTranscript returns the full message history for a session.
func (a *App) GetSessionTranscript(id string) ([]models.Message, error) {
	return a.db.GetMessages(id)
}

// ---------------------------------------------------------------------------
// Preferences
// ---------------------------------------------------------------------------

// GetPreferences returns the user's settings.
func (a *App) GetPreferences() (models.Preferences, error) {
	return a.db.GetPreferences()
}

// UpdatePreferences persists updated settings.
func (a *App) UpdatePreferences(prefs models.Preferences) error {
	return a.db.SavePreferences(prefs)
}

// ---------------------------------------------------------------------------
// Hardcoded problem (Phase 1)
// ---------------------------------------------------------------------------

func getHardcodedProblem() models.Problem {
	return models.Problem{
		ID:         "two-sum",
		Title:      "Two Sum",
		Difficulty: "easy",
		Description: `Given an array of integers nums and an integer target, return indices of the two numbers such that they add up to target.

You may assume that each input would have exactly one solution, and you may not use the same element twice.

You can return the answer in any order.`,
		Examples: `Example 1:
  Input: nums = [2,7,11,15], target = 9
  Output: [0,1]
  Explanation: Because nums[0] + nums[1] == 9, we return [0, 1].

Example 2:
  Input: nums = [3,2,4], target = 6
  Output: [1,2]

Example 3:
  Input: nums = [3,3], target = 6
  Output: [0,1]`,
		Constraints: `2 <= nums.length <= 10^4
-10^9 <= nums[i] <= 10^9
-10^9 <= target <= 10^9
Only one valid answer exists.`,
	}
}
