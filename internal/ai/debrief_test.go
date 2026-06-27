package ai

import "testing"

// TestParseDebrief covers scorecard parsing: clean JSON, code-fenced/prose-wrapped
// replies, verdict normalisation, rubric clamping, bullet cleanup, and garbage.
func TestParseDebrief(t *testing.T) {
	t.Run("clean json", func(t *testing.T) {
		raw := `{"verdict":"lean hire","summary":"Solid.","rubric":{"problemSolving":4,"coding":3,"communication":4,"complexity":3,"pace":2},"strengths":["Clear approach"],"improvements":["Edge cases"]}`
		got, err := parseDebrief(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Verdict != "Lean Hire" {
			t.Errorf("verdict = %q, want Lean Hire", got.Verdict)
		}
		if got.Rubric.ProblemSolving != 4 || got.Rubric.Coding != 3 || got.Rubric.Pace != 2 {
			t.Errorf("rubric = %+v", got.Rubric)
		}
		if len(got.Strengths) != 1 || len(got.Improvements) != 1 {
			t.Errorf("bullets = %+v / %+v", got.Strengths, got.Improvements)
		}
	})

	t.Run("code-fenced and prose-wrapped", func(t *testing.T) {
		raw := "Sure:\n```json\n{\"verdict\":\"Strong Hire\",\"summary\":\"Great.\",\"rubric\":{},\"strengths\":[],\"improvements\":[]}\n```"
		got, err := parseDebrief(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Verdict != "Strong Hire" {
			t.Errorf("verdict = %q", got.Verdict)
		}
	})

	t.Run("clamps scores and unknown verdict", func(t *testing.T) {
		raw := `{"verdict":"maybe","rubric":{"problemSolving":9,"coding":-2,"communication":3,"complexity":0,"pace":7}}`
		got, err := parseDebrief(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Verdict != "" {
			t.Errorf("verdict = %q, want empty", got.Verdict)
		}
		if got.Rubric.ProblemSolving != 5 || got.Rubric.Coding != 0 || got.Rubric.Pace != 5 {
			t.Errorf("clamp failed: %+v", got.Rubric)
		}
	})

	t.Run("drops empty bullets", func(t *testing.T) {
		got, err := parseDebrief(`{"strengths":["good","  ",""],"improvements":[]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Strengths) != 1 || got.Strengths[0] != "good" {
			t.Errorf("strengths = %+v", got.Strengths)
		}
	})

	t.Run("garbage errors", func(t *testing.T) {
		if _, err := parseDebrief("no json here"); err == nil {
			t.Error("expected error, got nil")
		}
	})
}
