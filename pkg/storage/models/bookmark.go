package models

import (
	"fmt"
	"time"
)

// BookmarkRecordType identifies the storage strategy used for a bookmark record.
const (
	BookmarkRecordTypeTime   = "TIME"
	BookmarkRecordTypeObject = "OBJECT"
	bookmarkKeyPrefixTime    = "TIME"
	bookmarkKeyPrefixObject  = "OBJECT"
	bookmarkKeyPrefixPK      = "BOOKMARK"
)

// Bookmark key prefixes exported for repository logic.
const (
	BookmarkSortKeyPrefixTime   = bookmarkKeyPrefixTime
	BookmarkSortKeyPrefixObject = bookmarkKeyPrefixObject
	BookmarkPartitionPrefix     = bookmarkKeyPrefixPK
)

// Bookmark represents a user's bookmark of a status/object.
// Phase 1 introduces a dual-write pattern where each logical bookmark is written twice:
//  1. A TIME# record keyed by TIME#{timestamp}#{objectID} for chronological reads
//  2. An OBJECT# record keyed by OBJECT#{objectID} for O(1) batch membership checks
//
// The data payload (Username, ObjectID, CreatedAt, etc.) remains identical across both
// physical records so that either copy can service downstream reads.
type Bookmark struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys
	PK string `theorydb:"pk,attr:PK" json:"-"` // BOOKMARK#username
	SK string `theorydb:"sk,attr:SK" json:"-"` // TIME#timestamp#objectID or OBJECT#objectID

	// GSI8 (OBJECT records only) – reverse index to delete all bookmarks for an object without scans.
	GSI8PK string `theorydb:"index:gsi8,pk,attr:gsi8PK,omitempty" json:"-"`
	GSI8SK string `theorydb:"index:gsi8,sk,attr:gsi8SK,omitempty" json:"-"`

	// Core fields
	Username  string    `theorydb:"attr:username" json:"username"`
	ObjectID  string    `theorydb:"attr:objectID" json:"object_id"`
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`

	// Record metadata for dual-write coordination
	RecordType   string `theorydb:"attr:recordType" json:"record_type,omitempty"`
	Locked       bool   `theorydb:"attr:locked" json:"locked,omitempty"`
	TimeRecordSK string `theorydb:"attr:timeRecordSK" json:"time_record_sk,omitempty"` // OBJECT records keep a pointer to the TIME SK

	// TTL field for automatic cleanup (optional)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing Bookmark.
func (Bookmark) TableName() string {
	return MainTableName
}

// NewTimeOrderedBookmark returns a bookmark configured for chronological reads.
func NewTimeOrderedBookmark(username, objectID string, createdAt time.Time) (*Bookmark, error) {
	b := &Bookmark{
		Username:   username,
		ObjectID:   objectID,
		CreatedAt:  createdAt,
		RecordType: BookmarkRecordTypeTime,
		Locked:     true,
	}
	if err := b.UpdateKeys(); err != nil {
		return nil, err
	}
	return b, nil
}

// NewObjectIndexedBookmark returns a bookmark configured for batch membership checks.
func NewObjectIndexedBookmark(username, objectID string, createdAt time.Time, timeRecordSK string) (*Bookmark, error) {
	if timeRecordSK == "" {
		return nil, fmt.Errorf("time_record_sk must be provided for OBJECT bookmarks")
	}

	b := &Bookmark{
		Username:     username,
		ObjectID:     objectID,
		CreatedAt:    createdAt,
		RecordType:   BookmarkRecordTypeObject,
		TimeRecordSK: timeRecordSK,
	}
	if err := b.UpdateKeys(); err != nil {
		return nil, err
	}
	return b, nil
}

// UpdateKeys sets the DynamoDB partition and sort keys for the bookmark.
// RecordType must be one of the BookmarkRecordType* constants; legacy callers that do not
// set a value default to the TIME record format for backwards compatibility.
func (b *Bookmark) UpdateKeys() error {
	if b.Username == "" {
		return fmt.Errorf("bookmark username must be set")
	}
	if b.ObjectID == "" {
		return fmt.Errorf("bookmark object_id must be set")
	}

	recordType := b.RecordType
	if recordType == "" {
		recordType = BookmarkRecordTypeTime
	}

	createdAt := b.CreatedAt
	if recordType == BookmarkRecordTypeTime {
		if createdAt.IsZero() {
			return fmt.Errorf("created_at must be set for TIME bookmarks")
		}
		createdAt = createdAt.UTC()
	}
	b.CreatedAt = createdAt

	// PK: BOOKMARK#username (matches legacy pattern exactly)
	b.PK = fmt.Sprintf("%s#%s", bookmarkKeyPrefixPK, b.Username)

	switch recordType {
	case BookmarkRecordTypeTime:
		// SK: TIME#{timestamp}#{objectID}
		b.SK = fmt.Sprintf("%s#%s#%s", bookmarkKeyPrefixTime, createdAt.Format(time.RFC3339Nano), b.ObjectID)
		b.TimeRecordSK = b.SK
	case BookmarkRecordTypeObject:
		if b.TimeRecordSK == "" {
			return fmt.Errorf("time_record_sk must be set for OBJECT bookmarks")
		}
		// SK: OBJECT#{objectID}
		b.SK = fmt.Sprintf("%s#%s", bookmarkKeyPrefixObject, b.ObjectID)
	default:
		return fmt.Errorf("unsupported bookmark record type %q", recordType)
	}

	// Only OBJECT rows participate in the object->bookmark index.
	b.GSI8PK = ""
	b.GSI8SK = ""
	if recordType == BookmarkRecordTypeObject {
		b.GSI8PK = fmt.Sprintf("BOOKMARK_OBJECT#%s", b.ObjectID)
		b.GSI8SK = fmt.Sprintf("USER#%s#TIME#%s", b.Username, b.CreatedAt.Format(time.RFC3339Nano))
	}

	b.RecordType = recordType
	return nil
}

// GetPK returns the partition key for BaseRepository compatibility
func (b *Bookmark) GetPK() string {
	return b.PK
}

// GetSK returns the sort key for BaseRepository compatibility
func (b *Bookmark) GetSK() string {
	return b.SK
}
