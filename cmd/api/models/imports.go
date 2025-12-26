package models

// ImportRequest represents a data import request.
type ImportRequest struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Mode string `json:"mode"`
}

// ImportResults represents the result summary for a completed import.
type ImportResults struct {
	Success int `json:"success"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// ImportJob represents an import job status.
type ImportJob struct {
	ID        string         `json:"id"`
	Status    string         `json:"status"`
	Type      string         `json:"type"`
	CreatedAt string         `json:"created_at"`
	Processed int            `json:"processed"`
	Total     *int           `json:"total,omitempty"`
	Errors    []string       `json:"errors,omitempty"`
	Results   *ImportResults `json:"results,omitempty"`
}
