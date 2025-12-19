package models

import (
	"fmt"
	"time"
)

// Report represents a user report stored in DynamoDB
type Report struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"-"` // REPORT#id
	SK string `dynamorm:"sk,attr:SK" json:"-"` // REPORT

	// GSI fields
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"-"` // USER#reporterID
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"-"` // REPORT#timestamp

	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK" json:"-"` // REPORTED#targetAccountID
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK" json:"-"` // REPORT#timestamp

	GSI3PK string `dynamorm:"index:gsi3,pk,attr:gsi3PK" json:"-"` // STATUS#status
	GSI3SK string `dynamorm:"index:gsi3,sk,attr:gsi3SK" json:"-"` // REPORT#timestamp

	// Report fields
	ID                string     `dynamorm:"attr:id" json:"id"`
	ReporterID        string     `dynamorm:"attr:reporterID" json:"reporter_id"`
	TargetAccountID   string     `dynamorm:"attr:targetAccountID" json:"target_account_id"`
	StatusIDs         []string   `dynamorm:"attr:statusIDs" json:"status_ids,omitempty"`
	Comment           string     `dynamorm:"attr:comment" json:"comment"`
	Category          string     `dynamorm:"attr:category" json:"category"`
	RuleIDs           []int      `dynamorm:"attr:ruleIDs" json:"rule_ids,omitempty"`
	Forwarded         bool       `dynamorm:"attr:forwarded" json:"forwarded"`
	Status            string     `dynamorm:"attr:status" json:"status"`
	ActionTaken       string     `dynamorm:"attr:actionTaken" json:"action_taken,omitempty"`
	ActionTakenAt     *time.Time `dynamorm:"attr:actionTakenAt" json:"action_taken_at,omitempty"`
	ModeratorID       string     `dynamorm:"attr:moderatorID" json:"moderator_id,omitempty"`
	ModerationEventID string     `dynamorm:"attr:moderationEventID" json:"moderation_event_id,omitempty"`
	CreatedAt         time.Time  `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt         time.Time  `dynamorm:"attr:updatedAt" json:"updated_at"`
	AssignedTo        string     `dynamorm:"attr:assignedTo" json:"assigned_to,omitempty"`

	// TTL for auto-deletion (90 days)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the GSI keys based on the report data
func (r *Report) UpdateKeys() {
	// Primary key
	r.PK = fmt.Sprintf("REPORT#%s", r.ID)
	r.SK = "REPORT"

	// GSI1: Query by reporter
	r.GSI1PK = fmt.Sprintf(KeyPatternUser, r.ReporterID)
	r.GSI1SK = fmt.Sprintf("REPORT#%d", r.CreatedAt.Unix())

	// GSI2: Query by target account
	r.GSI2PK = fmt.Sprintf("REPORTED#%s", r.TargetAccountID)
	r.GSI2SK = fmt.Sprintf("REPORT#%d", r.CreatedAt.Unix())

	// GSI3: Query by status
	r.GSI3PK = fmt.Sprintf(KeyPatternStatus, r.Status)
	r.GSI3SK = fmt.Sprintf("REPORT#%d", r.CreatedAt.Unix())

	// Set TTL to 90 days from creation
	if r.TTL == 0 {
		r.TTL = r.CreatedAt.Add(90 * 24 * time.Hour).Unix()
	}
}

// TableName returns the DynamoDB table backing Report.
func (Report) TableName() string {
	return MainTableName
}

// ReportStats represents reporting statistics for a user
type ReportStats struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"-"` // USER#username
	SK string `dynamorm:"sk,attr:SK" json:"-"` // REPORT_STATS

	// Stats fields
	TotalReports      int        `dynamorm:"attr:totalReports" json:"total_reports"`
	ResolvedReports   int        `dynamorm:"attr:resolvedReports" json:"resolved_reports"`
	FalseReports      int        `dynamorm:"attr:falseReports" json:"false_reports"`
	LastReportAt      *time.Time `dynamorm:"attr:lastReportAt" json:"last_report_at,omitempty"`
	LastFalseReportAt *time.Time `dynamorm:"attr:lastFalseReportAt" json:"last_false_report_at,omitempty"`
}

// UpdateKeys updates the primary key fields
func (rs *ReportStats) UpdateKeys(username string) {
	rs.PK = fmt.Sprintf(KeyPatternUser, username)
	rs.SK = "REPORT_STATS"
}

// TableName returns the DynamoDB table backing ReportStats.
func (ReportStats) TableName() string {
	return MainTableName
}
