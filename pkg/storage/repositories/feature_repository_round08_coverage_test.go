package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestFeatureRepository_Round08_CreateGetUpdateListDeleteAndChecks(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Custom: return a deterministic feature for GetFeature / IsFeatureEnabled calls.
	getFeatures := []*Feature{
		// GetFeature("demo")
		{Name: "demo", Enabled: false, Percentage: 0},
		// EnableFeature("demo")
		{Name: "demo", Enabled: false, Percentage: 0},
		// DisableFeature("demo")
		{Name: "demo", Enabled: true, Percentage: 25},
		// AddUserGroup("existing", "admins") no-op
		{Name: "existing", Enabled: true, Percentage: 100, UserGroups: []string{"admins"}},
		// AddUserGroup("grouped", "admins") update
		{Name: "grouped", Enabled: true, Percentage: 100, UserGroups: []string{}},
		// IsFeatureEnabled("demo") -> disabled
		{Name: "demo", Enabled: false, Percentage: 0},
		// IsFeatureEnabled("rollout") -> 0%
		{Name: "rollout", Enabled: true, Percentage: 0},
		// IsFeatureEnabled("open") -> enabled
		{Name: "open", Enabled: true, Percentage: 100},
		// IsFeatureEnabled("grouped", "nonmatch") -> groups present, no match
		{Name: "grouped", Enabled: true, Percentage: 100, UserGroups: []string{"admins"}},
		// IsFeatureEnabled("grouped", "admins") -> match
		{Name: "grouped", Enabled: true, Percentage: 100, UserGroups: []string{"admins"}},
	}
	firstCall := 0
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*Feature)
		src := getFeatures[firstCall%len(getFeatures)]
		*dest = *src
		dest.PK = "FEATURE#" + dest.Name
		dest.SK = "config"
		firstCall++
	}).Return(nil)

	// Custom: return a deterministic list of features for ListFeatures -> Query -> All.
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*Feature)
		*dest = []*Feature{
			{Name: "a", Enabled: false},
			{Name: "b", Enabled: true},
		}
	}).Return(nil)

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewFeatureRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	created, err := repo.CreateFeature(ctx, "demo", "desc", "alice")
	require.NoError(t, err)
	require.Equal(t, "demo", created.Name)

	fetched, err := repo.GetFeature(ctx, "demo")
	require.NoError(t, err)
	require.Equal(t, "demo", fetched.Name)

	require.NoError(t, repo.EnableFeature(ctx, "demo", 25))
	require.NoError(t, repo.DisableFeature(ctx, "demo"))

	// AddUserGroup no-op when already present.
	require.NoError(t, repo.AddUserGroup(ctx, "existing", "admins"))

	// AddUserGroup adds + updates.
	require.NoError(t, repo.AddUserGroup(ctx, "grouped", "admins"))

	all, err := repo.ListFeatures(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)

	enabled, err := repo.ListEnabledFeatures(ctx)
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	require.True(t, enabled[0].Enabled)

	// IsFeatureEnabled branches.
	ok, err := repo.IsFeatureEnabled(ctx, "demo", "admins")
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = repo.IsFeatureEnabled(ctx, "rollout", "admins")
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = repo.IsFeatureEnabled(ctx, "open", "any")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = repo.IsFeatureEnabled(ctx, "grouped", "nonmatch")
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = repo.IsFeatureEnabled(ctx, "grouped", "admins")
	require.NoError(t, err)
	require.True(t, ok)

	count, err := repo.GetFeatureCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	require.NoError(t, repo.DeleteFeature(ctx, "demo"))
}

func TestFeatureRepository_Round08_CreateFeature_ErrorPath(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(errors.New("create failed")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewFeatureRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	_, err := repo.CreateFeature(ctx, "demo", "desc", "alice")
	require.Error(t, err)
}
