package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/equaltoai/lesser/pkg/activitypub"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorydb "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

func TestClassifyRemoteNoteMaterializationEvidence(t *testing.T) {
	t.Run("materialized", func(t *testing.T) {
		require.Equal(t, "materialized", classifyRemoteNoteMaterializationEvidence(remoteNoteMaterializationEvidence{
			ObjectRowFound:        true,
			CanonicalStatusFound:  true,
			URLIndexFound:         true,
			AuthorTimelineChecked: true,
			AuthorTimelineFound:   true,
		}))
	})

	t.Run("missing canonical status", func(t *testing.T) {
		require.Equal(t, "missing_canonical_status", classifyRemoteNoteMaterializationEvidence(remoteNoteMaterializationEvidence{
			ObjectRowFound:       true,
			CanonicalStatusFound: false,
		}))
	})

	t.Run("missing url and author timeline index", func(t *testing.T) {
		require.Equal(t, "missing_url_and_author_timeline_index", classifyRemoteNoteMaterializationEvidence(remoteNoteMaterializationEvidence{
			ObjectRowFound:        true,
			CanonicalStatusFound:  true,
			URLIndexFound:         false,
			AuthorTimelineChecked: true,
			AuthorTimelineFound:   false,
		}))
	})

	t.Run("status found but author timeline unchecked", func(t *testing.T) {
		require.Equal(t, "status_materialized_without_object_row", classifyRemoteNoteMaterializationEvidence(remoteNoteMaterializationEvidence{
			CanonicalStatusFound:  true,
			URLIndexFound:         true,
			AuthorTimelineChecked: false,
		}))
	})

	t.Run("missing inbox object", func(t *testing.T) {
		require.Equal(t, "missing_inbox_object", classifyRemoteNoteMaterializationEvidence(remoteNoteMaterializationEvidence{}))
	})

	t.Run("missing url index", func(t *testing.T) {
		require.Equal(t, "missing_url_index", classifyRemoteNoteMaterializationEvidence(remoteNoteMaterializationEvidence{
			ObjectRowFound:       true,
			CanonicalStatusFound: true,
		}))
	})

	t.Run("missing author timeline index", func(t *testing.T) {
		require.Equal(t, "missing_author_timeline_index", classifyRemoteNoteMaterializationEvidence(remoteNoteMaterializationEvidence{
			ObjectRowFound:        true,
			CanonicalStatusFound:  true,
			URLIndexFound:         true,
			AuthorTimelineChecked: true,
			AuthorTimelineFound:   false,
		}))
	})

}

func TestRunVerify_DispatchesRemoteNoteMaterialization(t *testing.T) {
	previous := runVerifyRemoteNoteMaterializationFn
	t.Cleanup(func() {
		runVerifyRemoteNoteMaterializationFn = previous
	})

	var called bool
	runVerifyRemoteNoteMaterializationFn = func(argv []string) error {
		called = true
		require.Equal(t, []string{"--note-url", "https://remote.example/users/bob/statuses/1"}, argv)
		return nil
	}

	require.NoError(t, runVerify([]string{"remote-note-materialization", "--note-url", "https://remote.example/users/bob/statuses/1"}))
	require.True(t, called)
}

func TestRunVerifyRemoteNoteMaterialization_ValidatesInput(t *testing.T) {
	require.Error(t, runVerifyRemoteNoteMaterialization([]string{"--bogus"}))
	require.ErrorContains(t, runVerifyRemoteNoteMaterialization(nil), "--note-url is required")
	require.Error(t, runVerifyRemoteNoteMaterialization([]string{"--note-url", "not-a-url"}))
}

func TestRunVerifyRemoteNoteMaterialization_Success(t *testing.T) {
	previousResolve := resolveCommonMigrationCLIOptionsFn
	previousRegister := registerDefaultTypeConvertersFn
	previousExecute := executeVerifyRemoteNoteMaterializationFn
	previousPrint := printRemoteNoteMaterializationEvidenceFn
	previousNewDB := tabletheoryNewFn
	t.Cleanup(func() {
		resolveCommonMigrationCLIOptionsFn = previousResolve
		registerDefaultTypeConvertersFn = previousRegister
		executeVerifyRemoteNoteMaterializationFn = previousExecute
		printRemoteNoteMaterializationEvidenceFn = previousPrint
		tabletheoryNewFn = previousNewDB
	})

	mockDB := new(mocks.MockDB)
	mockDB.On("Close").Return(nil).Maybe()

	resolveCommonMigrationCLIOptionsFn = func(context.Context, commonMigrationCLIOptions) (aws.Config, string, string, error) {
		return aws.Config{
			Region:      "us-east-1",
			Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("key", "secret", "")),
		}, "test-table", "Sim", nil
	}
	registerDefaultTypeConvertersFn = func(theorydb.DB) error { return nil }
	tabletheoryNewFn = func(cfg session.Config) (theorydb.DB, error) {
		require.Equal(t, "us-east-1", cfg.Region)
		return mockDB, nil
	}

	captured := remoteNoteMaterializationEvidence{}
	executeVerifyRemoteNoteMaterializationFn = func(_ context.Context, db theorydb.DB, tableName, noteURL, authorID string) (remoteNoteMaterializationEvidence, error) {
		require.Same(t, mockDB, db)
		require.Equal(t, "test-table", tableName)
		require.Equal(t, "https://remote.example/users/bob/statuses/1", noteURL)
		require.Equal(t, "https://remote.example/users/bob", authorID)
		return remoteNoteMaterializationEvidence{Classification: "materialized"}, nil
	}
	printRemoteNoteMaterializationEvidenceFn = func(evidence remoteNoteMaterializationEvidence) {
		captured = evidence
	}

	previousTable := storagemodels.MainTableName
	require.NoError(t, runVerifyRemoteNoteMaterialization([]string{
		"--note-url", "https://remote.example/users/bob/statuses/1",
		"--author-id", "https://remote.example/users/bob",
	}))
	require.Equal(t, previousTable, storagemodels.MainTableName)
	require.Equal(t, "test-table", captured.TableName)
	require.Equal(t, valueDev, captured.ResolvedStage)
	require.Equal(t, "Sim", captured.ResolvedProfile)
	require.Equal(t, "materialized", captured.Classification)
}

func TestRunVerifyRemoteNoteMaterialization_PropagatesSetupErrors(t *testing.T) {
	previousResolve := resolveCommonMigrationCLIOptionsFn
	previousRegister := registerDefaultTypeConvertersFn
	previousExecute := executeVerifyRemoteNoteMaterializationFn
	previousPrint := printRemoteNoteMaterializationEvidenceFn
	previousNewDB := tabletheoryNewFn
	t.Cleanup(func() {
		resolveCommonMigrationCLIOptionsFn = previousResolve
		registerDefaultTypeConvertersFn = previousRegister
		executeVerifyRemoteNoteMaterializationFn = previousExecute
		printRemoteNoteMaterializationEvidenceFn = previousPrint
		tabletheoryNewFn = previousNewDB
	})

	resolveCommonMigrationCLIOptionsFn = func(context.Context, commonMigrationCLIOptions) (aws.Config, string, string, error) {
		return aws.Config{}, "", "", errors.New("resolve failed")
	}
	require.EqualError(t, runVerifyRemoteNoteMaterialization([]string{"--note-url", "https://remote.example/users/bob/statuses/1"}), "resolve failed")

	resolveCommonMigrationCLIOptionsFn = func(context.Context, commonMigrationCLIOptions) (aws.Config, string, string, error) {
		return aws.Config{
			Region:      "us-east-1",
			Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("key", "secret", "")),
		}, "test-table", "Sim", nil
	}
	tabletheoryNewFn = func(session.Config) (theorydb.DB, error) {
		return nil, errors.New("db failed")
	}
	require.EqualError(t, runVerifyRemoteNoteMaterialization([]string{"--note-url", "https://remote.example/users/bob/statuses/1"}), "db failed")

	mockDB := new(mocks.MockDB)
	mockDB.On("Close").Return(nil).Maybe()
	tabletheoryNewFn = func(session.Config) (theorydb.DB, error) { return mockDB, nil }
	registerDefaultTypeConvertersFn = func(theorydb.DB) error { return errors.New("register failed") }
	require.EqualError(t, runVerifyRemoteNoteMaterialization([]string{"--note-url", "https://remote.example/users/bob/statuses/1"}), "register failed")

	registerDefaultTypeConvertersFn = func(theorydb.DB) error { return nil }
	executeVerifyRemoteNoteMaterializationFn = func(context.Context, theorydb.DB, string, string, string) (remoteNoteMaterializationEvidence, error) {
		return remoteNoteMaterializationEvidence{}, errors.New("execute failed")
	}
	printRemoteNoteMaterializationEvidenceFn = func(remoteNoteMaterializationEvidence) {
		t.Fatal("printer should not run when execute fails")
	}
	require.EqualError(t, runVerifyRemoteNoteMaterialization([]string{"--note-url", "https://remote.example/users/bob/statuses/1"}), "execute failed")
}

func TestExecuteVerifyRemoteNoteMaterialization_AggregatesEvidence(t *testing.T) {
	previousObject := fetchRemoteNoteObjectEvidenceFn
	previousStatus := fetchRemoteNoteCanonicalStatusEvidenceFn
	previousURL := fetchRemoteNoteURLIndexEvidenceFn
	previousTimeline := fetchRemoteNoteAuthorTimelineEvidenceFn
	t.Cleanup(func() {
		fetchRemoteNoteObjectEvidenceFn = previousObject
		fetchRemoteNoteCanonicalStatusEvidenceFn = previousStatus
		fetchRemoteNoteURLIndexEvidenceFn = previousURL
		fetchRemoteNoteAuthorTimelineEvidenceFn = previousTimeline
	})

	fetchRemoteNoteObjectEvidenceFn = func(context.Context, *repositories.ObjectRepository, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{Found: true, AuthorID: "https://remote.example/users/object", PublishedAt: 11}, nil
	}
	fetchRemoteNoteCanonicalStatusEvidenceFn = func(context.Context, *repositories.StatusRepository, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{Found: true, AuthorID: "https://remote.example/users/status", PublishedAt: 22}, nil
	}
	fetchRemoteNoteURLIndexEvidenceFn = func(context.Context, *repositories.StatusRepository, string, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{Found: true, AuthorID: "https://remote.example/users/url", PublishedAt: 33}, nil
	}
	fetchRemoteNoteAuthorTimelineEvidenceFn = func(_ context.Context, db theorydb.DB, authorID string, publishedAtUnix int64, canonicalStatusID string) (bool, error) {
		require.Nil(t, db)
		require.Equal(t, "https://remote.example/users/status", authorID)
		require.EqualValues(t, 22, publishedAtUnix)
		require.Equal(t, storagemodels.CanonicalStatusIDForDomain("https://remote.example/users/bob/statuses/1", ""), canonicalStatusID)
		return true, nil
	}

	evidence, err := executeVerifyRemoteNoteMaterialization(context.Background(), nil, "test-table", "https://remote.example/users/bob/statuses/1", "")
	require.NoError(t, err)
	require.Equal(t, "https://remote.example/users/status", evidence.AuthorID)
	require.True(t, evidence.AuthorTimelineChecked)
	require.True(t, evidence.AuthorTimelineFound)
	require.Equal(t, "materialized", evidence.Classification)
}

func TestExecuteVerifyRemoteNoteMaterialization_PropagatesFetcherErrors(t *testing.T) {
	previousObject := fetchRemoteNoteObjectEvidenceFn
	previousStatus := fetchRemoteNoteCanonicalStatusEvidenceFn
	previousURL := fetchRemoteNoteURLIndexEvidenceFn
	previousTimeline := fetchRemoteNoteAuthorTimelineEvidenceFn
	t.Cleanup(func() {
		fetchRemoteNoteObjectEvidenceFn = previousObject
		fetchRemoteNoteCanonicalStatusEvidenceFn = previousStatus
		fetchRemoteNoteURLIndexEvidenceFn = previousURL
		fetchRemoteNoteAuthorTimelineEvidenceFn = previousTimeline
	})

	fetchRemoteNoteObjectEvidenceFn = func(context.Context, *repositories.ObjectRepository, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{}, errors.New("object failed")
	}
	_, err := executeVerifyRemoteNoteMaterialization(context.Background(), nil, "test-table", "https://remote.example/users/bob/statuses/1", "")
	require.EqualError(t, err, "object failed")

	fetchRemoteNoteObjectEvidenceFn = func(context.Context, *repositories.ObjectRepository, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{Found: true}, nil
	}
	fetchRemoteNoteCanonicalStatusEvidenceFn = func(context.Context, *repositories.StatusRepository, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{}, errors.New("status failed")
	}
	_, err = executeVerifyRemoteNoteMaterialization(context.Background(), nil, "test-table", "https://remote.example/users/bob/statuses/1", "")
	require.EqualError(t, err, "status failed")

	fetchRemoteNoteCanonicalStatusEvidenceFn = func(context.Context, *repositories.StatusRepository, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{Found: true, AuthorID: "https://remote.example/users/status", PublishedAt: 22}, nil
	}
	fetchRemoteNoteURLIndexEvidenceFn = func(context.Context, *repositories.StatusRepository, string, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{}, errors.New("url failed")
	}
	_, err = executeVerifyRemoteNoteMaterialization(context.Background(), nil, "test-table", "https://remote.example/users/bob/statuses/1", "")
	require.EqualError(t, err, "url failed")

	fetchRemoteNoteCanonicalStatusEvidenceFn = func(context.Context, *repositories.StatusRepository, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{Found: true, AuthorID: "https://remote.example/users/status", PublishedAt: 22}, nil
	}
	fetchRemoteNoteURLIndexEvidenceFn = func(context.Context, *repositories.StatusRepository, string, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{Found: true}, nil
	}
	fetchRemoteNoteAuthorTimelineEvidenceFn = func(context.Context, theorydb.DB, string, int64, string) (bool, error) {
		return false, errors.New("timeline failed")
	}
	_, err = executeVerifyRemoteNoteMaterialization(context.Background(), nil, "test-table", "https://remote.example/users/bob/statuses/1", "")
	require.EqualError(t, err, "timeline failed")
}

func TestExecuteVerifyRemoteNoteMaterialization_SkipsTimelineWithoutPublishedAt(t *testing.T) {
	previousObject := fetchRemoteNoteObjectEvidenceFn
	previousStatus := fetchRemoteNoteCanonicalStatusEvidenceFn
	previousURL := fetchRemoteNoteURLIndexEvidenceFn
	previousTimeline := fetchRemoteNoteAuthorTimelineEvidenceFn
	t.Cleanup(func() {
		fetchRemoteNoteObjectEvidenceFn = previousObject
		fetchRemoteNoteCanonicalStatusEvidenceFn = previousStatus
		fetchRemoteNoteURLIndexEvidenceFn = previousURL
		fetchRemoteNoteAuthorTimelineEvidenceFn = previousTimeline
	})

	fetchRemoteNoteObjectEvidenceFn = func(context.Context, *repositories.ObjectRepository, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{Found: true, AuthorID: "https://remote.example/users/object"}, nil
	}
	fetchRemoteNoteCanonicalStatusEvidenceFn = func(context.Context, *repositories.StatusRepository, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{Found: true, AuthorID: "https://remote.example/users/status"}, nil
	}
	fetchRemoteNoteURLIndexEvidenceFn = func(context.Context, *repositories.StatusRepository, string, string) (remoteNoteEvidence, error) {
		return remoteNoteEvidence{Found: true}, nil
	}
	fetchRemoteNoteAuthorTimelineEvidenceFn = func(context.Context, theorydb.DB, string, int64, string) (bool, error) {
		t.Fatal("timeline lookup should not run without a published timestamp")
		return false, nil
	}

	evidence, err := executeVerifyRemoteNoteMaterialization(context.Background(), nil, "test-table", "https://remote.example/users/bob/statuses/1", "https://remote.example/users/supplied")
	require.NoError(t, err)
	require.Equal(t, "https://remote.example/users/supplied", evidence.AuthorID)
	require.False(t, evidence.AuthorTimelineChecked)
	require.Equal(t, "materialized", evidence.Classification)
}

func TestRemoteNoteMaterializationHelpers(t *testing.T) {
	require.True(t, isRemoteNoteVerificationNotFound(dynamormerrors.ErrItemNotFound))
	require.True(t, isRemoteNoteVerificationNotFound(errors.New("status not found")))
	require.False(t, isRemoteNoteVerificationNotFound(nil))
	require.False(t, isRemoteNoteVerificationNotFound(errors.New("boom")))

	require.Equal(t, "value", firstNonEmptyString("", "  ", "value", "other"))
	require.Equal(t, "", firstNonEmptyString("", "  "))
	require.EqualValues(t, 7, firstNonZeroInt64(0, 0, 7, 9))
	require.EqualValues(t, 0, firstNonZeroInt64(0, 0))

	r, w, err := os.Pipe()
	require.NoError(t, err)
	previousStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = previousStdout })

	printRemoteNoteMaterializationEvidence(remoteNoteMaterializationEvidence{
		TableName:             "test-table",
		ResolvedStage:         "dev",
		ResolvedProfile:       "Sim",
		NoteURL:               "https://remote.example/users/bob/statuses/1",
		CanonicalStatusID:     "remote_abc",
		AuthorID:              "https://remote.example/users/bob",
		ObjectRowFound:        true,
		CanonicalStatusFound:  true,
		URLIndexFound:         false,
		AuthorTimelineChecked: true,
		AuthorTimelineFound:   false,
		Classification:        "missing_url_and_author_timeline_index",
	})

	require.NoError(t, w.Close())
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	require.NoError(t, err)
	require.Contains(t, output.String(), "verify remote-note-materialization complete")
	require.Contains(t, output.String(), "classification: missing_url_and_author_timeline_index")
}

func TestFetchRemoteNoteObjectEvidence(t *testing.T) {
	previous := objectRepoGetObjectFn
	t.Cleanup(func() { objectRepoGetObjectFn = previous })

	objectRepoGetObjectFn = func(context.Context, *repositories.ObjectRepository, string) (any, error) {
		return nil, dynamormerrors.ErrItemNotFound
	}
	evidence, err := fetchRemoteNoteObjectEvidence(context.Background(), nil, "https://remote.example/users/bob/statuses/1")
	require.NoError(t, err)
	require.Equal(t, remoteNoteEvidence{}, evidence)

	objectRepoGetObjectFn = func(context.Context, *repositories.ObjectRepository, string) (any, error) {
		return &activitypub.Actor{}, nil
	}
	evidence, err = fetchRemoteNoteObjectEvidence(context.Background(), nil, "https://remote.example/users/bob/statuses/1")
	require.NoError(t, err)
	require.True(t, evidence.Found)

	objectRepoGetObjectFn = func(context.Context, *repositories.ObjectRepository, string) (any, error) {
		return nil, errors.New("boom")
	}
	_, err = fetchRemoteNoteObjectEvidence(context.Background(), nil, "https://remote.example/users/bob/statuses/1")
	require.EqualError(t, err, "boom")

	objectRepoGetObjectFn = func(context.Context, *repositories.ObjectRepository, string) (any, error) {
		return &activitypub.Note{AttributedTo: "https://remote.example/users/bob"}, nil
	}
	evidence, err = fetchRemoteNoteObjectEvidence(context.Background(), nil, "https://remote.example/users/bob/statuses/1")
	require.NoError(t, err)
	require.True(t, evidence.Found)
	require.Zero(t, evidence.PublishedAt)

	now := time.Now().UTC()
	objectRepoGetObjectFn = func(context.Context, *repositories.ObjectRepository, string) (any, error) {
		return &activitypub.Note{
			BaseObject:   activitypub.BaseObject{Published: &now},
			AttributedTo: "https://remote.example/users/bob",
		}, nil
	}
	evidence, err = fetchRemoteNoteObjectEvidence(context.Background(), nil, "https://remote.example/users/bob/statuses/1")
	require.NoError(t, err)
	require.True(t, evidence.Found)
	require.Equal(t, "https://remote.example/users/bob", evidence.AuthorID)
	require.EqualValues(t, now.Unix(), evidence.PublishedAt)
}

func TestFetchRemoteNoteCanonicalStatusEvidence(t *testing.T) {
	previous := statusRepoGetStatusFn
	t.Cleanup(func() { statusRepoGetStatusFn = previous })

	statusRepoGetStatusFn = func(context.Context, *repositories.StatusRepository, string) (*storagemodels.Status, error) {
		return nil, dynamormerrors.ErrItemNotFound
	}
	evidence, err := fetchRemoteNoteCanonicalStatusEvidence(context.Background(), nil, "remote_abc")
	require.NoError(t, err)
	require.Equal(t, remoteNoteEvidence{}, evidence)

	statusRepoGetStatusFn = func(context.Context, *repositories.StatusRepository, string) (*storagemodels.Status, error) {
		return nil, nil
	}
	evidence, err = fetchRemoteNoteCanonicalStatusEvidence(context.Background(), nil, "remote_abc")
	require.NoError(t, err)
	require.Equal(t, remoteNoteEvidence{}, evidence)

	now := time.Now().UTC()
	statusRepoGetStatusFn = func(context.Context, *repositories.StatusRepository, string) (*storagemodels.Status, error) {
		return &storagemodels.Status{StatusID: "remote_abc", AuthorID: "https://remote.example/users/bob", PublishedAt: now}, nil
	}
	evidence, err = fetchRemoteNoteCanonicalStatusEvidence(context.Background(), nil, "remote_abc")
	require.NoError(t, err)
	require.True(t, evidence.Found)
	require.Equal(t, "https://remote.example/users/bob", evidence.AuthorID)
	require.EqualValues(t, now.Unix(), evidence.PublishedAt)

	statusRepoGetStatusFn = func(context.Context, *repositories.StatusRepository, string) (*storagemodels.Status, error) {
		return nil, errors.New("boom")
	}
	_, err = fetchRemoteNoteCanonicalStatusEvidence(context.Background(), nil, "remote_abc")
	require.EqualError(t, err, "boom")
}

func TestFetchRemoteNoteURLIndexEvidence(t *testing.T) {
	previous := statusRepoGetStatusByURLFn
	t.Cleanup(func() { statusRepoGetStatusByURLFn = previous })

	statusRepoGetStatusByURLFn = func(context.Context, *repositories.StatusRepository, string) (*storagemodels.Status, error) {
		return nil, dynamormerrors.ErrItemNotFound
	}
	evidence, err := fetchRemoteNoteURLIndexEvidence(context.Background(), nil, "https://remote.example/users/bob/statuses/1", "remote_abc")
	require.NoError(t, err)
	require.Equal(t, remoteNoteEvidence{}, evidence)

	statusRepoGetStatusByURLFn = func(context.Context, *repositories.StatusRepository, string) (*storagemodels.Status, error) {
		return &storagemodels.Status{StatusID: "remote_other"}, nil
	}
	evidence, err = fetchRemoteNoteURLIndexEvidence(context.Background(), nil, "https://remote.example/users/bob/statuses/1", "remote_abc")
	require.NoError(t, err)
	require.Equal(t, remoteNoteEvidence{}, evidence)

	now := time.Now().UTC()
	statusRepoGetStatusByURLFn = func(context.Context, *repositories.StatusRepository, string) (*storagemodels.Status, error) {
		return &storagemodels.Status{StatusID: "remote_abc", AuthorID: "https://remote.example/users/bob", PublishedAt: now}, nil
	}
	evidence, err = fetchRemoteNoteURLIndexEvidence(context.Background(), nil, "https://remote.example/users/bob/statuses/1", "remote_abc")
	require.NoError(t, err)
	require.True(t, evidence.Found)
	require.EqualValues(t, now.Unix(), evidence.PublishedAt)

	statusRepoGetStatusByURLFn = func(context.Context, *repositories.StatusRepository, string) (*storagemodels.Status, error) {
		return nil, errors.New("boom")
	}
	_, err = fetchRemoteNoteURLIndexEvidence(context.Background(), nil, "https://remote.example/users/bob/statuses/1", "remote_abc")
	require.EqualError(t, err, "boom")
}

func TestFetchRemoteNoteAuthorTimelineEvidence(t *testing.T) {
	previous := loadRemoteNoteAuthorTimelineStatusFn
	t.Cleanup(func() { loadRemoteNoteAuthorTimelineStatusFn = previous })

	loadRemoteNoteAuthorTimelineStatusFn = func(context.Context, theorydb.DB, string, int64, string) (*storagemodels.Status, error) {
		return nil, dynamormerrors.ErrItemNotFound
	}
	found, err := fetchRemoteNoteAuthorTimelineEvidence(context.Background(), nil, "https://remote.example/users/bob", 22, "remote_abc")
	require.NoError(t, err)
	require.False(t, found)

	loadRemoteNoteAuthorTimelineStatusFn = func(context.Context, theorydb.DB, string, int64, string) (*storagemodels.Status, error) {
		return nil, errors.New("still not found")
	}
	found, err = fetchRemoteNoteAuthorTimelineEvidence(context.Background(), nil, "https://remote.example/users/bob", 22, "remote_abc")
	require.NoError(t, err)
	require.False(t, found)

	loadRemoteNoteAuthorTimelineStatusFn = func(context.Context, theorydb.DB, string, int64, string) (*storagemodels.Status, error) {
		return nil, errors.New("boom")
	}
	_, err = fetchRemoteNoteAuthorTimelineEvidence(context.Background(), nil, "https://remote.example/users/bob", 22, "remote_abc")
	require.EqualError(t, err, "boom")

	loadRemoteNoteAuthorTimelineStatusFn = func(context.Context, theorydb.DB, string, int64, string) (*storagemodels.Status, error) {
		return &storagemodels.Status{StatusID: "remote_other"}, nil
	}
	found, err = fetchRemoteNoteAuthorTimelineEvidence(context.Background(), nil, "https://remote.example/users/bob", 22, "remote_abc")
	require.NoError(t, err)
	require.False(t, found)

	loadRemoteNoteAuthorTimelineStatusFn = func(context.Context, theorydb.DB, string, int64, string) (*storagemodels.Status, error) {
		return &storagemodels.Status{StatusID: "remote_abc"}, nil
	}
	found, err = fetchRemoteNoteAuthorTimelineEvidence(context.Background(), nil, "https://remote.example/users/bob", 22, "remote_abc")
	require.NoError(t, err)
	require.True(t, found)
}

func TestLoadRemoteNoteAuthorTimelineStatus(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "AUTHOR#https://remote.example/users/bob").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "=", "22#remote_abc").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Status")).Run(func(args mock.Arguments) {
		status := args.Get(0).(*storagemodels.Status)
		status.StatusID = "remote_abc"
	}).Return(nil).Once()

	status, err := loadRemoteNoteAuthorTimelineStatus(context.Background(), mockDB, "https://remote.example/users/bob", 22, "remote_abc")
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, "remote_abc", status.StatusID)

	mockDB2 := new(mocks.MockDB)
	mockQuery2 := new(mocks.MockQuery)
	mockDB2.On("WithContext", mock.Anything).Return(mockDB2).Once()
	mockDB2.On("Model", mock.Anything).Return(mockQuery2).Once()
	mockQuery2.On("Index", "gsi1").Return(mockQuery2).Once()
	mockQuery2.On("Where", "gsi1PK", "=", "AUTHOR#https://remote.example/users/bob").Return(mockQuery2).Once()
	mockQuery2.On("Where", "gsi1SK", "=", "22#remote_abc").Return(mockQuery2).Once()
	mockQuery2.On("First", mock.AnythingOfType("*models.Status")).Return(errors.New("boom")).Once()

	status, err = loadRemoteNoteAuthorTimelineStatus(context.Background(), mockDB2, "https://remote.example/users/bob", 22, "remote_abc")
	require.EqualError(t, err, "boom")
	require.Nil(t, status)
}
