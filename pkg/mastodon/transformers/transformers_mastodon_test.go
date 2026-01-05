package transformers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMastodonTransformer_StorageStatusToMastodon(t *testing.T) {
	tr := NewMastodonTransformer("https://example.com")

	_, err := tr.StorageStatusToMastodon(nil, "")
	require.Error(t, err)

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	status := &storageModels.Status{
		StatusID:       "st1",
		Content:        "hello",
		Sensitive:      true,
		Language:       "en",
		Visibility:     "public",
		CreatedAt:      now,
		AuthorID:       "aid",
		AuthorUsername: "alice",
		InReplyToID:    "parent",
		Hashtags:       []string{"tag"},
		Mentions:       []string{"bob"},
	}

	apiStatus, err := tr.StorageStatusToMastodon(status, "")
	require.NoError(t, err)
	require.NotNil(t, apiStatus)
	assert.Equal(t, status.StatusID, apiStatus.ID)
	require.NotNil(t, apiStatus.InReplyToID)
	assert.Equal(t, "parent", *apiStatus.InReplyToID)
	require.Len(t, apiStatus.Tags, 1)
	require.Len(t, apiStatus.Mentions, 1)
	assert.Equal(t, "alice", apiStatus.Account.Username)
}

func TestMastodonTransformer_StorageAccountToMastodon(t *testing.T) {
	tr := NewMastodonTransformer("https://example.com")

	_, err := tr.StorageAccountToMastodon(nil)
	require.Error(t, err)

	_, err = tr.StorageAccountToMastodon(&storage.Account{})
	require.Error(t, err)

	account := &storage.Account{
		User: &storage.User{
			Username:    "alice",
			DisplayName: "Alice",
			CreatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	apiAccount, err := tr.StorageAccountToMastodon(account)
	require.NoError(t, err)
	require.NotNil(t, apiAccount)
	assert.Equal(t, "alice", apiAccount.Username)
	assert.NotEmpty(t, apiAccount.Avatar)

	account.Actor = &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Type: "Person",
		},
		PreferredUsername: "alice",
		Name:              "Override",
	}

	apiAccount2, err := tr.StorageAccountToMastodon(account)
	require.NoError(t, err)
	require.NotNil(t, apiAccount2)
	assert.Equal(t, "alice", apiAccount2.Username)
}

func TestMastodonTransformer_StorageNotificationToMastodon(t *testing.T) {
	tr := NewMastodonTransformer("https://example.com")

	_, err := tr.StorageNotificationToMastodon(nil, nil, nil)
	require.Error(t, err)

	notif := &storageModels.Notification{ID: "n1", Type: "mention", CreatedAt: time.Now()}
	account := &models.Account{ID: "a1"}
	status := &models.Status{ID: "s1"}

	apiNotif, err := tr.StorageNotificationToMastodon(notif, account, status)
	require.NoError(t, err)
	require.NotNil(t, apiNotif)
	assert.Equal(t, notif.ID, apiNotif.ID)
	assert.Equal(t, "mention", apiNotif.Type)
	require.NotNil(t, apiNotif.Status)
	assert.Equal(t, "s1", apiNotif.Status.ID)
}

func TestMastodonTransformer_ParamsToStorage(t *testing.T) {
	tr := NewMastodonTransformer("https://example.com")

	_, err := tr.MastodonStatusParamsToStorage(nil, "alice")
	require.Error(t, err)

	req, err := tr.MastodonStatusParamsToStorage(&models.CreateStatusRequest{Status: "hi"}, "alice")
	require.NoError(t, err)
	assert.Equal(t, "public", req.Visibility)

	_, err = tr.MastodonAccountParamsToStorage(nil, "alice")
	require.Error(t, err)

	accReq, err := tr.MastodonAccountParamsToStorage(&models.UpdateCredentialsRequest{DisplayName: "Alice"}, "alice")
	require.NoError(t, err)
	assert.Equal(t, "alice", accReq.Username)
	assert.Equal(t, "Alice", accReq.DisplayName)
}

func TestMastodonTransformer_ResponseHelpers(t *testing.T) {
	tr := NewMastodonTransformer("https://example.com")

	resp := tr.FormatMastodonAPIResponse(map[string]string{"ok": "1"})
	assert.Contains(t, resp, "data")
	assert.Contains(t, resp, "timestamp")

	paginated := tr.FormatPaginatedResponse([]interface{}{"a"}, &PaginationInfo{NextCursor: "n", MaxID: "m", HasMore: true, Limit: 10})
	assert.Equal(t, []interface{}{"a"}, paginated["data"])
	assert.Equal(t, "n", paginated["next_cursor"])
	assert.Equal(t, "m", paginated["max_id"])
	assert.Equal(t, true, paginated["has_more"])

	assert.Equal(t, "", tr.BuildLinkHeader("https://example.com/api", nil))
	links := tr.BuildLinkHeader("https://example.com/api", &PaginationInfo{NextCursor: "n", MinID: "p", Limit: 20})
	assert.Contains(t, links, "rel=\"next\"")
	assert.Contains(t, links, "rel=\"prev\"")

	assert.Equal(t, map[string]interface{}{"error": "unknown error"}, tr.FormatMastodonError(nil))
	errResp := tr.FormatMastodonError(errors.New("boom"))
	assert.Equal(t, "boom", errResp["error"])
	assert.Contains(t, errResp, "error_type")
}

func TestMastodonTransformer_AugmentHelpers(t *testing.T) {
	tr := NewMastodonTransformer("https://example.com")

	tr.AugmentAccountWithCounts(nil, 1, 2, 3)
	tr.AugmentStatusWithCounts(nil, 1, 2, 3)
	tr.AugmentStatusWithUserInteractions(nil, true, true, true, true, true)

	account := &models.Account{}
	tr.AugmentAccountWithCounts(account, 1, 2, 3)
	assert.Equal(t, 1, account.FollowersCount)

	status := &models.Status{}
	tr.AugmentStatusWithCounts(status, 1, 2, 3)
	assert.Equal(t, 2, status.ReblogsCount)

	tr.AugmentStatusWithUserInteractions(status, true, false, true, false, true)
	assert.True(t, status.Favourited)
	assert.True(t, status.Bookmarked)
	assert.True(t, status.Pinned)
}

func TestMastodonTransformer_MediaAndEmojiTransforms(t *testing.T) {
	tr := NewMastodonTransformer("https://example.com")

	assert.Empty(t, tr.TransformStorageMediaToMastodon(nil))

	media := tr.TransformStorageMediaToMastodon([]interface{}{
		map[string]interface{}{
			"id":        "m1",
			"mediaType": "image/png",
			"url":       "https://example.com/m.png",
			"name":      "desc",
		},
		"bad",
	})
	require.Len(t, media, 1)
	obj, ok := media[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "image", obj["type"])
	assert.Equal(t, "desc", obj["description"])

	assert.Empty(t, tr.TransformStorageEmojiToMastodon(nil))
	emojis := tr.TransformStorageEmojiToMastodon([]interface{}{
		map[string]interface{}{"name": ":smile:", "url": "https://example.com/s.png"},
		"bad",
	})
	require.Len(t, emojis, 1)
	emoji, ok := emojis[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "smile", emoji["shortcode"])
	assert.Equal(t, "https://example.com/s.png", emoji["url"])
	assert.Equal(t, "https://example.com/s.png", emoji["static_url"])
}

func TestTransformationFrameworkBridge_TransformList(t *testing.T) {
	tr := NewMastodonTransformer("https://example.com")
	bridge := tr.WithTransformationFramework()

	accounts, err := bridge.TransformList(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, accounts)

	_, err = bridge.TransformList(context.Background(), []*storage.Account{nil})
	require.Error(t, err)
}

func TestCachedTransformer_ClearCache(t *testing.T) {
	ct := NewCachedTransformer("https://example.com")
	ct.cache["k"] = "v"
	ct.ClearCache()
	assert.Empty(t, ct.cache)
}

func TestBatchProcessor_ProcessBatches(t *testing.T) {
	bp := NewBatchProcessor("https://example.com")

	statuses, err := bp.ProcessStatusBatch(nil, "")
	require.NoError(t, err)
	assert.Empty(t, statuses)

	_, err = bp.ProcessStatusBatch([]*storageModels.Status{nil}, "")
	require.Error(t, err)

	accounts, err := bp.ProcessAccountBatch(nil)
	require.NoError(t, err)
	assert.Empty(t, accounts)

	_, err = bp.ProcessAccountBatch([]*storage.Account{nil})
	require.Error(t, err)
}
