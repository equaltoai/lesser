package testing

import (
	"context"
	"errors"
	"testing"
	"time"

	lesserlift "github.com/equaltoai/lesser/pkg/lift"
	lifttesting "github.com/pay-theory/lift/pkg/testing"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/mock"
)

func TestDefaultIntegrationConfig_SetsSaneDefaults(t *testing.T) {
	t.Parallel()

	cfg := DefaultIntegrationConfig()
	require.Equal(t, "http://localhost:8000", cfg.DynamoDBEndpoint)
	require.Equal(t, "us-east-1", cfg.DynamoDBRegion)
	require.Equal(t, "lesser-test", cfg.TableName)
	require.False(t, cfg.SkipTableCreation)
	require.False(t, cfg.PreserveData)
	require.Equal(t, 30*time.Second, cfg.DefaultTimeout)
}

func TestIntegrationTestSuite_CleanupRunsInReverseOrder(t *testing.T) {
	t.Parallel()

	suite := NewIntegrationTestSuite(t, &IntegrationConfig{SkipTableCreation: true})

	calls := make([]string, 0, 2)
	suite.AddCleanup(func() { calls = append(calls, "first") })
	suite.AddCleanup(func() { calls = append(calls, "second") })

	suite.RunCleanup()
	require.Equal(t, []string{"second", "first"}, calls)
}

func TestIntegrationTestSuite_SetupTablesAndDropTables_NoPanic(t *testing.T) {
	t.Parallel()

	suite := NewIntegrationTestSuite(t, nil)
	require.NotNil(t, suite.Storage)
	require.NotNil(t, suite.Config)

	app := suite.CreateTestApp()
	require.NotNil(t, app)

	suite.RunCleanup()
}

func TestTestDataManager_CreatesAndCleansUpEntities(t *testing.T) {
	t.Parallel()

	storage := &MockStorage{}

	storage.On("CreateActor", mock.Anything, mock.AnythingOfType("*models.Actor")).Return(nil)
	storage.On("DeleteActor", mock.Anything, mock.AnythingOfType("string")).Return(nil)
	storage.On("CreateStatus", mock.Anything, mock.AnythingOfType("*models.Status")).Return(nil)
	storage.On("DeleteStatus", mock.Anything, mock.AnythingOfType("string")).Return(nil)
	storage.On("CreateActivity", mock.Anything, "alice", "Create").Return(nil)

	tdm := NewTestDataManager()
	ctx := context.Background()

	_, err := tdm.CreateTestActor(ctx, storage, "alice")
	require.NoError(t, err)
	_, err = tdm.CreateTestStatus(ctx, storage, "alice", "hello")
	require.NoError(t, err)
	_, err = tdm.CreateTestActivity(ctx, storage, "alice", "Create")
	require.NoError(t, err)

	require.NoError(t, tdm.Cleanup())
	require.Len(t, tdm.CreatedActors, 0)
	require.Len(t, tdm.CreatedStatuses, 0)

	storage.AssertExpectations(t)
}

func TestIntegrationAssertions_CoverHelpers(t *testing.T) {
	t.Parallel()

	AssertIntegrationResponse(t, &TestResponse{TestResponse: &lifttesting.TestResponse{StatusCode: 200, Body: `{"ok":true}`}}, 200)
	AssertPerformanceThresholds(t, 10*time.Millisecond, &IntegrationConfig{MaxResponseTime: 1 * time.Second})
}

func TestIntegrationHelpers_AreCallable(t *testing.T) {
	t.Parallel()

	require.NoError(t, WaitForDatabase("http://localhost:8000", 50*time.Millisecond))
	require.NoError(t, CreateTestTables(context.Background()))
	require.NoError(t, DropTestTables(context.Background()))
}

func TestTestEnvironmentManager_Defaults(t *testing.T) {
	t.Parallel()

	mgr := NewTestEnvironmentManager()
	require.NotNil(t, mgr)
	require.True(t, mgr.IsServiceReady("any"))
	require.Equal(t, "", mgr.GetServiceEndpoint("missing"))
	require.NoError(t, mgr.StartServices())
	require.NoError(t, mgr.StopServices())
}

func TestIntegrationTestSuite_RunWorkflow_WrapsErrorsByPhase(t *testing.T) {
	t.Parallel()

	suite := &IntegrationTestSuite{
		Storage:  &MockStorage{},
		TestData: NewTestDataManager(),
		Config:   DefaultIntegrationConfig(),
	}

	t.Run("setup failure", func(t *testing.T) {
		want := errors.New("setup")
		err := suite.RunWorkflow(&TestWorkflow{
			Setup: func(_ *IntegrationTestSuite) error { return want },
			Execute: func(_ *IntegrationTestSuite) (*TestResponse, error) {
				return &TestResponse{}, nil
			},
		})
		require.Error(t, err)
		require.ErrorIs(t, err, lesserlift.ErrTestSetupFailed)
		require.ErrorIs(t, err, want)
	})

	t.Run("execute failure", func(t *testing.T) {
		want := errors.New("execute")
		err := suite.RunWorkflow(&TestWorkflow{
			Execute: func(_ *IntegrationTestSuite) (*TestResponse, error) { return nil, want },
		})
		require.Error(t, err)
		require.ErrorIs(t, err, lesserlift.ErrIntegrationTestFailed)
		require.ErrorIs(t, err, want)
	})

	t.Run("validate failure", func(t *testing.T) {
		want := errors.New("validate")
		err := suite.RunWorkflow(&TestWorkflow{
			Execute:  func(_ *IntegrationTestSuite) (*TestResponse, error) { return &TestResponse{}, nil },
			Validate: func(_ *TestResponse) error { return want },
		})
		require.Error(t, err)
		require.ErrorIs(t, err, lesserlift.ErrTestValidationFailed)
		require.ErrorIs(t, err, want)
	})

	t.Run("cleanup failure", func(t *testing.T) {
		want := errors.New("cleanup")
		err := suite.RunWorkflow(&TestWorkflow{
			Execute: func(_ *IntegrationTestSuite) (*TestResponse, error) { return &TestResponse{}, nil },
			Cleanup: func(_ *IntegrationTestSuite) error { return want },
		})
		require.Error(t, err)
		require.ErrorIs(t, err, lesserlift.ErrTestCleanupFailed)
		require.ErrorIs(t, err, want)
	})

	t.Run("success", func(t *testing.T) {
		err := suite.RunWorkflow(&TestWorkflow{
			Setup:   func(_ *IntegrationTestSuite) error { return nil },
			Execute: func(_ *IntegrationTestSuite) (*TestResponse, error) { return &TestResponse{}, nil },
			Validate: func(_ *TestResponse) error {
				return nil
			},
			Cleanup: func(_ *IntegrationTestSuite) error { return nil },
		})
		require.NoError(t, err)
	})
}

func TestWorkflows_ValidateHelpers(t *testing.T) {
	t.Parallel()

	userWorkflow := CreateUserWorkflow("alice")
	require.Contains(t, userWorkflow.Name, "CreateUser")

	require.NoError(t, userWorkflow.Validate(&TestResponse{TestResponse: &lifttesting.TestResponse{StatusCode: 200, Body: `ok`}}))
	err := userWorkflow.Validate(&TestResponse{TestResponse: &lifttesting.TestResponse{StatusCode: 500, Body: `bad`}})
	require.Error(t, err)
	require.ErrorIs(t, err, lesserlift.ErrTestExpectedSuccess)

	followWorkflow := FollowWorkflow("a", "b")
	require.Contains(t, followWorkflow.Name, "Follow")

	require.NoError(t, followWorkflow.Validate(&TestResponse{TestResponse: &lifttesting.TestResponse{StatusCode: 200, Body: `ok`}}))
	err = followWorkflow.Validate(&TestResponse{TestResponse: &lifttesting.TestResponse{StatusCode: 201, Body: `bad`}})
	require.Error(t, err)
	require.ErrorIs(t, err, lesserlift.ErrTestUnexpectedStatusCode)
}

func TestWorkflows_ExecuteAndCleanup_AreCallable(t *testing.T) {
	t.Parallel()

	suite := &IntegrationTestSuite{
		Storage:  &MockStorage{},
		TestData: NewTestDataManager(),
		Config:   DefaultIntegrationConfig(),
	}

	userWorkflow := CreateUserWorkflow("alice")
	resp, err := userWorkflow.Execute(suite)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, userWorkflow.Cleanup(suite))

	followWorkflow := FollowWorkflow("alice", "bob")

	storage := suite.Storage
	storage.On("CreateActor", mock.Anything, mock.AnythingOfType("*models.Actor")).Return(nil).Twice()
	storage.On("DeleteActor", mock.Anything, mock.AnythingOfType("string")).Return(nil).Twice()

	require.NoError(t, followWorkflow.Setup(suite))
	resp, err = followWorkflow.Execute(suite)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, followWorkflow.Cleanup(suite))
}
