package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// WeeklyActivity represents activity metrics for a specific week
// Stored in DynamoDB with pattern:
// PK: USER#username
// SK: ACTIVITY#WEEK#{weekStartDate}
type WeeklyActivity struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK            string `theorydb:"pk,attr:PK" json:"-"`
	SK            string `theorydb:"sk,attr:SK" json:"-"`
	UserID        string `theorydb:"attr:userID" json:"user_id,omitempty"`    // Optional: for user-specific activity
	Week          int64  `theorydb:"attr:week" json:"week"`                   // Unix timestamp of week start
	Statuses      int64  `theorydb:"attr:statuses" json:"statuses"`           // Number of statuses created
	Logins        int64  `theorydb:"attr:logins" json:"logins"`               // Number of unique logins
	Registrations int64  `theorydb:"attr:registrations" json:"registrations"` // Number of new registrations
}

// UpdateKeys updates the DynamoDB keys based on the activity data
func (w *WeeklyActivity) UpdateKeys() error {
	if w.UserID != "" {
		// User-specific weekly activity
		w.PK = fmt.Sprintf(KeyPatternUser, w.UserID)
	} else {
		// Instance-wide weekly activity
		w.PK = "INSTANCE#ACTIVITY"
	}

	if w.Week > 0 {
		weekStart := time.Unix(w.Week, 0).Format(common.DateFormat)
		w.SK = fmt.Sprintf("ACTIVITY#WEEK#%s", weekStart)
	}
	return nil
}

// GetPK returns the partition key
func (w *WeeklyActivity) GetPK() string {
	return w.PK
}

// GetSK returns the sort key
func (w *WeeklyActivity) GetSK() string {
	return w.SK
}

// NewWeeklyActivity creates a new weekly activity entry
func NewWeeklyActivity(week time.Time) *WeeklyActivity {
	// Normalize to start of week (Monday)
	weekStart := normalizeToWeekStart(week)

	activity := &WeeklyActivity{
		Week:          weekStart.Unix(),
		Statuses:      0,
		Logins:        0,
		Registrations: 0,
	}
	_ = activity.UpdateKeys() // Ignore error as this is internal model operation
	return activity
}

// NewUserWeeklyActivity creates a new user-specific weekly activity entry
func NewUserWeeklyActivity(userID string, week time.Time) *WeeklyActivity {
	activity := NewWeeklyActivity(week)
	activity.UserID = userID
	_ = activity.UpdateKeys() // Ignore error as this is internal model operation
	return activity
}

// normalizeToWeekStart returns the start of the week (Monday) for the given time
func normalizeToWeekStart(t time.Time) time.Time {
	// Get the weekday (0 = Sunday, 1 = Monday, ..., 6 = Saturday)
	weekday := int(t.Weekday())

	// Calculate days to subtract to get to Monday
	// If Sunday (0), subtract 6 days to get to previous Monday
	// Otherwise, subtract (weekday - 1) days
	daysToSubtract := weekday - 1
	if weekday == 0 {
		daysToSubtract = 6
	}

	// Get start of day for the calculated date
	return t.AddDate(0, 0, -daysToSubtract).Truncate(24 * time.Hour)
}

// IncrementStatuses increments the status count
func (w *WeeklyActivity) IncrementStatuses(count int64) {
	w.Statuses += count
}

// IncrementLogins increments the login count
func (w *WeeklyActivity) IncrementLogins(count int64) {
	w.Logins += count
}

// IncrementRegistrations increments the registration count
func (w *WeeklyActivity) IncrementRegistrations(count int64) {
	w.Registrations += count
}

// GetWeekStart returns the week start time
func (w *WeeklyActivity) GetWeekStart() time.Time {
	return time.Unix(w.Week, 0)
}

// GetWeekEnd returns the week end time
func (w *WeeklyActivity) GetWeekEnd() time.Time {
	return w.GetWeekStart().AddDate(0, 0, 7)
}

// GetTotalActivity returns the sum of all activity metrics
func (w *WeeklyActivity) GetTotalActivity() int64 {
	return w.Statuses + w.Logins + w.Registrations
}

// GetAverageDaily returns the average daily activity for the week
func (w *WeeklyActivity) GetAverageDaily() float64 {
	return float64(w.GetTotalActivity()) / 7.0
}

// IsActive returns true if there was any activity during the week
func (w *WeeklyActivity) IsActive() bool {
	return w.GetTotalActivity() > 0
}

// Merge combines another WeeklyActivity into this one
func (w *WeeklyActivity) Merge(other *WeeklyActivity) {
	if other == nil || other.Week != w.Week {
		return
	}
	w.Statuses += other.Statuses
	w.Logins += other.Logins
	w.Registrations += other.Registrations
}

// TableName returns the DynamoDB table backing WeeklyActivity.
func (WeeklyActivity) TableName() string {
	return MainTableName
}
