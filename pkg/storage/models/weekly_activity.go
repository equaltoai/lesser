package models

import (
	"fmt"
	"time"
)

// WeeklyActivity represents activity metrics for a specific week
// Stored in DynamoDB with pattern:
// PK: USER#username
// SK: ACTIVITY#WEEK#{weekStartDate}
type WeeklyActivity struct {
	PK            string `dynamorm:"pk" json:"-"`
	SK            string `dynamorm:"sk" json:"-"`
	UserID        string `json:"user_id,omitempty"`   // Optional: for user-specific activity
	Week          int64  `json:"week"`                // Unix timestamp of week start
	Statuses      int64  `json:"statuses"`            // Number of statuses created
	Logins        int64  `json:"logins"`              // Number of unique logins
	Registrations int64  `json:"registrations"`       // Number of new registrations
}

// UpdateKeys updates the DynamoDB keys based on the activity data
func (w *WeeklyActivity) UpdateKeys() {
	if w.UserID != "" {
		// User-specific weekly activity
		w.PK = fmt.Sprintf("USER#%s", w.UserID)
	} else {
		// Instance-wide weekly activity
		w.PK = "INSTANCE#ACTIVITY"
	}
	
	if w.Week > 0 {
		weekStart := time.Unix(w.Week, 0).Format("2006-01-02")
		w.SK = fmt.Sprintf("ACTIVITY#WEEK#%s", weekStart)
	}
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
	activity.UpdateKeys()
	return activity
}

// NewUserWeeklyActivity creates a new user-specific weekly activity entry
func NewUserWeeklyActivity(userID string, week time.Time) *WeeklyActivity {
	activity := NewWeeklyActivity(week)
	activity.UserID = userID
	activity.UpdateKeys()
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