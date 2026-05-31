package models

import (
	"testing"
	"time"

	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
	theorytypes "github.com/theory-cloud/tabletheory/pkg/types"
)

func TestBookmarkUpdateKeysTimeRecord(t *testing.T) {
	createdAt := time.Date(2024, 2, 3, 4, 5, 6, 7000, time.FixedZone("PST", -8*3600))
	b := &Bookmark{
		Username:   "alice",
		ObjectID:   "status123",
		CreatedAt:  createdAt,
		RecordType: BookmarkRecordTypeTime,
	}

	require.NoError(t, b.UpdateKeys())
	require.Equal(t, "BOOKMARK#alice", b.PK)
	require.Equal(t, "TIME#"+createdAt.UTC().Format(time.RFC3339Nano)+"#status123", b.SK)
	require.True(t, b.CreatedAt.Equal(createdAt.UTC()))
	require.Equal(t, b.SK, b.TimeRecordSK)
}

func TestBookmarkUpdateKeysDefaultsToTimeRecord(t *testing.T) {
	createdAt := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	b := &Bookmark{
		Username:  "bob",
		ObjectID:  "status456",
		CreatedAt: createdAt,
	}

	require.NoError(t, b.UpdateKeys())
	require.Equal(t, BookmarkRecordTypeTime, b.RecordType)
	require.Equal(t, "TIME#"+createdAt.Format(time.RFC3339Nano)+"#status456", b.SK)
}

func TestBookmarkUpdateKeysObjectRecord(t *testing.T) {
	b := &Bookmark{
		Username:     "carol",
		ObjectID:     "status789",
		TimeRecordSK: "TIME#2024-01-02T03:04:05Z#status789",
		RecordType:   BookmarkRecordTypeObject,
	}

	require.NoError(t, b.UpdateKeys())
	require.Equal(t, "BOOKMARK#carol", b.PK)
	require.Equal(t, "OBJECT#status789", b.SK)
}

func TestBookmarkUpdateKeysInvalidRecordType(t *testing.T) {
	b := &Bookmark{
		Username:   "dave",
		ObjectID:   "status000",
		RecordType: "INVALID",
	}

	err := b.UpdateKeys()
	require.Error(t, err)
}

func TestBookmarkFactories(t *testing.T) {
	createdAt := time.Date(2024, 6, 7, 8, 9, 10, 0, time.UTC)

	timeRecord, err := NewTimeOrderedBookmark("erin", "status111", createdAt)
	require.NoError(t, err)
	require.Equal(t, BookmarkRecordTypeTime, timeRecord.RecordType)
	require.Equal(t, createdAt, timeRecord.CreatedAt)
	require.Equal(t, "TIME#"+createdAt.Format(time.RFC3339Nano)+"#status111", timeRecord.SK)
	require.True(t, timeRecord.Locked)
	require.Equal(t, timeRecord.SK, timeRecord.TimeRecordSK)

	objectRecord, err := NewObjectIndexedBookmark("erin", "status111", createdAt, timeRecord.SK)
	require.NoError(t, err)
	require.Equal(t, BookmarkRecordTypeObject, objectRecord.RecordType)
	require.Equal(t, createdAt, objectRecord.CreatedAt)
	require.Equal(t, "OBJECT#status111", objectRecord.SK)
	require.Equal(t, timeRecord.SK, objectRecord.TimeRecordSK)

	require.Equal(t, timeRecord.Username, objectRecord.Username)
	require.Equal(t, timeRecord.ObjectID, objectRecord.ObjectID)
	require.True(t, timeRecord.CreatedAt.Equal(objectRecord.CreatedAt))
}

func TestNewObjectIndexedBookmarkRequiresTimeSK(t *testing.T) {
	_, err := NewObjectIndexedBookmark("erin", "status111", time.Now(), "")
	require.Error(t, err)
}

func TestUpdateKeysRequiresTimeSKForObject(t *testing.T) {
	b := &Bookmark{
		Username:   "carol",
		ObjectID:   "status789",
		RecordType: BookmarkRecordTypeObject,
	}
	require.Error(t, b.UpdateKeys())
}

func TestBookmarkUnmarshalAcceptsLegacyJSONObjectIDAttribute(t *testing.T) {
	createdAt := time.Date(2025, time.January, 1, 0, 0, 0, 123, time.UTC)
	legacySK := createdAt.Format(time.RFC3339Nano) + "#status-legacy-json"

	var bookmark Bookmark
	err := theorytypes.NewConverter().FromAttributeValue(&dynamodbtypes.AttributeValueMemberM{Value: map[string]dynamodbtypes.AttributeValue{
		"PK":         &dynamodbtypes.AttributeValueMemberS{Value: "BOOKMARK#legacy-user"},
		"SK":         &dynamodbtypes.AttributeValueMemberS{Value: legacySK},
		"username":   &dynamodbtypes.AttributeValueMemberS{Value: "legacy-user"},
		"object_id":  &dynamodbtypes.AttributeValueMemberS{Value: "status-legacy-json"},
		"created_at": &dynamodbtypes.AttributeValueMemberS{Value: createdAt.Format(time.RFC3339Nano)},
	}}, &bookmark)

	require.NoError(t, err)
	require.Equal(t, "legacy-user", bookmark.Username)
	require.Equal(t, "status-legacy-json", bookmark.ObjectID)
	require.True(t, bookmark.CreatedAt.Equal(createdAt))
}
