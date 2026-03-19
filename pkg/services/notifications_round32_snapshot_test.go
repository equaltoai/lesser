package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNotificationService_PostSnapshotHelpers_Round32(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	publishedAt := time.Date(2026, time.March, 19, 15, 0, 0, 0, time.UTC)

	t.Run("postSnapshotForActivity fetches stored string object", func(t *testing.T) {
		parentID := "https://local.example/objects/root"
		svc := &notificationService{
			storage: &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
				object: fakeObjectRepo{getValue: &models.Object{
					ID:           "https://local.example/objects/n1",
					URL:          "https://local.example/@alice/n1",
					Content:      "<p>stored body</p>",
					Published:    publishedAt,
					InReplyTo:    &parentID,
					Visibility:   models.VisibilityPrivate,
					AttributedTo: "https://local.example/users/alice",
				}},
				logger: zap.NewNop(),
				table:  "tbl",
			}},
			logger: zap.NewNop(),
		}

		snapshot := svc.postSnapshotForActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "https://local.example/activities/create-1"},
			Object:     "https://local.example/objects/n1",
		})

		require.NotNil(t, snapshot)
		assert.Equal(t, "https://local.example/objects/n1", snapshot["id"])
		assert.Equal(t, "https://local.example/@alice/n1", snapshot["url"])
		assert.Equal(t, "<p>stored body</p>", snapshot["content"])
		assert.Equal(t, publishedAt.Format(time.RFC3339), snapshot["createdAt"])
		assert.Equal(t, models.VisibilityPrivate, snapshot["visibility"])
		assert.Equal(t, parentID, snapshot["inReplyToId"])
	})

	t.Run("postSnapshotForActivity returns nil when lookup fails", func(t *testing.T) {
		svc := &notificationService{
			storage: &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
				object: fakeObjectRepo{getErr: errors.New("boom")},
				logger: zap.NewNop(),
				table:  "tbl",
			}},
			logger: zap.NewNop(),
		}

		snapshot := svc.postSnapshotForActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "https://local.example/activities/create-2"},
			Object:     "https://local.example/objects/missing",
		})

		require.Nil(t, snapshot)
	})

	t.Run("buildPostSnapshotFromObject supports article and quote note", func(t *testing.T) {
		svc := &notificationService{logger: zap.NewNop()}

		article := &activitypub.Article{
			Note: activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:        "https://local.example/articles/a1",
					Published: ptrTime(publishedAt),
					To:        []string{activitypub.PublicAddress},
				},
				Content:      "<p>article body</p>",
				AttributedTo: "https://local.example/users/alice",
			},
			Name: "Article",
		}
		quote := &activitypub.QuoteNote{
			Note: activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:        "https://local.example/objects/q1",
					Published: ptrTime(publishedAt.Add(time.Minute)),
					BTo:       []string{"https://local.example/users/bob"},
				},
				Content:      "<p>quote body</p>",
				AttributedTo: "https://local.example/users/alice",
			},
		}

		articleSnapshot := svc.buildPostSnapshotFromObject(article, "", "")
		quoteSnapshot := svc.buildPostSnapshotFromObject(quote, "", "")

		require.NotNil(t, articleSnapshot)
		require.NotNil(t, quoteSnapshot)
		assert.Equal(t, "<p>article body</p>", articleSnapshot["content"])
		assert.Equal(t, models.VisibilityPublic, articleSnapshot["visibility"])
		assert.Equal(t, "<p>quote body</p>", quoteSnapshot["content"])
		assert.Equal(t, models.VisibilityDirect, quoteSnapshot["visibility"])
	})

	t.Run("map and scalar helpers cover time parsing and slice extraction", func(t *testing.T) {
		createdAt := anyTimePointerFromMap(map[string]interface{}{
			"createdAt": publishedAt.Format(time.RFC3339),
		}, "createdAt")
		require.NotNil(t, createdAt)
		assert.Equal(t, publishedAt, createdAt.UTC())

		assert.Equal(t, []string{"one", "two"}, stringSliceFromAny([]interface{}{" one ", 2, "two"}))
		assert.Nil(t, stringSliceFromAny("nope"))
		assert.Equal(t, "", derefString(nil))

		value := "parent"
		assert.Equal(t, "parent", derefString(&value))
		assert.Nil(t, buildNotificationPostSnapshot("", "", "", "", "", "", ""))
	})
}
