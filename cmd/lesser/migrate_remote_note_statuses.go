package main

import (
	"context"
	stdErrors "errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	lessertheorydb "github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/theory-cloud/tabletheory/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"go.uber.org/zap"
)

const remoteNoteStatusBackfillTypePK = "object#type#Note"

type remoteNoteStatusMigrationClient interface {
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

type remoteNoteStatusBackfillWriter interface {
	GetStatus(ctx context.Context, statusID string) (*models.Status, error)
	CreateStatus(ctx context.Context, status *models.Status) error
}

type remoteNoteStatusMigrationSummary struct {
	ScannedRemoteObjects     int
	MaterializableObjects    int
	MalformedRemoteObjects   int
	ExistingStatuses         int
	MissingStatuses          int
	CreatedStatuses          int
	SampleObjectIDs          []string
	SampleStatusIDs          []string
	SampleMalformedObjectIDs []string
}

var newRemoteNoteStatusMigrationClientFn = func(cfg aws.Config) remoteNoteStatusMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

var openRemoteNoteStatusBackfillWriterFn = func(awsCfg aws.Config, tableName string) (remoteNoteStatusBackfillWriter, func() error, error) {
	db, err := tabletheoryNewFn(session.Config{
		Region:              awsCfg.Region,
		CredentialsProvider: awsCfg.Credentials,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := lessertheorydb.RegisterDefaultTypeConverters(db); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	return newRemoteNoteStatusBackfillWriter(db, tableName), db.Close, nil
}

func newRemoteNoteStatusBackfillWriter(db core.DB, tableName string) remoteNoteStatusBackfillWriter {
	return repositories.NewStatusRepository(db, tableName, zap.NewNop(), nil)
}

func runMigrateRemoteNoteStatuses(argv []string) error {
	return runCommonMigrationCLI(
		argv,
		"migrate-remote-note-statuses",
		"maximum number of remote note object rows to inspect after remote filtering (0 = all)",
		"materialize missing canonical status rows from remote note objects",
		func(ctx context.Context, awsCfg aws.Config, tableName string, apply bool, limit int) (remoteNoteStatusMigrationSummary, error) {
			prevTableName := models.MainTableName
			models.MainTableName = tableName
			defer func() {
				models.MainTableName = prevTableName
			}()

			writer, closeWriter, err := openRemoteNoteStatusBackfillWriterFn(awsCfg, tableName)
			if err != nil {
				return remoteNoteStatusMigrationSummary{}, err
			}
			if closeWriter != nil {
				defer func() { _ = closeWriter() }()
			}

			return executeRemoteNoteStatusMigration(
				ctx,
				newRemoteNoteStatusMigrationClientFn(awsCfg),
				writer,
				tableName,
				apply,
				limit,
			)
		},
		printRemoteNoteStatusMigrationSummary,
	)
}

func printRemoteNoteStatusMigrationSummary(
	summary remoteNoteStatusMigrationSummary,
	tableName string,
	resolvedProfile string,
	apply bool,
) {
	fmt.Printf("migrate-remote-note-statuses %s complete\n", selectedMigrationMode(apply))
	fmt.Printf("table: %s\n", tableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("scanned_remote_objects: %d\n", summary.ScannedRemoteObjects)
	fmt.Printf("materializable_remote_objects: %d\n", summary.MaterializableObjects)
	fmt.Printf("malformed_remote_objects: %d\n", summary.MalformedRemoteObjects)
	fmt.Printf("existing_statuses: %d\n", summary.ExistingStatuses)
	fmt.Printf("missing_statuses: %d\n", summary.MissingStatuses)
	fmt.Printf("created_statuses: %d\n", summary.CreatedStatuses)

	printMigrationSamples("sample_object_ids", summary.SampleObjectIDs)
	printMigrationSamples("sample_status_ids", summary.SampleStatusIDs)
	printMigrationSamples("sample_malformed_object_ids", summary.SampleMalformedObjectIDs)

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to materialize missing canonical statuses from remote note objects")
	}
}

func executeRemoteNoteStatusMigration(
	ctx context.Context,
	client remoteNoteStatusMigrationClient,
	writer remoteNoteStatusBackfillWriter,
	tableName string,
	apply bool,
	limit int,
) (remoteNoteStatusMigrationSummary, error) {
	summary := remoteNoteStatusMigrationSummary{}

	if client == nil {
		return summary, fmt.Errorf("migration client is required")
	}
	if writer == nil {
		return summary, fmt.Errorf("status backfill writer is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return summary, fmt.Errorf("table name is required")
	}

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("gsi2"),
		KeyConditionExpression: aws.String("gsi2PK = :noteType"),
		FilterExpression:       aws.String("isRemote = :isRemote"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":noteType": &types.AttributeValueMemberS{Value: remoteNoteStatusBackfillTypePK},
			":isRemote": &types.AttributeValueMemberBOOL{Value: true},
		},
	}

	for {
		out, err := client.Query(ctx, queryInput)
		if err != nil {
			return summary, fmt.Errorf("query remote note object rows: %w", err)
		}

		for _, item := range out.Items {
			var object models.Object
			if err := attributevalue.UnmarshalMap(item, &object); err != nil {
				summary.ScannedRemoteObjects++
				summary.MalformedRemoteObjects++
				appendRemoteNoteStatusMigrationSample(&summary.SampleMalformedObjectIDs, remoteNoteStatusObjectSampleID(item, nil))
				if limit > 0 && summary.ScannedRemoteObjects >= limit {
					return summary, nil
				}
				continue
			}

			summary.ScannedRemoteObjects++
			appendRemoteNoteStatusMigrationSample(&summary.SampleObjectIDs, strings.TrimSpace(object.ID))

			status, ok := buildRemoteNoteStatusBackfillStatus(&object)
			if !ok {
				summary.MalformedRemoteObjects++
				appendRemoteNoteStatusMigrationSample(&summary.SampleMalformedObjectIDs, remoteNoteStatusObjectSampleID(item, &object))
				if limit > 0 && summary.ScannedRemoteObjects >= limit {
					return summary, nil
				}
				continue
			}

			summary.MaterializableObjects++
			appendRemoteNoteStatusMigrationSample(&summary.SampleStatusIDs, status.StatusID)

			_, err = writer.GetStatus(ctx, status.StatusID)
			switch {
			case err == nil:
				summary.ExistingStatuses++
			case isRemoteNoteStatusBackfillNotFound(err):
				summary.MissingStatuses++
				if apply {
					if err := writer.CreateStatus(ctx, status); err != nil {
						if dynamormerrors.IsConditionFailed(err) {
							continue
						}
						return summary, fmt.Errorf("create canonical status %q from remote object %q: %w", status.StatusID, object.ID, err)
					}
					summary.CreatedStatuses++
				}
			default:
				return summary, fmt.Errorf("load canonical status %q for remote object %q: %w", status.StatusID, object.ID, err)
			}

			if limit > 0 && summary.ScannedRemoteObjects >= limit {
				return summary, nil
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			return summary, nil
		}
		if limit > 0 && summary.ScannedRemoteObjects >= limit {
			return summary, nil
		}

		queryInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func buildRemoteNoteStatusBackfillStatus(object *models.Object) (*models.Status, bool) {
	if object == nil || !object.IsRemote {
		return nil, false
	}
	if !strings.EqualFold(strings.TrimSpace(object.Type), activitypub.NoteType) {
		return nil, false
	}
	if strings.TrimSpace(object.ID) == "" || strings.TrimSpace(object.AttributedTo) == "" {
		return nil, false
	}

	note, err := transformations.StorageObjectToActivityPub(object)
	if err != nil || note == nil {
		return nil, false
	}
	if strings.TrimSpace(note.ConversationID) == "" {
		note.ConversationID = strings.TrimSpace(object.ConversationID)
	}
	if strings.TrimSpace(note.Visibility) == "" {
		note.Visibility = strings.TrimSpace(object.Visibility)
	}

	status := federation.BuildCanonicalRemoteStatus(note, "")
	if status == nil {
		return nil, false
	}

	return status, true
}

func isRemoteNoteStatusBackfillNotFound(err error) bool {
	if err == nil {
		return false
	}

	return dynamormerrors.IsNotFound(err) ||
		stdErrors.Is(err, storage.ErrNotFound) ||
		strings.Contains(strings.ToLower(err.Error()), "not found")
}

func remoteNoteStatusObjectSampleID(item map[string]types.AttributeValue, object *models.Object) string {
	if object != nil {
		if id := strings.TrimSpace(object.ID); id != "" {
			return id
		}
	}

	if idAttr, ok := item["id"]; ok {
		if id, ok := attributeString(idAttr); ok {
			return strings.TrimSpace(id)
		}
	}
	if pkAttr, ok := item["PK"]; ok {
		if pk, ok := attributeString(pkAttr); ok {
			return strings.TrimSpace(strings.TrimPrefix(pk, "object#"))
		}
	}

	return ""
}

func appendRemoteNoteStatusMigrationSample(samples *[]string, value string) {
	if samples == nil || strings.TrimSpace(value) == "" {
		return
	}
	for _, existing := range *samples {
		if existing == value {
			return
		}
	}
	if len(*samples) >= 5 {
		return
	}

	*samples = append(*samples, value)
}
