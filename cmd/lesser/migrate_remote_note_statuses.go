package main

import (
	"context"
	stdErrors "errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
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

var resolveRemoteNoteStatusMigrationLocalDomainFn = func(ctx context.Context, awsCfg aws.Config, app string, env string) (string, error) {
	app = strings.TrimSpace(app)
	if app == "" {
		app = naming.DefaultAppName
	}

	paramName := fmt.Sprintf("/%s/%s/lesser/exports/v1/domain", app, naming.StageForEnvironment(env))
	out, err := ssm.NewFromConfig(awsCfg).GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(paramName),
	})
	if err != nil {
		var notFound *ssmtypes.ParameterNotFound
		if stdErrors.As(err, &notFound) {
			return "", nil
		}
		return "", fmt.Errorf("resolve stage domain from SSM %s: %w", paramName, err)
	}

	return strings.TrimSpace(aws.ToString(out.Parameter.Value)), nil
}

func newRemoteNoteStatusBackfillWriter(db core.DB, tableName string) remoteNoteStatusBackfillWriter {
	return repositories.NewStatusRepository(db, tableName, zap.NewNop(), nil)
}

func runMigrateRemoteNoteStatuses(argv []string) error {
	options, err := parseCommonMigrationCLIOptions(
		argv,
		"migrate-remote-note-statuses",
		"maximum number of remote note object rows to inspect after remote filtering (0 = all)",
		"materialize missing canonical status rows from remote note objects",
	)
	if err != nil {
		return err
	}

	ctx := context.Background()
	awsCfg, resolvedTableName, resolvedProfile, err := resolveCommonMigrationCLIOptions(ctx, options)
	if err != nil {
		return err
	}

	localDomain, err := resolveRemoteNoteStatusMigrationLocalDomainFn(ctx, awsCfg, options.App, options.Env)
	if err != nil {
		return err
	}

	prevTableName := models.MainTableName
	models.MainTableName = resolvedTableName
	defer func() {
		models.MainTableName = prevTableName
	}()

	writer, closeWriter, err := openRemoteNoteStatusBackfillWriterFn(awsCfg, resolvedTableName)
	if err != nil {
		return err
	}
	if closeWriter != nil {
		defer func() { _ = closeWriter() }()
	}

	summary, err := executeRemoteNoteStatusMigration(
		ctx,
		newRemoteNoteStatusMigrationClientFn(awsCfg),
		writer,
		resolvedTableName,
		localDomain,
		options.Apply,
		options.Limit,
	)
	if err != nil {
		return err
	}

	printRemoteNoteStatusMigrationSummary(summary, resolvedTableName, resolvedProfile, options.Apply)
	return nil
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
	localDomain string,
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
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":noteType": &types.AttributeValueMemberS{Value: remoteNoteStatusBackfillTypePK},
		},
	}

	for {
		out, err := client.Query(ctx, queryInput)
		if err != nil {
			return summary, fmt.Errorf("query remote note object rows: %w", err)
		}

		stop, err := processRemoteNoteStatusMigrationPage(ctx, writer, out.Items, localDomain, apply, limit, &summary)
		if err != nil {
			return summary, err
		}
		if stop || len(out.LastEvaluatedKey) == 0 {
			return summary, nil
		}

		queryInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func processRemoteNoteStatusMigrationPage(
	ctx context.Context,
	writer remoteNoteStatusBackfillWriter,
	items []map[string]types.AttributeValue,
	localDomain string,
	apply bool,
	limit int,
	summary *remoteNoteStatusMigrationSummary,
) (bool, error) {
	for _, item := range items {
		stop, err := processRemoteNoteStatusMigrationItem(ctx, writer, item, localDomain, apply, limit, summary)
		if err != nil || stop {
			return stop, err
		}
	}

	return false, nil
}

func processRemoteNoteStatusMigrationItem(
	ctx context.Context,
	writer remoteNoteStatusBackfillWriter,
	item map[string]types.AttributeValue,
	localDomain string,
	apply bool,
	limit int,
	summary *remoteNoteStatusMigrationSummary,
) (bool, error) {
	object, ok := decodeRemoteNoteStatusMigrationObject(item, localDomain)
	if !ok {
		return false, nil
	}
	if object == nil {
		recordRemoteNoteStatusMalformedItem(summary, remoteNoteStatusObjectSampleID(item, nil))
		return remoteNoteStatusMigrationLimitReached(limit, summary), nil
	}

	summary.ScannedRemoteObjects++
	appendRemoteNoteStatusMigrationSample(&summary.SampleObjectIDs, strings.TrimSpace(object.ID))

	status, ok := buildRemoteNoteStatusBackfillStatus(object, localDomain)
	if !ok {
		summary.MalformedRemoteObjects++
		appendRemoteNoteStatusMigrationSample(&summary.SampleMalformedObjectIDs, remoteNoteStatusObjectSampleID(item, object))
		return remoteNoteStatusMigrationLimitReached(limit, summary), nil
	}

	summary.MaterializableObjects++
	appendRemoteNoteStatusMigrationSample(&summary.SampleStatusIDs, status.StatusID)

	if err := applyRemoteNoteStatusBackfillCandidate(ctx, writer, object, status, apply, summary); err != nil {
		return false, err
	}

	return remoteNoteStatusMigrationLimitReached(limit, summary), nil
}

func decodeRemoteNoteStatusMigrationObject(
	item map[string]types.AttributeValue,
	localDomain string,
) (*models.Object, bool) {
	var object models.Object
	if err := attributevalue.UnmarshalMap(item, &object); err != nil {
		if !rawRemoteNoteStatusItemIsRemote(item) &&
			!isRemoteNoteStatusBackfillIdentifier(remoteNoteStatusObjectSampleID(item, nil), localDomain) {
			return nil, false
		}

		return nil, true
	}

	if !shouldBackfillRemoteNoteStatusObject(&object, localDomain) {
		return nil, false
	}

	return &object, true
}

func recordRemoteNoteStatusMalformedItem(summary *remoteNoteStatusMigrationSummary, sampleID string) {
	if summary == nil {
		return
	}

	summary.ScannedRemoteObjects++
	summary.MalformedRemoteObjects++
	appendRemoteNoteStatusMigrationSample(&summary.SampleMalformedObjectIDs, sampleID)
}

func applyRemoteNoteStatusBackfillCandidate(
	ctx context.Context,
	writer remoteNoteStatusBackfillWriter,
	object *models.Object,
	status *models.Status,
	apply bool,
	summary *remoteNoteStatusMigrationSummary,
) error {
	_, err := writer.GetStatus(ctx, status.StatusID)
	switch {
	case err == nil:
		summary.ExistingStatuses++
		return nil
	case isRemoteNoteStatusBackfillNotFound(err):
		summary.MissingStatuses++
		if !apply {
			return nil
		}

		if err := writer.CreateStatus(ctx, status); err != nil {
			if dynamormerrors.IsConditionFailed(err) {
				return nil
			}
			return fmt.Errorf("create canonical status %q from remote object %q: %w", status.StatusID, object.ID, err)
		}
		summary.CreatedStatuses++
		return nil
	default:
		return fmt.Errorf("load canonical status %q for remote object %q: %w", status.StatusID, object.ID, err)
	}
}

func remoteNoteStatusMigrationLimitReached(limit int, summary *remoteNoteStatusMigrationSummary) bool {
	if limit <= 0 || summary == nil {
		return false
	}

	return summary.ScannedRemoteObjects >= limit
}

func buildRemoteNoteStatusBackfillStatus(object *models.Object, localDomain string) (*models.Status, bool) {
	if !shouldBackfillRemoteNoteStatusObject(object, localDomain) {
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

	status := federation.BuildCanonicalRemoteStatus(note, localDomain)
	if status == nil {
		return nil, false
	}

	return status, true
}

func shouldBackfillRemoteNoteStatusObject(object *models.Object, localDomain string) bool {
	if object == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(object.Type), activitypub.NoteType) {
		return false
	}
	if object.IsRemote {
		return true
	}

	return isRemoteNoteStatusBackfillIdentifier(object.ID, localDomain) ||
		isRemoteNoteStatusBackfillIdentifier(object.AttributedTo, localDomain)
}

func isRemoteNoteStatusBackfillNotFound(err error) bool {
	if err == nil {
		return false
	}

	return dynamormerrors.IsNotFound(err) ||
		stdErrors.Is(err, storage.ErrNotFound) ||
		strings.Contains(strings.ToLower(err.Error()), "not found")
}

func rawRemoteNoteStatusItemIsRemote(item map[string]types.AttributeValue) bool {
	attr, ok := item["isRemote"]
	if !ok || attr == nil {
		return false
	}

	value, ok := attr.(*types.AttributeValueMemberBOOL)
	return ok && value.Value
}

func isRemoteNoteStatusBackfillIdentifier(raw string, localDomain string) bool {
	host := normalizeRemoteNoteStatusBackfillHost(raw)
	localHost := normalizeRemoteNoteStatusBackfillHost(localDomain)
	if host == "" || localHost == "" {
		return false
	}

	return host != localHost
}

func normalizeRemoteNoteStatusBackfillHost(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ""
	}

	if parsed, err := url.Parse(value); err == nil && parsed != nil && parsed.Hostname() != "" {
		return strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	}

	value = strings.TrimPrefix(strings.TrimPrefix(value, "https://"), "http://")
	if idx := strings.Index(value, "/"); idx >= 0 {
		value = value[:idx]
	}
	if idx := strings.Index(value, ":"); idx >= 0 {
		value = value[:idx]
	}

	return strings.TrimSpace(value)
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
