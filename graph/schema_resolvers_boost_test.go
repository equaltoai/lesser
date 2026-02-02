package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubNotesService struct {
	getNoteFunc func(context.Context, string) (*models.Status, error)
}

func (s *stubNotesService) ReblogNote(context.Context, *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
	return nil, nil
}

func (s *stubNotesService) UnreblogNote(context.Context, *notes.UnreblogNoteCommand) (*notes.LikeResult, error) {
	return nil, nil
}

func (s *stubNotesService) GetNote(ctx context.Context, statusID string) (*models.Status, error) {
	if s.getNoteFunc == nil {
		return nil, nil
	}
	return s.getNoteFunc(ctx, statusID)
}

func (s *stubNotesService) HasReblogged(context.Context, string, string) (bool, error) {
	return false, nil
}

func TestConvertStatusToObjectIncludesBoostContext(t *testing.T) {
	original := &models.Status{
		StatusID:       "orig-1",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Content:        "hello world",
		Visibility:     VisibilityPublic,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice/statuses/orig-1"},
			Content:    "hello world",
		},
	}

	boost := &models.Status{
		StatusID:       "boost-1",
		AuthorID:       "https://example.com/users/bob",
		AuthorUsername: "bob",
		Visibility:     VisibilityPublic,
		ReblogOfID:     "orig-1",
	}

	resolver := &Resolver{
		Logger: zap.NewNop(),
		notesClient: &stubNotesService{getNoteFunc: func(ctx context.Context, statusID string) (*models.Status, error) {
			require.Equal(t, original.StatusID, statusID)
			return original, nil
		}},
	}

	obj := resolver.convertStatusToObject(context.Background(), boost)
	require.NotNil(t, obj)
	require.Equal(t, model.ObjectRelationshipTypeBoost, obj.RelationshipType)
	require.NotNil(t, obj.BoostedObject)
	require.Equal(t, original.StatusID, obj.BoostedObject.ID)
}
