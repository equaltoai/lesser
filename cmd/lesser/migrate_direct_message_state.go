package main

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

type directMessageStateMigrationClient interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

type directMessageStateMigrationSummary struct {
	ScannedConversations       int
	ActiveConversations        int
	StatusBackedConversations  int
	ThreadStatusesScanned      int
	CanonicalStateRowsPlanned  int
	CanonicalStateRowsUpserted int
	LookupRowsPlanned          int
	LookupRowsUpserted         int
	SampleConversationIDs      []string
}

type directMessageMigrationConversation struct {
	ConversationID     string
	Participants       []string
	MetadataItem       map[string]types.AttributeValue
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastStatusID       string
	LastMessageTime    time.Time
	TotalMessageCount  int64
	ThreadStatusItems  []map[string]types.AttributeValue
	ThreadStatusCount  int
	ThreadLastStatusID string
	ThreadLastTime     time.Time
}

type directMessageLegacyParticipantRow struct {
	ConversationID string
	ViewerID       string
	RequestState   models.DmRequestState
	Unread         bool
	LastReadAt     *time.Time
	DeletedAt      *time.Time
	RequestedAt    *time.Time
	AcceptedAt     *time.Time
	DeclinedAt     *time.Time
	SortAt         time.Time
	Item           map[string]types.AttributeValue
}

type directMessageLegacyReadState struct {
	ConversationID string
	ViewerID       string
	Unread         bool
	LastReadAt     *time.Time
	Item           map[string]types.AttributeValue
}

type directMessageCanonicalStateRecord struct {
	State *models.UserConversationState
	Item  map[string]types.AttributeValue
}

type directMessageLookupPlan struct {
	PK             string
	ConversationID string
	Participants   []string
	SortTime       time.Time
}

var newDirectMessageStateMigrationClientFn = func(cfg aws.Config) directMessageStateMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

func runMigrateDirectMessageState(argv []string) error {
	return runCommonMigrationCLI(
		argv,
		"migrate-direct-message-state",
		"maximum number of DM conversations to process (0 = all)",
		"rebuild canonical DM state, repair direct-message mentions, and retire legacy DM rows",
		func(ctx context.Context, awsCfg aws.Config, tableName string, apply bool, limit int) (directMessageStateMigrationSummary, error) {
			return executeDirectMessageStateMigration(
				ctx,
				newDirectMessageStateMigrationClientFn(awsCfg),
				tableName,
				apply,
				limit,
			)
		},
		printDirectMessageStateMigrationSummary,
	)
}

func printDirectMessageStateMigrationSummary(
	summary directMessageStateMigrationSummary,
	tableName string,
	resolvedProfile string,
	apply bool,
) {
	fmt.Printf("migrate-direct-message-state %s complete\n", selectedMigrationMode(apply))
	fmt.Printf("table: %s\n", tableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("scanned_conversations: %d\n", summary.ScannedConversations)
	fmt.Printf("active_conversations: %d\n", summary.ActiveConversations)
	fmt.Printf("status_backed_conversations: %d\n", summary.StatusBackedConversations)
	fmt.Printf("thread_statuses_scanned: %d\n", summary.ThreadStatusesScanned)
	fmt.Printf("canonical_state_rows_planned: %d\n", summary.CanonicalStateRowsPlanned)
	fmt.Printf("canonical_state_rows_upserted: %d\n", summary.CanonicalStateRowsUpserted)
	fmt.Printf("lookup_rows_planned: %d\n", summary.LookupRowsPlanned)
	fmt.Printf("lookup_rows_upserted: %d\n", summary.LookupRowsUpserted)
	printConversationMigrationSamples(summary.SampleConversationIDs)

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to backfill canonical DM state, repair missing mentions, and retire legacy DM rows")
	}
}

func executeDirectMessageStateMigration(
	ctx context.Context,
	client directMessageStateMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (directMessageStateMigrationSummary, error) {
	summary := directMessageStateMigrationSummary{}

	if client == nil {
		return summary, fmt.Errorf("migration client is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return summary, fmt.Errorf("table name is required")
	}

	if apply {
		if err := setDirectMessageMigrationWriteFreeze(ctx, client, tableName, true, "MIGRATING"); err != nil {
			return summary, fmt.Errorf("freeze direct message writes: %w", err)
		}
		defer func() {
			if err := setDirectMessageMigrationWriteFreeze(ctx, client, tableName, false, "COMPLETE"); err != nil {
				panic(fmt.Sprintf("unfreeze direct message writes: %v", err))
			}
		}()
	}

	legacyParticipantRows, err := loadDirectMessageLegacyParticipantRows(ctx, client, tableName)
	if err != nil {
		return summary, err
	}
	legacyReadStates, err := loadDirectMessageLegacyReadStates(ctx, client, tableName)
	if err != nil {
		return summary, err
	}
	existingCanonicalStates, err := loadDirectMessageCanonicalStates(ctx, client, tableName)
	if err != nil {
		return summary, err
	}
	existingLookupRows, err := loadDirectMessageLookupRows(ctx, client, tableName)
	if err != nil {
		return summary, err
	}
	desiredLookupPlans := map[string]directMessageLookupPlan{}

	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: conversationMetadataPartitionPrefix},
			":sk":     &types.AttributeValueMemberS{Value: conversationMetadataSortKey},
		},
	}

	stop := false
	for !stop {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return summary, fmt.Errorf("scan conversation metadata rows: %w", err)
		}

		for _, item := range out.Items {
			summary.ScannedConversations++

			conversation, ok := buildDirectMessageMigrationConversation(item)
			if !ok {
				continue
			}

			threadStatuses, statusCount, lastStatusID, lastStatusTime, err := loadDirectMessageThreadStatuses(ctx, client, tableName, conversation.ConversationID)
			if err != nil {
				return summary, fmt.Errorf("load thread statuses for %q: %w", conversation.ConversationID, err)
			}

			conversation.ThreadStatusItems = threadStatuses
			conversation.ThreadStatusCount = statusCount
			conversation.ThreadLastStatusID = lastStatusID
			conversation.ThreadLastTime = lastStatusTime

			summary.ActiveConversations++
			if statusCount > 0 {
				summary.StatusBackedConversations++
				summary.ThreadStatusesScanned += statusCount
			}
			appendConversationMigrationSample(&summary.SampleConversationIDs, conversation.ConversationID)

			existingByViewer := existingCanonicalStates[conversation.ConversationID]
			legacyParticipantsByViewer := legacyParticipantRows[conversation.ConversationID]
			legacyReadStatesByViewer := legacyReadStates[conversation.ConversationID]

			for _, viewerID := range conversation.Participants {
				viewerKey := models.CanonicalConversationParticipantID(viewerID)
				state, item, changed, err := buildMigratedUserConversationStateItem(
					conversation,
					viewerID,
					existingByViewer[viewerKey],
					legacyParticipantsByViewer[viewerKey],
					legacyReadStatesByViewer[viewerKey],
				)
				if err != nil {
					return summary, fmt.Errorf("build canonical user conversation state for %q/%q: %w", conversation.ConversationID, viewerID, err)
				}
				if state == nil {
					continue
				}

				summary.CanonicalStateRowsPlanned++
				if !apply || !changed {
					continue
				}

				if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
					TableName: aws.String(tableName),
					Item:      item,
				}); err != nil {
					return summary, fmt.Errorf("put canonical user conversation state %s/%s: %w", state.ViewerID, state.ConversationID, err)
				}
				summary.CanonicalStateRowsUpserted++
			}

			lookupPlan := buildDirectMessageLookupPlan(conversation)
			if current, ok := desiredLookupPlans[lookupPlan.PK]; !ok || shouldReplaceDirectMessageLookupPlan(current, lookupPlan) {
				desiredLookupPlans[lookupPlan.PK] = lookupPlan
			}

			if limit > 0 && summary.ActiveConversations >= limit {
				stop = true
				break
			}
		}

		if stop || len(out.LastEvaluatedKey) == 0 {
			break
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}

	lookupKeys := make([]string, 0, len(desiredLookupPlans))
	for key := range desiredLookupPlans {
		lookupKeys = append(lookupKeys, key)
	}
	sort.Strings(lookupKeys)
	summary.LookupRowsPlanned = len(lookupKeys)

	for _, lookupKey := range lookupKeys {
		plan := desiredLookupPlans[lookupKey]
		item := buildDirectMessageLookupItem(plan, existingLookupRows[lookupKey])
		if !apply || reflect.DeepEqual(item, existingLookupRows[lookupKey]) {
			continue
		}
		if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item:      item,
		}); err != nil {
			return summary, fmt.Errorf("put conversation participant lookup %s: %w", lookupKey, err)
		}
		summary.LookupRowsUpserted++
	}

	return summary, nil
}

func buildDirectMessageMigrationConversation(item map[string]types.AttributeValue) (directMessageMigrationConversation, bool) {
	conversationID, ok := firstAttributeString(item, "id", "ID")
	if !ok {
		pk, _ := attributeString(item["PK"])
		conversationID = strings.TrimSpace(strings.TrimPrefix(pk, conversationMetadataPartitionPrefix))
	}
	participants, ok := attributeStringSlice(item[conversationParticipantsAttribute])
	if !ok || strings.TrimSpace(conversationID) == "" || len(participants) == 0 {
		return directMessageMigrationConversation{}, false
	}

	return directMessageMigrationConversation{
		ConversationID:    conversationID,
		Participants:      append([]string(nil), participants...),
		MetadataItem:      cloneAttributeMap(item),
		CreatedAt:         firstConversationTime(item, conversationCreatedAtAttribute, "CreatedAt"),
		UpdatedAt:         firstConversationTime(item, conversationUpdatedAtAttribute, "UpdatedAt"),
		LastStatusID:      firstConversationString(item, conversationLastStatusIDAttribute, "LastStatusID"),
		LastMessageTime:   firstConversationTime(item, conversationLastMessageTimeAttribute, "LastMessageTime"),
		TotalMessageCount: firstConversationInt64(item, conversationTotalMessageCountAttribute, "TotalMessageCount"),
	}, true
}

func loadDirectMessageThreadStatuses(
	ctx context.Context,
	client directMessageStateMigrationClient,
	tableName string,
	conversationID string,
) ([]map[string]types.AttributeValue, int, string, time.Time, error) {
	statusItems := make([]map[string]types.AttributeValue, 0)
	statusCount := 0
	lastStatusID := ""
	lastStatusTime := time.Time{}

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("gsi3"),
		KeyConditionExpression: aws.String("gsi3PK = :conversation"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":conversation": &types.AttributeValueMemberS{Value: conversationMetadataPartitionPrefix + conversationID},
		},
	}

	for {
		out, err := client.Query(ctx, queryInput)
		if err != nil {
			return nil, 0, "", time.Time{}, err
		}

		for _, item := range out.Items {
			statusID, ok := firstAttributeString(item, conversationMessageStatusIDAttribute, "statusID")
			if !ok {
				continue
			}
			publishedAt := firstConversationTime(item, "publishedAt", "PublishedAt")
			if publishedAt.IsZero() {
				continue
			}

			statusItems = append(statusItems, cloneAttributeMap(item))
			statusCount++
			if publishedAt.After(lastStatusTime) || (publishedAt.Equal(lastStatusTime) && statusID > lastStatusID) {
				lastStatusTime = publishedAt.UTC()
				lastStatusID = statusID
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			return statusItems, statusCount, lastStatusID, lastStatusTime, nil
		}
		queryInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func loadDirectMessageLegacyParticipantRows(
	ctx context.Context,
	client directMessageStateMigrationClient,
	tableName string,
) (map[string]map[string]*directMessageLegacyParticipantRow, error) {
	rows := map[string]map[string]*directMessageLegacyParticipantRow{}
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: conversationParticipantPartitionPrefix},
		},
	}

	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return nil, fmt.Errorf("scan legacy participant rows: %w", err)
		}

		for _, item := range out.Items {
			row, ok := buildDirectMessageLegacyParticipantRow(item)
			if !ok {
				continue
			}
			if rows[row.ConversationID] == nil {
				rows[row.ConversationID] = map[string]*directMessageLegacyParticipantRow{}
			}
			rows[row.ConversationID][row.ViewerID] = row
		}

		if len(out.LastEvaluatedKey) == 0 {
			return rows, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func loadDirectMessageLegacyReadStates(
	ctx context.Context,
	client directMessageStateMigrationClient,
	tableName string,
) (map[string]map[string]*directMessageLegacyReadState, error) {
	rows := map[string]map[string]*directMessageLegacyReadState{}
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: conversationStatusPartitionPrefix},
		},
	}

	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return nil, fmt.Errorf("scan legacy conversation status rows: %w", err)
		}

		for _, item := range out.Items {
			row, ok := buildDirectMessageLegacyReadState(item)
			if !ok {
				continue
			}
			if rows[row.ConversationID] == nil {
				rows[row.ConversationID] = map[string]*directMessageLegacyReadState{}
			}
			rows[row.ConversationID][row.ViewerID] = row
		}

		if len(out.LastEvaluatedKey) == 0 {
			return rows, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func loadDirectMessageCanonicalStates(
	ctx context.Context,
	client directMessageStateMigrationClient,
	tableName string,
) (map[string]map[string]*directMessageCanonicalStateRecord, error) {
	rows := map[string]map[string]*directMessageCanonicalStateRecord{}
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: "USER_CONVERSATION_STATE#"},
		},
	}

	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return nil, fmt.Errorf("scan canonical user conversation state rows: %w", err)
		}

		for _, item := range out.Items {
			record, ok := buildDirectMessageCanonicalStateRecord(item)
			if !ok || record.State == nil {
				continue
			}
			if rows[record.State.ConversationID] == nil {
				rows[record.State.ConversationID] = map[string]*directMessageCanonicalStateRecord{}
			}
			rows[record.State.ConversationID][record.State.ViewerID] = record
		}

		if len(out.LastEvaluatedKey) == 0 {
			return rows, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func loadDirectMessageLookupRows(
	ctx context.Context,
	client directMessageStateMigrationClient,
	tableName string,
) (map[string]map[string]types.AttributeValue, error) {
	rows := map[string]map[string]types.AttributeValue{}
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: conversationParticipantLookupPrefix},
			":sk":     &types.AttributeValueMemberS{Value: conversationParticipantLookupSortKey},
		},
	}

	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return nil, fmt.Errorf("scan conversation participant lookup rows: %w", err)
		}

		for _, item := range out.Items {
			pk, ok := attributeString(item["PK"])
			if !ok || strings.TrimSpace(pk) == "" {
				continue
			}
			rows[pk] = cloneAttributeMap(item)
		}

		if len(out.LastEvaluatedKey) == 0 {
			return rows, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func buildDirectMessageLegacyParticipantRow(item map[string]types.AttributeValue) (*directMessageLegacyParticipantRow, bool) {
	pk, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(pk, conversationParticipantPartitionPrefix) {
		return nil, false
	}
	sk, _ := attributeString(item["SK"])
	conversationID := strings.TrimSpace(conversationIDFromParticipantItem(item, sk))
	if conversationID == "" {
		return nil, false
	}

	row := &directMessageLegacyParticipantRow{
		ConversationID: conversationID,
		ViewerID:       models.CanonicalConversationParticipantID(strings.TrimSpace(strings.TrimPrefix(pk, conversationParticipantPartitionPrefix))),
		RequestState:   models.DmRequestState(firstConversationString(item, conversationRequestStateAttribute)),
		Unread:         firstConversationBool(item, conversationUnreadAttribute),
		LastReadAt:     optionalConversationTime(item, conversationLastReadAtAttribute),
		DeletedAt:      optionalConversationTime(item, "deletedAt"),
		RequestedAt:    optionalConversationTime(item, "requestedAt"),
		AcceptedAt:     optionalConversationTime(item, "acceptedAt"),
		DeclinedAt:     optionalConversationTime(item, "declinedAt"),
		SortAt:         conversationParticipantSortTime(sk),
		Item:           cloneAttributeMap(item),
	}

	return row, row.ViewerID != ""
}

func buildDirectMessageLegacyReadState(item map[string]types.AttributeValue) (*directMessageLegacyReadState, bool) {
	pk, ok := attributeString(item["PK"])
	if !ok || !strings.HasPrefix(pk, conversationStatusPartitionPrefix) {
		return nil, false
	}
	conversationID := strings.TrimSpace(strings.TrimPrefix(pk, conversationStatusPartitionPrefix))
	viewerID, ok := firstAttributeString(item, conversationUserIDAttribute, "userID")
	if !ok {
		sk, _ := attributeString(item["SK"])
		viewerID = strings.TrimSpace(strings.TrimPrefix(sk, "USER#"))
	}
	viewerID = models.CanonicalConversationParticipantID(viewerID)
	if conversationID == "" || viewerID == "" {
		return nil, false
	}

	return &directMessageLegacyReadState{
		ConversationID: conversationID,
		ViewerID:       viewerID,
		Unread:         firstConversationBool(item, conversationUnreadAttribute),
		LastReadAt:     optionalConversationTime(item, conversationLastReadAtAttribute),
		Item:           cloneAttributeMap(item),
	}, true
}

func buildDirectMessageCanonicalStateRecord(item map[string]types.AttributeValue) (*directMessageCanonicalStateRecord, bool) {
	viewerID, ok := firstAttributeString(item, "viewerID")
	if !ok {
		pk, _ := attributeString(item["PK"])
		viewerID = strings.TrimSpace(strings.TrimPrefix(pk, "USER_CONVERSATION_STATE#"))
	}
	conversationID, ok := firstAttributeString(item, "conversationID")
	if !ok {
		sk, _ := attributeString(item["SK"])
		conversationID = strings.TrimSpace(strings.TrimPrefix(sk, "CONVERSATION#"))
	}
	viewerID = models.CanonicalConversationParticipantID(viewerID)
	if viewerID == "" || conversationID == "" {
		return nil, false
	}

	state := &models.UserConversationState{
		ViewerID:                 viewerID,
		ConversationID:           conversationID,
		CounterpartID:            firstConversationString(item, "counterpartID"),
		Folder:                   models.UserConversationFolder(firstConversationString(item, "folder")),
		RequestState:             models.DmRequestState(firstConversationString(item, "requestState")),
		PreviewStatusID:          firstConversationString(item, "previewStatusID"),
		PreviewStatusPublishedAt: firstConversationTime(item, "previewStatusPublishedAt"),
		SortAt:                   firstConversationTime(item, "sortAt"),
		Unread:                   firstConversationBool(item, "unread"),
		LastReadAt:               optionalConversationTime(item, "lastReadAt"),
		DeletedAt:                optionalConversationTime(item, "deletedAt"),
		RequestedAt:              optionalConversationTime(item, "requestedAt"),
		AcceptedAt:               optionalConversationTime(item, "acceptedAt"),
		DeclinedAt:               optionalConversationTime(item, "declinedAt"),
		CreatedAt:                firstConversationTime(item, "createdAt"),
		UpdatedAt:                firstConversationTime(item, "updatedAt"),
	}

	return &directMessageCanonicalStateRecord{
		State: state,
		Item:  cloneAttributeMap(item),
	}, true
}

func buildMigratedUserConversationStateItem(
	conversation directMessageMigrationConversation,
	viewerID string,
	existing *directMessageCanonicalStateRecord,
	legacyParticipant *directMessageLegacyParticipantRow,
	legacyReadState *directMessageLegacyReadState,
) (*models.UserConversationState, map[string]types.AttributeValue, bool, error) {
	canonicalViewerID := models.CanonicalConversationParticipantID(viewerID)
	if canonicalViewerID == "" {
		return nil, nil, false, nil
	}

	previewStatusID, previewTime := directMessagePreviewForMigration(conversation, existing)
	state := &models.UserConversationState{
		ViewerID:                 canonicalViewerID,
		ConversationID:           conversation.ConversationID,
		CounterpartID:            directMessageCounterpartIDForMigration(canonicalViewerID, conversation.Participants),
		PreviewStatusID:          previewStatusID,
		PreviewStatusPublishedAt: previewTime,
		SortAt:                   directMessageMigrationSortAt(previewTime, conversation, existing),
		CreatedAt:                directMessageMigrationCreatedAt(conversation, existing),
	}

	state.DeletedAt = chooseOptionalConversationTime(existingDeletedAt(existing), legacyParticipantDeletedAt(legacyParticipant))
	state.RequestedAt = chooseOptionalConversationTime(existingRequestedAt(existing), legacyParticipantRequestedAt(legacyParticipant))
	state.AcceptedAt = chooseOptionalConversationTime(existingAcceptedAt(existing), legacyParticipantAcceptedAt(legacyParticipant))
	state.DeclinedAt = chooseOptionalConversationTime(existingDeclinedAt(existing), legacyParticipantDeclinedAt(legacyParticipant))
	state.RequestState = directMessageMigrationRequestState(existing, legacyParticipant)
	state.Folder = directMessageMigrationFolder(existing, state.RequestState, state.DeletedAt)
	if state.RequestState == "" {
		state.RequestState = directMessageMigrationDefaultRequestState(state.Folder)
	}
	state.Unread, state.LastReadAt = directMessageMigrationReadState(existing, legacyReadState, legacyParticipant)
	state.LastReadAt = normalizeLegacyMigrationLastReadAt(state.LastReadAt, state.Unread)
	state.UpdatedAt = directMessageMigrationUpdatedAt(state, conversation, existing)

	var err error
	if existing != nil && existing.State != nil {
		err = state.BeforeUpdate()
	} else {
		err = state.BeforeCreate()
	}
	if err != nil {
		return nil, nil, false, err
	}

	var original map[string]types.AttributeValue
	if existing != nil {
		original = existing.Item
	}
	item := buildUserConversationStateItem(state, original)
	changed := existing == nil || !reflect.DeepEqual(item, original)
	return state, item, changed, nil
}

func buildUserConversationStateItem(state *models.UserConversationState, original map[string]types.AttributeValue) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{}
	setStringAttribute(item, original, "PK", state.PK)
	setStringAttribute(item, original, "SK", state.SK)
	setStringAttribute(item, original, "gsi1PK", state.GSI1PK)
	setStringAttribute(item, original, "gsi1SK", state.GSI1SK)
	setOptionalStringAttribute(item, original, "gsi2PK", state.GSI2PK)
	setOptionalStringAttribute(item, original, "gsi2SK", state.GSI2SK)
	setStringAttribute(item, original, "gsi3PK", state.GSI3PK)
	setStringAttribute(item, original, "gsi3SK", state.GSI3SK)
	setStringAttribute(item, original, "viewerID", state.ViewerID)
	setStringAttribute(item, original, "conversationID", state.ConversationID)
	setStringAttribute(item, original, "counterpartID", state.CounterpartID)
	setStringAttribute(item, original, "folder", string(state.Folder))
	setOptionalStringAttribute(item, original, "requestState", string(state.RequestState))
	setOptionalStringAttribute(item, original, "previewStatusID", state.PreviewStatusID)
	setTimeAttribute(item, original, "previewStatusPublishedAt", state.PreviewStatusPublishedAt)
	setTimeAttribute(item, original, "sortAt", state.SortAt)
	setBoolAttribute(item, original, "unread", state.Unread)
	setOptionalTimeAttribute(item, original, "lastReadAt", state.LastReadAt)
	setOptionalTimeAttribute(item, original, "deletedAt", state.DeletedAt)
	setOptionalTimeAttribute(item, original, "requestedAt", state.RequestedAt)
	setOptionalTimeAttribute(item, original, "acceptedAt", state.AcceptedAt)
	setOptionalTimeAttribute(item, original, "declinedAt", state.DeclinedAt)
	setTimeAttribute(item, original, "createdAt", state.CreatedAt)
	setTimeAttribute(item, original, "updatedAt", state.UpdatedAt)
	return item
}

func buildDirectMessageLookupPlan(conversation directMessageMigrationConversation) directMessageLookupPlan {
	pk := conversationParticipantLookupPrefix + strings.Join(models.CanonicalConversationParticipants(conversation.Participants), ",")
	return directMessageLookupPlan{
		PK:             pk,
		ConversationID: conversation.ConversationID,
		Participants:   append([]string(nil), conversation.Participants...),
		SortTime:       directMessageLookupPlanSortTime(conversation),
	}
}

func shouldReplaceDirectMessageLookupPlan(current directMessageLookupPlan, candidate directMessageLookupPlan) bool {
	if candidate.SortTime.After(current.SortTime) {
		return true
	}
	if candidate.SortTime.Equal(current.SortTime) && candidate.ConversationID < current.ConversationID {
		return true
	}
	return false
}

func directMessageLookupPlanSortTime(conversation directMessageMigrationConversation) time.Time {
	for _, candidate := range []time.Time{
		conversation.ThreadLastTime,
		conversation.LastMessageTime,
		conversation.UpdatedAt,
		conversation.CreatedAt,
	} {
		if !candidate.IsZero() {
			return candidate.UTC()
		}
	}
	return time.Time{}
}

func buildDirectMessageLookupItem(plan directMessageLookupPlan, original map[string]types.AttributeValue) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{}
	setStringAttribute(item, original, "PK", plan.PK)
	setStringAttribute(item, original, "SK", conversationParticipantLookupSortKey)
	setStringAttribute(item, original, "gsi1PK", plan.PK)
	setStringAttribute(item, original, conversationLookupConversationIDAttr, plan.ConversationID)
	return item
}

func setOptionalTimeAttribute(item map[string]types.AttributeValue, original map[string]types.AttributeValue, key string, value *time.Time) {
	if value == nil || value.IsZero() {
		delete(item, key)
		return
	}
	setTimeAttribute(item, original, key, value.UTC())
}

func firstConversationString(item map[string]types.AttributeValue, keys ...string) string {
	value, _ := firstAttributeString(item, keys...)
	return value
}

func firstConversationBool(item map[string]types.AttributeValue, keys ...string) bool {
	for _, key := range keys {
		if value, ok := attributeBool(item[key]); ok {
			return value
		}
	}
	return false
}

func firstConversationInt64(item map[string]types.AttributeValue, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := attributeInt64(item[key]); ok {
			return value
		}
	}
	return 0
}

func firstConversationTime(item map[string]types.AttributeValue, keys ...string) time.Time {
	for _, key := range keys {
		if value, ok := attributeTime(item[key]); ok {
			return value.UTC()
		}
	}
	return time.Time{}
}

func optionalConversationTime(item map[string]types.AttributeValue, key string) *time.Time {
	value, ok := attributeTime(item[key])
	if !ok || value.IsZero() {
		return nil
	}
	return conversationTimePtr(value)
}

func chooseOptionalConversationTime(candidates ...*time.Time) *time.Time {
	for _, candidate := range candidates {
		if candidate == nil || candidate.IsZero() {
			continue
		}
		return conversationTimePtr(candidate.UTC())
	}
	return nil
}

func conversationTimePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

func existingDeletedAt(existing *directMessageCanonicalStateRecord) *time.Time {
	if existing == nil || existing.State == nil {
		return nil
	}
	return existing.State.DeletedAt
}

func existingRequestedAt(existing *directMessageCanonicalStateRecord) *time.Time {
	if existing == nil || existing.State == nil {
		return nil
	}
	return existing.State.RequestedAt
}

func existingAcceptedAt(existing *directMessageCanonicalStateRecord) *time.Time {
	if existing == nil || existing.State == nil {
		return nil
	}
	return existing.State.AcceptedAt
}

func existingDeclinedAt(existing *directMessageCanonicalStateRecord) *time.Time {
	if existing == nil || existing.State == nil {
		return nil
	}
	return existing.State.DeclinedAt
}

func legacyParticipantDeletedAt(participant *directMessageLegacyParticipantRow) *time.Time {
	if participant == nil {
		return nil
	}
	return participant.DeletedAt
}

func legacyParticipantRequestedAt(participant *directMessageLegacyParticipantRow) *time.Time {
	if participant == nil {
		return nil
	}
	return participant.RequestedAt
}

func legacyParticipantAcceptedAt(participant *directMessageLegacyParticipantRow) *time.Time {
	if participant == nil {
		return nil
	}
	return participant.AcceptedAt
}

func legacyParticipantDeclinedAt(participant *directMessageLegacyParticipantRow) *time.Time {
	if participant == nil {
		return nil
	}
	return participant.DeclinedAt
}

func directMessagePreviewForMigration(
	conversation directMessageMigrationConversation,
	existing *directMessageCanonicalStateRecord,
) (string, time.Time) {
	if strings.TrimSpace(conversation.ThreadLastStatusID) != "" {
		return strings.TrimSpace(conversation.ThreadLastStatusID), conversation.ThreadLastTime.UTC()
	}
	if strings.TrimSpace(conversation.LastStatusID) != "" {
		return strings.TrimSpace(conversation.LastStatusID), conversation.LastMessageTime.UTC()
	}
	if existing != nil && existing.State != nil && strings.TrimSpace(existing.State.PreviewStatusID) != "" {
		return strings.TrimSpace(existing.State.PreviewStatusID), existing.State.PreviewStatusPublishedAt.UTC()
	}
	return "", time.Time{}
}

func directMessageMigrationSortAt(
	previewTime time.Time,
	conversation directMessageMigrationConversation,
	existing *directMessageCanonicalStateRecord,
) time.Time {
	if !previewTime.IsZero() {
		return previewTime.UTC()
	}
	if existing != nil && existing.State != nil && !existing.State.SortAt.IsZero() {
		return existing.State.SortAt.UTC()
	}
	for _, candidate := range []time.Time{conversation.LastMessageTime, conversation.UpdatedAt, conversation.CreatedAt} {
		if !candidate.IsZero() {
			return candidate.UTC()
		}
	}
	return time.Now().UTC()
}

func directMessageMigrationCreatedAt(
	conversation directMessageMigrationConversation,
	existing *directMessageCanonicalStateRecord,
) time.Time {
	if existing != nil && existing.State != nil && !existing.State.CreatedAt.IsZero() {
		return existing.State.CreatedAt.UTC()
	}
	for _, candidate := range []time.Time{conversation.CreatedAt, conversation.UpdatedAt, conversation.LastMessageTime} {
		if !candidate.IsZero() {
			return candidate.UTC()
		}
	}
	return time.Now().UTC()
}

func directMessageMigrationRequestState(
	existing *directMessageCanonicalStateRecord,
	participant *directMessageLegacyParticipantRow,
) models.DmRequestState {
	if existing != nil && existing.State != nil && existing.State.RequestState != "" {
		return existing.State.RequestState
	}
	if participant != nil && participant.RequestState != "" {
		return participant.RequestState
	}
	return ""
}

func directMessageMigrationFolder(
	existing *directMessageCanonicalStateRecord,
	requestState models.DmRequestState,
	deletedAt *time.Time,
) models.UserConversationFolder {
	if deletedAt != nil && !deletedAt.IsZero() {
		return models.UserConversationFolderHidden
	}
	if existing != nil && existing.State != nil && existing.State.Folder != "" {
		return existing.State.Folder
	}
	switch requestState {
	case models.DmRequestStatePending:
		return models.UserConversationFolderRequests
	case models.DmRequestStateDeclined:
		return models.UserConversationFolderDeclined
	default:
		return models.UserConversationFolderInbox
	}
}

func directMessageMigrationDefaultRequestState(folder models.UserConversationFolder) models.DmRequestState {
	switch folder {
	case models.UserConversationFolderRequests:
		return models.DmRequestStatePending
	case models.UserConversationFolderDeclined:
		return models.DmRequestStateDeclined
	default:
		return models.DmRequestStateAccepted
	}
}

func directMessageMigrationReadState(
	existing *directMessageCanonicalStateRecord,
	legacyReadState *directMessageLegacyReadState,
	legacyParticipant *directMessageLegacyParticipantRow,
) (bool, *time.Time) {
	if existing != nil && existing.State != nil {
		return existing.State.Unread, existing.State.LastReadAt
	}
	if legacyReadState != nil {
		return legacyReadState.Unread, legacyReadState.LastReadAt
	}
	if legacyParticipant != nil {
		return legacyParticipant.Unread, legacyParticipant.LastReadAt
	}
	return false, nil
}

func normalizeLegacyMigrationLastReadAt(value *time.Time, unread bool) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	normalized := value.UTC()
	if normalized.Before(time.Date(1971, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return nil
	}
	if unread && normalized.Equal(time.Unix(0, 0).UTC()) {
		return nil
	}
	return &normalized
}

func directMessageMigrationUpdatedAt(
	state *models.UserConversationState,
	conversation directMessageMigrationConversation,
	existing *directMessageCanonicalStateRecord,
) time.Time {
	candidates := []time.Time{
		state.CreatedAt,
		state.SortAt,
		state.PreviewStatusPublishedAt,
		conversation.UpdatedAt,
		conversation.LastMessageTime,
	}
	if existing != nil && existing.State != nil && !existing.State.UpdatedAt.IsZero() {
		candidates = append(candidates, existing.State.UpdatedAt)
	}
	for _, candidate := range []*time.Time{
		state.LastReadAt,
		state.DeletedAt,
		state.RequestedAt,
		state.AcceptedAt,
		state.DeclinedAt,
	} {
		if candidate != nil && !candidate.IsZero() {
			candidates = append(candidates, candidate.UTC())
		}
	}

	latest := time.Time{}
	for _, candidate := range candidates {
		if candidate.IsZero() {
			continue
		}
		candidate = candidate.UTC()
		if candidate.After(latest) {
			latest = candidate
		}
	}
	if latest.IsZero() {
		return time.Now().UTC()
	}
	return latest
}

func directMessageCounterpartIDForMigration(viewerID string, participants []string) string {
	canonicalViewerID := models.CanonicalConversationParticipantID(viewerID)
	for _, participantID := range participants {
		if models.CanonicalConversationParticipantID(participantID) == canonicalViewerID {
			continue
		}
		return participantID
	}
	return ""
}

func setDirectMessageMigrationWriteFreeze(
	ctx context.Context,
	client directMessageStateMigrationClient,
	tableName string,
	writesFrozen bool,
	phase string,
) error {
	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: models.DirectMessageMigrationStatePK},
			"SK": &types.AttributeValueMemberS{Value: models.DirectMessageMigrationStateSK},
		},
	})
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	createdAt := firstConversationTime(out.Item, "createdAt")
	if createdAt.IsZero() {
		createdAt = now
	}

	item := map[string]types.AttributeValue{
		"PK":           &types.AttributeValueMemberS{Value: models.DirectMessageMigrationStatePK},
		"SK":           &types.AttributeValueMemberS{Value: models.DirectMessageMigrationStateSK},
		"writesFrozen": &types.AttributeValueMemberBOOL{Value: writesFrozen},
		"phase":        &types.AttributeValueMemberS{Value: phase},
		"reason":       &types.AttributeValueMemberS{Value: "lesser migrate-direct-message-state"},
		"owner":        &types.AttributeValueMemberS{Value: "lesser migrate-direct-message-state"},
		"createdAt":    &types.AttributeValueMemberS{Value: createdAt.Format(time.RFC3339Nano)},
		"updatedAt":    &types.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)},
	}

	_, err = client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})
	return err
}
