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
			errMsg:   "cannot be blank",
		},
		{
			name:     "too long",
			username: strings.Repeat("a", 65),
			wantErr:  true,
			errMsg:   "cannot be longer than 30 characters",
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

func TestValidateResolvedActor(t *testing.T) {
	baseActor := &Actor{
		BaseObject: BaseObject{
			ID:   "https://example.com/users/alice",
			Type: PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
	}

	t.Run("valid minimal actor", func(t *testing.T) {
		err := ValidateResolvedActor(baseActor)
		assert.NoError(t, err)
	})

	t.Run("nil actor", func(t *testing.T) {
		err := ValidateResolvedActor(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("missing id", func(t *testing.T) {
		actor := *baseActor
		actor.ID = ""
		err := ValidateResolvedActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "id")
	})

	t.Run("missing type", func(t *testing.T) {
		actor := *baseActor
		actor.Type = ""
		err := ValidateResolvedActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "type")
	})

	t.Run("missing preferred username", func(t *testing.T) {
		actor := *baseActor
		actor.PreferredUsername = ""
		err := ValidateResolvedActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "preferredUsername")
	})

	t.Run("invalid inbox", func(t *testing.T) {
		actor := *baseActor
		actor.Inbox = "not-a-url"
		err := ValidateResolvedActor(&actor)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inbox")
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
			name:     "script tag",
			input:    `<script>alert('xss')</script>`,
			expected: ``,
		},
		{
			name:     "javascript protocol",
			input:    `<a href="javascript:alert('xss')">link</a>`,
			expected: `link`,
		},
		{
			name:     "javascript protocol with encoding",
			input:    `<a href="java&#115;cript:alert('xss')">link</a>`,
			expected: `link`,
		},
		{
			name:     "event handler",
			input:    `<img src="x" onerror="alert('xss')">`,
			expected: `<img src="x">`,
		},
		{
			name:     "data uri script",
			input:    `<img src="data:text/html,<script>alert('xss')</script>">`,
			expected: ``,
		},
		{
			name:     "svg with script",
			input:    `<svg onload="alert('xss')"></svg>`,
			expected: ``,
		},
		{
			name:     "iframe",
			input:    `<iframe src="javascript:alert('xss')"></iframe>`,
			expected: ``,
		},
		{
			name:     "form with action",
			input:    `<form action="javascript:alert('xss')"><input type="submit"></form>`,
			expected: ``,
		},
		{
			name:     "style tag with expression",
			input:    `<style>body{background:url("javascript:alert('xss')")}</style>`,
			expected: ``,
		},
		{
			name:     "object tag",
			input:    `<object data="javascript:alert('xss')"></object>`,
			expected: ``,
		},
		{
			name:     "embed tag",
			input:    `<embed src="javascript:alert('xss')">`,
			expected: ``,
		},
		{
			name:     "meta refresh",
			input:    `<meta http-equiv="refresh" content="0;url=javascript:alert('xss')">`,
			expected: ``,
		},
		{
			name:     "input with javascript",
			input:    `<input type="image" src="javascript:alert('xss')">`,
			expected: ``,
		},
		{
			name:     "link tag",
			input:    `<link rel="stylesheet" href="javascript:alert('xss')">`,
			expected: ``,
		},
		{
			name:     "base tag",
			input:    `<base href="javascript:alert('xss')">`,
			expected: ``,
		},
		{
			name:     "bgsound",
			input:    `<bgsound src="javascript:alert('xss')">`,
			expected: ``,
		},
		{
			name:     "expression in style attribute",
			input:    `<div style="background-image: expression(alert('xss'))">test</div>`,
			expected: `<div>test</div>`,
		},
		{
			name:     "vbscript protocol",
			input:    `<a href="vbscript:alert('xss')">link</a>`,
			expected: `link`,
		},
		{
			name:     "data protocol with base64",
			input:    `<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgneHNzJyk8L3NjcmlwdD4=">link</a>`,
			expected: `link`,
		},
		{
			name:     "nested tags",
			input:    `<div><script><img src="x" onerror="alert('xss')"></script></div>`,
			expected: `<div></div>`,
		},
		// Test that safe HTML is preserved
		{
			name:     "safe link with rel",
			input:    `<a href="https://example.com" rel="nofollow">Safe Link</a>`,
			expected: `<a href="https://example.com" rel="nofollow">Safe Link</a>`,
		},
		{
			name:     "safe paragraph",
			input:    `<p>This is a <strong>safe</strong> paragraph with <em>emphasis</em>.</p>`,
			expected: `<p>This is a <strong>safe</strong> paragraph with <em>emphasis</em>.</p>`,
		},
		{
			name:     "safe list",
			input:    `<ul><li>Item 1</li><li>Item 2</li></ul>`,
			expected: `<ul><li>Item 1</li><li>Item 2</li></ul>`,
		},
		{
			name:     "safe image",
			input:    `<img src="https://example.com/image.jpg" alt="Safe Image">`,
			expected: `<img src="https://example.com/image.jpg" alt="Safe Image">`,
		},
		{
			name:     "safe span with class",
			input:    `<span class="highlight">Highlighted text</span>`,
			expected: `<span class="highlight">Highlighted text</span>`,
		},
		{
			name:     "blockquote",
			input:    `<blockquote>This is a quote</blockquote>`,
			expected: `<blockquote>This is a quote</blockquote>`,
		},
		{
			name:     "code block",
			input:    `<pre><code>console.log('hello');</code></pre>`,
			expected: `<pre><code>console.log(&#39;hello&#39;);</code></pre>`,
		},
		{
			name:     "empty input",
			input:    ``,
			expected: ``,
		},
		{
			name:     "plain text",
			input:    `Just plain text with no HTML`,
			expected: `Just plain text with no HTML`,
		},
		{
			name:     "escaped entities",
			input:    `&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;`,
			expected: `&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeHTML(tt.input)
			assert.Equal(t, tt.expected, got, "SanitizeHTML(%q) = %q, want %q", tt.input, got, tt.expected)
		})
	}
}

func TestSanitizeHTMLRelaxed(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "div with class",
			input:    `<div class="container">Content</div>`,
			expected: `<div class="container">Content</div>`,
		},
		{
			name:     "still blocks script",
			input:    `<div><script>alert('xss')</script></div>`,
			expected: `<div></div>`,
		},
		{
			name:     "still blocks event handlers",
			input:    `<div onclick="alert('xss')">Click me</div>`,
			expected: `<div>Click me</div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeHTMLRelaxed(tt.input)
			assert.Equal(t, tt.expected, got, "SanitizeHTMLRelaxed(%q) = %q, want %q", tt.input, got, tt.expected)
		})
	}
}
