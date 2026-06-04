package models

// Problem is a coding interview problem shown to the candidate.
type Problem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Difficulty  string `json:"difficulty"` // "easy" | "medium" | "hard"
	Description string `json:"description"`
	Examples    string `json:"examples"`
	Constraints string `json:"constraints"`
}
