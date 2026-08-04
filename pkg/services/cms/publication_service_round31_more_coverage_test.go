package cms

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

type publicationRepoWithGetErr struct {
	*fakePublicationRepo
	getErr error
}

func (r *publicationRepoWithGetErr) GetPublication(ctx context.Context, id string) (*models.Publication, error) {
	return nil, r.getErr
}

type trackingPublicationDeleteRepo struct {
	lastPK string
	lastSK string
}

func (r *trackingPublicationDeleteRepo) GetDB() dynamormcore.DB { return nil }

func (r *trackingPublicationDeleteRepo) CreatePublication(ctx context.Context, publication *models.Publication) error {
	return nil
}

func (r *trackingPublicationDeleteRepo) GetPublication(ctx context.Context, id string) (*models.Publication, error) {
	return nil, errors.New("not implemented")
}

func (r *trackingPublicationDeleteRepo) Update(ctx context.Context, publication *models.Publication) error {
	return nil
}

func (r *trackingPublicationDeleteRepo) Delete(ctx context.Context, pk, sk string) error {
	r.lastPK = pk
	r.lastSK = sk
	return nil
}

func TestPublicationService_UpdatePublication_Validations(t *testing.T) {
	t.Parallel()

	svc := NewPublicationService(&fakePublicationRepo{}, &fakePublicationMemberRepo{}, zap.NewNop())

	require.Error(t, svc.UpdatePublication(context.Background(), nil))
	require.Error(t, svc.UpdatePublication(context.Background(), &models.Publication{ID: "p1"}))
	require.Error(t, svc.UpdatePublication(context.Background(), &models.Publication{Slug: "slug"}))
}

func TestPublicationService_UpdatePublication_RollsBackSlugIndexOnUpdateFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)

	q.On("Create").Return(nil).Once() // slug index
	q.On("Delete").Return(nil).Once() // rollback

	pubRepo := &fakePublicationRepo{
		db:           db,
		publications: map[string]*models.Publication{},
		updateErr:    errors.New("update failed"),
	}

	svc := NewPublicationService(pubRepo, &fakePublicationMemberRepo{members: map[string]*models.PublicationMember{}}, zap.NewNop())
	require.Error(t, svc.UpdatePublication(ctx, &models.Publication{
		ID:   "p1",
		Slug: "slug-1",
		Name: "n",
	}))
}

func TestPublicationService_UpdatePublication_ReturnsLegacyLookupErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, _ := newCMSMockDB(t)

	pubRepo := &publicationRepoWithGetErr{
		fakePublicationRepo: &fakePublicationRepo{db: db, publications: map[string]*models.Publication{}},
		getErr:              errors.New("legacy lookup failed"),
	}

	svc := NewPublicationService(pubRepo, &fakePublicationMemberRepo{members: map[string]*models.PublicationMember{}}, zap.NewNop())
	require.Error(t, svc.UpdatePublication(ctx, &models.Publication{
		ID:   "https://example.com/publications/p1",
		Slug: "slug-1",
		Name: "n",
	}))
}

func TestPublicationService_UpdateMemberRole_ReturnsGetErrors(t *testing.T) {
	t.Parallel()

	memberRepo := &fakePublicationMemberRepo{
		members: map[string]*models.PublicationMember{},
		getErr:  errors.New("get member failed"),
	}
	svc := NewPublicationService(&fakePublicationRepo{}, memberRepo, zap.NewNop())

	require.Error(t, svc.UpdateMemberRole(context.Background(), "pub-1", "user-1", "editor"))
}

func TestPublicationService_DeletePublication_BuildsKeys(t *testing.T) {
	t.Parallel()

	repo := &trackingPublicationDeleteRepo{}
	svc := NewPublicationService(repo, &fakePublicationMemberRepo{}, zap.NewNop())

	require.NoError(t, svc.DeletePublication(context.Background(), "pub-1"))
	require.Equal(t, "PUBLICATION#pub-1", repo.lastPK)
	require.Equal(t, "METADATA", repo.lastSK)
}
