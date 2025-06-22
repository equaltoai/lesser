package mastodon

import (
	"strconv"
	"testing"
)

func TestGenerateNumericID(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantLen  int // Check ID has at least this many digits
	}{
		{
			name:     "simple username",
			username: "aron",
			wantLen:  10,
		},
		{
			name:     "longer username",
			username: "verylongusername123",
			wantLen:  10,
		},
		{
			name:     "username with numbers",
			username: "user123",
			wantLen:  10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id1 := GenerateNumericID(tt.username)
			id2 := GenerateNumericID(tt.username)

			// IDs should be stable (same username = same ID)
			if id1 != id2 {
				t.Errorf("GenerateNumericID() not stable: %s != %s", id1, id2)
			}

			// ID should be numeric
			if _, err := strconv.ParseInt(id1, 10, 64); err != nil {
				t.Errorf("GenerateNumericID() returned non-numeric ID: %s", id1)
			}

			// ID should have minimum length
			if len(id1) < tt.wantLen {
				t.Errorf("GenerateNumericID() ID too short: got %d chars, want at least %d", len(id1), tt.wantLen)
			}

			// ID should not exceed 15 digits (to avoid client overflow)
			if len(id1) > 15 {
				t.Errorf("GenerateNumericID() ID too long: got %d chars, want max 15", len(id1))
			}
		})
	}

	// Test that different usernames produce different IDs
	id1 := GenerateNumericID("user1")
	id2 := GenerateNumericID("user2")
	if id1 == id2 {
		t.Errorf("Different usernames produced same ID: %s", id1)
	}
}

func TestGenerateNumericIDFromActorID(t *testing.T) {
	tests := []struct {
		name     string
		actorID  string
		username string // Expected extracted username
	}{
		{
			name:     "ActivityPub URL format",
			actorID:  "https://lesser.host/users/aron",
			username: "aron",
		},
		{
			name:     "Mastodon URL format",
			actorID:  "https://mastodon.social/@Gargron",
			username: "Gargron",
		},
		{
			name:     "URL with trailing slash",
			actorID:  "https://lesser.host/users/aron/",
			username: "aron",
		},
		{
			name:     "Plain username",
			actorID:  "aron",
			username: "aron",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := GenerateNumericIDFromActorID(tt.actorID)
			expectedID := GenerateNumericID(tt.username)

			if id != expectedID {
				t.Errorf("GenerateNumericIDFromActorID() = %s, want %s", id, expectedID)
			}
		})
	}
}
