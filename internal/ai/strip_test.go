package ai

import (
	"testing"
)

func TestStripPastImages(t *testing.T) {
	imageMsg := func(text string) ChatMessage {
		return ChatMessage{
			Role: "user",
			Content: []ContentPart{
				{Type: "text", Text: text},
				{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,xxx"}},
			},
		}
	}

	t.Run("strips images from all but the last user message", func(t *testing.T) {
		msgs := []ChatMessage{
			{Role: "system", Content: "system prompt"},
			imageMsg("turn 1"),
			{Role: "assistant", Content: "response 1"},
			imageMsg("turn 2"),
			{Role: "assistant", Content: "response 2"},
			imageMsg("turn 3"),
		}

		result := stripPastImages(msgs)

		// turns 1 and 2 should be plain strings; turn 3 keeps its image.
		if _, ok := result[1].Content.(string); !ok {
			t.Errorf("turn 1: expected plain string after strip, got %T", result[1].Content)
		}
		if result[1].Content.(string) != "turn 1" {
			t.Errorf("turn 1: text mismatch: %q", result[1].Content)
		}
		if _, ok := result[3].Content.(string); !ok {
			t.Errorf("turn 2: expected plain string after strip, got %T", result[3].Content)
		}
		if _, ok := result[5].Content.([]ContentPart); !ok {
			t.Errorf("last user message: expected []ContentPart, got %T", result[5].Content)
		}
	})

	t.Run("original slice is not mutated", func(t *testing.T) {
		msgs := []ChatMessage{
			imageMsg("first"),
			{Role: "assistant", Content: "ok"},
			imageMsg("last"),
		}

		_ = stripPastImages(msgs)

		// msgs[0] must still have []ContentPart, not a string.
		if _, ok := msgs[0].Content.([]ContentPart); !ok {
			t.Errorf("original msgs[0] was mutated: now %T", msgs[0].Content)
		}
	})

	t.Run("plain string user message is unchanged", func(t *testing.T) {
		msgs := []ChatMessage{
			{Role: "user", Content: "just text"},
			{Role: "assistant", Content: "ok"},
			{Role: "user", Content: "also text"},
		}

		result := stripPastImages(msgs)

		if result[0].Content != "just text" {
			t.Errorf("first user message changed: %v", result[0].Content)
		}
		if result[2].Content != "also text" {
			t.Errorf("last user message changed: %v", result[2].Content)
		}
	})

	t.Run("nil input does not panic", func(t *testing.T) {
		result := stripPastImages(nil)
		if result != nil {
			t.Errorf("expected nil for nil input, got %v", result)
		}
	})

	t.Run("empty slice does not panic", func(t *testing.T) {
		result := stripPastImages([]ChatMessage{})
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %v", result)
		}
	})

	t.Run("single user message keeps its image", func(t *testing.T) {
		msgs := []ChatMessage{imageMsg("only turn")}

		result := stripPastImages(msgs)

		if _, ok := result[0].Content.([]ContentPart); !ok {
			t.Errorf("single user message should keep its image, got %T", result[0].Content)
		}
	})

	t.Run("no user messages leaves slice unchanged", func(t *testing.T) {
		msgs := []ChatMessage{
			{Role: "system", Content: "sys"},
			{Role: "assistant", Content: "hi"},
		}

		result := stripPastImages(msgs)

		if result[0].Content != "sys" || result[1].Content != "hi" {
			t.Errorf("non-user messages should be unchanged")
		}
	})
}
