package models

// GroupedNotificationAccount represents an account summary in a grouped notification response.
type GroupedNotificationAccount struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
	Bot         bool   `json:"bot"`
	CreatedAt   string `json:"created_at"`
}

// GroupedNotificationStatus represents a minimal status payload for grouped notifications.
type GroupedNotificationStatus struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
	URL        string `json:"url"`
	Visibility string `json:"visibility"`
}

// GroupedNotificationMostRecent represents a minimal \"most recent\" notification entry.
type GroupedNotificationMostRecent struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	ActorID   string `json:"actor_id"`
}

// GroupedNotificationEntry represents a minimal entry in the optional all_notifications list.
type GroupedNotificationEntry struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	ActorID   string `json:"actor_id"`
	TargetID  string `json:"target_id"`
	Read      bool   `json:"read"`
}

// GroupedNotificationGroup represents a grouped notification response entry.
type GroupedNotificationGroup struct {
	ID                string                     `json:"id"`
	Type              string                     `json:"type"`
	GroupKey          string                     `json:"group_key"`
	Count             int                        `json:"count"`
	LatestCreatedAt   string                     `json:"latest_created_at"`
	EarliestCreatedAt string                     `json:"earliest_created_at"`
	Read              bool                       `json:"read"`
	SampleAccounts    []GroupedNotificationAccount `json:"sample_accounts"`
	Summary           string                     `json:"summary"`
	Status            *GroupedNotificationStatus  `json:"status,omitempty"`
	MostRecent        *GroupedNotificationMostRecent `json:"most_recent,omitempty"`
	AllNotifications  []GroupedNotificationEntry `json:"all_notifications,omitempty"`
}

