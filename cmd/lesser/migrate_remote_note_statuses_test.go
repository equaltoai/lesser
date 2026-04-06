package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

type fakeRemoteNoteStatusBackfillWriter struct {
	statuses   map[string]*models.Status
	getErrs    map[string]error
	createErrs map[string]error
}

func (f *fakeRemoteNoteStatusBackfillWriter) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
	if err := f.getErrs[statusID]; err != nil {
		return nil, err
	}
	if status, ok := f.statuses[statusID]; ok {
		return cloneStatus(status), nil
	}
	return nil, storage.ErrNotFound
}

func (f *fakeRemoteNoteStatusBackfillWriter) CreateStatus(_ context.Context, status *models.Status) error {
	if err := f.createErrs[status.StatusID]; err != nil {
		return err
	}

	cloned := cloneStatus(status)
	if err := cloned.BeforeCreate(); err != nil {
		return err
	}

	if f.statuses == nil {
		f.statuses = map[string]*models.Status{}
	}
	f.statuses[cloned.StatusID] = cloned
	return nil
}

func cloneStatus(status *models.Status) *models.Status {
	if status == nil {
		return nil
	}

	cloned := *status
	if status.Note != nil {
		noteCopy := *status.Note
		cloned.Note = &noteCopy
	}
	return &cloned
}

func TestRunMigrateRemoteNoteStatuses_PrintsDryRunSummary(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newRemoteNoteStatusMigrationClientFn
	previousOpenWriter := openRemoteNoteStatusBackfillWriterFn
	previousResolveDomain := resolveRemoteNoteStatusMigrationLocalDomainFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newRemoteNoteStatusMigrationClientFn = previousClientFactory
		openRemoteNoteStatusBackfillWriterFn = previousOpenWriter
		resolveRemoteNoteStatusMigrationLocalDomainFn = previousResolveDomain
	})

	now := time.Date(2026, 4, 6, 15, 4, 0, 0, time.UTC)
	client := &fakeUserKeyMigrationClient{
		queryOutputs: []*dynamodb.QueryOutput{{
			Items: []map[string]types.AttributeValue{
				remoteNoteObjectItem(t, "https://remote.example/users/bob/statuses/1", "https://remote.example/users/bob", "hello from remote", now, true, ""),
			},
		}},
	}
	writer := &fakeRemoteNoteStatusBackfillWriter{statuses: map[string]*models.Status{}}

	var seenProfile string
	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		seenProfile = awsProfile
		return aws.Config{}, "Sim", nil
	}
	newRemoteNoteStatusMigrationClientFn = func(aws.Config) remoteNoteStatusMigrationClient {
		return client
	}
	openRemoteNoteStatusBackfillWriterFn = func(aws.Config, string) (remoteNoteStatusBackfillWriter, func() error, error) {
		return writer, func() error { return nil }, nil
	}
	resolveRemoteNoteStatusMigrationLocalDomainFn = func(context.Context, aws.Config, string, string) (string, error) {
		return "https://dev.simulacrum.greater.website", nil
	}

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateRemoteNoteStatuses([]string{
			"--app", "simulacrum",
			"--env", "dev",
			"--aws-profile", "Sim",
		}))
	})

	require.Equal(t, "Sim", seenProfile)
	require.Contains(t, output, "migrate-remote-note-statuses dry-run complete")
	require.Contains(t, output, "table: simulacrum-dev-main-table")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "scanned_remote_objects: 1")
	require.Contains(t, output, "materializable_remote_objects: 1")
	require.Contains(t, output, "malformed_remote_objects: 0")
	require.Contains(t, output, "existing_statuses: 0")
	require.Contains(t, output, "missing_statuses: 1")
	require.Contains(t, output, "created_statuses: 0")
	require.Contains(t, output, "sample_object_ids:")
	require.Contains(t, output, "sample_status_ids:")
	require.Contains(t, output, "no writes performed; re-run with --apply")
	require.Len(t, client.queryInputs, 1)
	require.Equal(t, "gsi2", aws.ToString(client.queryInputs[0].IndexName))
	require.Equal(t, remoteNoteStatusBackfillTypePK, strAttr(t, client.queryInputs[0].ExpressionAttributeValues[":noteType"]))
	require.Nil(t, client.queryInputs[0].FilterExpression)
}

func TestRunMigrateRemoteNoteStatuses_ErrorPaths(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newRemoteNoteStatusMigrationClientFn
	previousOpenWriter := openRemoteNoteStatusBackfillWriterFn
	previousResolveDomain := resolveRemoteNoteStatusMigrationLocalDomainFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newRemoteNoteStatusMigrationClientFn = previousClientFactory
		openRemoteNoteStatusBackfillWriterFn = previousOpenWriter
		resolveRemoteNoteStatusMigrationLocalDomainFn = previousResolveDomain
	})

	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		return aws.Config{}, awsProfile, nil
	}
	newRemoteNoteStatusMigrationClientFn = func(aws.Config) remoteNoteStatusMigrationClient {
		return &fakeUserKeyMigrationClient{}
	}

	t.Run("domain resolution errors return immediately", func(t *testing.T) {
		resolveRemoteNoteStatusMigrationLocalDomainFn = func(context.Context, aws.Config, string, string) (string, error) {
			return "", errors.New("resolve domain failed")
		}

		err := runMigrateRemoteNoteStatuses([]string{"--app", "simulacrum", "--env", "dev"})
		require.EqualError(t, err, "resolve domain failed")
	})

	t.Run("writer open errors return immediately", func(t *testing.T) {
		resolveRemoteNoteStatusMigrationLocalDomainFn = func(context.Context, aws.Config, string, string) (string, error) {
			return "https://dev.simulacrum.greater.website", nil
		}
		openRemoteNoteStatusBackfillWriterFn = func(aws.Config, string) (remoteNoteStatusBackfillWriter, func() error, error) {
			return nil, nil, errors.New("open writer failed")
		}

		err := runMigrateRemoteNoteStatuses([]string{"--app", "simulacrum", "--env", "dev"})
		require.EqualError(t, err, "open writer failed")
	})
}

func TestRunMigrateRemoteNoteStatuses_InvalidCLIArgsFailEarly(t *testing.T) {
	err := runMigrateRemoteNoteStatuses([]string{"--limit", "not-a-number"})
	require.Error(t, err)
}

func TestRunMigrateRemoteNoteStatuses_ResolveOptionsAndExecuteErrors(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newRemoteNoteStatusMigrationClientFn
	previousOpenWriter := openRemoteNoteStatusBackfillWriterFn
	previousResolveDomain := resolveRemoteNoteStatusMigrationLocalDomainFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newRemoteNoteStatusMigrationClientFn = previousClientFactory
		openRemoteNoteStatusBackfillWriterFn = previousOpenWriter
		resolveRemoteNoteStatusMigrationLocalDomainFn = previousResolveDomain
	})

	t.Run("common option resolution errors return immediately", func(t *testing.T) {
		loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
			return aws.Config{}, "", errors.New("load aws failed")
		}

		err := runMigrateRemoteNoteStatuses([]string{"--app", "simulacrum", "--env", "dev"})
		require.EqualError(t, err, "load aws failed")
	})

	t.Run("execute errors return after writer open and close", func(t *testing.T) {
		closed := false
		loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
			return aws.Config{}, awsProfile, nil
		}
		resolveRemoteNoteStatusMigrationLocalDomainFn = func(context.Context, aws.Config, string, string) (string, error) {
			return "https://dev.simulacrum.greater.website", nil
		}
		openRemoteNoteStatusBackfillWriterFn = func(aws.Config, string) (remoteNoteStatusBackfillWriter, func() error, error) {
			return &fakeRemoteNoteStatusBackfillWriter{statuses: map[string]*models.Status{}}, func() error {
				closed = true
				return nil
			}, nil
		}
		newRemoteNoteStatusMigrationClientFn = func(aws.Config) remoteNoteStatusMigrationClient {
			return &fakeUserKeyMigrationClient{queryErr: errors.New("query failed")}
		}

		err := runMigrateRemoteNoteStatuses([]string{"--app", "simulacrum", "--env", "dev"})
		require.ErrorContains(t, err, "query remote note object rows")
		require.True(t, closed)
	})
}

func TestExecuteRemoteNoteStatusMigration_ApplyCreatesMissingStatusesAndClassifiesMalformedRows(t *testing.T) {
	now := time.Date(2026, 4, 6, 15, 4, 0, 0, time.UTC)
	localDomain := "https://dev.simulacrum.greater.website"
	firstRemoteURL := "https://remote.example/users/bob/statuses/1"
	secondRemoteURL := "https://remote.example/users/bob/statuses/2"
	parentRemoteURL := "https://another.remote/users/alice/statuses/root"
	existingStatusID := models.CanonicalStatusIDForDomain(secondRemoteURL, localDomain)

	client := &fakeUserKeyMigrationClient{
		queryOutputs: []*dynamodb.QueryOutput{{
			Items: []map[string]types.AttributeValue{
				remoteNoteObjectItem(t, firstRemoteURL, "https://remote.example/users/bob", "hello from remote", now, true, parentRemoteURL),
				remoteNoteObjectItem(t, secondRemoteURL, "https://remote.example/users/bob", "already materialized", now.Add(time.Minute), true, ""),
				remoteNoteMalformedObjectItem(t, "https://remote.example/users/bob/statuses/bad", now.Add(2*time.Minute)),
			},
		}},
	}
	writer := &fakeRemoteNoteStatusBackfillWriter{
		statuses: map[string]*models.Status{
			existingStatusID: {StatusID: existingStatusID},
		},
	}

	summary, err := executeRemoteNoteStatusMigration(context.Background(), client, writer, "simulacrum-dev-main-table", localDomain, true, 0)
	require.NoError(t, err)
	require.Equal(t, 3, summary.ScannedRemoteObjects)
	require.Equal(t, 2, summary.MaterializableObjects)
	require.Equal(t, 1, summary.MalformedRemoteObjects)
	require.Equal(t, 1, summary.ExistingStatuses)
	require.Equal(t, 1, summary.MissingStatuses)
	require.Equal(t, 1, summary.CreatedStatuses)
	require.Contains(t, summary.SampleObjectIDs, firstRemoteURL)
	require.Contains(t, summary.SampleStatusIDs, models.CanonicalStatusIDForDomain(firstRemoteURL, localDomain))
	require.Contains(t, summary.SampleMalformedObjectIDs, "https://remote.example/users/bob/statuses/bad")

	created := writer.statuses[models.CanonicalStatusIDForDomain(firstRemoteURL, localDomain)]
	require.NotNil(t, created)
	require.Equal(t, models.CanonicalStatusIDForDomain(firstRemoteURL, localDomain), created.StatusID)
	require.Equal(t, "https://remote.example/users/bob", created.AuthorID)
	require.Equal(t, "bob@remote.example", created.AuthorUsername)
	require.Equal(t, models.VisibilityPublic, created.Visibility)
	require.Equal(t, models.CanonicalStatusIDForDomain(parentRemoteURL, localDomain), created.InReplyToID)
	require.Equal(t, []string{firstRemoteURL}, created.URLs)
	require.Equal(t, firstRemoteURL, created.Note.ID)
}

func TestExecuteRemoteNoteStatusMigration_FindsHistoricalRemoteRowsWithoutIsRemoteFlag(t *testing.T) {
	now := time.Date(2026, 4, 6, 15, 4, 0, 0, time.UTC)
	localDomain := "https://dev.simulacrum.greater.website"
	remoteURL := "https://remote.example/users/bob/statuses/3"
	localURL := "https://dev.simulacrum.greater.website/users/alice/statuses/3"

	remoteItem := remoteNoteObjectItem(t, remoteURL, "https://remote.example/users/bob", "backfill me", now, false, "")
	delete(remoteItem, "isRemote")

	localItem := remoteNoteObjectItem(t, localURL, "https://dev.simulacrum.greater.website/users/alice", "skip me", now.Add(time.Minute), false, "")
	delete(localItem, "isRemote")

	client := &fakeUserKeyMigrationClient{
		queryOutputs: []*dynamodb.QueryOutput{{
			Items: []map[string]types.AttributeValue{remoteItem, localItem},
		}},
	}
	writer := &fakeRemoteNoteStatusBackfillWriter{statuses: map[string]*models.Status{}}

	summary, err := executeRemoteNoteStatusMigration(context.Background(), client, writer, "simulacrum-dev-main-table", localDomain, true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedRemoteObjects)
	require.Equal(t, 1, summary.MaterializableObjects)
	require.Equal(t, 1, summary.MissingStatuses)
	require.Equal(t, 1, summary.CreatedStatuses)
	require.NotContains(t, summary.SampleObjectIDs, localURL)

	created := writer.statuses[models.CanonicalStatusIDForDomain(remoteURL, localDomain)]
	require.NotNil(t, created)
	require.Equal(t, remoteURL, created.Note.ID)
	require.Nil(t, writer.statuses[models.CanonicalStatusIDForDomain(localURL, localDomain)])
}

func TestExecuteRemoteNoteStatusMigration_ValidatesInputsAndTreatsConditionFailuresAsIdempotent(t *testing.T) {
	now := time.Date(2026, 4, 6, 15, 4, 0, 0, time.UTC)
	localDomain := "https://dev.simulacrum.greater.website"
	remoteURL := "https://remote.example/users/bob/statuses/1"
	statusID := models.CanonicalStatusIDForDomain(remoteURL, localDomain)

	_, err := executeRemoteNoteStatusMigration(context.Background(), nil, &fakeRemoteNoteStatusBackfillWriter{}, "table", localDomain, false, 0)
	require.EqualError(t, err, "migration client is required")

	_, err = executeRemoteNoteStatusMigration(context.Background(), &fakeUserKeyMigrationClient{}, nil, "table", localDomain, false, 0)
	require.EqualError(t, err, "status backfill writer is required")

	_, err = executeRemoteNoteStatusMigration(context.Background(), &fakeUserKeyMigrationClient{}, &fakeRemoteNoteStatusBackfillWriter{}, "   ", localDomain, false, 0)
	require.EqualError(t, err, "table name is required")

	writer := &fakeRemoteNoteStatusBackfillWriter{
		statuses:   map[string]*models.Status{},
		createErrs: map[string]error{statusID: dynamormerrors.ErrConditionFailed},
	}
	client := &fakeUserKeyMigrationClient{
		queryOutputs: []*dynamodb.QueryOutput{{
			Items: []map[string]types.AttributeValue{
				remoteNoteObjectItem(t, remoteURL, "https://remote.example/users/bob", "race-safe", now, true, ""),
			},
		}},
	}

	summary, err := executeRemoteNoteStatusMigration(context.Background(), client, writer, "simulacrum-dev-main-table", localDomain, true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedRemoteObjects)
	require.Equal(t, 1, summary.MaterializableObjects)
	require.Equal(t, 1, summary.MissingStatuses)
	require.Equal(t, 0, summary.CreatedStatuses)
}

func TestExecuteRemoteNoteStatusMigration_QueryPaginationAndErrors(t *testing.T) {
	localDomain := "https://dev.simulacrum.greater.website"
	now := time.Date(2026, 4, 6, 15, 30, 0, 0, time.UTC)

	t.Run("query errors bubble up", func(t *testing.T) {
		client := &fakeUserKeyMigrationClient{queryErr: errors.New("query failed")}
		_, err := executeRemoteNoteStatusMigration(context.Background(), client, &fakeRemoteNoteStatusBackfillWriter{}, "simulacrum-dev-main-table", localDomain, false, 0)
		require.ErrorContains(t, err, "query remote note object rows")
	})

	t.Run("pagination carries forward the exclusive start key", func(t *testing.T) {
		client := &fakeUserKeyMigrationClient{
			queryOutputs: []*dynamodb.QueryOutput{
				{
					Items: []map[string]types.AttributeValue{
						remoteNoteObjectItem(t, "https://remote.example/users/bob/statuses/4", "https://remote.example/users/bob", "page one", now, true, ""),
					},
					LastEvaluatedKey: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "next"},
					},
				},
				{
					Items: []map[string]types.AttributeValue{
						remoteNoteObjectItem(t, "https://remote.example/users/bob/statuses/5", "https://remote.example/users/bob", "page two", now.Add(time.Minute), true, ""),
					},
				},
			},
		}
		writer := &fakeRemoteNoteStatusBackfillWriter{statuses: map[string]*models.Status{}}

		summary, err := executeRemoteNoteStatusMigration(context.Background(), client, writer, "simulacrum-dev-main-table", localDomain, false, 0)
		require.NoError(t, err)
		require.Equal(t, 2, summary.ScannedRemoteObjects)
		require.Len(t, client.queryInputs, 2)
		require.Equal(t, "next", strAttr(t, client.queryInputs[1].ExclusiveStartKey["PK"]))
	})

	t.Run("limit stops pagination before following the cursor", func(t *testing.T) {
		client := &fakeUserKeyMigrationClient{
			queryOutputs: []*dynamodb.QueryOutput{
				{
					Items: []map[string]types.AttributeValue{
						remoteNoteObjectItem(t, "https://remote.example/users/bob/statuses/limit-stop", "https://remote.example/users/bob", "page one", now, true, ""),
					},
					LastEvaluatedKey: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "next"},
					},
				},
				{
					Items: []map[string]types.AttributeValue{
						remoteNoteObjectItem(t, "https://remote.example/users/bob/statuses/should-not-be-read", "https://remote.example/users/bob", "page two", now.Add(time.Minute), true, ""),
					},
				},
			},
		}
		writer := &fakeRemoteNoteStatusBackfillWriter{statuses: map[string]*models.Status{}}

		summary, err := executeRemoteNoteStatusMigration(context.Background(), client, writer, "simulacrum-dev-main-table", localDomain, false, 1)
		require.NoError(t, err)
		require.Equal(t, 1, summary.ScannedRemoteObjects)
		require.Equal(t, 1, summary.MaterializableObjects)
		require.Len(t, client.queryInputs, 1)
	})
}

func TestRemoteNoteStatusMigration_HelperCoverage(t *testing.T) {
	require.NotNil(t, newRemoteNoteStatusBackfillWriter(nil, "table"))

	t.Run("record malformed items and enforce sample limits", func(t *testing.T) {
		summary := &remoteNoteStatusMigrationSummary{}
		recordRemoteNoteStatusMalformedItem(nil, "ignored")
		recordRemoteNoteStatusMalformedItem(summary, "https://remote.example/users/alice/statuses/1")
		require.Equal(t, 1, summary.ScannedRemoteObjects)
		require.Equal(t, 1, summary.MalformedRemoteObjects)
		require.Equal(t, []string{"https://remote.example/users/alice/statuses/1"}, summary.SampleMalformedObjectIDs)

		samples := []string{}
		appendRemoteNoteStatusMigrationSample(nil, "ignored")
		for _, value := range []string{"a", "b", "b", "c", "d", "e", "f"} {
			appendRemoteNoteStatusMigrationSample(&samples, value)
		}
		require.Equal(t, []string{"a", "b", "c", "d", "e"}, samples)
	})

	t.Run("limit and not found helpers", func(t *testing.T) {
		require.False(t, remoteNoteStatusMigrationLimitReached(0, &remoteNoteStatusMigrationSummary{ScannedRemoteObjects: 9}))
		require.False(t, remoteNoteStatusMigrationLimitReached(1, nil))
		require.True(t, remoteNoteStatusMigrationLimitReached(1, &remoteNoteStatusMigrationSummary{ScannedRemoteObjects: 1}))

		require.False(t, isRemoteNoteStatusBackfillNotFound(nil))
		require.True(t, isRemoteNoteStatusBackfillNotFound(storage.ErrNotFound))
		require.True(t, isRemoteNoteStatusBackfillNotFound(errors.New("status not found in table")))
		require.False(t, isRemoteNoteStatusBackfillNotFound(errors.New("boom")))
	})

	t.Run("remote identifier helpers normalize hosts and read raw flags", func(t *testing.T) {
		require.True(t, rawRemoteNoteStatusItemIsRemote(map[string]types.AttributeValue{
			"isRemote": &types.AttributeValueMemberBOOL{Value: true},
		}))
		require.False(t, rawRemoteNoteStatusItemIsRemote(map[string]types.AttributeValue{
			"isRemote": &types.AttributeValueMemberS{Value: "true"},
		}))
		require.False(t, rawRemoteNoteStatusItemIsRemote(nil))

		require.True(t, isRemoteNoteStatusBackfillIdentifier("https://remote.example/users/bob/statuses/1", "https://local.example"))
		require.False(t, isRemoteNoteStatusBackfillIdentifier("https://local.example/users/alice/statuses/1", "https://local.example"))
		require.Equal(t, "remote.example", normalizeRemoteNoteStatusBackfillHost(" HTTPS://Remote.Example:443/users/bob "))
		require.Equal(t, "remote.example", normalizeRemoteNoteStatusBackfillHost("remote.example/path"))
		require.Equal(t, "", normalizeRemoteNoteStatusBackfillHost(""))
	})

	t.Run("sample id prefers object id then item fields", func(t *testing.T) {
		object := &models.Object{ID: "https://remote.example/users/bob/statuses/1"}
		require.Equal(t, object.ID, remoteNoteStatusObjectSampleID(nil, object))
		require.Equal(t, "https://remote.example/users/bob/statuses/2", remoteNoteStatusObjectSampleID(map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "https://remote.example/users/bob/statuses/2"},
		}, nil))
		require.Equal(t, "https://remote.example/users/bob/statuses/3", remoteNoteStatusObjectSampleID(map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "object#https://remote.example/users/bob/statuses/3"},
		}, nil))
		require.Equal(t, "", remoteNoteStatusObjectSampleID(nil, nil))
	})
}

func TestRemoteNoteStatusMigration_DecodeAndProcessHelpers(t *testing.T) {
	localDomain := "https://local.example"
	now := time.Date(2026, 4, 6, 16, 0, 0, 0, time.UTC)
	remoteItem := remoteNoteObjectItem(t, "https://remote.example/users/bob/statuses/1", "https://remote.example/users/bob", "hello", now, false, "")
	localItem := remoteNoteObjectItem(t, "https://local.example/users/alice/statuses/1", "https://local.example/users/alice", "hello", now, false, "")

	object, ok := decodeRemoteNoteStatusMigrationObject(remoteItem, localDomain)
	require.True(t, ok)
	require.NotNil(t, object)

	object, ok = decodeRemoteNoteStatusMigrationObject(localItem, localDomain)
	require.False(t, ok)
	require.Nil(t, object)

	malformedRemote := map[string]types.AttributeValue{
		"PK":       &types.AttributeValueMemberS{Value: "object#https://remote.example/users/bob/statuses/bad"},
		"isRemote": &types.AttributeValueMemberBOOL{Value: true},
		"id":       &types.AttributeValueMemberBOOL{Value: true},
	}
	object, ok = decodeRemoteNoteStatusMigrationObject(malformedRemote, localDomain)
	require.True(t, ok)
	require.Nil(t, object)

	malformedLocal := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "object#https://local.example/users/alice/statuses/bad"},
		"id": &types.AttributeValueMemberBOOL{Value: true},
	}
	object, ok = decodeRemoteNoteStatusMigrationObject(malformedLocal, localDomain)
	require.False(t, ok)
	require.Nil(t, object)

	writer := &fakeRemoteNoteStatusBackfillWriter{statuses: map[string]*models.Status{}}
	summary := &remoteNoteStatusMigrationSummary{}
	stop, err := processRemoteNoteStatusMigrationPage(context.Background(), writer, []map[string]types.AttributeValue{remoteItem, localItem}, localDomain, false, 1, summary)
	require.NoError(t, err)
	require.True(t, stop)
	require.Equal(t, 1, summary.ScannedRemoteObjects)
}

func TestApplyRemoteNoteStatusBackfillCandidate_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	localDomain := "https://local.example"
	object := &models.Object{
		ID:           "https://remote.example/users/bob/statuses/9",
		Type:         activitypub.NoteType,
		AttributedTo: "https://remote.example/users/bob",
		Content:      "hello",
	}
	status, ok := buildRemoteNoteStatusBackfillStatus(object, localDomain)
	require.True(t, ok)
	require.NotNil(t, status)

	t.Run("load errors bubble up", func(t *testing.T) {
		writer := &fakeRemoteNoteStatusBackfillWriter{
			statuses: map[string]*models.Status{},
			getErrs:  map[string]error{status.StatusID: errors.New("boom")},
		}
		summary := &remoteNoteStatusMigrationSummary{}
		err := applyRemoteNoteStatusBackfillCandidate(ctx, writer, object, status, true, summary)
		require.ErrorContains(t, err, "load canonical status")
	})

	t.Run("dry run marks missing without writing", func(t *testing.T) {
		writer := &fakeRemoteNoteStatusBackfillWriter{statuses: map[string]*models.Status{}}
		summary := &remoteNoteStatusMigrationSummary{}
		err := applyRemoteNoteStatusBackfillCandidate(ctx, writer, object, status, false, summary)
		require.NoError(t, err)
		require.Equal(t, 1, summary.MissingStatuses)
		require.Equal(t, 0, summary.CreatedStatuses)
	})

	t.Run("create errors bubble up", func(t *testing.T) {
		writer := &fakeRemoteNoteStatusBackfillWriter{
			statuses:   map[string]*models.Status{},
			createErrs: map[string]error{status.StatusID: errors.New("boom")},
		}
		summary := &remoteNoteStatusMigrationSummary{}
		err := applyRemoteNoteStatusBackfillCandidate(ctx, writer, object, status, true, summary)
		require.ErrorContains(t, err, "create canonical status")
	})
}

func TestBuildRemoteNoteStatusBackfillStatusAndSelectionHelpers(t *testing.T) {
	localDomain := "https://local.example"

	require.False(t, shouldBackfillRemoteNoteStatusObject(nil, localDomain))
	require.False(t, shouldBackfillRemoteNoteStatusObject(&models.Object{Type: activitypub.ArticleType}, localDomain))
	require.False(t, shouldBackfillRemoteNoteStatusObject(&models.Object{
		ID:           "https://local.example/users/alice/statuses/1",
		Type:         activitypub.NoteType,
		AttributedTo: "https://local.example/users/alice",
	}, localDomain))
	require.True(t, shouldBackfillRemoteNoteStatusObject(&models.Object{
		ID:           "https://remote.example/users/bob/statuses/1",
		Type:         activitypub.NoteType,
		AttributedTo: "https://remote.example/users/bob",
	}, localDomain))
	require.False(t, isRemoteNoteStatusBackfillIdentifier("https://remote.example/users/bob/statuses/1", ""))

	status, ok := buildRemoteNoteStatusBackfillStatus(&models.Object{
		ID:             "https://remote.example/users/bob/statuses/10",
		Type:           activitypub.NoteType,
		AttributedTo:   "https://remote.example/users/bob",
		Content:        "hello",
		ConversationID: "conv-10",
		Visibility:     models.VisibilityDirect,
		To:             []string{activitypub.PublicAddress},
	}, localDomain)
	require.True(t, ok)
	require.NotNil(t, status)
	require.Equal(t, "conv-10", status.Note.ConversationID)
	require.Equal(t, models.VisibilityDirect, status.Note.Visibility)

	status, ok = buildRemoteNoteStatusBackfillStatus(&models.Object{
		Type:         activitypub.NoteType,
		AttributedTo: "https://remote.example/users/bob",
	}, localDomain)
	require.False(t, ok)
	require.Nil(t, status)

	status, ok = buildRemoteNoteStatusBackfillStatus(&models.Object{
		ID:   "https://remote.example/users/bob/statuses/11",
		Type: activitypub.NoteType,
	}, localDomain)
	require.False(t, ok)
	require.Nil(t, status)
}

func TestProcessRemoteNoteStatusMigrationItem_CoversMalformedAndWriterErrors(t *testing.T) {
	ctx := context.Background()
	localDomain := "https://local.example"
	now := time.Date(2026, 4, 6, 17, 0, 0, 0, time.UTC)

	t.Run("remote object that cannot materialize is counted as malformed", func(t *testing.T) {
		item := remoteNoteObjectItem(t, "https://remote.example/users/bob/statuses/malformed", "", "hello", now, true, "")
		summary := &remoteNoteStatusMigrationSummary{}
		writer := &fakeRemoteNoteStatusBackfillWriter{statuses: map[string]*models.Status{}}

		stop, err := processRemoteNoteStatusMigrationItem(ctx, writer, item, localDomain, false, 0, summary)
		require.NoError(t, err)
		require.False(t, stop)
		require.Equal(t, 1, summary.ScannedRemoteObjects)
		require.Equal(t, 1, summary.MalformedRemoteObjects)
		require.Contains(t, summary.SampleObjectIDs, "https://remote.example/users/bob/statuses/malformed")
		require.Contains(t, summary.SampleMalformedObjectIDs, "https://remote.example/users/bob/statuses/malformed")
	})

	t.Run("malformed remote row can trip the limit via object nil path", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"PK":       &types.AttributeValueMemberS{Value: "object#https://remote.example/users/bob/statuses/decode-bad"},
			"isRemote": &types.AttributeValueMemberBOOL{Value: true},
			"id":       &types.AttributeValueMemberBOOL{Value: true},
		}
		summary := &remoteNoteStatusMigrationSummary{}
		writer := &fakeRemoteNoteStatusBackfillWriter{statuses: map[string]*models.Status{}}

		stop, err := processRemoteNoteStatusMigrationItem(ctx, writer, item, localDomain, false, 1, summary)
		require.NoError(t, err)
		require.True(t, stop)
		require.Equal(t, 1, summary.ScannedRemoteObjects)
		require.Equal(t, 1, summary.MalformedRemoteObjects)
	})

	t.Run("writer lookup errors are returned", func(t *testing.T) {
		item := remoteNoteObjectItem(t, "https://remote.example/users/bob/statuses/error", "https://remote.example/users/bob", "hello", now, true, "")
		statusID := models.CanonicalStatusIDForDomain("https://remote.example/users/bob/statuses/error", localDomain)
		summary := &remoteNoteStatusMigrationSummary{}
		writer := &fakeRemoteNoteStatusBackfillWriter{
			statuses: map[string]*models.Status{},
			getErrs:  map[string]error{statusID: errors.New("boom")},
		}

		stop, err := processRemoteNoteStatusMigrationItem(ctx, writer, item, localDomain, true, 0, summary)
		require.ErrorContains(t, err, "load canonical status")
		require.False(t, stop)
	})

	t.Run("limit reached on a successful item returns stop true", func(t *testing.T) {
		item := remoteNoteObjectItem(t, "https://remote.example/users/bob/statuses/stop", "https://remote.example/users/bob", "hello", now, true, "")
		summary := &remoteNoteStatusMigrationSummary{}
		writer := &fakeRemoteNoteStatusBackfillWriter{statuses: map[string]*models.Status{}}

		stop, err := processRemoteNoteStatusMigrationItem(ctx, writer, item, localDomain, false, 1, summary)
		require.NoError(t, err)
		require.True(t, stop)
		require.Equal(t, 1, summary.ScannedRemoteObjects)
		require.Equal(t, 1, summary.MaterializableObjects)
	})
}

func TestNormalizeRemoteNoteStatusBackfillHost_EdgeCases(t *testing.T) {
	require.Equal(t, "", normalizeRemoteNoteStatusBackfillHost("https:///missing-host"))
	require.Equal(t, "remote.example", normalizeRemoteNoteStatusBackfillHost("remote.example:8443/path"))
}

func remoteNoteObjectItem(
	t *testing.T,
	id string,
	actorID string,
	content string,
	publishedAt time.Time,
	isRemote bool,
	inReplyTo string,
) map[string]types.AttributeValue {
	t.Helper()

	item := map[string]any{
		"PK":           "object#" + id,
		"SK":           "object#" + id,
		"gsi2PK":       remoteNoteStatusBackfillTypePK,
		"gsi2SK":       publishedAt.UTC().Format(time.RFC3339Nano) + "#" + id,
		"id":           id,
		"type":         activitypub.NoteType,
		"attributedTo": actorID,
		"content":      content,
		"published":    publishedAt.UTC(),
		"updated":      publishedAt.UTC(),
		"to":           []string{activitypub.PublicAddress},
		"isRemote":     isRemote,
		"createdAt":    publishedAt.UTC(),
	}
	if strings.TrimSpace(inReplyTo) != "" {
		item["inReplyTo"] = inReplyTo
	}

	encoded, err := attributevalue.MarshalMap(item)
	require.NoError(t, err)
	return encoded
}

func remoteNoteMalformedObjectItem(t *testing.T, id string, publishedAt time.Time) map[string]types.AttributeValue {
	t.Helper()

	encoded, err := attributevalue.MarshalMap(map[string]any{
		"PK":        "object#" + id,
		"SK":        "object#" + id,
		"gsi2PK":    remoteNoteStatusBackfillTypePK,
		"gsi2SK":    publishedAt.UTC().Format(time.RFC3339Nano) + "#" + id,
		"id":        id,
		"type":      activitypub.NoteType,
		"isRemote":  true,
		"createdAt": publishedAt.UTC(),
	})
	require.NoError(t, err)
	return encoded
}
