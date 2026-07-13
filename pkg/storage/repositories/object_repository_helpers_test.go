package repositories

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// 1) modelToActivityPubObject Tests - Note Conversion Branch
// ============================================================================

func TestModelToActivityPubObject_NoteType(t *testing.T) {
	logger := zap.NewNop()
	repo := NewObjectRepository(nil, "test-table", "example.com", logger)

	baseTime := time.Date(2024, 12, 27, 10, 0, 0, 0, time.UTC)
	updatedTime := time.Date(2024, 12, 27, 12, 0, 0, 0, time.UTC)
	inReplyTo := "https://example.com/objects/parent-123"

	tests := []struct {
		name           string
		model          *models.Object
		checkNote      func(t *testing.T, note *activitypub.Note)
		expectedFields map[string]interface{}
	}{
		{
			name: "basic note with all core fields",
			model: &models.Object{
				ID:           "https://example.com/objects/note-123",
				Type:         activitypub.NoteType,
				Published:    baseTime,
				Updated:      updatedTime,
				To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
				CC:           []string{"https://example.com/followers"},
				Sensitive:    true,
				Content:      "Hello, world!",
				AttributedTo: "https://example.com/users/alice",
			},
			checkNote: func(t *testing.T, note *activitypub.Note) {
				assert.Equal(t, "https://example.com/objects/note-123", note.ID)
				assert.Equal(t, activitypub.NoteType, note.Type)
				assert.NotNil(t, note.Published)
				assert.Equal(t, baseTime, *note.Published)
				assert.NotNil(t, note.Updated)
				assert.Equal(t, updatedTime, *note.Updated)
				assert.Equal(t, []string{"https://www.w3.org/ns/activitystreams#Public"}, note.To)
				assert.Equal(t, []string{"https://example.com/followers"}, note.CC)
				assert.True(t, note.Sensitive)
				assert.Equal(t, "Hello, world!", note.Content)
				assert.Equal(t, "https://example.com/users/alice", note.AttributedTo)
				assert.Empty(t, note.InReplyTo)
			},
		},
		{
			name: "note omits summary and hidden recipients from readback shape",
			model: &models.Object{
				ID:           "https://example.com/objects/hidden-note",
				Type:         activitypub.NoteType,
				Published:    baseTime,
				Updated:      updatedTime,
				To:           []string{activitypub.PublicAddress},
				BTo:          []string{"https://remote.example/users/hidden"},
				BCC:          []string{"https://remote.example/users/also-hidden"},
				Summary:      "note summary should not be emitted by repository readback",
				Content:      "Hidden addressing must stay private",
				AttributedTo: "https://example.com/users/alice",
			},
			checkNote: func(t *testing.T, note *activitypub.Note) {
				assert.Empty(t, note.BTo)
				assert.Empty(t, note.BCC)
				assert.Empty(t, note.Summary)

				bodyBytes, err := json.Marshal(note)
				require.NoError(t, err)
				var body map[string]any
				require.NoError(t, json.Unmarshal(bodyBytes, &body))
				assert.NotContains(t, body, "bto")
				assert.NotContains(t, body, "bcc")
				assert.NotContains(t, body, "summary")
			},
		},
		{
			name: "note with InReplyTo pointer set",
			model: &models.Object{
				ID:           "https://example.com/objects/reply-456",
				Type:         activitypub.NoteType,
				Published:    baseTime,
				Updated:      updatedTime,
				Content:      "This is a reply",
				AttributedTo: "https://example.com/users/bob",
				InReplyTo:    &inReplyTo,
			},
			checkNote: func(t *testing.T, note *activitypub.Note) {
				assert.Equal(t, "https://example.com/objects/reply-456", note.ID)
				assert.Equal(t, inReplyTo, note.InReplyTo)
			},
		},
		{
			name: "note with valid AttachmentJSON",
			model: &models.Object{
				ID:             "https://example.com/objects/media-note",
				Type:           activitypub.NoteType,
				Published:      baseTime,
				Updated:        updatedTime,
				Content:        "Check out this image!",
				AttributedTo:   "https://example.com/users/alice",
				AttachmentJSON: `[{"type":"Image","url":"https://example.com/image.jpg","mediaType":"image/jpeg"}]`,
			},
			checkNote: func(t *testing.T, note *activitypub.Note) {
				assert.Len(t, note.Attachment, 1)
				assert.Equal(t, "Image", note.Attachment[0].Type)
				assert.Equal(t, "https://example.com/image.jpg", note.Attachment[0].URL)
			},
		},
		{
			name: "note with valid TagJSON",
			model: &models.Object{
				ID:           "https://example.com/objects/tagged-note",
				Type:         activitypub.NoteType,
				Published:    baseTime,
				Updated:      updatedTime,
				Content:      "Hello @alice!",
				AttributedTo: "https://example.com/users/bob",
				TagJSON:      `[{"type":"Mention","href":"https://example.com/users/alice","name":"@alice"}]`,
			},
			checkNote: func(t *testing.T, note *activitypub.Note) {
				assert.Len(t, note.Tag, 1)
				assert.Equal(t, "Mention", note.Tag[0].Type)
				assert.Equal(t, "https://example.com/users/alice", note.Tag[0].Href)
				assert.Equal(t, "@alice", note.Tag[0].Name)
			},
		},
		{
			name: "note with valid ContextJSON",
			model: &models.Object{
				ID:           "https://example.com/objects/context-note",
				Type:         activitypub.NoteType,
				Published:    baseTime,
				Updated:      updatedTime,
				Content:      "Some content",
				AttributedTo: "https://example.com/users/alice",
				ContextJSON:  `["https://www.w3.org/ns/activitystreams"]`,
			},
			checkNote: func(t *testing.T, note *activitypub.Note) {
				// Context was parsed - just verify no error occurred
				assert.NotNil(t, note)
			},
		},
		{
			name: "note with invalid AttachmentJSON ignores unmarshal error",
			model: &models.Object{
				ID:             "https://example.com/objects/bad-attachment",
				Type:           activitypub.NoteType,
				Published:      baseTime,
				Updated:        updatedTime,
				Content:        "Some content",
				AttributedTo:   "https://example.com/users/alice",
				AttachmentJSON: `{invalid json}`,
			},
			checkNote: func(t *testing.T, note *activitypub.Note) {
				// The code ignores unmarshal errors, so attachment should be nil/empty
				assert.Nil(t, note.Attachment)
			},
		},
		{
			name: "note with invalid TagJSON ignores unmarshal error",
			model: &models.Object{
				ID:           "https://example.com/objects/bad-tags",
				Type:         activitypub.NoteType,
				Published:    baseTime,
				Updated:      updatedTime,
				Content:      "Some content",
				AttributedTo: "https://example.com/users/alice",
				TagJSON:      `not valid json at all`,
			},
			checkNote: func(t *testing.T, note *activitypub.Note) {
				// The code ignores unmarshal errors, so tag should be nil/empty
				assert.Nil(t, note.Tag)
			},
		},
		{
			name: "note with invalid ContextJSON ignores unmarshal error",
			model: &models.Object{
				ID:           "https://example.com/objects/bad-context",
				Type:         activitypub.NoteType,
				Published:    baseTime,
				Updated:      updatedTime,
				Content:      "Some content",
				AttributedTo: "https://example.com/users/alice",
				ContextJSON:  `{broken`,
			},
			checkNote: func(t *testing.T, note *activitypub.Note) {
				// The code ignores unmarshal errors
				assert.NotNil(t, note)
			},
		},
		{
			name: "note with empty JSON fields",
			model: &models.Object{
				ID:             "https://example.com/objects/empty-json",
				Type:           activitypub.NoteType,
				Published:      baseTime,
				Updated:        updatedTime,
				Content:        "Simple content",
				AttributedTo:   "https://example.com/users/alice",
				AttachmentJSON: "",
				TagJSON:        "",
				ContextJSON:    "",
			},
			checkNote: func(t *testing.T, note *activitypub.Note) {
				assert.Nil(t, note.Attachment)
				assert.Nil(t, note.Tag)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.modelToActivityPubObject(context.Background(), tt.model)
			require.NoError(t, err)
			require.NotNil(t, result)

			note, ok := result.(*activitypub.Note)
			require.True(t, ok, "expected *activitypub.Note, got %T", result)
			tt.checkNote(t, note)
		})
	}
}

func TestModelToActivityPubObject_ArticleType(t *testing.T) {
	repo := NewObjectRepository(nil, "test-table", "example.com", zap.NewNop())

	baseTime := time.Date(2024, 12, 27, 10, 0, 0, 0, time.UTC)
	updatedTime := time.Date(2024, 12, 27, 12, 0, 0, 0, time.UTC)
	result, err := repo.modelToActivityPubObject(context.Background(), &models.Object{
		ID:             "https://example.com/articles/article-123",
		Type:           activitypub.ArticleType,
		Name:           "Article Title",
		Summary:        "Article summary",
		Published:      baseTime,
		Updated:        updatedTime,
		To:             []string{activitypub.PublicAddress},
		CC:             []string{"https://example.com/users/alice/followers"},
		BTo:            []string{"https://remote.example/users/hidden"},
		BCC:            []string{"https://remote.example/users/also-hidden"},
		Content:        "Article body content",
		AttributedTo:   "https://example.com/users/alice",
		AttachmentJSON: `[{"type":"Image","url":"https://example.com/cover.jpg","mediaType":"image/jpeg","name":"cover"}]`,
		TagJSON:        `[{"type":"Hashtag","href":"https://example.com/tags/cms","name":"#cms"}]`,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	article, ok := result.(*activitypub.Article)
	require.True(t, ok, "expected *activitypub.Article, got %T", result)
	require.Equal(t, "https://example.com/articles/article-123", article.ID)
	require.Equal(t, activitypub.ArticleType, article.Type)
	require.Equal(t, "Article Title", article.Name)
	require.Equal(t, "Article summary", article.Summary)
	require.Equal(t, "<p>Article body content</p>\n", article.Content)
	require.Equal(t, "https://example.com/users/alice", article.AttributedTo)
	require.Equal(t, baseTime, *article.Published)
	require.Equal(t, updatedTime, *article.Updated)
	require.Equal(t, []string{activitypub.PublicAddress}, article.To)
	require.Equal(t, []string{"https://example.com/users/alice/followers"}, article.CC)
	require.Empty(t, article.BTo)
	require.Empty(t, article.BCC)
	require.Len(t, article.Attachment, 1)
	require.Len(t, article.Tag, 1)

	bodyBytes, err := json.Marshal(article)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(bodyBytes, &body))
	require.Equal(t, "Article summary", body["summary"])
	require.NotContains(t, body, "bto")
	require.NotContains(t, body, "bcc")
}

func TestModelToActivityPubObject_ArticleUsesStoredArticleFormat(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2026, time.May, 20, 3, 10, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Article")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "object#https://example.com/articles/stored-html").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "object#https://example.com/articles/stored-html").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Article")).Run(func(args mock.Arguments) {
		article := args.Get(0).(*models.Article)
		*article = models.Article{
			Object: models.Object{
				ID:           "https://example.com/articles/stored-html",
				Type:         activitypub.ArticleType,
				Name:         "Stored HTML",
				Summary:      "stored article summary",
				Content:      `<p onclick="evil()">Stored <strong>HTML</strong><script>alert(1)</script></p>`,
				Published:    baseTime,
				Updated:      baseTime.Add(time.Hour),
				AttributedTo: "https://example.com/users/alice",
				To:           []string{activitypub.PublicAddress},
			},
			ContentFormat: "html",
			Slug:          "stored-html",
		}
	}).Return(nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	result, err := repo.modelToActivityPubObject(ctx, &models.Object{
		ID:           "https://example.com/articles/stored-html",
		Type:         activitypub.ArticleType,
		Name:         "Fallback Object",
		Content:      "**fallback markdown**",
		AttributedTo: "https://example.com/users/alice",
	})
	require.NoError(t, err)

	article, ok := result.(*activitypub.Article)
	require.True(t, ok, "expected *activitypub.Article, got %T", result)
	require.Equal(t, "Stored HTML", article.Name)
	require.Equal(t, "stored article summary", article.Summary)
	require.Contains(t, article.Content, `<p>Stored <strong>HTML</strong></p>`)
	require.NotContains(t, article.Content, "onclick")
	require.NotContains(t, article.Content, "<script")
	require.Equal(t, []string{activitypub.PublicAddress}, article.To)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetArticleModelForObjectRejectsUnavailableLookup(t *testing.T) {
	ctx := context.Background()

	var nilRepo *ObjectRepository
	article, err := nilRepo.getArticleModelForObject(ctx, &models.Object{ID: "https://example.com/articles/1"})
	require.Error(t, err)
	require.Nil(t, article)

	repo := NewObjectRepository(nil, "test-table", "example.com", zap.NewNop())
	article, err = repo.getArticleModelForObject(ctx, &models.Object{ID: "https://example.com/articles/1"})
	require.Error(t, err)
	require.Nil(t, article)

	mockDB := new(mocks.MockDB)
	repo = NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	article, err = repo.getArticleModelForObject(ctx, &models.Object{})
	require.Error(t, err)
	require.Nil(t, article)
}

// ============================================================================
// 2) modelToActivityPubObject Tests - Default Conversion Branch
// ============================================================================

func TestModelToActivityPubObject_DefaultBranch(t *testing.T) {
	logger := zap.NewNop()
	repo := NewObjectRepository(nil, "test-table", "example.com", logger)

	baseTime := time.Date(2024, 12, 27, 10, 0, 0, 0, time.UTC)
	updatedTime := time.Date(2024, 12, 27, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		model    *models.Object
		checkMap func(t *testing.T, result map[string]any)
	}{
		{
			name: "Tombstone type returns map",
			model: &models.Object{
				ID:           "https://example.com/objects/tombstone-456",
				Type:         "Tombstone",
				Published:    baseTime,
				Updated:      updatedTime,
				Content:      "This object was deleted",
				AttributedTo: "https://example.com/users/bob",
			},
			checkMap: func(t *testing.T, result map[string]any) {
				assert.Equal(t, "https://example.com/objects/tombstone-456", result["id"])
				assert.Equal(t, "Tombstone", result["type"])
			},
		},
		{
			name: "includes 'to' when present",
			model: &models.Object{
				ID:           "https://example.com/objects/with-to",
				Type:         "Event",
				Published:    baseTime,
				Updated:      updatedTime,
				Content:      "Event description",
				AttributedTo: "https://example.com/users/alice",
				To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
			},
			checkMap: func(t *testing.T, result map[string]any) {
				to, hasTo := result["to"]
				assert.True(t, hasTo, "to should be present when not nil")
				assert.Equal(t, []string{"https://www.w3.org/ns/activitystreams#Public"}, to)
			},
		},
		{
			name: "includes 'cc' when present",
			model: &models.Object{
				ID:           "https://example.com/objects/with-cc",
				Type:         "Event",
				Published:    baseTime,
				Updated:      updatedTime,
				Content:      "Event description",
				AttributedTo: "https://example.com/users/alice",
				CC:           []string{"https://example.com/followers"},
			},
			checkMap: func(t *testing.T, result map[string]any) {
				cc, hasCC := result["cc"]
				assert.True(t, hasCC, "cc should be present when not nil")
				assert.Equal(t, []string{"https://example.com/followers"}, cc)
			},
		},
		{
			name: "includes both 'to' and 'cc' when both present",
			model: &models.Object{
				ID:           "https://example.com/objects/with-both",
				Type:         "Page",
				Published:    baseTime,
				Updated:      updatedTime,
				Content:      "Page content",
				AttributedTo: "https://example.com/users/alice",
				To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
				CC:           []string{"https://example.com/followers", "https://example.com/users/bob"},
			},
			checkMap: func(t *testing.T, result map[string]any) {
				to, hasTo := result["to"]
				cc, hasCC := result["cc"]
				assert.True(t, hasTo)
				assert.True(t, hasCC)
				assert.Equal(t, []string{"https://www.w3.org/ns/activitystreams#Public"}, to)
				assert.Equal(t, []string{"https://example.com/followers", "https://example.com/users/bob"}, cc)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.modelToActivityPubObject(context.Background(), tt.model)
			require.NoError(t, err)
			require.NotNil(t, result)

			resultMap, ok := result.(map[string]any)
			require.True(t, ok, "expected map[string]any, got %T", result)
			tt.checkMap(t, resultMap)
		})
	}
}

// ============================================================================
// 3) Mention Parsing Helper Tests - parseMentionsFromTags
// ============================================================================

func TestParseMentionsFromTags_TypeSwitch(t *testing.T) {
	logger := zap.NewNop()
	repo := NewObjectRepository(nil, "test-table", "example.com", logger)

	tests := []struct {
		name     string
		input    any
		expected []string
	}{
		{
			name:     "nil input returns empty slice",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "unsupported type returns empty slice",
			input:    123, // int type
			expected: []string{},
		},
		{
			name:     "empty string returns empty slice",
			input:    "",
			expected: []string{},
		},
		{
			name: "[]any with valid mentions",
			input: []any{
				map[string]any{"type": "Mention", "href": "https://example.com/@alice"},
				map[string]any{"type": "Mention", "href": "https://example.com/@bob"},
			},
			expected: []string{"https://example.com/@alice", "https://example.com/@bob"},
		},
		{
			name: "[]activitypub.Tag with mentions",
			input: []activitypub.Tag{
				{Type: TagTypeMention, Href: "https://example.com/@carol"},
				{Type: TagTypeMention, Href: "https://example.com/@dave"},
			},
			expected: []string{"https://example.com/@carol", "https://example.com/@dave"},
		},
		{
			name:     "JSON string with valid mentions",
			input:    `[{"type":"Mention","href":"https://example.com/@eve"},{"type":"Hashtag","href":"https://example.com/tags/test"}]`,
			expected: []string{"https://example.com/@eve"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.parseMentionsFromTags(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// 4) Mention Parsing Helper Tests - parseMentionsFromAnySlice
// ============================================================================

func TestParseMentionsFromAnySlice(t *testing.T) {
	logger := zap.NewNop()
	repo := NewObjectRepository(nil, "test-table", "example.com", logger)

	tests := []struct {
		name     string
		input    []any
		expected []string
	}{
		{
			name:     "empty slice returns empty",
			input:    []any{},
			expected: nil,
		},
		{
			name: "valid Mention maps are extracted",
			input: []any{
				map[string]any{"type": "Mention", "href": "https://example.com/@alice"},
			},
			expected: []string{"https://example.com/@alice"},
		},
		{
			name: "multiple valid mentions",
			input: []any{
				map[string]any{"type": "Mention", "href": "https://example.com/@alice"},
				map[string]any{"type": "Mention", "href": "https://example.com/@bob"},
				map[string]any{"type": "Mention", "href": "https://other.com/@carol"},
			},
			expected: []string{"https://example.com/@alice", "https://example.com/@bob", "https://other.com/@carol"},
		},
		{
			name: "non-map items are ignored",
			input: []any{
				"not a map",
				123,
				map[string]any{"type": "Mention", "href": "https://example.com/@alice"},
				nil,
			},
			expected: []string{"https://example.com/@alice"},
		},
		{
			name: "wrong type is ignored",
			input: []any{
				map[string]any{"type": "Hashtag", "href": "https://example.com/tags/test"},
				map[string]any{"type": "Mention", "href": "https://example.com/@alice"},
			},
			expected: []string{"https://example.com/@alice"},
		},
		{
			name: "missing href is ignored",
			input: []any{
				map[string]any{"type": "Mention", "name": "@alice"},
				map[string]any{"type": "Mention", "href": "https://example.com/@bob"},
			},
			expected: []string{"https://example.com/@bob"},
		},
		{
			name: "empty href is ignored",
			input: []any{
				map[string]any{"type": "Mention", "href": ""},
				map[string]any{"type": "Mention", "href": "https://example.com/@alice"},
			},
			expected: []string{"https://example.com/@alice"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.parseMentionsFromAnySlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// 5) Mention Parsing Helper Tests - extractMentionFromMap
// ============================================================================

func TestExtractMentionFromMap(t *testing.T) {
	logger := zap.NewNop()
	repo := NewObjectRepository(nil, "test-table", "example.com", logger)

	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "non-map returns empty",
			input:    "not a map",
			expected: "",
		},
		{
			name:     "nil returns empty",
			input:    nil,
			expected: "",
		},
		{
			name:     "map without type returns empty",
			input:    map[string]any{"href": "https://example.com/@alice"},
			expected: "",
		},
		{
			name:     "map with non-string type returns empty",
			input:    map[string]any{"type": 123, "href": "https://example.com/@alice"},
			expected: "",
		},
		{
			name:     "map with type not Mention returns empty",
			input:    map[string]any{"type": "Hashtag", "href": "https://example.com/tags/test"},
			expected: "",
		},
		{
			name:     "map with type Mention but no href returns empty",
			input:    map[string]any{"type": "Mention", "name": "@alice"},
			expected: "",
		},
		{
			name:     "map with type Mention and non-string href returns empty",
			input:    map[string]any{"type": "Mention", "href": 123},
			expected: "",
		},
		{
			name:     "map with type Mention and empty href returns empty",
			input:    map[string]any{"type": "Mention", "href": ""},
			expected: "",
		},
		{
			name:     "valid Mention map returns href",
			input:    map[string]any{"type": "Mention", "href": "https://example.com/@alice"},
			expected: "https://example.com/@alice",
		},
		{
			name:     "valid Mention with additional fields returns href",
			input:    map[string]any{"type": "Mention", "href": "https://example.com/@alice", "name": "@alice"},
			expected: "https://example.com/@alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.extractMentionFromMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// 6) Mention Parsing Helper Tests - parseMentionsFromTagSlice
// ============================================================================

func TestParseMentionsFromTagSlice(t *testing.T) {
	logger := zap.NewNop()
	repo := NewObjectRepository(nil, "test-table", "example.com", logger)

	tests := []struct {
		name     string
		input    []activitypub.Tag
		expected []string
	}{
		{
			name:     "empty slice returns nil",
			input:    []activitypub.Tag{},
			expected: nil,
		},
		{
			name: "extracts TagTypeMention with non-empty Href",
			input: []activitypub.Tag{
				{Type: TagTypeMention, Href: "https://example.com/@alice", Name: "@alice"},
			},
			expected: []string{"https://example.com/@alice"},
		},
		{
			name: "ignores non-Mention types",
			input: []activitypub.Tag{
				{Type: "Hashtag", Href: "https://example.com/tags/test", Name: "#test"},
				{Type: TagTypeMention, Href: "https://example.com/@alice", Name: "@alice"},
				{Type: "Emoji", Href: "", Name: ":smile:"},
			},
			expected: []string{"https://example.com/@alice"},
		},
		{
			name: "ignores Mention with empty Href",
			input: []activitypub.Tag{
				{Type: TagTypeMention, Href: "", Name: "@alice"},
				{Type: TagTypeMention, Href: "https://example.com/@bob", Name: "@bob"},
			},
			expected: []string{"https://example.com/@bob"},
		},
		{
			name: "multiple valid mentions",
			input: []activitypub.Tag{
				{Type: TagTypeMention, Href: "https://example.com/@alice", Name: "@alice"},
				{Type: TagTypeMention, Href: "https://example.com/@bob", Name: "@bob"},
				{Type: TagTypeMention, Href: "https://other.com/@carol", Name: "@carol"},
			},
			expected: []string{"https://example.com/@alice", "https://example.com/@bob", "https://other.com/@carol"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.parseMentionsFromTagSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// 7) Mention Parsing Helper Tests - parseMentionsFromJSONString
// ============================================================================

func TestParseMentionsFromJSONString(t *testing.T) {
	logger := zap.NewNop()
	repo := NewObjectRepository(nil, "test-table", "example.com", logger)

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string returns empty slice",
			input:    "",
			expected: []string{},
		},
		{
			name:     "invalid JSON returns empty slice",
			input:    "{not valid json",
			expected: []string{},
		},
		{
			name:     "empty array returns nil",
			input:    "[]",
			expected: nil,
		},
		{
			name:     "valid JSON with mentions",
			input:    `[{"type":"Mention","href":"https://example.com/@alice","name":"@alice"}]`,
			expected: []string{"https://example.com/@alice"},
		},
		{
			name:     "valid JSON with mixed tags",
			input:    `[{"type":"Mention","href":"https://example.com/@alice"},{"type":"Hashtag","href":"https://example.com/tags/test"},{"type":"Mention","href":"https://example.com/@bob"}]`,
			expected: []string{"https://example.com/@alice", "https://example.com/@bob"},
		},
		{
			name:     "JSON with malformed objects still parses known fields",
			input:    `[{"type":"Mention","href":"https://example.com/@alice","extra":123}]`,
			expected: []string{"https://example.com/@alice"},
		},
		{
			name:     "JSON object (not array) returns empty slice",
			input:    `{"type":"Mention","href":"https://example.com/@alice"}`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.parseMentionsFromJSONString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// 8) CreateUpdateHistory Tests with DynamORM Mocks
// ============================================================================

func TestCreateUpdateHistory_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewObjectRepository(mockDB, "test-table", "example.com", logger)

	ctx := context.Background()
	history := &storage.UpdateHistory{
		ObjectID:  "https://example.com/objects/status-123",
		Version:   1,
		UpdatedAt: time.Date(2024, 12, 27, 10, 0, 0, 0, time.UTC),
		UpdatedBy: "https://example.com/users/alice",
		PreviousState: map[string]interface{}{
			"content": "Original content",
		},
		Summary: "Updated status content",
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	err := repo.CreateUpdateHistory(ctx, history)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCreateUpdateHistory_SerializesPreviousState(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewObjectRepository(mockDB, "test-table", "example.com", logger)

	ctx := context.Background()

	previousState := map[string]interface{}{
		"content":   "Original content here",
		"sensitive": false,
		"tags":      []string{"test", "example"},
	}

	history := &storage.UpdateHistory{
		ObjectID:      "https://example.com/objects/status-789",
		Version:       2,
		UpdatedAt:     time.Now(),
		UpdatedBy:     "https://example.com/users/bob",
		PreviousState: previousState,
		Summary:       "Content update",
	}

	var capturedModel *models.UpdateHistory

	// Set up expectations, capturing the model
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Run(func(args mock.Arguments) {
		capturedModel = args.Get(0).(*models.UpdateHistory)
	}).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	err := repo.CreateUpdateHistory(ctx, history)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, capturedModel)

	// Verify PreviousState was serialized to JSON
	assert.NotEmpty(t, capturedModel.PreviousState)

	// Parse and verify the JSON
	var parsedState map[string]interface{}
	err = json.Unmarshal([]byte(capturedModel.PreviousState), &parsedState)
	require.NoError(t, err)
	assert.Equal(t, "Original content here", parsedState["content"])

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCreateUpdateHistory_NilPreviousState(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewObjectRepository(mockDB, "test-table", "example.com", logger)

	ctx := context.Background()

	history := &storage.UpdateHistory{
		ObjectID:      "https://example.com/objects/status-abc",
		Version:       1,
		UpdatedAt:     time.Now(),
		UpdatedBy:     "https://example.com/users/alice",
		PreviousState: nil, // No previous state
		Summary:       "Initial creation",
	}

	var capturedModel *models.UpdateHistory

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Run(func(args mock.Arguments) {
		capturedModel = args.Get(0).(*models.UpdateHistory)
	}).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	err := repo.CreateUpdateHistory(ctx, history)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, capturedModel)

	// Verify PreviousState is empty when nil
	assert.Empty(t, capturedModel.PreviousState)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCreateUpdateHistory_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewObjectRepository(mockDB, "test-table", "example.com", logger)

	ctx := context.Background()

	history := &storage.UpdateHistory{
		ObjectID:  "https://example.com/objects/status-fail",
		Version:   1,
		UpdatedAt: time.Now(),
		UpdatedBy: "https://example.com/users/alice",
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	// Execute
	err := repo.CreateUpdateHistory(ctx, history)

	// Assert - the ErrorHandler wraps the error generically
	require.Error(t, err)
	// The error message contains "object" from the entity type
	assert.Contains(t, err.Error(), "object")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// 9) GetUpdateHistory Tests with DynamORM Mocks
// ============================================================================

func TestGetUpdateHistory_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewObjectRepository(mockDB, "test-table", "example.com", logger)

	ctx := context.Background()
	objectID := "https://example.com/objects/status-123"
	limit := 10

	previousStateJSON := `{"content":"Old content","sensitive":true}`

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OBJECT#https://example.com/objects/status-123#HISTORY").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.UpdateHistory")).Run(func(args mock.Arguments) {
		histories := args.Get(0).(*[]models.UpdateHistory)
		*histories = []models.UpdateHistory{
			{
				ObjectID:      objectID,
				Version:       2,
				UpdatedAt:     time.Date(2024, 12, 27, 12, 0, 0, 0, time.UTC),
				UpdatedBy:     "https://example.com/users/alice",
				PreviousState: previousStateJSON,
				Summary:       "Second edit",
			},
			{
				ObjectID:      objectID,
				Version:       1,
				UpdatedAt:     time.Date(2024, 12, 27, 10, 0, 0, 0, time.UTC),
				UpdatedBy:     "https://example.com/users/alice",
				PreviousState: `{"content":"Original content"}`,
				Summary:       "First edit",
			},
		}
	}).Return(nil)

	// Execute
	result, err := repo.GetUpdateHistory(ctx, objectID, limit)

	// Assert
	require.NoError(t, err)
	require.Len(t, result, 2)

	// First result (newest version)
	assert.Equal(t, objectID, result[0].ObjectID)
	assert.Equal(t, 2, result[0].Version)
	assert.Equal(t, "https://example.com/users/alice", result[0].UpdatedBy)
	assert.Equal(t, "Second edit", result[0].Summary)
	assert.NotNil(t, result[0].PreviousState)
	assert.Equal(t, "Old content", result[0].PreviousState["content"])
	assert.Equal(t, true, result[0].PreviousState["sensitive"])

	// Second result (older version)
	assert.Equal(t, 1, result[1].Version)
	assert.Equal(t, "First edit", result[1].Summary)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetUpdateHistory_InvalidPreviousStateJSON(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewObjectRepository(mockDB, "test-table", "example.com", logger)

	ctx := context.Background()
	objectID := "https://example.com/objects/status-bad"
	limit := 10

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OBJECT#https://example.com/objects/status-bad#HISTORY").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.UpdateHistory")).Run(func(args mock.Arguments) {
		histories := args.Get(0).(*[]models.UpdateHistory)
		*histories = []models.UpdateHistory{
			{
				ObjectID:      objectID,
				Version:       1,
				UpdatedAt:     time.Now(),
				UpdatedBy:     "https://example.com/users/alice",
				PreviousState: "{invalid json", // Invalid JSON
				Summary:       "Bad state",
			},
		}
	}).Return(nil)

	// Execute
	result, err := repo.GetUpdateHistory(ctx, objectID, limit)

	// Assert - should not fail, but PreviousState should be nil/empty
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Nil(t, result[0].PreviousState, "PreviousState should be nil when JSON is invalid")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetUpdateHistory_EmptyPreviousState(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewObjectRepository(mockDB, "test-table", "example.com", logger)

	ctx := context.Background()
	objectID := "https://example.com/objects/status-empty"
	limit := 10

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OBJECT#https://example.com/objects/status-empty#HISTORY").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.UpdateHistory")).Run(func(args mock.Arguments) {
		histories := args.Get(0).(*[]models.UpdateHistory)
		*histories = []models.UpdateHistory{
			{
				ObjectID:      objectID,
				Version:       1,
				UpdatedAt:     time.Now(),
				UpdatedBy:     "https://example.com/users/alice",
				PreviousState: "", // Empty string
				Summary:       "No state",
			},
		}
	}).Return(nil)

	// Execute
	result, err := repo.GetUpdateHistory(ctx, objectID, limit)

	// Assert
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Nil(t, result[0].PreviousState)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetUpdateHistory_LimitValidation(t *testing.T) {
	// Note: ValidateQueryLimit only returns error for limit < 0 or limit > maxLimit (100)
	// So limit = 0 passes validation and is used as-is
	// Only negative or > max limits trigger the default of 10
	tests := []struct {
		name          string
		inputLimit    int
		expectedLimit int
	}{
		{
			name:          "valid limit is used",
			inputLimit:    10,
			expectedLimit: 10,
		},
		{
			name:          "limit exceeding max is clamped to default 10",
			inputLimit:    200,
			expectedLimit: 10, // Default when validation fails
		},
		{
			name:          "zero limit passes validation and is used as-is",
			inputLimit:    0,
			expectedLimit: 0, // 0 passes ValidateQueryLimit (not < 0, not > 100)
		},
		{
			name:          "negative limit defaults to 10",
			inputLimit:    -5,
			expectedLimit: 10,
		},
		{
			name:          "limit at max boundary is used",
			inputLimit:    100,
			expectedLimit: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			logger := zap.NewNop()
			repo := NewObjectRepository(mockDB, "test-table", "example.com", logger)

			ctx := context.Background()

			// Set up expectations
			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Return(mockQuery)
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
			mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
			mockQuery.On("Limit", tt.expectedLimit).Return(mockQuery)
			mockQuery.On("All", mock.AnythingOfType("*[]models.UpdateHistory")).Return(nil)

			// Execute
			_, err := repo.GetUpdateHistory(ctx, "test-object", tt.inputLimit)

			// Assert
			require.NoError(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestGetUpdateHistory_EmptyResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewObjectRepository(mockDB, "test-table", "example.com", logger)

	ctx := context.Background()
	objectID := "https://example.com/objects/no-history"
	limit := 10

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OBJECT#https://example.com/objects/no-history#HISTORY").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.UpdateHistory")).Run(func(args mock.Arguments) {
		histories := args.Get(0).(*[]models.UpdateHistory)
		*histories = []models.UpdateHistory{}
	}).Return(nil)

	// Execute
	result, err := repo.GetUpdateHistory(ctx, objectID, limit)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetUpdateHistory_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewObjectRepository(mockDB, "test-table", "example.com", logger)

	ctx := context.Background()
	objectID := "https://example.com/objects/error-test"
	limit := 10

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OBJECT#https://example.com/objects/error-test#HISTORY").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.UpdateHistory")).Return(ErrTestMockError)

	// Execute
	result, err := repo.GetUpdateHistory(ctx, objectID, limit)

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "update_history")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
