package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	conversationsvc "github.com/equaltoai/lesser/pkg/services/conversations"
	notessvc "github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	lessertheorydb "github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"go.uber.org/zap"
)

type statusContractAccountWriter interface {
	CreateAccount(ctx context.Context, account *storage.Account) error
}

type statusContractUserPreferenceWriter interface {
	UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error
}

type statusContractNoteCreator interface {
	CreateNote(ctx context.Context, cmd *notessvc.CreateNoteCommand) (*notessvc.NoteResult, error)
}

type statusContractConversationSender interface {
	SendDirectMessage(ctx context.Context, cmd *conversationsvc.SendDirectMessageCommand) (*conversationsvc.MessageResult, error)
	SendMessage(ctx context.Context, cmd *conversationsvc.SendMessageCommand) (*conversationsvc.MessageResult, error)
}

type statusContractStatusReader interface {
	GetStatus(ctx context.Context, statusID string) (*models.Status, error)
	GetStatusesByHashtag(ctx context.Context, hashtag string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error)
	GetConversationThread(ctx context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error)
}

type statusContractConversationStateReader interface {
	GetUserConversationState(ctx context.Context, viewerID, conversationID string) (*interfaces.UserConversationStateContract, error)
}

type statusContractPersistenceReader interface {
	VerifyStoredStatusContext(ctx context.Context, statusID string) error
}

type statusContractVerifierDeps struct {
	accountWriter           statusContractAccountWriter
	userPreferenceWriter    statusContractUserPreferenceWriter
	noteCreator             statusContractNoteCreator
	conversationSender      statusContractConversationSender
	statusReader            statusContractStatusReader
	conversationStateReader statusContractConversationStateReader
	persistenceReader       statusContractPersistenceReader
}

type statusContractVerificationFixture struct {
	senderUsername    string
	recipientUsername string
	publicTag         string
	publicContent     string
	firstDMContent    string
	secondDMContent   string
}

type statusContractVerificationSummary struct {
	TableName         string
	ResolvedStage     string
	ResolvedApp       string
	ResolvedEnv       string
	SenderUsername    string
	RecipientUsername string
	PublicStatusID    string
	FirstDMStatusID   string
	SecondDMStatusID  string
	ConversationID    string
	PublicHashtag     string
}

type verifiedStatusContractSend struct {
	statusID       string
	conversationID string
	recipientActor string
}

var (
	statusContractNowFn      = time.Now
	newStatusContractRunIDFn = func() string {
		return strings.ToLower(strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
	}
	statusContractDirectReadAttempts           = 5
	statusContractDirectReadWait               = 200 * time.Millisecond
	statusContractEventuallyConsistentAttempts = 12
	statusContractEventuallyConsistentReadWait = 500 * time.Millisecond
	newStatusContractVerifierDepsFn            = func(db core.DB, tableName string, domain string) statusContractVerifierDeps {
		logger := zap.NewNop()
		statusRepo := repositories.NewStatusRepository(db, tableName, logger, nil)
		accountRepo := repositories.NewAccountRepository(db, tableName, domain, logger)
		userRepo := repositories.NewUserRepository(db, tableName, logger)
		conversationRepo := repositories.NewConversationRepository(db, tableName, logger, nil)
		noops := streaming.NewNoopPublisher()

		return statusContractVerifierDeps{
			accountWriter:           accountRepo,
			userPreferenceWriter:    userRepo,
			noteCreator:             notessvc.NewService(statusRepo, accountRepo, nil, nil, nil, nil, nil, conversationRepo, nil, nil, nil, userRepo, nil, noops, nil, nil, nil, nil, logger, domain),
			conversationSender:      conversationsvc.NewService(conversationRepo, statusRepo, nil, accountRepo, nil, userRepo, nil, nil, noops, nil, logger, domain),
			statusReader:            statusRepo,
			conversationStateReader: conversationRepo,
		}
	}
)

type dynamoStatusContractPersistenceReader struct {
	client    *dynamodb.Client
	tableName string
}

func (r dynamoStatusContractPersistenceReader) VerifyStoredStatusContext(ctx context.Context, statusID string) error {
	if r.client == nil {
		return fmt.Errorf("dynamodb client is required")
	}
	if strings.TrimSpace(r.tableName) == "" {
		return fmt.Errorf("table name is required")
	}

	output, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(r.tableName),
		ConsistentRead: aws.Bool(true),
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: "status#" + statusID},
			"SK": &ddbtypes.AttributeValueMemberS{Value: "status#" + statusID},
		},
		ProjectionExpression: aws.String("#note"),
		ExpressionAttributeNames: map[string]string{
			"#note": "note",
		},
	})
	if err != nil {
		return err
	}
	if len(output.Item) == 0 {
		return fmt.Errorf("status row missing")
	}

	noteAttr, ok := output.Item["note"]
	if !ok {
		return fmt.Errorf("persisted note payload missing")
	}
	noteMap, ok := noteAttr.(*ddbtypes.AttributeValueMemberM)
	if !ok {
		return fmt.Errorf("persisted note payload malformed: %T", noteAttr)
	}

	if hasAttributeListValues(noteMap.Value["Context"]) {
		return fmt.Errorf("persisted note context must be nested under BaseObject")
	}

	baseObjectAttr, ok := noteMap.Value["BaseObject"]
	if !ok {
		return fmt.Errorf("persisted note base object missing")
	}
	baseObjectMap, ok := baseObjectAttr.(*ddbtypes.AttributeValueMemberM)
	if !ok {
		return fmt.Errorf("persisted note base object malformed: %T", baseObjectAttr)
	}
	if !hasAttributeListValues(baseObjectMap.Value["Context"]) {
		return fmt.Errorf("note base object context missing after persistence")
	}

	return nil
}

func hasAttributeListValues(av ddbtypes.AttributeValue) bool {
	switch typed := av.(type) {
	case *ddbtypes.AttributeValueMemberL:
		return len(typed.Value) > 0
	case *ddbtypes.AttributeValueMemberS:
		return strings.TrimSpace(typed.Value) != ""
	default:
		return false
	}
}

func runVerifyStatusContract(argv []string) error {
	fs := flag.NewFlagSet("lesser verify status-contract", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var app string
	var env string
	var awsProfile string
	var tableName string
	var baseDomain string

	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&env, "env", valueDev, "deployment stage (dev|staging; live is blocked because this verifier writes synthetic data)")
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (env: AWS_PROFILE)")
	fs.StringVar(&tableName, "table", "", "explicit DynamoDB table name override")
	fs.StringVar(&baseDomain, "base-domain", envOrDefault("LESSER_BASE_DOMAIN", ""), "base domain used to normalize local actor urls")

	if err := fs.Parse(argv); err != nil {
		return err
	}
	if strings.TrimSpace(baseDomain) == "" {
		return fmt.Errorf("--base-domain is required (or set LESSER_BASE_DOMAIN)")
	}
	if err := validateStatusContractVerificationTarget(env, tableName); err != nil {
		return err
	}

	ctx := context.Background()
	awsCfg, resolvedTableName, resolvedProfile, err := resolveCommonMigrationCLIOptions(ctx, commonMigrationCLIOptions{
		App:        app,
		Env:        env,
		AWSProfile: awsProfile,
		TableName:  tableName,
	})
	if err != nil {
		return err
	}
	if err := validateStatusContractVerificationTarget(env, resolvedTableName); err != nil {
		return err
	}

	db, err := tabletheoryNewFn(session.Config{
		Region:              awsCfg.Region,
		CredentialsProvider: awsCfg.Credentials,
	})
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := lessertheorydb.RegisterDefaultTypeConverters(db); err != nil {
		return err
	}
	dynamoClient := dynamodb.NewFromConfig(awsCfg)

	prevTableName := models.MainTableName
	models.MainTableName = resolvedTableName
	defer func() {
		models.MainTableName = prevTableName
	}()

	deps := newStatusContractVerifierDepsFn(db, resolvedTableName, resolveAccountHydrationDomain(env, baseDomain))
	deps.persistenceReader = dynamoStatusContractPersistenceReader{
		client:    dynamoClient,
		tableName: resolvedTableName,
	}

	summary, err := executeStatusContractVerification(
		ctx,
		deps,
		newStatusContractVerificationFixture(newStatusContractRunIDFn()),
	)
	if err != nil {
		return err
	}

	summary.TableName = resolvedTableName
	summary.ResolvedApp = strings.TrimSpace(app)
	if summary.ResolvedApp == "" {
		summary.ResolvedApp = naming.DefaultAppName
	}
	summary.ResolvedEnv = strings.TrimSpace(env)

	fmt.Println("verify status-contract complete")
	fmt.Println("table:", summary.TableName)
	fmt.Println("env:", summary.ResolvedEnv)
	if resolvedProfile != "" {
		fmt.Println("aws_profile:", resolvedProfile)
	}
	fmt.Println("sender:", summary.SenderUsername)
	fmt.Println("recipient:", summary.RecipientUsername)
	fmt.Println("public_status_id:", summary.PublicStatusID)
	fmt.Println("first_dm_status_id:", summary.FirstDMStatusID)
	fmt.Println("second_dm_status_id:", summary.SecondDMStatusID)
	fmt.Println("conversation_id:", summary.ConversationID)
	fmt.Println("public_hashtag:", summary.PublicHashtag)

	return nil
}

func executeStatusContractVerification(
	ctx context.Context,
	deps statusContractVerifierDeps,
	fixture statusContractVerificationFixture,
) (statusContractVerificationSummary, error) {
	summary := statusContractVerificationSummary{
		SenderUsername:    fixture.senderUsername,
		RecipientUsername: fixture.recipientUsername,
		PublicHashtag:     fixture.publicTag,
	}

	if err := validateStatusContractVerifierDeps(deps); err != nil {
		return summary, err
	}
	if err := prepareStatusContractVerificationActors(ctx, deps, fixture); err != nil {
		return summary, err
	}

	publicStatusID, err := createAndVerifyPublicStatus(ctx, deps, fixture)
	if err != nil {
		return summary, err
	}
	summary.PublicStatusID = publicStatusID

	firstDirectMessage, err := createAndVerifyFirstDirectMessage(ctx, deps, fixture)
	if err != nil {
		return summary, err
	}
	summary.FirstDMStatusID = firstDirectMessage.statusID
	summary.ConversationID = firstDirectMessage.conversationID

	secondDirectMessage, err := createAndVerifyFollowupDirectMessage(ctx, deps, fixture, firstDirectMessage)
	if err != nil {
		return summary, err
	}
	summary.SecondDMStatusID = secondDirectMessage.statusID

	if err := verifyConversationThread(ctx, deps.statusReader, firstDirectMessage.conversationID, firstDirectMessage.statusID, secondDirectMessage.statusID); err != nil {
		return summary, fmt.Errorf("verify conversation thread %s: %w", firstDirectMessage.conversationID, err)
	}
	if err := verifyConversationStates(ctx, deps.conversationStateReader, fixture.senderUsername, fixture.recipientUsername, firstDirectMessage.conversationID, secondDirectMessage.statusID); err != nil {
		return summary, fmt.Errorf("verify conversation states %s: %w", firstDirectMessage.conversationID, err)
	}

	return summary, nil
}

func validateStatusContractVerificationTarget(env string, tableName string) error {
	if naming.IsLiveEnvironment(env) {
		return fmt.Errorf("verify status-contract writes synthetic data and must not run against live")
	}
	if statusContractTargetsLiveTable(tableName) {
		return fmt.Errorf("verify status-contract writes synthetic data and must not target live table %q", strings.TrimSpace(tableName))
	}
	return nil
}

func statusContractTargetsLiveTable(tableName string) bool {
	tableName = strings.ToLower(strings.TrimSpace(tableName))
	if tableName == "" {
		return false
	}
	return strings.HasSuffix(tableName, "-live-main-table")
}

func validateStatusContractVerifierDeps(deps statusContractVerifierDeps) error {
	if deps.accountWriter == nil || deps.userPreferenceWriter == nil || deps.noteCreator == nil || deps.conversationSender == nil || deps.statusReader == nil || deps.conversationStateReader == nil {
		return fmt.Errorf("status contract verifier dependencies are incomplete")
	}
	if deps.persistenceReader == nil {
		return fmt.Errorf("status contract persistence reader is required")
	}
	return nil
}

func prepareStatusContractVerificationActors(ctx context.Context, deps statusContractVerifierDeps, fixture statusContractVerificationFixture) error {
	if err := deps.accountWriter.CreateAccount(ctx, newStatusContractVerificationAccount(fixture.senderUsername)); err != nil {
		return fmt.Errorf("create sender account: %w", err)
	}
	if err := deps.accountWriter.CreateAccount(ctx, newStatusContractVerificationAccount(fixture.recipientUsername)); err != nil {
		return fmt.Errorf("create recipient account: %w", err)
	}
	if err := deps.userPreferenceWriter.UpdateUserPreferences(ctx, fixture.recipientUsername, &storage.UserPreferences{
		Username:           fixture.recipientUsername,
		DirectMessagesFrom: "ANYONE",
	}); err != nil {
		return fmt.Errorf("set recipient dm preference: %w", err)
	}
	return nil
}

func createAndVerifyPublicStatus(ctx context.Context, deps statusContractVerifierDeps, fixture statusContractVerificationFixture) (string, error) {
	publicResult, err := deps.noteCreator.CreateNote(ctx, &notessvc.CreateNoteCommand{
		AuthorID:   fixture.senderUsername,
		Content:    fixture.publicContent,
		Visibility: models.VisibilityPublic,
	})
	if err != nil {
		return "", common.WrapErrorWithLeafCauses("create public note", err)
	}
	if publicResult == nil || publicResult.Note == nil {
		return "", fmt.Errorf("create public note: nil note result")
	}
	statusID := publicResult.Note.StatusID

	storedPublic, err := loadVerifiedStatus(ctx, deps.statusReader, statusID)
	if err != nil {
		return "", fmt.Errorf("load stored public note %s: %w", statusID, err)
	}
	if err := verifyStoredStatusMetadata(storedPublic, models.VisibilityPublic, publicResult.Note.ConversationID, fixture.publicContent, ""); err != nil {
		return "", fmt.Errorf("verify stored public note %s: %w", statusID, err)
	}
	if err := deps.persistenceReader.VerifyStoredStatusContext(ctx, statusID); err != nil {
		return "", common.WrapErrorWithLeafCauses(fmt.Sprintf("verify persisted public note %s", statusID), err)
	}
	if err := verifyHashtagIndex(ctx, deps.statusReader, fixture.publicTag, statusID); err != nil {
		return "", fmt.Errorf("verify hashtag side effect %s: %w", fixture.publicTag, err)
	}

	return statusID, nil
}

func createAndVerifyFirstDirectMessage(ctx context.Context, deps statusContractVerifierDeps, fixture statusContractVerificationFixture) (verifiedStatusContractSend, error) {
	result, err := deps.conversationSender.SendDirectMessage(ctx, &conversationsvc.SendDirectMessageCommand{
		SenderID:   fixture.senderUsername,
		Recipients: []string{fixture.recipientUsername},
		Content:    fixture.firstDMContent,
	})
	if err != nil {
		return verifiedStatusContractSend{}, common.WrapErrorWithLeafCauses("send first direct message", err)
	}
	return verifyDirectMessageResult(ctx, deps, result, fixture.firstDMContent, "send first direct message", "load stored first direct message", "verify stored first direct message", "verify persisted first direct message")
}

func createAndVerifyFollowupDirectMessage(
	ctx context.Context,
	deps statusContractVerifierDeps,
	fixture statusContractVerificationFixture,
	firstDirectMessage verifiedStatusContractSend,
) (verifiedStatusContractSend, error) {
	result, err := deps.conversationSender.SendMessage(ctx, &conversationsvc.SendMessageCommand{
		SenderID:       fixture.senderUsername,
		ConversationID: firstDirectMessage.conversationID,
		Content:        fixture.secondDMContent,
	})
	if err != nil {
		return verifiedStatusContractSend{}, common.WrapErrorWithLeafCauses("send follow-up direct message", err)
	}
	return verifyDirectMessageResult(ctx, deps, result, fixture.secondDMContent, "send follow-up direct message", "load stored follow-up direct message", "verify stored follow-up direct message", "verify persisted follow-up direct message")
}

func verifyDirectMessageResult(
	ctx context.Context,
	deps statusContractVerifierDeps,
	result *conversationsvc.MessageResult,
	expectedContent string,
	nilResultLabel string,
	loadLabel string,
	verifyStoredLabel string,
	verifyPersistedLabel string,
) (verifiedStatusContractSend, error) {
	if result == nil || result.Message == nil || result.Conversation == nil {
		return verifiedStatusContractSend{}, fmt.Errorf("%s: nil message result", nilResultLabel)
	}
	if len(result.Message.ToRecipients) == 0 {
		return verifiedStatusContractSend{}, fmt.Errorf("%s: missing to recipients", nilResultLabel)
	}

	statusID := result.Message.StatusID
	conversationID := result.Conversation.ID
	recipientActor := result.Message.ToRecipients[0]

	storedStatus, err := loadVerifiedStatus(ctx, deps.statusReader, statusID)
	if err != nil {
		return verifiedStatusContractSend{}, fmt.Errorf("%s %s: %w", loadLabel, statusID, err)
	}
	if err := verifyStoredStatusMetadata(storedStatus, models.VisibilityDirect, conversationID, expectedContent, recipientActor); err != nil {
		return verifiedStatusContractSend{}, fmt.Errorf("%s %s: %w", verifyStoredLabel, statusID, err)
	}
	if err := deps.persistenceReader.VerifyStoredStatusContext(ctx, statusID); err != nil {
		return verifiedStatusContractSend{}, common.WrapErrorWithLeafCauses(fmt.Sprintf("%s %s", verifyPersistedLabel, statusID), err)
	}

	return verifiedStatusContractSend{
		statusID:       statusID,
		conversationID: conversationID,
		recipientActor: recipientActor,
	}, nil
}

func newStatusContractVerificationFixture(runID string) statusContractVerificationFixture {
	runID = strings.TrimSpace(strings.ToLower(runID))
	if runID == "" {
		runID = "statuscheck"
	}

	return statusContractVerificationFixture{
		senderUsername:    "statuschecksender" + runID,
		recipientUsername: "statuscheckrecipient" + runID,
		publicTag:         "statuscontract" + runID,
		publicContent:     fmt.Sprintf("status contract public verification #%s", "statuscontract"+runID),
		firstDMContent:    fmt.Sprintf("status contract first dm %s", runID),
		secondDMContent:   fmt.Sprintf("status contract second dm %s", runID),
	}
}

func newStatusContractVerificationAccount(username string) *storage.Account {
	now := statusContractNowFn().UTC()
	return &storage.Account{
		User: &storage.User{
			Username:     username,
			Email:        fmt.Sprintf("%s@lesser.local", username),
			PasswordHash: "verify-status-contract",
			DisplayName:  username,
			Approved:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
}

func loadVerifiedStatus(ctx context.Context, reader statusContractStatusReader, statusID string) (*models.Status, error) {
	var status *models.Status
	err := retryStatusContractRead(ctx, statusContractDirectReadAttempts, statusContractDirectReadWait, func() error {
		loaded, err := reader.GetStatus(ctx, statusID)
		if err != nil {
			return err
		}
		if loaded == nil {
			return fmt.Errorf("returned nil status")
		}
		status = loaded
		return nil
	})
	return status, err
}

func verifyStoredStatusMetadata(status *models.Status, visibility string, conversationID string, expectedContent string, expectedRecipientActor string) error {
	if status == nil {
		return fmt.Errorf("status is nil")
	}
	if status.Visibility != visibility {
		return fmt.Errorf("visibility mismatch: %s != %s", status.Visibility, visibility)
	}
	if strings.TrimSpace(status.ConversationID) != strings.TrimSpace(conversationID) {
		return fmt.Errorf("conversation id mismatch: %s != %s", status.ConversationID, conversationID)
	}
	if strings.TrimSpace(status.Content) != strings.TrimSpace(expectedContent) {
		return fmt.Errorf("content mismatch")
	}
	if status.Note == nil {
		return fmt.Errorf("note payload missing")
	}
	if visibility == models.VisibilityDirect && strings.TrimSpace(expectedRecipientActor) != "" && !containsString(status.ToRecipients, expectedRecipientActor) {
		return fmt.Errorf("missing direct recipient %s", expectedRecipientActor)
	}
	return nil
}

func verifyHashtagIndex(ctx context.Context, reader statusContractStatusReader, hashtag string, statusID string) error {
	return retryEventuallyConsistentStatusContractRead(ctx, func() error {
		result, err := reader.GetStatusesByHashtag(ctx, hashtag, interfaces.PaginationOptions{Limit: 10})
		if err != nil {
			return err
		}
		if result == nil || !containsStatus(result.Items, statusID) {
			return fmt.Errorf("hashtag query missing status %s", statusID)
		}
		return nil
	})
}

func verifyConversationThread(ctx context.Context, reader statusContractStatusReader, conversationID string, firstStatusID string, secondStatusID string) error {
	return retryEventuallyConsistentStatusContractRead(ctx, func() error {
		thread, err := reader.GetConversationThread(ctx, conversationID, interfaces.PaginationOptions{Limit: 10})
		if err != nil {
			return err
		}
		if thread == nil || !containsStatus(thread.Items, firstStatusID) || !containsStatus(thread.Items, secondStatusID) {
			return fmt.Errorf("conversation thread missing expected statuses")
		}
		return nil
	})
}

func verifyConversationStates(ctx context.Context, reader statusContractConversationStateReader, senderUsername string, recipientUsername string, conversationID string, latestStatusID string) error {
	return retryEventuallyConsistentStatusContractRead(ctx, func() error {
		senderState, err := reader.GetUserConversationState(ctx, senderUsername, conversationID)
		if err != nil {
			return err
		}
		recipientState, err := reader.GetUserConversationState(ctx, recipientUsername, conversationID)
		if err != nil {
			return err
		}
		if senderState == nil || recipientState == nil {
			return fmt.Errorf("conversation states missing")
		}
		if senderState.PreviewStatusID != latestStatusID || recipientState.PreviewStatusID != latestStatusID {
			return fmt.Errorf("preview status mismatch")
		}
		if senderState.Unread {
			return fmt.Errorf("sender state unexpectedly unread")
		}
		if !recipientState.Unread {
			return fmt.Errorf("recipient state unexpectedly read")
		}
		return nil
	})
}

func retryEventuallyConsistentStatusContractRead(ctx context.Context, fn func() error) error {
	return retryStatusContractRead(ctx, statusContractEventuallyConsistentAttempts, statusContractEventuallyConsistentReadWait, fn)
}

func retryStatusContractRead(ctx context.Context, attempts int, wait time.Duration, fn func() error) error {
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == attempts-1 {
			break
		}
		time.Sleep(wait)
	}

	return lastErr
}

func containsStatus(statuses []*models.Status, statusID string) bool {
	for _, status := range statuses {
		if status != nil && status.StatusID == statusID {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(want) {
			return true
		}
	}
	return false
}
