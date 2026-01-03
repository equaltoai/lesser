package main

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type objectCreatorFunc func(ctx context.Context, obj any) error

func (f objectCreatorFunc) CreateObject(ctx context.Context, obj any) error { return f(ctx, obj) }

type actorGetterFunc func(ctx context.Context, username string) (*activitypub.Actor, error)

func (f actorGetterFunc) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	return f(ctx, username)
}

type activityCreatorFunc func(ctx context.Context, activity *activitypub.Activity) error

func (f activityCreatorFunc) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	return f(ctx, activity)
}

type bookmarkCreatorFunc func(ctx context.Context, username, objectID string) (*models.Bookmark, error)

func (f bookmarkCreatorFunc) CreateBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
	return f(ctx, username, objectID)
}

type importStorageStub struct {
	object   objectCreator
	actor    actorGetter
	activity activityCreator
	bookmark bookmarkCreator
}

func (s importStorageStub) Object() objectCreator     { return s.object }
func (s importStorageStub) Actor() actorGetter        { return s.actor }
func (s importStorageStub) Activity() activityCreator { return s.activity }
func (s importStorageStub) Bookmark() bookmarkCreator { return s.bookmark }

type importRepoStub struct {
	updateStatusCalls   int
	updateProgressCalls []int
	updateProgressErr   error
	updateBudgetCalls   int
}

func (s *importRepoStub) UpdateImportStatus(_ context.Context, _ string, _ string, _ map[string]any, _ string) error {
	s.updateStatusCalls++
	return nil
}

func (s *importRepoStub) UpdateBudgetUsage(_ context.Context, _ string, _ string, _ int64, _ int64) error {
	s.updateBudgetCalls++
	return nil
}

func (s *importRepoStub) UpdateImportProgress(_ context.Context, _ string, progress int) error {
	s.updateProgressCalls = append(s.updateProgressCalls, progress)
	return s.updateProgressErr
}

func TestImportTransaction_Execute_RollbackPaths(t *testing.T) {
	logger := zap.NewNop()

	t.Run("success executes all operations", func(t *testing.T) {
		tx := NewImportTransaction("import-1", logger)
		executed := 0

		tx.AddOperation(func() error { executed++; return nil }, func() error { return nil })
		tx.AddOperation(func() error { executed++; return nil }, func() error { return nil })

		require.NoError(t, tx.Execute(context.Background()))
		require.Equal(t, 2, executed)
	})

	t.Run("failure rolls back prior operations", func(t *testing.T) {
		tx := NewImportTransaction("import-1", logger)
		rolledBack := 0

		tx.AddOperation(func() error { return nil }, func() error { rolledBack++; return nil })
		tx.AddOperation(func() error { return errors.New("boom") }, func() error { rolledBack++; return nil })

		err := tx.Execute(context.Background())
		require.Error(t, err)
		require.Equal(t, 1, rolledBack)
	})
}

func TestImportProcessor_HelperUtilities(t *testing.T) {
	p := &ImportProcessor{
		baseURL: "https://example.com",
		logger:  zap.NewNop(),
		importRepo: &importRepoStub{
			updateProgressErr: errors.New("progress update failed"),
		},
	}

	t.Run("resolveAccount handles local and remote forms", func(t *testing.T) {
		require.Equal(t, "https://remote.example/users/alice", p.resolveAccount(context.Background(), "https://remote.example/users/alice"))
		require.Equal(t, "https://example.com/users/bob", p.resolveAccount(context.Background(), "bob"))
		require.Equal(t, "https://example.com/users/alice", p.resolveAccount(context.Background(), "alice@example.com"))
		require.Equal(t, "https://remote.example/users/alice", p.resolveAccount(context.Background(), "alice@remote.example"))
	})

	t.Run("hide notifications parsing", func(t *testing.T) {
		require.Equal(t, 1, p.findHideNotificationsIndex([]string{"Account address", "Hide notifications"}))
		require.Equal(t, -1, p.findHideNotificationsIndex([]string{"Account address"}))

		require.False(t, p.shouldHideNotifications([]string{"acct"}, -1))
		require.False(t, p.shouldHideNotifications([]string{"acct"}, 3))
		require.True(t, p.shouldHideNotifications([]string{"acct", "true"}, 1))
		require.False(t, p.shouldHideNotifications([]string{"acct", "notabool"}, 1))
	})

	t.Run("cost tracking helpers mutate when non-nil", func(t *testing.T) {
		ct := &models.ImportCostTracking{}
		p.trackFollowCosts(ct)
		p.trackBlockCosts(ct)
		require.Greater(t, ct.DynamoDBOperations, int64(0))
		require.GreaterOrEqual(t, ct.DynamoDBWriteUnits, float64(0))
	})
}

func TestImportProcessor_CSVProcessing_Paths(t *testing.T) {
	progressRepo := &importRepoStub{
		updateProgressErr: errors.New("progress update failed"),
	}

	callCount := 0
	objectRepo := objectCreatorFunc(func(_ context.Context, _ any) error {
		callCount++
		if callCount == 2 {
			return errors.New("create failed")
		}
		return nil
	})

	p := &ImportProcessor{
		baseURL:    "https://example.com",
		logger:     zap.NewNop(),
		importRepo: progressRepo,
		repos: importStorageStub{
			object: objectRepo,
			actor: actorGetterFunc(func(_ context.Context, _ string) (*activitypub.Actor, error) {
				return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil
			}),
			activity: activityCreatorFunc(func(_ context.Context, _ *activitypub.Activity) error {
				return errors.New("delivery failed")
			}),
			bookmark: bookmarkCreatorFunc(func(_ context.Context, _ string, _ string) (*models.Bookmark, error) {
				return &models.Bookmark{}, nil
			}),
		},
	}

	ctx := context.Background()
	event := ImportProcessorEvent{ImportID: "imp-1", Username: "alice"}

	t.Run("followers CSV counts skipped and captures parse errors", func(t *testing.T) {
		reader := csv.NewReader(strings.NewReader("Account address\nalice@example.com\n\"bad\n"))
		result, err := p.processFollowersCSV(reader)
		require.NoError(t, err)
		require.Equal(t, 2, result.Skipped) // includes header + first row
		require.Equal(t, 1, result.Failed)
		require.NotEmpty(t, result.Errors)
	})

	t.Run("following CSV records success and failure", func(t *testing.T) {
		reader := csv.NewReader(strings.NewReader("bob@remote.example\ncarol@remote.example\n"))
		ct := &models.ImportCostTracking{}
		result, err := p.processFollowingCSV(ctx, event, reader, ct)
		require.NoError(t, err)
		require.Equal(t, 1, result.Success)
		require.Equal(t, 1, result.Failed)
		require.NotEmpty(t, result.Errors)
		require.NotEmpty(t, progressRepo.updateProgressCalls)
	})

	t.Run("blocks CSV records success and failure", func(t *testing.T) {
		reader := csv.NewReader(strings.NewReader("bob@remote.example\ncarol@remote.example\n"))
		ct := &models.ImportCostTracking{}
		result, err := p.processBlocksCSV(ctx, event, reader, ct)
		require.NoError(t, err)
		require.GreaterOrEqual(t, result.Success+result.Failed, 1)
	})

	t.Run("mutes CSV uses hide notifications column", func(t *testing.T) {
		header := []string{"Account address", "Hide notifications"}
		reader := csv.NewReader(strings.NewReader("bob@remote.example,true\n"))
		result, err := p.processMutesCSV(ctx, event, reader, header)
		require.NoError(t, err)
		require.Equal(t, 1, result.Success+result.Failed+result.Skipped)
	})

	t.Run("bookmarks CSV handles success", func(t *testing.T) {
		reader := csv.NewReader(strings.NewReader("https://example.com/statuses/1\n"))
		result, err := p.processBookmarksCSV(ctx, event, reader)
		require.NoError(t, err)
		require.Equal(t, 1, result.Success)
	})
}
