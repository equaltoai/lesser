package graph

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func seedMutationReachParent(t *testing.T, statusRepo interface {
	CreateStatus(context.Context, *models.Status) error
}, id, visibility string) {
	t.Helper()

	now := time.Now().UTC()
	require.NoError(t, statusRepo.CreateStatus(context.Background(), &models.Status{
		StatusID:       id,
		AuthorID:       "https://localhost/users/alice",
		AuthorUsername: "alice",
		Content:        "parent " + id,
		Visibility:     visibility,
		PublishedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
		ModifiedAt:     now,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   fmt.Sprintf("https://localhost/users/alice/statuses/%s", id),
				Type: activitypub.NoteType,
			},
			Content:      "parent " + id,
			AttributedTo: "https://localhost/users/alice",
			Visibility:   visibility,
		},
	}))
}

func requireStructuredReachRefusal(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	appErr, ok := apperrors.AsAppError(err)
	require.True(t, ok, "reach refusals must retain a structured application error")
	require.Equal(t, apperrors.CodeUnprocessableEntity, appErr.Code)
}

func TestCreateNoteRejectsReplyReachWidening(t *testing.T) {
	tests := []struct {
		name             string
		parentVisibility string
	}{
		{name: "direct parent", parentVisibility: models.VisibilityDirect},
		{name: "followers-only parent", parentVisibility: models.VisibilityPrivate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, storageRepo := newRound12GraphResolver(t)
			seedMutationReachParent(t, storageRepo.Status(), "parent", tt.parentVisibility)
			parentID := "parent"

			_, err := resolver.Mutation().CreateNote(round12AuthContext("alice"), model.CreateNoteInput{
				Content:     "wider reply",
				InReplyToID: &parentID,
				Visibility:  model.VisibilityPublic,
			})
			requireStructuredReachRefusal(t, err)
		})
	}
}

func TestCreateNoteAcceptsEqualAndNarrowerReplyReach(t *testing.T) {
	tests := []struct {
		name             string
		parentVisibility string
		requested        model.Visibility
	}{
		{name: "equal followers-only", parentVisibility: models.VisibilityPrivate, requested: model.VisibilityFollowers},
		{name: "narrower followers-only", parentVisibility: models.VisibilityPublic, requested: model.VisibilityFollowers},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, storageRepo := newRound12GraphResolver(t)
			seedMutationReachParent(t, storageRepo.Status(), "parent", tt.parentVisibility)
			parentID := "parent"

			payload, err := resolver.Mutation().CreateNote(round12AuthContext("alice"), model.CreateNoteInput{
				Content:     "bounded reply",
				InReplyToID: &parentID,
				Visibility:  tt.requested,
			})
			require.NoError(t, err)
			require.NotNil(t, payload)
			require.NotNil(t, payload.Object)
			require.Equal(t, tt.requested, payload.Object.Visibility)
		})
	}
}

func TestCreateNoteRejectsQuoteReachWidening(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	seedMutationReachParent(t, storageRepo.Status(), "parent", models.VisibilityPrivate)
	quoteID := "parent"

	_, err := resolver.Mutation().CreateNote(round12AuthContext("alice"), model.CreateNoteInput{
		Content:    "wider quote",
		QuoteID:    &quoteID,
		Visibility: model.VisibilityPublic,
	})
	requireStructuredReachRefusal(t, err)
}

func TestCreateQuoteNoteInheritsReachAndRejectsWidening(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	seedMutationReachParent(t, storageRepo.Status(), "parent", models.VisibilityPrivate)

	_, err := resolver.Mutation().CreateQuoteNote(round12AuthContext("alice"), model.CreateQuoteNoteInput{
		Content:    "wider quote",
		QuoteURL:   "parent",
		Visibility: ptrVisibility(model.VisibilityPublic),
	})
	requireStructuredReachRefusal(t, err)

	payload, err := resolver.Mutation().CreateQuoteNote(round12AuthContext("alice"), model.CreateQuoteNoteInput{
		Content:  "inherited quote",
		QuoteURL: "parent",
	})
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.NotNil(t, payload.Object)
	require.Equal(t, model.VisibilityFollowers, payload.Object.Visibility)
}

func ptrVisibility(v model.Visibility) *model.Visibility { return &v }
