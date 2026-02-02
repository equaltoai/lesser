package repositories

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func setupPermissiveObjectRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, baseTime time.Time) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateObjectRepositorySliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateObjectRepositorySliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateObjectRepositoryStructForCoverage(args.Get(0), 0, baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(2), nil).Maybe()
}

func populateObjectRepositorySliceForCoverage(target any, baseTime time.Time) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.Elem().Kind() != reflect.Slice {
		return
	}

	slice := value.Elem()
	elemType := slice.Type().Elem()

	// Avoid interface slices to prevent type assertion pitfalls.
	if elemType.Kind() == reflect.Interface {
		return
	}

	baseElemType := elemType
	if baseElemType.Kind() == reflect.Ptr {
		baseElemType = baseElemType.Elem()
	}

	count := 2

	for i := range count {
		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseElemType)
			populateObjectRepositoryStructForCoverage(element.Interface(), i, baseTime)
		} else {
			elemPtr := reflect.New(baseElemType)
			populateObjectRepositoryStructForCoverage(elemPtr.Interface(), i, baseTime)
			element = elemPtr.Elem()
		}
		slice = reflect.Append(slice, element)
	}

	value.Elem().Set(slice)
}

func populateObjectRepositoryStructForCoverage(target any, idx int, baseTime time.Time) {
	now := baseTime.Add(time.Duration(idx) * time.Minute)

	switch model := target.(type) {
	case *models.Object:
		objID := "note-1"
		if idx%2 == 1 {
			objID = "note-2"
		}

		obj := models.NewObject(objID, activitypub.NoteType, "alice")
		obj.Content = "hello"
		obj.Published = now
		obj.Updated = now.Add(1 * time.Minute)
		obj.Sensitive = true
		obj.To = []string{"https://www.w3.org/ns/activitystreams#Public"}
		obj.CC = []string{"https://example.com/followers"}
		replyTo := "https://example.com/objects/parent"
		obj.InReplyTo = &replyTo
		obj.UpdateGSIKeys()
		*model = *obj

	case *models.CollectionItem:
		item := models.NewCollectionItem("featured", "item-1", "Note", "alice")
		item.Position = idx
		item.AddedAt = now
		item.CreatedAt = now
		*model = *item

	case *models.UpdateHistory:
		model.ObjectID = "note-1"
		model.Version = idx + 1
		model.UpdatedAt = now
		model.UpdatedBy = "alice"
		model.PreviousState = `{"content":"old"}`
		model.Summary = "edit"
		model.CreatedAt = now
		model.UpdateKeys()

	case *models.QuoteRelationship:
		model.QuoterNoteID = "quote-note-1"
		if idx%2 == 1 {
			model.QuoterNoteID = "quote-note-2"
		}
		model.TargetNoteID = "note-1"
		model.QuoterID = "alice"
		model.TargetAuthorID = "bob"
		model.Timestamp = now
		model.Withdrawn = idx%2 == 1
		model.GenerateID()
		_ = model.UpdateKeys()

	case *models.ThreadSync:
		sync := models.NewThreadSync("note-1")
		sync.MissingReplies = []string{"https://remote.example/objects/1", "local-reply-1"}
		sync.UpdatedAt = now
		_ = sync.UpdateKeys()
		*model = *sync

	case *models.ThreadContext:
		model.RootStatusID = "root-1"
		model.StatusID = "note-1"
		model.Depth = 2
		model.Path = "root-1/note-1/reply-1"
		model.AuthorID = "alice"
		model.AuthorHandle = "@alice@example.com"
		model.CreatedAt = now.Add(-10 * time.Minute)
		model.UpdatedAt = now
		model.Participants = []string{"alice", "bob"}
		model.Visibility = models.VisibilityPublic
		model.UpdateKeys()

	case *models.StatusMetadata:
		model.StatusID = "note-1"
		model.QuoteType = models.VisibilityPublic
		model.AllowQuotes = true
		model.WithdrawnFromQuotes = false
		model.AllowReplies = true
		model.ReplyCount = 1
		model.CreatedAt = now.Add(-1 * time.Hour)
		model.UpdatedAt = now
		model.UpdateKeys()

	case *models.StatusSearchResult:
		model.StatusID = "note-1"
		model.Content = "cached content"
		model.AuthorID = "alice"
		model.Published = now.Add(-1 * time.Hour)
		model.Score = 0.8

	case *models.Tombstone:
		model.ID = "note-1"
		model.FormerType = activitypub.NoteType
		model.DeletedBy = "alice"
		model.Deleted = now.Add(-1 * time.Hour)
		model.CreatedAt = now.Add(-1 * time.Hour)
		_ = model.BeforeCreate()

	case *models.Follow:
		acceptedAt := now.Add(-1 * time.Hour)
		model.FollowerUsername = "alice"
		model.FollowedUsername = "bob"
		model.ActivityID = "follow-1"
		model.State = models.FollowStateAccepted
		model.CreatedAt = now.Add(-2 * time.Hour)
		model.UpdatedAt = now
		model.AcceptedAt = &acceptedAt
		*model = *models.NewFollow(model.FollowerUsername, model.FollowedUsername, model.ActivityID)
		model.State = models.FollowStateAccepted
		model.AcceptedAt = &acceptedAt

	case *models.BackgroundFetchJob:
		job := models.NewBackgroundFetchJob("note-1", "thread_sync")
		*model = *job
	}
}

func TestObjectRepository_CoverageSweep(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	_ = repo.getDomainURL()

	notePublished := baseTime.Add(-1 * time.Hour)
	noteUpdated := baseTime
	noteInReplyTo := "https://example.com/objects/parent-1"
	note := activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "note-1",
			Type:      activitypub.NoteType,
			Published: &notePublished,
			Updated:   &noteUpdated,
			Context:   []any{"https://www.w3.org/ns/activitystreams"},
			To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
			CC:        []string{"https://example.com/followers"},
			InReplyTo: noteInReplyTo,
			Sensitive: true,
		},
		Content:      "Hello world",
		AttributedTo: "alice",
		Attachment: []activitypub.Attachment{
			{Type: "Image", URL: "https://example.com/image.jpg", MediaType: "image/jpeg"},
		},
		Tag: []activitypub.Tag{
			{Type: TagTypeMention, Href: "https://example.com/users/bob", Name: "@bob"},
		},
	}

	require.NoError(t, repo.CreateObject(ctx, note))
	_, err := repo.GetObject(ctx, note.ID)
	require.NoError(t, err)

	note.Content = "Updated content"
	require.NoError(t, repo.UpdateObject(ctx, note))
	require.NoError(t, repo.UpdateObjectWithHistory(ctx, note, "alice"))

	require.NoError(t, repo.DeleteObject(ctx, note.ID))

	_, _, err = repo.GetObjectsByActor(ctx, "alice", "", 1)
	require.NoError(t, err)
	_, _, err = repo.GetObjectsByActor(ctx, "alice", "cursor-1", 10)
	require.NoError(t, err)

	_, err = repo.CountObjectReplies(ctx, note.ID)
	require.NoError(t, err)

	require.NoError(t, repo.TombstoneObject(ctx, note.ID, "alice"))

	require.NoError(t, repo.AddToCollection(ctx, "featured", &storage.CollectionItem{
		CollectionID: "featured",
		ItemID:       note.ID,
		ItemType:     activitypub.NoteType,
		AddedBy:      "alice",
		AddedAt:      baseTime,
		Position:     1,
	}))
	require.NoError(t, repo.RemoveFromCollection(ctx, "featured", note.ID))
	_, _, err = repo.GetCollectionItems(ctx, "featured", 1, "")
	require.NoError(t, err)
	_, _, err = repo.GetCollectionItems(ctx, "featured", 10, "cursor-1")
	require.NoError(t, err)
	_, err = repo.IsInCollection(ctx, "featured", note.ID)
	require.NoError(t, err)
	_, err = repo.CountCollectionItems(ctx, "featured")
	require.NoError(t, err)

	_, err = repo.CountQuotes(ctx, note.ID)
	require.NoError(t, err)
	_, err = repo.CountWithdrawnQuotes(ctx, note.ID)
	require.NoError(t, err)
	_, err = repo.CountReplies(ctx, note.ID)
	require.NoError(t, err)
	_, err = repo.CountReplies(ctx, "https://example.com/objects/note-1")
	require.NoError(t, err)

	require.NoError(t, repo.CreateQuoteRelationship(ctx, &storage.QuoteRelationship{
		ID:             "",
		QuoterNoteID:   "quote-note-1",
		TargetNoteID:   note.ID,
		QuoterID:       "alice",
		TargetAuthorID: "bob",
		Timestamp:      baseTime,
	}))

	_, err = repo.GetMissingReplies(ctx, note.ID)
	require.NoError(t, err)
	require.NoError(t, repo.MarkThreadAsSynced(ctx, note.ID))

	_, err = repo.GetStatus(ctx, note.ID)
	require.NoError(t, err)
	_, err = repo.GetUserStatusCount(ctx, "alice")
	require.NoError(t, err)
	_, err = repo.GetStatusReplyCount(ctx, note.ID)
	require.NoError(t, err)

	_, _, err = repo.GetReplies(ctx, note.ID, 1, "")
	require.NoError(t, err)
	_, _, err = repo.GetReplies(ctx, "https://example.com/objects/note-1", 10, "cursor-1")
	require.NoError(t, err)

	require.NoError(t, repo.IncrementReplyCount(ctx, note.ID))
	_, err = repo.GetReplyCount(ctx, note.ID)
	require.NoError(t, err)

	_, err = repo.SyncThreadFromRemote(ctx, note.ID)
	require.NoError(t, err)
	_, err = repo.SyncMissingRepliesFromRemote(ctx, note.ID)
	require.NoError(t, err)
	_, err = repo.GetThreadContext(ctx, note.ID)
	require.NoError(t, err)

	_, _, err = repo.GetQuotesForNote(ctx, note.ID, 1, "")
	require.NoError(t, err)
	_, _, err = repo.GetQuotesForNote(ctx, note.ID, 10, "cursor-1")
	require.NoError(t, err)
	_, err = repo.IsQuoted(ctx, "alice", note.ID)
	require.NoError(t, err)
	require.NoError(t, repo.WithdrawQuote(ctx, "quote-note-1"))

	require.NoError(t, repo.WithdrawStatusFromQuotes(ctx, note.ID))

	require.NoError(t, repo.UpdateQuotePermissions(ctx, note.ID, &storage.QuotePermissions{AllowPublic: true}))
	require.NoError(t, repo.UpdateQuotePermissions(ctx, note.ID, &storage.QuotePermissions{AllowFollowers: true}))
	require.NoError(t, repo.UpdateQuotePermissions(ctx, note.ID, &storage.QuotePermissions{AllowMentioned: true}))
	require.NoError(t, repo.UpdateQuotePermissions(ctx, note.ID, &storage.QuotePermissions{}))

	_, err = repo.IsQuoteAllowed(ctx, note.ID, "alice")
	require.NoError(t, err)
	_, err = repo.GetQuoteType(ctx, note.ID)
	require.NoError(t, err)
	_, err = repo.IsWithdrawnFromQuotes(ctx, note.ID)
	require.NoError(t, err)
	_, err = repo.GetQuotesOfStatus(ctx, note.ID, 10)
	require.NoError(t, err)

	require.NoError(t, repo.CreateTombstone(ctx, &models.Tombstone{
		ID:         note.ID,
		FormerType: activitypub.NoteType,
		DeletedBy:  "alice",
		Deleted:    baseTime.Add(-1 * time.Hour),
	}))
	_, err = repo.GetTombstone(ctx, note.ID)
	require.NoError(t, err)
	_, err = repo.IsTombstoned(ctx, note.ID)
	require.NoError(t, err)

	_, _, err = repo.GetTombstonesByActor(ctx, "alice", 1, "")
	require.NoError(t, err)
	_, _, err = repo.GetTombstonesByActor(ctx, "alice", 10, "cursor-1")
	require.NoError(t, err)
	_, _, err = repo.GetTombstonesByType(ctx, activitypub.NoteType, 1, "")
	require.NoError(t, err)

	_, err = repo.CleanupExpiredTombstones(ctx, 2)
	require.NoError(t, err)

	_, err = repo.GetObjectHistory(ctx, note.ID)
	require.NoError(t, err)
	require.NoError(t, repo.ReplaceObjectWithTombstone(ctx, note.ID, activitypub.NoteType, "alice"))
}
