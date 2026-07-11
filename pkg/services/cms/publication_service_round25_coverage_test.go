package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"
)

type fakePublicationRepo struct {
	db dynamormcore.DB

	publications map[string]*models.Publication

	createErr error
	updateErr error
	deleteErr error
}

func (f *fakePublicationRepo) GetDB() dynamormcore.DB { return f.db }

func (f *fakePublicationRepo) CreatePublication(_ context.Context, publication *models.Publication) error {
	if f.createErr != nil {
		return f.createErr
	}
	if f.publications == nil {
		f.publications = map[string]*models.Publication{}
	}
	f.publications[publication.ID] = publication
	return nil
}

func (f *fakePublicationRepo) GetPublication(_ context.Context, id string) (*models.Publication, error) {
	if f.publications == nil {
		return nil, apperrors.ItemNotFoundWithID("publication", id)
	}
	p, ok := f.publications[id]
	if !ok {
		return nil, apperrors.ItemNotFoundWithID("publication", id)
	}
	return p, nil
}

func (f *fakePublicationRepo) Update(_ context.Context, publication *models.Publication) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.publications == nil {
		f.publications = map[string]*models.Publication{}
	}
	f.publications[publication.ID] = publication
	return nil
}

func (f *fakePublicationRepo) Delete(_ context.Context, _ string, _ string) error {
	return f.deleteErr
}

type fakePublicationMemberRepo struct {
	members map[string]*models.PublicationMember

	createErr error
	deleteErr error
	getErr    error
	updateErr error
	listErr   error
}

func memberKey(pubID, userID string) string { return pubID + ":" + userID }

func (f *fakePublicationMemberRepo) CreateMember(_ context.Context, member *models.PublicationMember) error {
	if f.createErr != nil {
		return f.createErr
	}
	if f.members == nil {
		f.members = map[string]*models.PublicationMember{}
	}
	f.members[memberKey(member.PublicationID, member.UserID)] = member
	return nil
}

func (f *fakePublicationMemberRepo) DeleteMember(_ context.Context, publicationID, userID string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.members, memberKey(publicationID, userID))
	return nil
}

func (f *fakePublicationMemberRepo) GetMember(_ context.Context, publicationID, userID string) (*models.PublicationMember, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	m, ok := f.members[memberKey(publicationID, userID)]
	if !ok {
		return nil, apperrors.ItemNotFoundWithID("publication member", memberKey(publicationID, userID))
	}
	return m, nil
}

func (f *fakePublicationMemberRepo) Update(_ context.Context, member *models.PublicationMember) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.members[memberKey(member.PublicationID, member.UserID)] = member
	return nil
}

func (f *fakePublicationMemberRepo) ListMembers(_ context.Context, publicationID string) ([]*models.PublicationMember, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*models.PublicationMember, 0)
	for _, m := range f.members {
		if m.PublicationID == publicationID {
			out = append(out, m)
		}
	}
	return out, nil
}

func TestPublicationService_Round25_CreateUpdateDeleteAndMembers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)

	pubRepo := &fakePublicationRepo{db: db, publications: map[string]*models.Publication{}}
	memberRepo := &fakePublicationMemberRepo{members: map[string]*models.PublicationMember{}}
	svc := NewPublicationService(pubRepo, memberRepo, zap.NewNop())

	t.Run("create validates", func(t *testing.T) {
		require.Error(t, svc.CreatePublication(ctx, nil))
		require.Error(t, svc.CreatePublication(ctx, &models.Publication{ID: "p1"}))
		require.Error(t, svc.CreatePublication(ctx, &models.Publication{Slug: "slug"}))
	})

	t.Run("create blocks legacy slug collision", func(t *testing.T) {
		slug := "slug"
		legacy := common.GenerateObjectID("example.com", "publications", slug)
		pubRepo.publications[legacy] = &models.Publication{ID: legacy, Slug: slug, Name: "legacy"}

		err := svc.CreatePublication(ctx, &models.Publication{ID: "https://example.com/publications/other", Slug: slug, Name: "name"})
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeAlreadyExists))
	})

	t.Run("create rolls back slug index on repo failure", func(t *testing.T) {
		db, q = newCMSMockDB(t)
		pubRepo.db = db
		q.On("Create").Return(nil).Once()
		q.On("Delete").Return(nil).Once()

		pubRepo.createErr = errors.New("create failed")
		err := svc.CreatePublication(ctx, &models.Publication{ID: "p2", Slug: "slug-2", Name: "name"})
		require.Error(t, err)
		pubRepo.createErr = nil
	})

	t.Run("update reserves slug index and persists", func(t *testing.T) {
		db, q = newCMSMockDB(t)
		pubRepo.db = db
		q.On("Create").Return(nil).Once()

		pubRepo.publications["p3"] = &models.Publication{ID: "p3", Slug: "old", Name: "name"}
		err := svc.UpdatePublication(ctx, &models.Publication{ID: "p3", Slug: "new", Name: "name"})
		require.NoError(t, err)
	})

	t.Run("delete issues repo delete", func(t *testing.T) {
		err := svc.DeletePublication(ctx, "p3")
		require.NoError(t, err)
	})

	t.Run("member operations", func(t *testing.T) {
		member := &models.PublicationMember{PublicationID: "p3", UserID: "u1", Role: "writer"}
		err := svc.AddMember(ctx, member)
		require.NoError(t, err)
		assert.False(t, member.JoinedAt.IsZero())
		assert.False(t, member.CreatedAt.IsZero())
		assert.False(t, member.UpdatedAt.IsZero())

		got, err := svc.GetMember(ctx, "p3", "u1")
		require.NoError(t, err)
		require.NotNil(t, got)

		before := got.UpdatedAt
		time.Sleep(1 * time.Millisecond)
		err = svc.UpdateMemberRole(ctx, "p3", "u1", "editor")
		require.NoError(t, err)
		got, _ = svc.GetMember(ctx, "p3", "u1")
		assert.Equal(t, "editor", got.Role)
		assert.True(t, got.UpdatedAt.After(before))

		list, err := svc.ListMembers(ctx, "p3")
		require.NoError(t, err)
		assert.Len(t, list, 1)

		err = svc.RemoveMember(ctx, "p3", "u1")
		require.NoError(t, err)
	})
}
