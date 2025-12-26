package models

// CreateReportRequest represents the request body for POST /api/v1/reports.
type CreateReportRequest struct {
	AccountID string   `json:"account_id"`
	StatusIDs []string `json:"status_ids,omitempty"`
	Comment   string   `json:"comment,omitempty"`
	Forward   bool     `json:"forward"`
	Category  string   `json:"category,omitempty"`
	RuleIDs   []int    `json:"rule_ids,omitempty"`
}

