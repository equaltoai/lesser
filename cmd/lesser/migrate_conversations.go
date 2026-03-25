package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

const (
	conversationMetadataPartitionPrefix      = "CONVERSATION#"
	conversationMetadataSortKey              = "METADATA"
	conversationParticipantPartitionPrefix   = "USER_CONVERSATIONS#"
	conversationParticipantLookupPrefix      = "CONVERSATION_PARTICIPANTS#"
	conversationParticipantLookupSortKey     = "LOOKUP"
	conversationStatusPartitionPrefix        = "CONVERSATION_STATUS#"
	conversationMessageSortKeyPrefix         = "STATUS#"
	conversationRequestStateAttribute        = "requestState"
	conversationSnapshotAttribute            = "conversation"
	conversationIDAttribute                  = "conversationID"
	conversationUserIDAttribute              = "userID"
	conversationUnreadAttribute              = "unread"
	conversationLastReadAtAttribute          = "lastReadAt"
	conversationParticipantsAttribute        = "participants"
	conversationLastStatusIDAttribute        = "lastStatusID"
	conversationCreatedAtAttribute           = "createdAt"
	conversationUpdatedAtAttribute           = "updatedAt"
	conversationTotalMessageCountAttribute   = "totalMessageCount"
	conversationLastMessageTimeAttribute     = "lastMessageTime"
	conversationLookupConversationIDAttr     = "conversationID"
	conversationMessageStatusIDAttribute     = "statusID"
	conversationMigrationStatusUnreadDefault = false
)

type conversationMigrationClient interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

type conversationMigrationSummary struct {
	ScannedConversations     int
	CandidateGroups          int
	CandidateConversations   int
	ConversationRowsUpserted int
	ConversationRowsDeleted  int
	ParticipantRowsUpserted  int
	ParticipantRowsDeleted   int
	StatusRowsUpserted       int
	StatusRowsDeleted        int
	LookupRowsUpserted       int
	LookupRowsDeleted        int
}

type conversationMigrationScanData struct {
	ConversationItems []map[string]types.AttributeValue
	ParticipantItems  []map[string]types.AttributeValue
	StatusItems       []map[string]types.AttributeValue
	LookupItems       []map[string]types.AttributeValue
}

type conversationMigrationDataset struct {
	ScannedConversations int
	ConversationsByID    map[string]conversationMetadataRecord
	GroupsByKey          map[string]*conversationMigrationGroup
}

type conversationMetadataRecord struct {
	ConversationID        string
	Item                  map[string]types.AttributeValue
	Participants          []string
	CanonicalParticipants []string
	ParticipantKey        string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	LastMessageTime       time.Time
	LastStatusID          string
	TotalMessageCount     int64
}

type conversationParticipantRecordItem struct {
	PK                   string
	SK                   string
	Item                 map[string]types.AttributeValue
	ConversationID       string
	ParticipantID        string
	CanonicalParticipant string
	SortTime             time.Time
	Unread               bool
}

type conversationStatusRecord struct {
	PK             string
	SK             string
	Item           map[string]types.AttributeValue
	ConversationID string
	UserID         string
	CanonicalUser  string
	Unread         bool
	LastReadAt     time.Time
}

type conversationLookupRecord struct {
	PK                      string
	SK                      string
	Item                    map[string]types.AttributeValue
	ConversationID          string
	ParticipantKey          string
	CanonicalParticipantKey string
}

type conversationMigrationGroup struct {
	ParticipantKey  string
	Conversations   []conversationMetadataRecord
	ParticipantRows []conversationParticipantRecordItem
	StatusRows      []conversationStatusRecord
	LookupRows      []conversationLookupRecord
}

type conversationCanonicalState struct {
	ID                string
	Participants      []string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastMessageTime   time.Time
	LastStatusID      string
	TotalMessageCount int64
	Unread            bool
}

type conversationPartitionPlanState struct {
	DesiredRows       map[string]conversationMigrationPut
	OriginalRows      map[string]map[string]types.AttributeValue
	Deletes           map[string]conversationMigrationDelete
	MessageCount      int64
	LatestMessageTime time.Time
	LatestStatusID    string
}

type conversationMigrationPlan struct {
	CandidateConversations int
	Puts                   map[string]conversationMigrationPut
	Deletes                map[string]conversationMigrationDelete
}

type conversationMigrationPut struct {
	Category string
	Item     map[string]types.AttributeValue
}

type conversationMigrationDelete struct {
	Category string
	Key      map[string]types.AttributeValue
}

var newConversationMigrationClientFn = func(cfg aws.Config) conversationMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

func runMigrateConversations(argv []string) error {
	fs := flag.NewFlagSet("lesser migrate-conversations", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		app        string
		env        string
		awsProfile string
		tableName  string
		limit      int
		apply      bool
	)
	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&env, "env", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (optional; sets AWS_PROFILE)")
	fs.StringVar(&tableName, "table", "", "explicit DynamoDB table name override")
	fs.IntVar(&limit, "limit", 0, "maximum number of conversation groups to process (0 = all)")
	fs.BoolVar(&apply, "apply", false, "rewrite mixed-case conversation rows, lowercase participant keys, and deduplicate duplicate conversations")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	resolvedTableName, err := resolveUserKeyMigrationTableName(app, env, tableName)
	if err != nil {
		return err
	}

	ctx := context.Background()
	awsCfg, resolvedProfile, err := loadAWSConfigForCLIFn(ctx, awsProfile)
	if err != nil {
		return err
	}

	summary, err := executeConversationMigration(ctx, newConversationMigrationClientFn(awsCfg), resolvedTableName, apply, limit)
	if err != nil {
		return err
	}

	mode := migrationModeDryRun
	if apply {
		mode = migrationModeApply
	}

	fmt.Printf("migrate-conversations %s complete\n", mode)
	fmt.Printf("table: %s\n", resolvedTableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("scanned_conversations: %d\n", summary.ScannedConversations)
	fmt.Printf("candidate_groups: %d\n", summary.CandidateGroups)
	fmt.Printf("candidate_conversations: %d\n", summary.CandidateConversations)
	fmt.Printf("conversation_rows_upserted: %d\n", summary.ConversationRowsUpserted)
	fmt.Printf("conversation_rows_deleted: %d\n", summary.ConversationRowsDeleted)
	fmt.Printf("participant_rows_upserted: %d\n", summary.ParticipantRowsUpserted)
	fmt.Printf("participant_rows_deleted: %d\n", summary.ParticipantRowsDeleted)
	fmt.Printf("status_rows_upserted: %d\n", summary.StatusRowsUpserted)
	fmt.Printf("status_rows_deleted: %d\n", summary.StatusRowsDeleted)
	fmt.Printf("lookup_rows_upserted: %d\n", summary.LookupRowsUpserted)
	fmt.Printf("lookup_rows_deleted: %d\n", summary.LookupRowsDeleted)

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to rewrite lowercase conversation partitions and delete duplicate rows")
	}

	return nil
}

func executeConversationMigration(
	ctx context.Context,
	client conversationMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (conversationMigrationSummary, error) {
	summary := conversationMigrationSummary{}

	if err := validateConversationMigrationInputs(client, tableName); err != nil {
		return summary, err
	}

	dataset, err := loadConversationMigrationDataset(ctx, client, tableName)
	if err != nil {
		return summary, err
	}
	summary.ScannedConversations = dataset.ScannedConversations

	groupKeys := sortedConversationMigrationGroupKeys(dataset.GroupsByKey)
	for _, groupKey := range groupKeys {
		if limit > 0 && summary.CandidateGroups >= limit {
			break
		}

		group := dataset.GroupsByKey[groupKey]
		if group == nil || len(group.Conversations) == 0 {
			continue
		}

		plan, err := buildConversationMigrationPlan(ctx, client, tableName, group)
		if err != nil {
			return summary, err
		}
		if plan == nil || (len(plan.Puts) == 0 && len(plan.Deletes) == 0) {
			continue
		}

		summary.CandidateGroups++
		summary.CandidateConversations += plan.CandidateConversations
		if err := maybeApplyConversationMigrationPlan(ctx, client, tableName, apply, plan, &summary); err != nil {
			return summary, err
		}
	}

	return summary, nil
}

func validateConversationMigrationInputs(client conversationMigrationClient, tableName string) error {
	if client == nil {
		return fmt.Errorf("migration client is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return fmt.Errorf("table name is required")
	}
	return nil
}

func maybeApplyConversationMigrationPlan(
	ctx context.Context,
	client conversationMigrationClient,
	tableName string,
	apply bool,
	plan *conversationMigrationPlan,
	summary *conversationMigrationSummary,
) error {
	if !apply {
		return nil
	}
	return applyConversationMigrationPlan(ctx, client, tableName, plan, summary)
}

func loadConversationMigrationDataset(
	ctx context.Context,
	client conversationMigrationClient,
	tableName string,
) (conversationMigrationDataset, error) {
	scanData, err := scanConversationMigrationData(ctx, client, tableName)
	if err != nil {
		return conversationMigrationDataset{}, err
	}
	return buildConversationMigrationDataset(scanData)
}

func scanConversationMigrationData(
	ctx context.Context,
	client conversationMigrationClient,
	tableName string,
) (conversationMigrationScanData, error) {
	conversationItems, err := scanConversationMigrationItems(
		ctx,
		client,
		tableName,
		"begins_with(PK, :prefix) AND SK = :sk",
		map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: conversationMetadataPartitionPrefix},
			":sk":     &types.AttributeValueMemberS{Value: conversationMetadataSortKey},
		},
	)
	if err != nil {
		return conversationMigrationScanData{}, fmt.Errorf("scan conversation metadata: %w", err)
	}

	participantItems, err := scanConversationMigrationItems(
		ctx,
		client,
		tableName,
		"begins_with(PK, :prefix)",
		map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: conversationParticipantPartitionPrefix},
		},
	)
	if err != nil {
		return conversationMigrationScanData{}, fmt.Errorf("scan participant rows: %w", err)
	}

	statusItems, err := scanConversationMigrationItems(
		ctx,
		client,
		tableName,
		"begins_with(PK, :prefix)",
		map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: conversationStatusPartitionPrefix},
		},
	)
	if err != nil {
		return conversationMigrationScanData{}, fmt.Errorf("scan status rows: %w", err)
	}

	lookupItems, err := scanConversationMigrationItems(
		ctx,
		client,
		tableName,
		"begins_with(PK, :prefix) AND SK = :sk",
		map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: conversationParticipantLookupPrefix},
			":sk":     &types.AttributeValueMemberS{Value: conversationParticipantLookupSortKey},
		},
	)
	if err != nil {
		return conversationMigrationScanData{}, fmt.Errorf("scan participant lookup rows: %w", err)
	}

	return conversationMigrationScanData{
		ConversationItems: conversationItems,
		ParticipantItems:  participantItems,
		StatusItems:       statusItems,
		LookupItems:       lookupItems,
	}, nil
}

func buildConversationMigrationDataset(scanData conversationMigrationScanData) (conversationMigrationDataset, error) {
	dataset := conversationMigrationDataset{
		ConversationsByID: make(map[string]conversationMetadataRecord, len(scanData.ConversationItems)),
		GroupsByKey:       map[string]*conversationMigrationGroup{},
	}

	for _, item := range scanData.ConversationItems {
		if err := addConversationMetadataToDataset(&dataset, item); err != nil {
			return conversationMigrationDataset{}, err
		}
	}
	for _, item := range scanData.ParticipantItems {
		if err := addConversationParticipantToDataset(&dataset, item); err != nil {
			return conversationMigrationDataset{}, err
		}
	}
	for _, item := range scanData.StatusItems {
		if err := addConversationStatusToDataset(&dataset, item); err != nil {
			return conversationMigrationDataset{}, err
		}
	}
	for _, item := range scanData.LookupItems {
		if err := addConversationLookupToDataset(&dataset, item); err != nil {
			return conversationMigrationDataset{}, err
		}
	}

	return dataset, nil
}

func addConversationMetadataToDataset(dataset *conversationMigrationDataset, item map[string]types.AttributeValue) error {
	record, ok, err := parseConversationMetadataRecord(item)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	dataset.ScannedConversations++
	dataset.ConversationsByID[record.ConversationID] = record
	group := ensureConversationMigrationGroup(dataset.GroupsByKey, record.ParticipantKey)
	group.Conversations = append(group.Conversations, record)
	return nil
}

func addConversationParticipantToDataset(dataset *conversationMigrationDataset, item map[string]types.AttributeValue) error {
	record, ok, err := parseConversationParticipantRecordItem(item)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	meta, ok := dataset.ConversationsByID[record.ConversationID]
	if !ok {
		return fmt.Errorf("participant row %s %s references unknown conversation %q", record.PK, record.SK, record.ConversationID)
	}
	group := ensureConversationMigrationGroup(dataset.GroupsByKey, meta.ParticipantKey)
	group.ParticipantRows = append(group.ParticipantRows, record)
	return nil
}

func addConversationStatusToDataset(dataset *conversationMigrationDataset, item map[string]types.AttributeValue) error {
	record, ok, err := parseConversationStatusRecord(item)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	meta, ok := dataset.ConversationsByID[record.ConversationID]
	if !ok {
		return fmt.Errorf("status row %s %s references unknown conversation %q", record.PK, record.SK, record.ConversationID)
	}
	group := ensureConversationMigrationGroup(dataset.GroupsByKey, meta.ParticipantKey)
	group.StatusRows = append(group.StatusRows, record)
	return nil
}

func addConversationLookupToDataset(dataset *conversationMigrationDataset, item map[string]types.AttributeValue) error {
	record, ok, err := parseConversationLookupRecord(item)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	group := ensureConversationMigrationGroup(dataset.GroupsByKey, record.CanonicalParticipantKey)
	group.LookupRows = append(group.LookupRows, record)
	return nil
}

func sortedConversationMigrationGroupKeys(groupsByKey map[string]*conversationMigrationGroup) []string {
	groupKeys := make([]string, 0, len(groupsByKey))
	for key := range groupsByKey {
		groupKeys = append(groupKeys, key)
	}
	sort.Strings(groupKeys)
	return groupKeys
}

func scanConversationMigrationItems(
	ctx context.Context,
	client conversationMigrationClient,
	tableName string,
	filterExpression string,
	values map[string]types.AttributeValue,
) ([]map[string]types.AttributeValue, error) {
	scanInput := &dynamodb.ScanInput{
		TableName:                 aws.String(tableName),
		FilterExpression:          aws.String(filterExpression),
		ExpressionAttributeValues: values,
	}

	var items []map[string]types.AttributeValue
	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return nil, err
		}
		items = append(items, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return items, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func queryConversationPartitionItems(
	ctx context.Context,
	client conversationMigrationClient,
	tableName string,
	conversationID string,
) ([]map[string]types.AttributeValue, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: conversationPartitionKey(conversationID)},
		},
	}

	var items []map[string]types.AttributeValue
	for {
		out, err := client.Query(ctx, queryInput)
		if err != nil {
			return nil, err
		}
		items = append(items, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return items, nil
		}
		queryInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func parseConversationMetadataRecord(item map[string]types.AttributeValue) (conversationMetadataRecord, bool, error) {
	pk, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(pk, conversationMetadataPartitionPrefix) {
		return conversationMetadataRecord{}, false, nil
	}
	sk, ok := attributeString(item["SK"])
	if !ok || sk != conversationMetadataSortKey {
		return conversationMetadataRecord{}, false, nil
	}

	conversationID := strings.TrimPrefix(pk, conversationMetadataPartitionPrefix)
	if conversationID == "" {
		return conversationMetadataRecord{}, false, fmt.Errorf("conversation metadata row missing conversation ID for PK %q", pk)
	}

	participants, ok := attributeStringSlice(item[conversationParticipantsAttribute])
	if !ok {
		return conversationMetadataRecord{}, false, fmt.Errorf("conversation metadata %q is missing participants", conversationID)
	}
	canonicalParticipants := models.CanonicalConversationParticipants(participants)
	if len(canonicalParticipants) == 0 {
		return conversationMetadataRecord{}, false, fmt.Errorf("conversation metadata %q has no canonical participants", conversationID)
	}

	createdAt, _ := attributeTime(item[conversationCreatedAtAttribute])
	updatedAt, _ := attributeTime(item[conversationUpdatedAtAttribute])
	lastMessageTime, _ := attributeTime(item[conversationLastMessageTimeAttribute])
	lastStatusID, _ := attributeString(item[conversationLastStatusIDAttribute])
	totalMessageCount, _ := attributeInt64(item[conversationTotalMessageCountAttribute])

	return conversationMetadataRecord{
		ConversationID:        conversationID,
		Item:                  item,
		Participants:          participants,
		CanonicalParticipants: canonicalParticipants,
		ParticipantKey:        strings.Join(canonicalParticipants, ","),
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
		LastMessageTime:       lastMessageTime,
		LastStatusID:          lastStatusID,
		TotalMessageCount:     totalMessageCount,
	}, true, nil
}

func parseConversationParticipantRecordItem(item map[string]types.AttributeValue) (conversationParticipantRecordItem, bool, error) {
	pk, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(pk, conversationParticipantPartitionPrefix) {
		return conversationParticipantRecordItem{}, false, nil
	}
	sk, ok := attributeString(item["SK"])
	if !ok || strings.TrimSpace(sk) == "" {
		return conversationParticipantRecordItem{}, false, fmt.Errorf("participant row %q is missing SK", pk)
	}

	participantID := strings.TrimPrefix(pk, conversationParticipantPartitionPrefix)
	if participantID == "" {
		return conversationParticipantRecordItem{}, false, fmt.Errorf("participant row %q is missing participant ID", pk)
	}

	conversationID := conversationIDFromParticipantItem(item, sk)
	if conversationID == "" {
		return conversationParticipantRecordItem{}, false, fmt.Errorf("participant row %s %s is missing conversation ID", pk, sk)
	}

	unread, _ := attributeBool(item[conversationUnreadAttribute])

	return conversationParticipantRecordItem{
		PK:                   pk,
		SK:                   sk,
		Item:                 item,
		ConversationID:       conversationID,
		ParticipantID:        participantID,
		CanonicalParticipant: models.CanonicalConversationParticipantID(participantID),
		SortTime:             conversationParticipantSortTime(sk),
		Unread:               unread,
	}, true, nil
}

func parseConversationStatusRecord(item map[string]types.AttributeValue) (conversationStatusRecord, bool, error) {
	pk, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(pk, conversationStatusPartitionPrefix) {
		return conversationStatusRecord{}, false, nil
	}
	sk, ok := attributeString(item["SK"])
	if !ok || !strings.HasPrefix(sk, "USER#") {
		return conversationStatusRecord{}, false, fmt.Errorf("status row %q has invalid SK", pk)
	}

	conversationID := strings.TrimPrefix(pk, conversationStatusPartitionPrefix)
	userID := strings.TrimPrefix(sk, "USER#")
	if conversationID == "" || userID == "" {
		return conversationStatusRecord{}, false, fmt.Errorf("status row %s %s is missing conversation or user ID", pk, sk)
	}

	unread, _ := attributeBool(item[conversationUnreadAttribute])
	lastReadAt, _ := attributeTime(item[conversationLastReadAtAttribute])

	return conversationStatusRecord{
		PK:             pk,
		SK:             sk,
		Item:           item,
		ConversationID: conversationID,
		UserID:         userID,
		CanonicalUser:  models.CanonicalConversationParticipantID(userID),
		Unread:         unread,
		LastReadAt:     lastReadAt,
	}, true, nil
}

func parseConversationLookupRecord(item map[string]types.AttributeValue) (conversationLookupRecord, bool, error) {
	pk, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(pk, conversationParticipantLookupPrefix) {
		return conversationLookupRecord{}, false, nil
	}
	sk, ok := attributeString(item["SK"])
	if !ok || sk != conversationParticipantLookupSortKey {
		return conversationLookupRecord{}, false, nil
	}

	participantKey := strings.TrimPrefix(pk, conversationParticipantLookupPrefix)
	if participantKey == "" {
		return conversationLookupRecord{}, false, fmt.Errorf("lookup row %q is missing participant key", pk)
	}
	canonicalParticipantKey := canonicalConversationParticipantKey(participantKey)
	if canonicalParticipantKey == "" {
		return conversationLookupRecord{}, false, fmt.Errorf("lookup row %q has no canonical participant key", pk)
	}

	conversationID, ok := firstAttributeString(item, conversationLookupConversationIDAttr, "ConversationID")
	if !ok || strings.TrimSpace(conversationID) == "" {
		return conversationLookupRecord{}, false, fmt.Errorf("lookup row %q is missing ConversationID", pk)
	}

	return conversationLookupRecord{
		PK:                      pk,
		SK:                      sk,
		Item:                    item,
		ConversationID:          conversationID,
		ParticipantKey:          participantKey,
		CanonicalParticipantKey: canonicalParticipantKey,
	}, true, nil
}

func buildConversationMigrationPlan(
	ctx context.Context,
	client conversationMigrationClient,
	tableName string,
	group *conversationMigrationGroup,
) (*conversationMigrationPlan, error) {
	if group == nil || len(group.Conversations) == 0 {
		return nil, nil
	}

	sort.Slice(group.Conversations, func(i, j int) bool {
		return conversationMetadataLess(group.Conversations[j], group.Conversations[i])
	})
	canonicalMeta := group.Conversations[0]
	partitionPlan, err := planConversationPartitionRows(ctx, client, tableName, group.Conversations, canonicalMeta)
	if err != nil {
		return nil, err
	}

	canonicalState := buildConversationCanonicalState(
		group.Conversations,
		canonicalMeta,
		partitionPlan.MessageCount,
		partitionPlan.LatestMessageTime,
		partitionPlan.LatestStatusID,
	)
	canonicalState.Unread = anyParticipantUnread(group.ParticipantRows) || anyStatusUnread(group.StatusRows)

	plan := newConversationMigrationPlan(len(group.Conversations))
	planConversationMetadataRow(plan, canonicalMeta, canonicalState)
	plan.deleteDuplicateConversationMetadata(group.Conversations)
	planConversationPartitionPlan(plan, partitionPlan)
	if err := planConversationParticipantRows(plan, group.ParticipantRows, canonicalMeta.ConversationID, canonicalState); err != nil {
		return nil, err
	}
	planConversationStatusRows(plan, group.StatusRows, canonicalMeta.ConversationID, canonicalState)
	planConversationLookupRow(plan, group, canonicalMeta.ConversationID)

	if len(plan.Puts) == 0 && len(plan.Deletes) == 0 {
		return nil, nil
	}
	return plan, nil
}

func newConversationMigrationPlan(candidateConversations int) *conversationMigrationPlan {
	return &conversationMigrationPlan{
		CandidateConversations: candidateConversations,
		Puts:                   map[string]conversationMigrationPut{},
		Deletes:                map[string]conversationMigrationDelete{},
	}
}

func planConversationPartitionRows(
	ctx context.Context,
	client conversationMigrationClient,
	tableName string,
	conversations []conversationMetadataRecord,
	canonicalMeta conversationMetadataRecord,
) (conversationPartitionPlanState, error) {
	state := conversationPartitionPlanState{
		DesiredRows:    map[string]conversationMigrationPut{},
		OriginalRows:   map[string]map[string]types.AttributeValue{},
		Deletes:        map[string]conversationMigrationDelete{},
		LatestStatusID: canonicalMeta.LastStatusID,
	}

	for _, conversationID := range orderedConversationIDs(conversations, canonicalMeta.ConversationID) {
		partitionItems, err := queryConversationPartitionItems(ctx, client, tableName, conversationID)
		if err != nil {
			return conversationPartitionPlanState{}, fmt.Errorf("query conversation partition %q: %w", conversationID, err)
		}
		updateConversationPartitionPlanState(&state, partitionItems, canonicalMeta.ConversationID)
	}

	updateConversationPartitionMessageStats(&state)
	return state, nil
}

func orderedConversationIDs(conversations []conversationMetadataRecord, canonicalConversationID string) []string {
	orderedConversationIDs := make([]string, 0, len(conversations))
	orderedConversationIDs = append(orderedConversationIDs, canonicalConversationID)
	for _, record := range conversations {
		if record.ConversationID == canonicalConversationID {
			continue
		}
		orderedConversationIDs = append(orderedConversationIDs, record.ConversationID)
	}
	return orderedConversationIDs
}

func updateConversationPartitionPlanState(
	state *conversationPartitionPlanState,
	partitionItems []map[string]types.AttributeValue,
	canonicalConversationID string,
) {
	for _, item := range partitionItems {
		itemSK, _ := attributeString(item["SK"])
		if itemSK == conversationMetadataSortKey {
			continue
		}

		desiredItem := cloneAttributeMap(item)
		setStringAttribute(desiredItem, item, "PK", conversationPartitionKey(canonicalConversationID))
		if _, ok := item[conversationIDAttribute]; ok {
			setStringAttribute(desiredItem, item, conversationIDAttribute, canonicalConversationID)
		}

		putKey := attributeMapKey(desiredItem)
		if _, exists := state.DesiredRows[putKey]; !exists {
			state.DesiredRows[putKey] = conversationMigrationPut{
				Category: "conversation",
				Item:     desiredItem,
			}
			state.OriginalRows[putKey] = item
		}

		originalKey := attributeMapKey(item)
		if originalKey != putKey {
			state.Deletes[originalKey] = conversationMigrationDelete{
				Category: "conversation",
				Key:      attributeMapKeyAttributes(item),
			}
		}
	}
}

func updateConversationPartitionMessageStats(state *conversationPartitionPlanState) {
	for _, put := range state.DesiredRows {
		if !isConversationMessageRow(put.Item) {
			continue
		}
		state.MessageCount++
		createdAt := conversationMessageCreatedAt(put.Item)
		if createdAt.After(state.LatestMessageTime) {
			state.LatestMessageTime = createdAt
			if statusID, ok := attributeString(put.Item[conversationMessageStatusIDAttribute]); ok && strings.TrimSpace(statusID) != "" {
				state.LatestStatusID = statusID
			}
		}
	}
}

func planConversationMetadataRow(
	plan *conversationMigrationPlan,
	canonicalMeta conversationMetadataRecord,
	canonicalState conversationCanonicalState,
) {
	desiredMetadata := cloneAttributeMap(canonicalMeta.Item)
	setStringAttribute(desiredMetadata, canonicalMeta.Item, "PK", conversationPartitionKey(canonicalState.ID))
	setStringAttribute(desiredMetadata, canonicalMeta.Item, "SK", conversationMetadataSortKey)
	setStringAttribute(desiredMetadata, canonicalMeta.Item, "id", canonicalState.ID)
	setStringSliceAttribute(desiredMetadata, canonicalMeta.Item, conversationParticipantsAttribute, canonicalState.Participants)
	setTimeAttribute(desiredMetadata, canonicalMeta.Item, conversationCreatedAtAttribute, canonicalState.CreatedAt)
	setTimeAttribute(desiredMetadata, canonicalMeta.Item, conversationUpdatedAtAttribute, canonicalState.UpdatedAt)
	setOptionalStringAttribute(desiredMetadata, canonicalMeta.Item, conversationLastStatusIDAttribute, canonicalState.LastStatusID)
	setInt64Attribute(desiredMetadata, canonicalMeta.Item, conversationTotalMessageCountAttribute, canonicalState.TotalMessageCount)
	setTimeAttribute(desiredMetadata, canonicalMeta.Item, conversationLastMessageTimeAttribute, canonicalState.LastMessageTime)
	plan.putIfChanged("conversation", canonicalMeta.Item, desiredMetadata)
}

func (p *conversationMigrationPlan) deleteDuplicateConversationMetadata(conversations []conversationMetadataRecord) {
	for _, record := range conversations[1:] {
		p.delete("conversation", conversationPartitionKey(record.ConversationID), conversationMetadataSortKey)
	}
}

func planConversationPartitionPlan(plan *conversationMigrationPlan, state conversationPartitionPlanState) {
	for key, put := range state.DesiredRows {
		plan.putIfChanged(put.Category, state.OriginalRows[key], put.Item)
	}
	for _, del := range state.Deletes {
		plan.deleteWithKey(del.Category, del.Key)
	}
}

func planConversationParticipantRows(
	plan *conversationMigrationPlan,
	records []conversationParticipantRecordItem,
	canonicalConversationID string,
	canonicalState conversationCanonicalState,
) error {
	for participantID, bucket := range bucketParticipantRecords(records) {
		best := selectCanonicalParticipantRecord(bucket, canonicalConversationID)
		if best == nil {
			continue
		}

		desiredParticipant := cloneAttributeMap(best.Item)
		setStringAttribute(desiredParticipant, best.Item, "PK", conversationParticipantPartitionPrefix+participantID)
		setStringAttribute(desiredParticipant, best.Item, "SK", conversationParticipantSortKey(canonicalState.UpdatedAt, canonicalState.ID))
		setStringAttribute(desiredParticipant, best.Item, "gsi1PK", conversationPartitionKey(canonicalState.ID))
		setStringAttribute(desiredParticipant, best.Item, "gsi1SK", "PARTICIPANT#"+participantID)
		setBoolAttribute(desiredParticipant, best.Item, conversationUnreadAttribute, best.Unread)
		if err := setConversationSnapshotAttribute(desiredParticipant, canonicalState, best.Unread); err != nil {
			return fmt.Errorf("marshal conversation snapshot for %q: %w", participantID, err)
		}

		plan.putIfChanged("participant", best.Item, desiredParticipant)
		deleteLegacyParticipantRows(plan, bucket, desiredParticipant)
	}
	return nil
}

func bucketParticipantRecords(records []conversationParticipantRecordItem) map[string][]conversationParticipantRecordItem {
	buckets := map[string][]conversationParticipantRecordItem{}
	for _, record := range records {
		buckets[record.CanonicalParticipant] = append(buckets[record.CanonicalParticipant], record)
	}
	return buckets
}

func deleteLegacyParticipantRows(
	plan *conversationMigrationPlan,
	records []conversationParticipantRecordItem,
	desiredParticipant map[string]types.AttributeValue,
) {
	for _, legacy := range records {
		if attributeMapKey(legacy.Item) == attributeMapKey(desiredParticipant) {
			continue
		}
		plan.delete("participant", legacy.PK, legacy.SK)
	}
}

func planConversationStatusRows(
	plan *conversationMigrationPlan,
	records []conversationStatusRecord,
	canonicalConversationID string,
	canonicalState conversationCanonicalState,
) {
	for userID, bucket := range bucketConversationStatusRecords(records) {
		best := selectCanonicalStatusRecord(bucket, canonicalConversationID)
		if best == nil {
			continue
		}

		mergedUnread, mergedLastReadAt := mergeStatusBucket(bucket)
		desiredStatus := cloneAttributeMap(best.Item)
		setStringAttribute(desiredStatus, best.Item, "PK", conversationStatusPartitionPrefix+canonicalState.ID)
		setStringAttribute(desiredStatus, best.Item, "SK", "USER#"+userID)
		setStringAttribute(desiredStatus, best.Item, conversationIDAttribute, canonicalState.ID)
		setStringAttribute(desiredStatus, best.Item, conversationUserIDAttribute, userID)
		setBoolAttribute(desiredStatus, best.Item, conversationUnreadAttribute, mergedUnread)
		setTimeAttribute(desiredStatus, best.Item, conversationLastReadAtAttribute, mergedLastReadAt)

		plan.putIfChanged("status", best.Item, desiredStatus)
		deleteLegacyStatusRows(plan, bucket, desiredStatus)
	}
}

func bucketConversationStatusRecords(records []conversationStatusRecord) map[string][]conversationStatusRecord {
	buckets := map[string][]conversationStatusRecord{}
	for _, record := range records {
		buckets[record.CanonicalUser] = append(buckets[record.CanonicalUser], record)
	}
	return buckets
}

func deleteLegacyStatusRows(
	plan *conversationMigrationPlan,
	records []conversationStatusRecord,
	desiredStatus map[string]types.AttributeValue,
) {
	for _, legacy := range records {
		if attributeMapKey(legacy.Item) == attributeMapKey(desiredStatus) {
			continue
		}
		plan.delete("status", legacy.PK, legacy.SK)
	}
}

func planConversationLookupRow(
	plan *conversationMigrationPlan,
	group *conversationMigrationGroup,
	canonicalConversationID string,
) {
	lookupOriginal := selectCanonicalLookupOriginal(group, canonicalConversationID)
	desiredLookup := cloneAttributeMap(lookupOriginal)
	if desiredLookup == nil {
		desiredLookup = map[string]types.AttributeValue{}
	}
	setStringAttribute(desiredLookup, lookupOriginal, "PK", conversationParticipantLookupPrefix+group.ParticipantKey)
	setStringAttribute(desiredLookup, lookupOriginal, "SK", conversationParticipantLookupSortKey)
	setStringAttribute(desiredLookup, lookupOriginal, "gsi1PK", conversationParticipantLookupPrefix+group.ParticipantKey)
	delete(desiredLookup, "ConversationID")
	setStringAttribute(desiredLookup, lookupOriginal, conversationLookupConversationIDAttr, canonicalConversationID)

	plan.putIfChanged("lookup", lookupOriginal, desiredLookup)
	for _, lookup := range group.LookupRows {
		if attributeMapKey(lookup.Item) == attributeMapKey(desiredLookup) {
			continue
		}
		plan.delete("lookup", lookup.PK, lookup.SK)
	}
}

func selectCanonicalLookupOriginal(
	group *conversationMigrationGroup,
	canonicalConversationID string,
) map[string]types.AttributeValue {
	for _, lookup := range group.LookupRows {
		if lookup.CanonicalParticipantKey == group.ParticipantKey && lookup.ConversationID == canonicalConversationID {
			return lookup.Item
		}
	}
	if len(group.LookupRows) == 0 {
		return nil
	}
	return group.LookupRows[0].Item
}

func buildConversationCanonicalState(
	conversations []conversationMetadataRecord,
	canonicalMeta conversationMetadataRecord,
	messageCount int64,
	latestMessageTime time.Time,
	latestStatusID string,
) conversationCanonicalState {
	state := conversationCanonicalState{
		ID:           canonicalMeta.ConversationID,
		Participants: append([]string(nil), canonicalMeta.CanonicalParticipants...),
		CreatedAt:    canonicalMeta.CreatedAt,
		UpdatedAt:    canonicalMeta.UpdatedAt,
		LastStatusID: canonicalMeta.LastStatusID,
	}

	maxExistingMessageCount := canonicalMeta.TotalMessageCount
	maxExistingLastMessageTime := canonicalMeta.LastMessageTime
	mostRecentStatusID := canonicalMeta.LastStatusID

	for _, record := range conversations {
		if record.TotalMessageCount > maxExistingMessageCount {
			maxExistingMessageCount = record.TotalMessageCount
		}
		if record.CreatedAt.Before(state.CreatedAt) || state.CreatedAt.IsZero() {
			state.CreatedAt = record.CreatedAt
		}
		if record.UpdatedAt.After(state.UpdatedAt) {
			state.UpdatedAt = record.UpdatedAt
		}
		if record.LastMessageTime.After(maxExistingLastMessageTime) {
			maxExistingLastMessageTime = record.LastMessageTime
			mostRecentStatusID = record.LastStatusID
		}
	}

	if messageCount > 0 {
		state.TotalMessageCount = messageCount
	} else {
		state.TotalMessageCount = maxExistingMessageCount
	}
	if latestMessageTime.After(maxExistingLastMessageTime) {
		state.LastMessageTime = latestMessageTime
		state.LastStatusID = latestStatusID
	} else {
		state.LastMessageTime = maxExistingLastMessageTime
		state.LastStatusID = mostRecentStatusID
	}
	if state.LastMessageTime.After(state.UpdatedAt) {
		state.UpdatedAt = state.LastMessageTime
	}
	if state.LastStatusID == "" {
		state.LastStatusID = latestStatusID
	}
	return state
}

func (p *conversationMigrationPlan) putIfChanged(category string, original map[string]types.AttributeValue, desired map[string]types.AttributeValue) {
	if desired == nil {
		return
	}
	if original != nil && attributeMapKey(original) == attributeMapKey(desired) && reflect.DeepEqual(original, desired) {
		return
	}
	key := attributeMapKey(desired)
	delete(p.Deletes, key)
	p.Puts[key] = conversationMigrationPut{Category: category, Item: desired}
}

func (p *conversationMigrationPlan) delete(category string, pk string, sk string) {
	p.deleteWithKey(category, map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: pk},
		"SK": &types.AttributeValueMemberS{Value: sk},
	})
}

func (p *conversationMigrationPlan) deleteWithKey(category string, key map[string]types.AttributeValue) {
	if key == nil {
		return
	}
	keyName := attributeMapKey(key)
	if _, ok := p.Puts[keyName]; ok {
		return
	}
	p.Deletes[keyName] = conversationMigrationDelete{Category: category, Key: key}
}

func applyConversationMigrationPlan(
	ctx context.Context,
	client conversationMigrationClient,
	tableName string,
	plan *conversationMigrationPlan,
	summary *conversationMigrationSummary,
) error {
	putKeys := make([]string, 0, len(plan.Puts))
	for key := range plan.Puts {
		putKeys = append(putKeys, key)
	}
	sort.Strings(putKeys)

	for _, key := range putKeys {
		put := plan.Puts[key]
		if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item:      put.Item,
		}); err != nil {
			return fmt.Errorf("put %s row %s: %w", put.Category, key, err)
		}
		switch put.Category {
		case "conversation":
			summary.ConversationRowsUpserted++
		case "participant":
			summary.ParticipantRowsUpserted++
		case "status":
			summary.StatusRowsUpserted++
		case "lookup":
			summary.LookupRowsUpserted++
		}
	}

	deleteKeys := make([]string, 0, len(plan.Deletes))
	for key := range plan.Deletes {
		deleteKeys = append(deleteKeys, key)
	}
	sort.Strings(deleteKeys)

	for _, key := range deleteKeys {
		del := plan.Deletes[key]
		if _, err := client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key:       del.Key,
		}); err != nil {
			return fmt.Errorf("delete %s row %s: %w", del.Category, key, err)
		}
		switch del.Category {
		case "conversation":
			summary.ConversationRowsDeleted++
		case "participant":
			summary.ParticipantRowsDeleted++
		case "status":
			summary.StatusRowsDeleted++
		case "lookup":
			summary.LookupRowsDeleted++
		}
	}

	return nil
}

func ensureConversationMigrationGroup(groups map[string]*conversationMigrationGroup, key string) *conversationMigrationGroup {
	group := groups[key]
	if group != nil {
		return group
	}
	group = &conversationMigrationGroup{ParticipantKey: key}
	groups[key] = group
	return group
}

func conversationMetadataLess(a conversationMetadataRecord, b conversationMetadataRecord) bool {
	if a.LastMessageTime.Equal(b.LastMessageTime) {
		if a.UpdatedAt.Equal(b.UpdatedAt) {
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.ConversationID < b.ConversationID
			}
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.UpdatedAt.Before(b.UpdatedAt)
	}
	return a.LastMessageTime.Before(b.LastMessageTime)
}

func anyParticipantUnread(records []conversationParticipantRecordItem) bool {
	for _, record := range records {
		if record.Unread {
			return true
		}
	}
	return false
}

func anyStatusUnread(records []conversationStatusRecord) bool {
	for _, record := range records {
		if record.Unread {
			return true
		}
	}
	return false
}

func selectCanonicalParticipantRecord(records []conversationParticipantRecordItem, canonicalConversationID string) *conversationParticipantRecordItem {
	if len(records) == 0 {
		return nil
	}
	best := records[0]
	for _, record := range records[1:] {
		if participantRecordPreferred(record, best, canonicalConversationID) {
			best = record
		}
	}
	return &best
}

func participantRecordPreferred(a conversationParticipantRecordItem, b conversationParticipantRecordItem, canonicalConversationID string) bool {
	if a.SortTime.Equal(b.SortTime) {
		aCanonical := a.ConversationID == canonicalConversationID
		bCanonical := b.ConversationID == canonicalConversationID
		if aCanonical != bCanonical {
			return aCanonical
		}
		return attributeMapKey(a.Item) < attributeMapKey(b.Item)
	}
	return a.SortTime.After(b.SortTime)
}

func selectCanonicalStatusRecord(records []conversationStatusRecord, canonicalConversationID string) *conversationStatusRecord {
	if len(records) == 0 {
		return nil
	}
	best := records[0]
	for _, record := range records[1:] {
		if statusRecordPreferred(record, best, canonicalConversationID) {
			best = record
		}
	}
	return &best
}

func statusRecordPreferred(a conversationStatusRecord, b conversationStatusRecord, canonicalConversationID string) bool {
	if a.LastReadAt.Equal(b.LastReadAt) {
		aCanonical := a.ConversationID == canonicalConversationID
		bCanonical := b.ConversationID == canonicalConversationID
		if aCanonical != bCanonical {
			return aCanonical
		}
		if a.Unread != b.Unread {
			return a.Unread
		}
		return attributeMapKey(a.Item) < attributeMapKey(b.Item)
	}
	return a.LastReadAt.After(b.LastReadAt)
}

func mergeStatusBucket(records []conversationStatusRecord) (bool, time.Time) {
	if len(records) == 0 {
		return conversationMigrationStatusUnreadDefault, time.Time{}
	}

	anyUnread := false
	earliestUnreadReadAt := time.Time{}
	latestReadAt := time.Time{}
	for _, record := range records {
		if record.LastReadAt.After(latestReadAt) {
			latestReadAt = record.LastReadAt
		}
		if !record.Unread {
			continue
		}
		anyUnread = true
		if earliestUnreadReadAt.IsZero() || record.LastReadAt.Before(earliestUnreadReadAt) {
			earliestUnreadReadAt = record.LastReadAt
		}
	}
	if anyUnread {
		return true, earliestUnreadReadAt
	}
	return false, latestReadAt
}

func conversationIDFromParticipantItem(item map[string]types.AttributeValue, sk string) string {
	if gsi1PK, ok := attributeString(item["gsi1PK"]); ok && strings.HasPrefix(gsi1PK, conversationMetadataPartitionPrefix) {
		return strings.TrimPrefix(gsi1PK, conversationMetadataPartitionPrefix)
	}
	if conversationValue, ok := item[conversationSnapshotAttribute].(*types.AttributeValueMemberM); ok {
		for _, key := range []string{"ID", "id"} {
			if conversationID, ok := attributeString(conversationValue.Value[key]); ok && strings.TrimSpace(conversationID) != "" {
				return conversationID
			}
		}
	}
	parts := strings.SplitN(sk, "#", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func conversationParticipantSortTime(sk string) time.Time {
	parts := strings.SplitN(sk, "#", 2)
	if len(parts) == 0 {
		return time.Time{}
	}
	ts, _ := parseFlexibleTime(parts[0])
	return ts
}

func conversationPartitionKey(conversationID string) string {
	return conversationMetadataPartitionPrefix + conversationID
}

func conversationParticipantSortKey(updatedAt time.Time, conversationID string) string {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	return updatedAt.UTC().Format(time.RFC3339) + "#" + conversationID
}

func canonicalConversationParticipantKey(participantKey string) string {
	parts := strings.Split(participantKey, ",")
	return strings.Join(models.CanonicalConversationParticipants(parts), ",")
}

func isConversationMessageRow(item map[string]types.AttributeValue) bool {
	sk, ok := attributeString(item["SK"])
	return ok && strings.HasPrefix(sk, conversationMessageSortKeyPrefix)
}

func conversationMessageCreatedAt(item map[string]types.AttributeValue) time.Time {
	if createdAt, ok := attributeTime(item[conversationCreatedAtAttribute]); ok {
		return createdAt
	}
	sk, _ := attributeString(item["SK"])
	parts := strings.Split(sk, "#")
	if len(parts) >= 3 {
		createdAt, _ := parseFlexibleTime(parts[1])
		return createdAt
	}
	return time.Time{}
}

func setConversationSnapshotAttribute(item map[string]types.AttributeValue, state conversationCanonicalState, unread bool) error {
	snapshot := &models.ConversationSnapshot{
		ID:                state.ID,
		Participants:      append([]string(nil), state.Participants...),
		LastStatusID:      state.LastStatusID,
		Unread:            unread,
		CreatedAt:         state.CreatedAt,
		UpdatedAt:         state.UpdatedAt,
		TotalMessageCount: state.TotalMessageCount,
		LastMessageTime:   state.LastMessageTime,
	}

	encoded, err := attributevalue.Marshal(snapshot)
	if err != nil {
		return err
	}
	item[conversationSnapshotAttribute] = encoded
	return nil
}

func attributeMapKey(item map[string]types.AttributeValue) string {
	pk, _ := attributeString(item["PK"])
	sk, _ := attributeString(item["SK"])
	return pk + "\x00" + sk
}

func attributeMapKeyAttributes(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	pk, _ := attributeString(item["PK"])
	sk, _ := attributeString(item["SK"])
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: pk},
		"SK": &types.AttributeValueMemberS{Value: sk},
	}
}

func setStringAttribute(item map[string]types.AttributeValue, original map[string]types.AttributeValue, key string, value string) {
	if original != nil {
		if current, ok := attributeString(original[key]); ok && current == value {
			item[key] = cloneAttributeValue(original[key])
			return
		}
	}
	item[key] = &types.AttributeValueMemberS{Value: value}
}

func setOptionalStringAttribute(item map[string]types.AttributeValue, original map[string]types.AttributeValue, key string, value string) {
	if strings.TrimSpace(value) == "" {
		delete(item, key)
		return
	}
	setStringAttribute(item, original, key, value)
}

func setStringSliceAttribute(item map[string]types.AttributeValue, original map[string]types.AttributeValue, key string, value []string) {
	if original != nil {
		if current, ok := attributeStringSlice(original[key]); ok && reflect.DeepEqual(current, value) {
			item[key] = cloneAttributeValue(original[key])
			return
		}
	}
	encoded, err := attributevalue.Marshal(value)
	if err == nil {
		item[key] = encoded
	}
}

func setBoolAttribute(item map[string]types.AttributeValue, original map[string]types.AttributeValue, key string, value bool) {
	if original != nil {
		if current, ok := attributeBool(original[key]); ok && current == value {
			item[key] = cloneAttributeValue(original[key])
			return
		}
	}
	item[key] = &types.AttributeValueMemberBOOL{Value: value}
}

func setInt64Attribute(item map[string]types.AttributeValue, original map[string]types.AttributeValue, key string, value int64) {
	if original != nil {
		if current, ok := attributeInt64(original[key]); ok && current == value {
			item[key] = cloneAttributeValue(original[key])
			return
		}
	}
	item[key] = &types.AttributeValueMemberN{Value: strconv.FormatInt(value, 10)}
}

func setTimeAttribute(item map[string]types.AttributeValue, original map[string]types.AttributeValue, key string, value time.Time) {
	if value.IsZero() {
		delete(item, key)
		return
	}
	if original != nil {
		if current, ok := attributeTime(original[key]); ok && current.Equal(value) {
			item[key] = cloneAttributeValue(original[key])
			return
		}
	}
	item[key] = &types.AttributeValueMemberS{Value: value.UTC().Format(time.RFC3339Nano)}
}

func attributeStringSlice(value types.AttributeValue) ([]string, bool) {
	switch typed := value.(type) {
	case *types.AttributeValueMemberL:
		values := make([]string, 0, len(typed.Value))
		for _, raw := range typed.Value {
			s, ok := attributeString(raw)
			if !ok {
				return nil, false
			}
			values = append(values, s)
		}
		return values, true
	case *types.AttributeValueMemberSS:
		return append([]string(nil), typed.Value...), true
	default:
		return nil, false
	}
}

func firstAttributeString(item map[string]types.AttributeValue, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := attributeString(item[key]); ok && strings.TrimSpace(value) != "" {
			return value, true
		}
	}
	return "", false
}

func attributeBool(value types.AttributeValue) (bool, bool) {
	typed, ok := value.(*types.AttributeValueMemberBOOL)
	if !ok {
		return false, false
	}
	return typed.Value, true
}

func attributeInt64(value types.AttributeValue) (int64, bool) {
	switch typed := value.(type) {
	case *types.AttributeValueMemberN:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed.Value), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case *types.AttributeValueMemberS:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed.Value), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func attributeTime(value types.AttributeValue) (time.Time, bool) {
	switch typed := value.(type) {
	case *types.AttributeValueMemberS:
		return parseFlexibleTime(typed.Value)
	case *types.AttributeValueMemberN:
		return parseFlexibleTime(typed.Value)
	default:
		return time.Time{}, false
	}
}

func parseFlexibleTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}

	if unixSeconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		switch {
		case len(value) >= 16:
			return time.Unix(0, unixSeconds).UTC(), true
		case len(value) >= 13:
			return time.UnixMilli(unixSeconds).UTC(), true
		default:
			return time.Unix(unixSeconds, 0).UTC(), true
		}
	}

	return time.Time{}, false
}
