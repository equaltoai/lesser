package models

import (
	"time"
)

// UserPreferences represents user preferences in DynamoDB
type UserPreferences struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys
	PK string `theorydb:"pk,attr:PK" json:"pk"`
	SK string `theorydb:"sk,attr:SK" json:"sk"`

	// User preferences fields (matching storage.UserPreferences exactly)
	Language                  string          `theorydb:"attr:language" json:"language"`
	DefaultPostingVisibility  string          `theorydb:"attr:defaultPostingVisibility" json:"default_posting_visibility"`
	DefaultMediaSensitive     bool            `theorydb:"attr:defaultMediaSensitive" json:"default_media_sensitive"`
	ExpandSpoilers            bool            `theorydb:"attr:expandSpoilers" json:"expand_spoilers"`
	ExpandMedia               string          `theorydb:"attr:expandMedia" json:"expand_media"`
	AutoplayGifs              bool            `theorydb:"attr:autoplayGifs" json:"autoplay_gifs"`
	ShowFollowCounts          bool            `theorydb:"attr:showFollowCounts" json:"show_follow_counts"`
	PreferredTimelineOrder    string          `theorydb:"attr:preferredTimelineOrder" json:"preferred_timeline_order"`
	SearchSuggestionsEnabled  bool            `theorydb:"attr:searchSuggestionsEnabled" json:"search_suggestions_enabled"`
	PersonalizedSearchEnabled bool            `theorydb:"attr:personalizedSearchEnabled" json:"personalized_search_enabled"`
	ReblogFilters             map[string]bool `theorydb:"attr:reblogFilters" json:"reblog_filters,omitempty"`
	StreamingDefaultQuality   string          `theorydb:"attr:streamingDefaultQuality" json:"streaming_default_quality"`
	StreamingAutoQuality      bool            `theorydb:"attr:streamingAutoQuality" json:"streaming_auto_quality"`
	StreamingPreloadNext      bool            `theorydb:"attr:streamingPreloadNext" json:"streaming_preload_next"`
	StreamingDataSaver        bool            `theorydb:"attr:streamingDataSaver" json:"streaming_data_saver"`

	// Metadata
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// Username for key generation
	Username string `json:"-"`
}

// TableName returns the DynamoDB table name for user preferences.
func (UserPreferences) TableName() string {
	return MainTableName
}

// UpdateKeys sets the DynamoDB keys for user preferences
func (up *UserPreferences) UpdateKeys() {
	up.PK = "USER#" + up.Username
	up.SK = "PREFERENCES"
}

// UserPreferencesStorage represents the data structure for user preferences
// This mirrors storage.UserPreferences but avoids circular dependencies
type UserPreferencesStorage struct {
	Language                  string          `json:"language"`
	DefaultPostingVisibility  string          `json:"default_posting_visibility"`
	DefaultMediaSensitive     bool            `json:"default_media_sensitive"`
	ExpandSpoilers            bool            `json:"expand_spoilers"`
	ExpandMedia               string          `json:"expand_media"`
	AutoplayGifs              bool            `json:"autoplay_gifs"`
	ShowFollowCounts          bool            `json:"show_follow_counts"`
	PreferredTimelineOrder    string          `json:"preferred_timeline_order"`
	SearchSuggestionsEnabled  bool            `json:"search_suggestions_enabled"`
	PersonalizedSearchEnabled bool            `json:"personalized_search_enabled"`
	ReblogFilters             map[string]bool `json:"reblog_filters,omitempty"`
	StreamingDefaultQuality   string          `json:"streaming_default_quality"`
	StreamingAutoQuality      bool            `json:"streaming_auto_quality"`
	StreamingPreloadNext      bool            `json:"streaming_preload_next"`
	StreamingDataSaver        bool            `json:"streaming_data_saver"`
}

// TableName returns the DynamoDB table backing UserPreferencesStorage.
func (UserPreferencesStorage) TableName() string {
	return MainTableName
}

// ToStorage converts the DynamORM model to UserPreferencesStorage
func (up *UserPreferences) ToStorage() *UserPreferencesStorage {
	return &UserPreferencesStorage{
		Language:                  up.Language,
		DefaultPostingVisibility:  up.DefaultPostingVisibility,
		DefaultMediaSensitive:     up.DefaultMediaSensitive,
		ExpandSpoilers:            up.ExpandSpoilers,
		ExpandMedia:               up.ExpandMedia,
		AutoplayGifs:              up.AutoplayGifs,
		ShowFollowCounts:          up.ShowFollowCounts,
		PreferredTimelineOrder:    up.PreferredTimelineOrder,
		SearchSuggestionsEnabled:  up.SearchSuggestionsEnabled,
		PersonalizedSearchEnabled: up.PersonalizedSearchEnabled,
		ReblogFilters:             up.ReblogFilters,
		StreamingDefaultQuality:   up.StreamingDefaultQuality,
		StreamingAutoQuality:      up.StreamingAutoQuality,
		StreamingPreloadNext:      up.StreamingPreloadNext,
		StreamingDataSaver:        up.StreamingDataSaver,
	}
}

// FromStorage populates the DynamORM model from UserPreferencesStorage
func (up *UserPreferences) FromStorage(username string, prefs *UserPreferencesStorage) {
	up.Username = username
	up.Language = prefs.Language
	up.DefaultPostingVisibility = prefs.DefaultPostingVisibility
	up.DefaultMediaSensitive = prefs.DefaultMediaSensitive
	up.ExpandSpoilers = prefs.ExpandSpoilers
	up.ExpandMedia = prefs.ExpandMedia
	up.AutoplayGifs = prefs.AutoplayGifs
	up.ShowFollowCounts = prefs.ShowFollowCounts
	up.PreferredTimelineOrder = prefs.PreferredTimelineOrder
	up.SearchSuggestionsEnabled = prefs.SearchSuggestionsEnabled
	up.PersonalizedSearchEnabled = prefs.PersonalizedSearchEnabled
	up.ReblogFilters = prefs.ReblogFilters
	up.StreamingDefaultQuality = prefs.StreamingDefaultQuality
	up.StreamingAutoQuality = prefs.StreamingAutoQuality
	up.StreamingPreloadNext = prefs.StreamingPreloadNext
	up.StreamingDataSaver = prefs.StreamingDataSaver
	up.UpdatedAt = time.Now()
	up.UpdateKeys()
}

// GetDefaultPreferences returns the default user preferences
func GetDefaultPreferences() *UserPreferencesStorage {
	return &UserPreferencesStorage{
		Language:                  "en",
		DefaultPostingVisibility:  "public",
		DefaultMediaSensitive:     false,
		ExpandSpoilers:            false,
		ExpandMedia:               "default",
		AutoplayGifs:              true,
		ShowFollowCounts:          true,
		PreferredTimelineOrder:    "newest",
		SearchSuggestionsEnabled:  true,
		PersonalizedSearchEnabled: true,
		ReblogFilters:             make(map[string]bool),
		StreamingDefaultQuality:   "AUTO",
		StreamingAutoQuality:      true,
		StreamingPreloadNext:      true,
		StreamingDataSaver:        false,
	}
}
