package models

import (
	"fmt"
	"time"
)

// Report represents a user report stored in DynamoDB
type Report struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"-"` // REPORT#id
	SK string `dynamorm:"sk" json:"-"` // REPORT

	// GSI fields
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"-"` // USER#reporterID
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"-"` // REPORT#timestamp

	GSI2PK string `dynamorm:"index:GSI2,pk" json:"-"` // REPORTED#targetAccountID
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"-"` // REPORT#timestamp

	GSI3PK string `dynamorm:"index:GSI3,pk" json:"-"` // STATUS#status
	GSI3SK string `dynamorm:"index:GSI3,sk" json:"-"` // REPORT#timestamp

	// Report fields
	ID                string                `json:"id"`
	ReporterID        string                `json:"reporter_id"`
	TargetAccountID   string                `json:"target_account_id"`
	StatusIDs         []string              `json:"status_ids,omitempty"`
	Comment           string                `json:"comment"`
	Category          string                `json:"category"`
	RuleIDs           []int                 `json:"rule_ids,omitempty"`
	Forwarded         bool                  `json:"forwarded"`
	Status            string                `json:"status"`
	ActionTaken       string                `json:"action_taken,omitempty"`
	ActionTakenAt     *time.Time            `json:"action_taken_at,omitempty"`
	ModeratorID       string                `json:"moderator_id,omitempty"`
	ModerationEventID string                `json:"moderation_event_id,omitempty"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
	AssignedTo        string                `json:"assigned_to,omitempty"`
	
	// TTL for auto-deletion (90 days)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the GSI keys based on the report data
func (r *Report) UpdateKeys() {
	// Primary key
	r.PK = fmt.Sprintf("REPORT#%s", r.ID)
	r.SK = "REPORT"

	// GSI1: Query by reporter
	r.GSI1PK = fmt.Sprintf("USER#%s", r.ReporterID)
	r.GSI1SK = fmt.Sprintf("REPORT#%d", r.CreatedAt.Unix())

	// GSI2: Query by target account
	r.GSI2PK = fmt.Sprintf("REPORTED#%s", r.TargetAccountID)
	r.GSI2SK = fmt.Sprintf("REPORT#%d", r.CreatedAt.Unix())

	// GSI3: Query by status
	r.GSI3PK = fmt.Sprintf("STATUS#%s", r.Status)
	r.GSI3SK = fmt.Sprintf("REPORT#%d", r.CreatedAt.Unix())

	// Set TTL to 90 days from creation
	if r.TTL == 0 {
		r.TTL = r.CreatedAt.Add(90 * 24 * time.Hour).Unix()
	}
}


// ReportStats represents reporting statistics for a user
type ReportStats struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"-"` // USER#username
	SK string `dynamorm:"sk" json:"-"` // REPORT_STATS
	
	// Stats fields
	TotalReports       int        `json:"total_reports"`
	ResolvedReports    int        `json:"resolved_reports"`
	FalseReports       int        `json:"false_reports"`
	LastReportAt       *time.Time `json:"last_report_at,omitempty"`
	LastFalseReportAt  *time.Time `json:"last_false_report_at,omitempty"`
}

// UpdateKeys updates the primary key fields
func (rs *ReportStats) UpdateKeys(username string) {
	rs.PK = fmt.Sprintf("USER#%s", username)
	rs.SK = "REPORT_STATS"
}

