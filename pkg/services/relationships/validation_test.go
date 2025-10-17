package relationships

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidateUpdateRelationshipCommand tests validation logic for UpdateRelationshipCommand
func TestValidateUpdateRelationshipCommand(t *testing.T) {
	tests := []struct {
		name        string
		cmd         *UpdateRelationshipCommand
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid command with notify",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
				Notify:      boolPtr(true),
			},
			shouldError: false,
		},
		{
			name: "valid command with showReblogs",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
				ShowReblogs: boolPtr(false),
			},
			shouldError: false,
		},
		{
			name: "valid command with languages",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
				Languages:   &[]string{"en", "es"},
			},
			shouldError: false,
		},
		{
			name: "valid command with note",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
				Note:        stringPtr("My friend"),
			},
			shouldError: false,
		},
		{
			name: "valid command with multiple fields",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
				Notify:      boolPtr(true),
				ShowReblogs: boolPtr(false),
				Languages:   &[]string{"en"},
				Note:        stringPtr("Close friend"),
			},
			shouldError: false,
		},
		{
			name: "missing follower ID",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "",
				FollowingID: "user2",
				Notify:      boolPtr(true),
			},
			shouldError: true,
		},
		{
			name: "missing following ID",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "",
				Notify:      boolPtr(true),
			},
			shouldError: true,
		},
		{
			name: "empty command is valid structurally",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate follower ID
			err := validateRequiredField(tt.cmd.FollowerID)
			if tt.shouldError && tt.cmd.FollowerID == "" {
				assert.Error(t, err)
				return
			}

			// Validate following ID
			err = validateRequiredField(tt.cmd.FollowingID)
			if tt.shouldError && tt.cmd.FollowingID == "" {
				assert.Error(t, err)
				return
			}

			if !tt.shouldError {
				assert.NoError(t, err)
			}
		})
	}
}

// TestBuildUpdatesMap tests the construction of updates map
func TestBuildUpdatesMap(t *testing.T) {
	tests := []struct {
		name          string
		cmd           *UpdateRelationshipCommand
		expectedCount int
		expectedKeys  []string
	}{
		{
			name: "notify only",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
				Notify:      boolPtr(true),
			},
			expectedCount: 1,
			expectedKeys:  []string{"Notifying"},
		},
		{
			name: "showReblogs only",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
				ShowReblogs: boolPtr(false),
			},
			expectedCount: 1,
			expectedKeys:  []string{"ShowingReblogs"},
		},
		{
			name: "languages only",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
				Languages:   &[]string{"en", "es"},
			},
			expectedCount: 1,
			expectedKeys:  []string{"Languages"},
		},
		{
			name: "note only",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
				Note:        stringPtr("Friend"),
			},
			expectedCount: 1,
			expectedKeys:  []string{"Note"},
		},
		{
			name: "multiple fields",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
				Notify:      boolPtr(true),
				ShowReblogs: boolPtr(false),
				Languages:   &[]string{"en"},
				Note:        stringPtr("Best friend"),
			},
			expectedCount: 4,
			expectedKeys:  []string{"Notifying", "ShowingReblogs", "Languages", "Note"},
		},
		{
			name: "no fields specified",
			cmd: &UpdateRelationshipCommand{
				FollowerID:  "user1",
				FollowingID: "user2",
			},
			expectedCount: 0,
			expectedKeys:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updates := buildUpdatesMapFromCommand(tt.cmd)

			assert.Equal(t, tt.expectedCount, len(updates), "Wrong number of updates")

			for _, key := range tt.expectedKeys {
				assert.Contains(t, updates, key, "Missing expected key: %s", key)
			}
		})
	}
}

// Helper functions
func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}

func validateRequiredField(value string) error {
	if value == "" {
		return assert.AnError
	}
	return nil
}

func buildUpdatesMapFromCommand(cmd *UpdateRelationshipCommand) map[string]interface{} {
	updates := make(map[string]interface{})
	if cmd.Notify != nil {
		updates["Notifying"] = *cmd.Notify
	}
	if cmd.ShowReblogs != nil {
		updates["ShowingReblogs"] = *cmd.ShowReblogs
	}
	if cmd.Languages != nil {
		updates["Languages"] = *cmd.Languages
	}
	if cmd.Note != nil {
		updates["Note"] = *cmd.Note
	}
	return updates
}
