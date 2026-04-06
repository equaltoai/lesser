package main

import (
	"context"
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
	createErrs map[string]error
}

func (f *fakeRemoteNoteStatusBackfillWriter) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
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
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newRemoteNoteStatusMigrationClientFn = previousClientFactory
		openRemoteNoteStatusBackfillWriterFn = previousOpenWriter
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
}

func TestExecuteRemoteNoteStatusMigration_ApplyCreatesMissingStatusesAndClassifiesMalformedRows(t *testing.T) {
	now := time.Date(2026, 4, 6, 15, 4, 0, 0, time.UTC)
	firstRemoteURL := "https://remote.example/users/bob/statuses/1"
	secondRemoteURL := "https://remote.example/users/bob/statuses/2"
	parentRemoteURL := "https://another.remote/users/alice/statuses/root"
	existingStatusID := models.CanonicalStatusID(secondRemoteURL)

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

	summary, err := executeRemoteNoteStatusMigration(context.Background(), client, writer, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 3, summary.ScannedRemoteObjects)
	require.Equal(t, 2, summary.MaterializableObjects)
	require.Equal(t, 1, summary.MalformedRemoteObjects)
	require.Equal(t, 1, summary.ExistingStatuses)
	require.Equal(t, 1, summary.MissingStatuses)
	require.Equal(t, 1, summary.CreatedStatuses)
	require.Contains(t, summary.SampleObjectIDs, firstRemoteURL)
	require.Contains(t, summary.SampleStatusIDs, models.CanonicalStatusID(firstRemoteURL))
	require.Contains(t, summary.SampleMalformedObjectIDs, "https://remote.example/users/bob/statuses/bad")

	created := writer.statuses[models.CanonicalStatusID(firstRemoteURL)]
	require.NotNil(t, created)
	require.Equal(t, models.CanonicalStatusID(firstRemoteURL), created.StatusID)
	require.Equal(t, "https://remote.example/users/bob", created.AuthorID)
	require.Equal(t, "bob@remote.example", created.AuthorUsername)
	require.Equal(t, models.VisibilityPublic, created.Visibility)
	require.Equal(t, models.CanonicalStatusID(parentRemoteURL), created.InReplyToID)
	require.Equal(t, []string{firstRemoteURL}, created.URLs)
	require.Equal(t, firstRemoteURL, created.Note.ID)
}

func TestExecuteRemoteNoteStatusMigration_ValidatesInputsAndTreatsConditionFailuresAsIdempotent(t *testing.T) {
	now := time.Date(2026, 4, 6, 15, 4, 0, 0, time.UTC)
	remoteURL := "https://remote.example/users/bob/statuses/1"
	statusID := models.CanonicalStatusID(remoteURL)

	_, err := executeRemoteNoteStatusMigration(context.Background(), nil, &fakeRemoteNoteStatusBackfillWriter{}, "table", false, 0)
	require.EqualError(t, err, "migration client is required")

	_, err = executeRemoteNoteStatusMigration(context.Background(), &fakeUserKeyMigrationClient{}, nil, "table", false, 0)
	require.EqualError(t, err, "status backfill writer is required")

	_, err = executeRemoteNoteStatusMigration(context.Background(), &fakeUserKeyMigrationClient{}, &fakeRemoteNoteStatusBackfillWriter{}, "   ", false, 0)
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

	summary, err := executeRemoteNoteStatusMigration(context.Background(), client, writer, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedRemoteObjects)
	require.Equal(t, 1, summary.MaterializableObjects)
	require.Equal(t, 1, summary.MissingStatuses)
	require.Equal(t, 0, summary.CreatedStatuses)
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
