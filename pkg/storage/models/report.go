package models

import (
	"fmt"
	"time"
)

// Report represents a user report stored in DynamoDB
type Report struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"-"` // REPORT#id
	SK string `theorydb:"sk,attr:SK" json:"-"` // REPORT

	// GSI fields
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"-"` // USER#reporterID
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"-"` // REPORT#timestamp

	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK" json:"-"` // REPORTED#targetAccountID
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK" json:"-"` // REPORT#timestamp

	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK" json:"-"` // STATUS#status
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK" json:"-"` // REPORT#timestamp

	// Report fields
	ID                string     `theorydb:"attr:id" json:"id"`
	ReporterID        string     `theorydb:"attr:reporterID" json:"reporter_id"`
	TargetAccountID   string     `theorydb:"attr:targetAccountID" json:"target_account_id"`
	StatusIDs         []string   `theorydb:"attr:statusIDs" json:"status_ids,omitempty"`
	Comment           string     `theorydb:"attr:comment" json:"comment"`
	Category          string     `theorydb:"attr:category" json:"category"`
	RuleIDs           []int      `theorydb:"attr:ruleIDs" json:"rule_ids,omitempty"`
	Forwarded         bool       `theorydb:"attr:forwarded" json:"forwarded"`
	Status            string     `theorydb:"attr:status" json:"status"`
	ActionTaken       string     `theorydb:"attr:actionTaken" json:"action_taken,omitempty"`
	ActionTakenAt     *time.Time `theorydb:"attr:actionTakenAt" json:"action_taken_at,omitempty"`
	ModeratorID       string     `theorydb:"attr:moderatorID" json:"moderator_id,omitempty"`
	ModerationEventID string     `theorydb:"attr:moderationEventID" json:"moderation_event_id,omitempty"`
	CreatedAt         time.Time  `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt         time.Time  `theorydb:"attr:updatedAt" json:"updated_at"`
	AssignedTo        string     `theorydb:"attr:assignedTo" json:"assigned_to,omitempty"`

	// TTL for auto-deletion (90 days)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"-"` // USER#username
	SK string `theorydb:"sk,attr:SK" json:"-"` // REPORT_STATS

	// Stats fields
	TotalReports      int        `theorydb:"attr:totalReports" json:"total_reports"`
	ResolvedReports   int        `theorydb:"attr:resolvedReports" json:"resolved_reports"`
	FalseReports      int        `theorydb:"attr:falseReports" json:"false_reports"`
	LastReportAt      *time.Time `theorydb:"attr:lastReportAt" json:"last_report_at,omitempty"`
	LastFalseReportAt *time.Time `theorydb:"attr:lastFalseReportAt" json:"last_false_report_at,omitempty"`
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
