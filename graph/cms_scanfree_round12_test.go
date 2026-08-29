package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/config"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	storagemocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// cmsScanFreeDB fails the test if any request-adjacent read issues a full-table
// Scan, proving SeriesBySlug and MyPublications resolve through keyed paths.
type cmsScanFreeDB struct {
	*fakedb.Fake
	scanCalls int
}

func (s *cmsScanFreeDB) Scan(_ context.Context, _ *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	s.scanCalls++
	return nil, errors.New("full-table scan forbidden on request path")
}

type cmsScanFreeStorage struct {
	storagecore.RepositoryStorage
	db dynamormcore.DB
}

func (s *cmsScanFreeStorage) GetDB() dynamormcore.DB { return s.db }

func newCMSScanFreeStorage(t *testing.T, db dynamormcore.DB) *cmsScanFreeStorage {
	t.Helper()
	base := pkgtesting.NewMockRepositoryStorage(
		pkgtesting.WithSeriesRepository(storagemocks.NewMockSeriesRepository()),
		pkgtesting.WithPublicationRepository(inmemory.NewPublicationRepository()),
		pkgtesting.WithPublicationMemberRepository(inmemory.NewPublicationMemberRepository()),
	)
	return &cmsScanFreeStorage{RepositoryStorage: base, db: db}
}

// TestRound12CMS_SeriesBySlugAndMyPublications_NeverScan pins that the legacy
// scan fallbacks removed from SeriesBySlug and MyPublications are not
// re-introduced: both resolve through index/list paths only.
func TestRound12CMS_SeriesBySlugAndMyPublications_NeverScan(t *testing.T) {
	s := &cmsScanFreeDB{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, s)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.CMSSeriesSlugIndex{}))

	storage := newCMSScanFreeStorage(t, db)
	resolver := &Resolver{
		Config: &config.Config{
			Domain:                       "localhost",
			CMSLongFormPublishingEnabled: true,
			CMSSeriesEnabled:             true,
		},
		Storage: storage,
		Logger:  zap.NewNop(),
	}

	// A legacy series row with no slug-index entry is simply not found by slug:
	// the previous full-table scan fallback must not run.
	series, err := resolver.Query().SeriesBySlug(context.Background(), "legacy-slug")
	require.NoError(t, err)
	require.Nil(t, series)

	// MyPublications resolves solely through the GSI1 membership list; no DB
	// scan may backfill legacy rows.
	publications, err := resolver.Query().MyPublications(round12AuthContext("alice"))
	require.NoError(t, err)
	require.Empty(t, publications)

	require.Zero(t, s.scanCalls, "SeriesBySlug and MyPublications must never scan")
}
