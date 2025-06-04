package activitypub

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid username",
			username: "alice",
			wantErr:  false,
		},
		{
			name:     "valid with numbers",
			username: "alice123",
			wantErr:  false,
		},
		{
			name:     "valid with underscore",
			username: "alice_bob",
			wantErr:  false,
		},
		{
			name:     "valid with hyphen",
			username: "alice-bob",
			wantErr:  false,
		},
		{
			name:     "empty username",
			username: "",
			wantErr:  true,
			errMsg:   "cannot be empty",
		},
		{
			name:     "too long",
			username: strings.Repeat("a", 65),
			wantErr:  true,
			errMsg:   "too long",
		},
		{
			name:     "special characters",
			username: "alice@bob",
			wantErr:  true,
			errMsg:   "can only contain",
		},
		{
			name:     "spaces",
			username: "alice bob",
			wantErr:  true,
			errMsg:   "can only contain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		fieldName string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid https URL",
			url:       "https://example.com/actor",
			fieldName: "id",
			wantErr:   false,
		},
		{
			name:      "valid http URL",
			url:       "http://localhost:8080/actor",
			fieldName: "id",
			wantErr:   false,
		},
		{
			name:      "empty URL",
			url:       "",
			fieldName: "id",
			wantErr:   true,
			errMsg:    "required field missing",
		},
		{
			name:      "invalid scheme",
			url:       "ftp://example.com/actor",
			fieldName: "id",
			wantErr:   true,
			errMsg:    "must use http or https",
		},
		{
			name:      "no host",
			url:       "https:///path",
			fieldName: "id",
			wantErr:   true,
			errMsg:    "must have a host",
		},
		{
			name:      "not a URL",
			url:       "not a url",
			fieldName: "id",
			wantErr:   true,
			errMsg:    "must use http or https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url, tt.fieldName)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateActor_RequiredFields(t *testing.T) {
	baseActor := &Actor{
		BaseObject: BaseObject{
			ID:   "https://example.com/users/alice",
			Type: PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}

	t.Run("valid actor", func(t *testing.T) {
		err := ValidateActor(baseActor)
		assert.NoError(t, err)
	})

	t.Run("nil actor", func(t *testing.T) {
		err := ValidateActor(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("missing ID", func(t *testing.T) {
		actor := *baseActor
		actor.ID = ""
		err := ValidateActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "id")
	})

	t.Run("missing type", func(t *testing.T) {
		actor := *baseActor
		actor.Type = ""
		err := ValidateActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "type")
	})

	t.Run("invalid type", func(t *testing.T) {
		actor := *baseActor
		actor.Type = "InvalidType"
		err := ValidateActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid actor type")
	})

	t.Run("missing username", func(t *testing.T) {
		actor := *baseActor
		actor.PreferredUsername = ""
		err := ValidateActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "username")
	})

	t.Run("missing inbox", func(t *testing.T) {
		actor := *baseActor
		actor.Inbox = ""
		err := ValidateActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inbox")
	})

	t.Run("missing outbox", func(t *testing.T) {
		actor := *baseActor
		actor.Outbox = ""
		err := ValidateActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "outbox")
	})
}

func TestValidateActor_OptionalFields(t *testing.T) {
	baseActor := &Actor{
		BaseObject: BaseObject{
			ID:   "https://example.com/users/alice",
			Type: PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}

	t.Run("display name too long", func(t *testing.T) {
		actor := *baseActor
		actor.Name = strings.Repeat("a", 256)
		err := ValidateActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name")
		assert.Contains(t, err.Error(), "too long")
	})

	t.Run("summary too long", func(t *testing.T) {
		actor := *baseActor
		actor.Summary = strings.Repeat("a", 5001)
		err := ValidateActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "summary")
		assert.Contains(t, err.Error(), "too long")
	})

	t.Run("invalid followers URL", func(t *testing.T) {
		actor := *baseActor
		actor.Followers = "not-a-url"
		err := ValidateActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "followers")
	})

	t.Run("invalid following URL", func(t *testing.T) {
		actor := *baseActor
		actor.Following = "not-a-url"
		err := ValidateActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "following")
	})
}

func TestValidateActivity(t *testing.T) {
	baseActivity := &Activity{
		BaseObject: BaseObject{
			ID:   "https://example.com/activities/1",
			Type: CreateType,
		},
		Actor:  "https://example.com/users/alice",
		Object: "https://example.com/objects/1",
	}

	t.Run("valid activity", func(t *testing.T) {
		err := ValidateActivity(baseActivity)
		assert.NoError(t, err)
	})

	t.Run("nil activity", func(t *testing.T) {
		err := ValidateActivity(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("invalid type", func(t *testing.T) {
		activity := *baseActivity
		activity.Type = "InvalidType"
		err := ValidateActivity(&activity)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid activity type")
	})

	t.Run("missing actor", func(t *testing.T) {
		activity := *baseActivity
		activity.Actor = ""
		err := ValidateActivity(&activity)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "actor")
	})

	t.Run("public addressing", func(t *testing.T) {
		activity := *baseActivity
		activity.To = []string{PublicAddress}
		err := ValidateActivity(&activity)
		assert.NoError(t, err)
	})

	t.Run("invalid address in to", func(t *testing.T) {
		activity := *baseActivity
		activity.To = []string{"not-a-url"}
		err := ValidateActivity(&activity)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "to")
	})
}

func TestValidateNote(t *testing.T) {
	baseNote := &Note{
		BaseObject: BaseObject{
			ID:   "https://example.com/notes/1",
			Type: NoteType,
		},
		Content:      "Hello, world!",
		AttributedTo: "https://example.com/users/alice",
	}

	t.Run("valid note", func(t *testing.T) {
		err := ValidateNote(baseNote)
		assert.NoError(t, err)
	})

	t.Run("wrong type", func(t *testing.T) {
		note := *baseNote
		note.Type = ArticleType
		err := ValidateNote(&note)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be 'Note'")
	})

	t.Run("empty content", func(t *testing.T) {
		note := *baseNote
		note.Content = ""
		err := ValidateNote(&note)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "content")
		assert.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("content too long", func(t *testing.T) {
		note := *baseNote
		note.Content = strings.Repeat("a", 100001)
		err := ValidateNote(&note)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "content")
		assert.Contains(t, err.Error(), "too long")
	})

	t.Run("missing attributedTo", func(t *testing.T) {
		note := *baseNote
		note.AttributedTo = ""
		err := ValidateNote(&note)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "attributedTo")
	})
}

func TestSanitizeHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal text",
			input:    "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name:     "script tag",
			input:    "Hello <script>alert('xss')</script> world",
			expected: "Hello &lt;script>alert('xss')&lt;/script&gt; world",
		},
		{
			name:     "javascript protocol",
			input:    `<a href="javascript:alert('xss')">link</a>`,
			expected: `<a href="alert('xss')">link</a>`,
		},
		{
			name:     "event handlers",
			input:    `<img src="x" onerror="alert('xss')" onclick="alert('xss')">`,
			expected: `<img src="x" "alert('xss')" "alert('xss')">`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
