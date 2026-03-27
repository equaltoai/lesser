package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	conversationsvc "github.com/equaltoai/lesser/pkg/services/conversations"
	notessvc "github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/notecontract"
	"github.com/stretchr/testify/require"
	theorydb "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type fakeStatusContractAccountWriter struct {
	created []string
	errs    []error
}

func (f *fakeStatusContractAccountWriter) CreateAccount(_ context.Context, account *storage.Account) error {
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return err
		}
	}
	if account != nil && account.User != nil {
		f.created = append(f.created, account.User.Username)
	}
	return nil
}

type fakeStatusContractUserPreferenceWriter struct {
	username string
	value    string
	err      error
}

func (f *fakeStatusContractUserPreferenceWriter) UpdateUserPreferences(_ context.Context, username string, preferences *storage.UserPreferences) error {
	if f.err != nil {
		return f.err
	}
	f.username = username
	if preferences != nil {
		f.value = preferences.DirectMessagesFrom
	}
	return nil
}

type fakeStatusContractNoteCreator struct {
	result *notessvc.NoteResult
	cmds   []*notessvc.CreateNoteCommand
	err    error
}

func (f *fakeStatusContractNoteCreator) CreateNote(_ context.Context, cmd *notessvc.CreateNoteCommand) (*notessvc.NoteResult, error) {
	f.cmds = append(f.cmds, cmd)
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

type fakeStatusContractConversationSender struct {
	firstResult  *conversationsvc.MessageResult
	secondResult *conversationsvc.MessageResult
	firstErr     error
	secondErr    error
	sendDMCalls  []*conversationsvc.SendDirectMessageCommand
	sendMsgCalls []*conversationsvc.SendMessageCommand
}

func (f *fakeStatusContractConversationSender) SendDirectMessage(_ context.Context, cmd *conversationsvc.SendDirectMessageCommand) (*conversationsvc.MessageResult, error) {
	f.sendDMCalls = append(f.sendDMCalls, cmd)
	if f.firstErr != nil {
		return nil, f.firstErr
	}
	return f.firstResult, nil
}

func (f *fakeStatusContractConversationSender) SendMessage(_ context.Context, cmd *conversationsvc.SendMessageCommand) (*conversationsvc.MessageResult, error) {
	f.sendMsgCalls = append(f.sendMsgCalls, cmd)
	if f.secondErr != nil {
		return nil, f.secondErr
	}
	return f.secondResult, nil
}

type fakeStatusContractStatusReader struct {
	byID                    map[string]*models.Status
	byHashtag               map[string][]*models.Status
	threads                 map[string][]*models.Status
	getStatusFn             func(context.Context, string) (*models.Status, error)
	getStatusesByHashtagFn  func(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error)
	getConversationThreadFn func(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error)
}

func (f *fakeStatusContractStatusReader) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
	if f.getStatusFn != nil {
		return f.getStatusFn(context.Background(), statusID)
	}
	status, ok := f.byID[statusID]
	if !ok {
		return nil, errors.New("missing status")
	}
	return status, nil
}

func (f *fakeStatusContractStatusReader) GetStatusesByHashtag(_ context.Context, hashtag string, _ interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	if f.getStatusesByHashtagFn != nil {
		return f.getStatusesByHashtagFn(context.Background(), hashtag, interfaces.PaginationOptions{Limit: 10})
	}
	return &interfaces.PaginatedResult[*models.Status]{Items: f.byHashtag[hashtag]}, nil
}

func (f *fakeStatusContractStatusReader) GetConversationThread(_ context.Context, conversationID string, _ interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	if f.getConversationThreadFn != nil {
		return f.getConversationThreadFn(context.Background(), conversationID, interfaces.PaginationOptions{Limit: 10})
	}
	return &interfaces.PaginatedResult[*models.Status]{Items: f.threads[conversationID]}, nil
}

type fakeStatusContractConversationStateReader struct {
	states                     map[string]*interfaces.UserConversationStateContract
	getUserConversationStateFn func(context.Context, string, string) (*interfaces.UserConversationStateContract, error)
}

func (f *fakeStatusContractConversationStateReader) GetUserConversationState(_ context.Context, viewerID, conversationID string) (*interfaces.UserConversationStateContract, error) {
	if f.getUserConversationStateFn != nil {
		return f.getUserConversationStateFn(context.Background(), viewerID, conversationID)
	}
	return f.states[viewerID+":"+conversationID], nil
}

type fakeStatusContractPersistenceReader struct {
	called []string
	errs   map[string]error
}

func (f *fakeStatusContractPersistenceReader) VerifyStoredStatusContext(_ context.Context, statusID string) error {
	f.called = append(f.called, statusID)
	if f.errs != nil {
		return f.errs[statusID]
	}
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestExecuteStatusContractVerification_Success(t *testing.T) {
	fixture := statusContractVerificationFixture{
		senderUsername:    "sender",
		recipientUsername: "recipient",
		publicTag:         "verifytag",
		publicContent:     "public hello #verifytag",
		firstDMContent:    "dm one",
		secondDMContent:   "dm two",
	}

	recipientActor := "https://example.com/users/recipient"
	publicStored := testVerificationStatus("public-1", models.VisibilityPublic, "public-1", fixture.publicContent, nil)
	firstStored := testVerificationStatus("dm-1", models.VisibilityDirect, "conv-1", fixture.firstDMContent, []string{recipientActor})
	secondStored := testVerificationStatus("dm-2", models.VisibilityDirect, "conv-1", fixture.secondDMContent, []string{recipientActor})

	accountWriter := &fakeStatusContractAccountWriter{}
	prefWriter := &fakeStatusContractUserPreferenceWriter{}
	noteCreator := &fakeStatusContractNoteCreator{
		result: &notessvc.NoteResult{Note: publicStored},
	}
	conversationSender := &fakeStatusContractConversationSender{
		firstResult: &conversationsvc.MessageResult{
			Message:      firstStored,
			Conversation: &models.Conversation{ID: "conv-1"},
		},
		secondResult: &conversationsvc.MessageResult{
			Message:      secondStored,
			Conversation: &models.Conversation{ID: "conv-1"},
		},
	}
	statusReader := &fakeStatusContractStatusReader{
		byID: map[string]*models.Status{
			"public-1": publicStored,
			"dm-1":     firstStored,
			"dm-2":     secondStored,
		},
		byHashtag: map[string][]*models.Status{
			fixture.publicTag: []*models.Status{publicStored},
		},
		threads: map[string][]*models.Status{
			"conv-1": []*models.Status{firstStored, secondStored},
		},
	}
	stateReader := &fakeStatusContractConversationStateReader{
		states: map[string]*interfaces.UserConversationStateContract{
			"sender:conv-1": {
				ViewerID:        "sender",
				ConversationID:  "conv-1",
				PreviewStatusID: "dm-2",
				Unread:          false,
			},
			"recipient:conv-1": {
				ViewerID:        "recipient",
				ConversationID:  "conv-1",
				PreviewStatusID: "dm-2",
				Unread:          true,
			},
		},
	}
	persistenceReader := &fakeStatusContractPersistenceReader{}

	summary, err := executeStatusContractVerification(context.Background(), statusContractVerifierDeps{
		accountWriter:           accountWriter,
		userPreferenceWriter:    prefWriter,
		noteCreator:             noteCreator,
		conversationSender:      conversationSender,
		statusReader:            statusReader,
		conversationStateReader: stateReader,
		persistenceReader:       persistenceReader,
	}, fixture)
	require.NoError(t, err)

	require.Equal(t, []string{"sender", "recipient"}, accountWriter.created)
	require.Equal(t, "recipient", prefWriter.username)
	require.Equal(t, "ANYONE", prefWriter.value)
	require.Len(t, noteCreator.cmds, 1)
	require.Equal(t, fixture.publicContent, noteCreator.cmds[0].Content)
	require.Len(t, conversationSender.sendDMCalls, 1)
	require.Len(t, conversationSender.sendMsgCalls, 1)
	require.Equal(t, "conv-1", conversationSender.sendMsgCalls[0].ConversationID)
	require.Equal(t, "public-1", summary.PublicStatusID)
	require.Equal(t, "dm-1", summary.FirstDMStatusID)
	require.Equal(t, "dm-2", summary.SecondDMStatusID)
	require.Equal(t, "conv-1", summary.ConversationID)
	require.Equal(t, []string{"public-1", "dm-1", "dm-2"}, persistenceReader.called)
}

func TestExecuteStatusContractVerification_PreservesDirectMessageFailure(t *testing.T) {
	fixture := statusContractVerificationFixture{
		senderUsername:    "sender",
		recipientUsername: "recipient",
		publicTag:         "verifytag",
		publicContent:     "public hello #verifytag",
		firstDMContent:    "dm one",
		secondDMContent:   "dm two",
	}

	_, err := executeStatusContractVerification(context.Background(), statusContractVerifierDeps{
		accountWriter:        &fakeStatusContractAccountWriter{},
		userPreferenceWriter: &fakeStatusContractUserPreferenceWriter{},
		noteCreator: &fakeStatusContractNoteCreator{
			result: &notessvc.NoteResult{Note: testVerificationStatus("public-1", models.VisibilityPublic, "public-1", fixture.publicContent, nil)},
		},
		conversationSender: &fakeStatusContractConversationSender{
			firstErr: apperrors.FailedToCreate("status", errors.New("dynamo conditional check failed")),
		},
		statusReader: &fakeStatusContractStatusReader{
			byID: map[string]*models.Status{
				"public-1": testVerificationStatus("public-1", models.VisibilityPublic, "public-1", fixture.publicContent, nil),
			},
			byHashtag: map[string][]*models.Status{
				fixture.publicTag: []*models.Status{testVerificationStatus("public-1", models.VisibilityPublic, "public-1", fixture.publicContent, nil)},
			},
		},
		conversationStateReader: &fakeStatusContractConversationStateReader{},
		persistenceReader:       &fakeStatusContractPersistenceReader{},
	}, fixture)
	require.ErrorContains(t, err, "send first direct message")
	require.ErrorContains(t, err, "root causes: dynamo conditional check failed")
}

func TestExecuteStatusContractVerification_PreservesPersistenceFailure(t *testing.T) {
	fixture := statusContractVerificationFixture{
		senderUsername:    "sender",
		recipientUsername: "recipient",
		publicTag:         "verifytag",
		publicContent:     "public hello #verifytag",
		firstDMContent:    "dm one",
		secondDMContent:   "dm two",
	}

	_, err := executeStatusContractVerification(context.Background(), statusContractVerifierDeps{
		accountWriter:        &fakeStatusContractAccountWriter{},
		userPreferenceWriter: &fakeStatusContractUserPreferenceWriter{},
		noteCreator: &fakeStatusContractNoteCreator{
			result: &notessvc.NoteResult{Note: testVerificationStatus("public-1", models.VisibilityPublic, "public-1", fixture.publicContent, nil)},
		},
		conversationSender: &fakeStatusContractConversationSender{},
		statusReader: &fakeStatusContractStatusReader{
			byID: map[string]*models.Status{
				"public-1": testVerificationStatus("public-1", models.VisibilityPublic, "public-1", fixture.publicContent, nil),
			},
			byHashtag: map[string][]*models.Status{
				fixture.publicTag: []*models.Status{testVerificationStatus("public-1", models.VisibilityPublic, "public-1", fixture.publicContent, nil)},
			},
		},
		conversationStateReader: &fakeStatusContractConversationStateReader{},
		persistenceReader: &fakeStatusContractPersistenceReader{
			errs: map[string]error{
				"public-1": errors.New("raw item missing context"),
			},
		},
	}, fixture)
	require.ErrorContains(t, err, "verify persisted public note public-1")
	require.ErrorContains(t, err, "raw item missing context")
	require.ErrorContains(t, err, "root causes: raw item missing context")
}

func TestRunVerifyStatusContract_RequiresBaseDomain(t *testing.T) {
	err := runVerifyStatusContract(nil)
	require.ErrorContains(t, err, "--base-domain is required")
}

func TestRunVerifyStatusContract_RejectsLiveEnvironment(t *testing.T) {
	require.ErrorContains(t, runVerifyStatusContract([]string{
		"--base-domain", "example.com",
		"--env", "live",
	}), "must not run against live")
	require.ErrorContains(t, validateStatusContractVerificationTarget("production", ""), "must not run against live")
	require.ErrorContains(t, validateStatusContractVerificationTarget("dev", "simulacrum-live-main-table"), "must not target live table")
	require.NoError(t, validateStatusContractVerificationTarget("staging", "simulacrum-staging-main-table"))
	require.False(t, statusContractTargetsLiveTable("olive-dev-main-table"))
	require.True(t, statusContractTargetsLiveTable("simulacrum-live-main-table"))
}

func TestRunVerifyStatusContract_Success(t *testing.T) {
	previousLoadAWS := loadAWSConfigForCLIFn
	previousNewDB := tabletheoryNewFn
	previousNewVerifierDeps := newStatusContractVerifierDepsFn
	previousRunID := newStatusContractRunIDFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWS
		tabletheoryNewFn = previousNewDB
		newStatusContractVerifierDepsFn = previousNewVerifierDeps
		newStatusContractRunIDFn = previousRunID
	})

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{
			Region:      "us-east-1",
			Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("key", "secret", "")),
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}},
					Body: io.NopCloser(strings.NewReader(
						`{"Item":{"note":{"M":{"BaseObject":{"M":{"Context":{"L":[{"S":"https://www.w3.org/ns/activitystreams"}]}}}}}}}`,
					)),
				}, nil
			})},
		}, "Sim", nil
	}

	mockDB := new(dynamormmocks.MockDB)
	mockDB.On("Close").Return(nil).Maybe()
	tabletheoryNewFn = func(cfg session.Config) (theorydb.DB, error) {
		require.Equal(t, "us-east-1", cfg.Region)
		return mockDB, nil
	}

	newStatusContractRunIDFn = func() string { return "abc12345" }
	newStatusContractVerifierDepsFn = func(_ theorydb.DB, tableName string, domain string) statusContractVerifierDeps {
		require.Equal(t, "test-table", tableName)
		require.NotEmpty(t, domain)

		fixture := newStatusContractVerificationFixture(newStatusContractRunIDFn())
		recipientActor := "https://example.com/users/" + fixture.recipientUsername
		publicStored := testVerificationStatus("public-1", models.VisibilityPublic, "public-1", fixture.publicContent, nil)
		firstStored := testVerificationStatus("dm-1", models.VisibilityDirect, "conv-1", fixture.firstDMContent, []string{recipientActor})
		secondStored := testVerificationStatus("dm-2", models.VisibilityDirect, "conv-1", fixture.secondDMContent, []string{recipientActor})

		return statusContractVerifierDeps{
			accountWriter:        &fakeStatusContractAccountWriter{},
			userPreferenceWriter: &fakeStatusContractUserPreferenceWriter{},
			noteCreator: &fakeStatusContractNoteCreator{
				result: &notessvc.NoteResult{Note: publicStored},
			},
			conversationSender: &fakeStatusContractConversationSender{
				firstResult: &conversationsvc.MessageResult{
					Message:      firstStored,
					Conversation: &models.Conversation{ID: "conv-1"},
				},
				secondResult: &conversationsvc.MessageResult{
					Message:      secondStored,
					Conversation: &models.Conversation{ID: "conv-1"},
				},
			},
			statusReader: &fakeStatusContractStatusReader{
				byID: map[string]*models.Status{
					"public-1": publicStored,
					"dm-1":     firstStored,
					"dm-2":     secondStored,
				},
				byHashtag: map[string][]*models.Status{
					fixture.publicTag: []*models.Status{publicStored},
				},
				threads: map[string][]*models.Status{
					"conv-1": []*models.Status{firstStored, secondStored},
				},
			},
			conversationStateReader: &fakeStatusContractConversationStateReader{
				states: map[string]*interfaces.UserConversationStateContract{
					fixture.senderUsername + ":conv-1": {
						PreviewStatusID: "dm-2",
					},
					fixture.recipientUsername + ":conv-1": {
						PreviewStatusID: "dm-2",
						Unread:          true,
					},
				},
			},
			persistenceReader: &fakeStatusContractPersistenceReader{},
		}
	}

	output := captureStdout(t, func() {
		require.NoError(t, runVerifyStatusContract([]string{
			"--table", "test-table",
			"--base-domain", "example.com",
			"--aws-profile", "Sim",
			"--app", "simulacrum",
			"--env", "dev",
		}))
	})

	require.Contains(t, output, "verify status-contract complete")
	require.Contains(t, output, "table: test-table")
	require.Contains(t, output, "env: dev")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "public_status_id: public-1")
	require.Contains(t, output, "first_dm_status_id: dm-1")
	require.Contains(t, output, "second_dm_status_id: dm-2")
	require.Contains(t, output, "conversation_id: conv-1")
	require.Contains(t, output, "public_hashtag: statuscontractabc12345")
}

func TestDynamoStatusContractPersistenceReader(t *testing.T) {
	t.Run("requires client", func(t *testing.T) {
		err := dynamoStatusContractPersistenceReader{tableName: "test-table"}.VerifyStoredStatusContext(context.Background(), "status-1")
		require.ErrorContains(t, err, "dynamodb client is required")
	})

	t.Run("requires table", func(t *testing.T) {
		reader := dynamoStatusContractPersistenceReader{
			client: dynamodb.New(dynamodb.Options{
				Region: "us-east-1",
				HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					t.Fatal("unexpected request")
					return nil, nil
				})},
			}),
		}
		err := reader.VerifyStoredStatusContext(context.Background(), "status-1")
		require.ErrorContains(t, err, "table name is required")
	})

	for _, tc := range []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing row",
			body:    `{}`,
			wantErr: "status row missing",
		},
		{
			name:    "missing note payload",
			body:    `{"Item":{"PK":{"S":"status#status-1"}}}`,
			wantErr: "persisted note payload missing",
		},
		{
			name:    "malformed note payload",
			body:    `{"Item":{"note":{"S":"bad"}}}`,
			wantErr: "persisted note payload malformed",
		},
		{
			name:    "missing base object",
			body:    `{"Item":{"note":{"M":{}}}}`,
			wantErr: "persisted note base object missing",
		},
		{
			name:    "malformed base object",
			body:    `{"Item":{"note":{"M":{"BaseObject":{"S":"bad"}}}}}`,
			wantErr: "persisted note base object malformed",
		},
		{
			name:    "missing nested context",
			body:    `{"Item":{"note":{"M":{"BaseObject":{"M":{}}}}}}`,
			wantErr: "note base object context missing after persistence",
		},
		{
			name:    "rejects top level context",
			body:    `{"Item":{"note":{"M":{"Context":{"L":[{"S":"https://www.w3.org/ns/activitystreams"}]}}}}}`,
			wantErr: "persisted note context must be nested under BaseObject",
		},
		{
			name: "accepts nested context",
			body: `{"Item":{"note":{"M":{"BaseObject":{"M":{"Context":{"L":[{"S":"https://www.w3.org/ns/activitystreams"}]}}}}}}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := dynamoStatusContractPersistenceReader{
				client: dynamodb.New(dynamodb.Options{
					Region: "us-east-1",
					HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}},
							Body:       io.NopCloser(strings.NewReader(tc.body)),
						}, nil
					})},
				}),
				tableName: "test-table",
			}

			err := reader.VerifyStoredStatusContext(context.Background(), "status-1")
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestStatusContractVerificationHelpers(t *testing.T) {
	previousDirectWait := statusContractDirectReadWait
	previousEventualWait := statusContractEventuallyConsistentReadWait
	statusContractDirectReadWait = 0
	statusContractEventuallyConsistentReadWait = 0
	t.Cleanup(func() {
		statusContractDirectReadWait = previousDirectWait
		statusContractEventuallyConsistentReadWait = previousEventualWait
	})

	t.Run("hasAttributeListValues handles supported attribute types", func(t *testing.T) {
		require.True(t, hasAttributeListValues(&ddbtypes.AttributeValueMemberL{Value: []ddbtypes.AttributeValue{&ddbtypes.AttributeValueMemberS{Value: "ctx"}}}))
		require.False(t, hasAttributeListValues(&ddbtypes.AttributeValueMemberL{}))
		require.True(t, hasAttributeListValues(&ddbtypes.AttributeValueMemberS{Value: "ctx"}))
		require.False(t, hasAttributeListValues(&ddbtypes.AttributeValueMemberS{Value: "   "}))
		require.False(t, hasAttributeListValues(&ddbtypes.AttributeValueMemberBOOL{Value: true}))
	})

	t.Run("fixture normalizes run ids", func(t *testing.T) {
		fixture := newStatusContractVerificationFixture(" Ab-CD ")
		require.Equal(t, "statuschecksenderab-cd", fixture.senderUsername)
		require.Equal(t, "statuscheckrecipientab-cd", fixture.recipientUsername)
		require.Equal(t, "statuscontractab-cd", fixture.publicTag)

		defaultFixture := newStatusContractVerificationFixture("")
		require.Equal(t, "statuschecksenderstatuscheck", defaultFixture.senderUsername)
	})

	t.Run("test verification statuses reuse shared note fixtures", func(t *testing.T) {
		publicStatus := testVerificationStatus("public-1", models.VisibilityPublic, "public-1", "public content", nil)
		require.NotNil(t, publicStatus.Note)
		require.Len(t, publicStatus.Note.Attachment, 1)
		require.Len(t, publicStatus.Note.Tag, 2)
		require.NotNil(t, publicStatus.Note.QuoteContext)

		directStatus := testVerificationStatus("dm-1", models.VisibilityDirect, "conv-1", "dm content", []string{"https://example.com/users/recipient"})
		require.NotNil(t, directStatus.Note)
		require.Nil(t, directStatus.Note.Attachment)
		require.Len(t, directStatus.Note.Tag, 2)
		require.Equal(t, []string{"https://example.com/users/recipient"}, directStatus.Note.To)
	})

	t.Run("validate deps catches missing fields", func(t *testing.T) {
		require.ErrorContains(t, validateStatusContractVerifierDeps(statusContractVerifierDeps{}), "dependencies are incomplete")
		require.ErrorContains(t, validateStatusContractVerifierDeps(statusContractVerifierDeps{
			accountWriter:           &fakeStatusContractAccountWriter{},
			userPreferenceWriter:    &fakeStatusContractUserPreferenceWriter{},
			noteCreator:             &fakeStatusContractNoteCreator{},
			conversationSender:      &fakeStatusContractConversationSender{},
			statusReader:            &fakeStatusContractStatusReader{},
			conversationStateReader: &fakeStatusContractConversationStateReader{},
		}), "persistence reader is required")
		require.NoError(t, validateStatusContractVerifierDeps(statusContractVerifierDeps{
			accountWriter:           &fakeStatusContractAccountWriter{},
			userPreferenceWriter:    &fakeStatusContractUserPreferenceWriter{},
			noteCreator:             &fakeStatusContractNoteCreator{},
			conversationSender:      &fakeStatusContractConversationSender{},
			statusReader:            &fakeStatusContractStatusReader{},
			conversationStateReader: &fakeStatusContractConversationStateReader{},
			persistenceReader:       &fakeStatusContractPersistenceReader{},
		}))
	})

	t.Run("prepare actors surfaces write failures", func(t *testing.T) {
		fixture := statusContractVerificationFixture{senderUsername: "sender", recipientUsername: "recipient"}
		require.ErrorContains(t, prepareStatusContractVerificationActors(context.Background(), statusContractVerifierDeps{
			accountWriter:        &fakeStatusContractAccountWriter{errs: []error{errors.New("sender boom")}},
			userPreferenceWriter: &fakeStatusContractUserPreferenceWriter{},
		}, fixture), "create sender account")
		require.ErrorContains(t, prepareStatusContractVerificationActors(context.Background(), statusContractVerifierDeps{
			accountWriter:        &fakeStatusContractAccountWriter{errs: []error{nil, errors.New("recipient boom")}},
			userPreferenceWriter: &fakeStatusContractUserPreferenceWriter{},
		}, fixture), "create recipient account")
		require.ErrorContains(t, prepareStatusContractVerificationActors(context.Background(), statusContractVerifierDeps{
			accountWriter:        &fakeStatusContractAccountWriter{},
			userPreferenceWriter: &fakeStatusContractUserPreferenceWriter{err: errors.New("pref boom")},
		}, fixture), "set recipient dm preference")
	})

	t.Run("load verified status retries", func(t *testing.T) {
		attempts := 0
		status, err := loadVerifiedStatus(context.Background(), &fakeStatusContractStatusReader{
			getStatusFn: func(_ context.Context, statusID string) (*models.Status, error) {
				attempts++
				if attempts == 1 {
					return nil, errors.New("not yet")
				}
				return &models.Status{StatusID: statusID}, nil
			},
		}, "status-1")
		require.NoError(t, err)
		require.Equal(t, "status-1", status.StatusID)
		require.Equal(t, 2, attempts)
	})

	t.Run("retryStatusContractRead respects context cancellation and zero attempts", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, retryStatusContractRead(ctx, 0, 0, func() error { return nil }), context.Canceled)

		attempts := 0
		require.ErrorContains(t, retryStatusContractRead(context.Background(), 0, 0, func() error {
			attempts++
			return errors.New("still failing")
		}), "still failing")
		require.Equal(t, 1, attempts)
	})

	t.Run("verifyStoredStatusMetadata validates status shape", func(t *testing.T) {
		require.ErrorContains(t, verifyStoredStatusMetadata(nil, models.VisibilityPublic, "conv", "content", ""), "status is nil")
		require.ErrorContains(t, verifyStoredStatusMetadata(&models.Status{Visibility: models.VisibilityDirect, ConversationID: "conv", Content: "content", Note: &activitypub.Note{}}, models.VisibilityPublic, "conv", "content", ""), "visibility mismatch")
		require.ErrorContains(t, verifyStoredStatusMetadata(&models.Status{Visibility: models.VisibilityPublic, ConversationID: "other", Content: "content", Note: &activitypub.Note{}}, models.VisibilityPublic, "conv", "content", ""), "conversation id mismatch")
		require.ErrorContains(t, verifyStoredStatusMetadata(&models.Status{Visibility: models.VisibilityPublic, ConversationID: "conv", Content: "other", Note: &activitypub.Note{}}, models.VisibilityPublic, "conv", "content", ""), "content mismatch")
		require.ErrorContains(t, verifyStoredStatusMetadata(&models.Status{Visibility: models.VisibilityPublic, ConversationID: "conv", Content: "content"}, models.VisibilityPublic, "conv", "content", ""), "note payload missing")
		require.ErrorContains(t, verifyStoredStatusMetadata(&models.Status{
			Visibility:     models.VisibilityDirect,
			ConversationID: "conv",
			Content:        "content",
			Note:           &activitypub.Note{},
			ToRecipients:   []string{"https://example.com/users/alice"},
		}, models.VisibilityDirect, "conv", "content", "https://example.com/users/bob"), "missing direct recipient")
		require.NoError(t, verifyStoredStatusMetadata(&models.Status{
			Visibility:     models.VisibilityDirect,
			ConversationID: "conv",
			Content:        "content",
			Note:           &activitypub.Note{},
			ToRecipients:   []string{" https://example.com/users/bob "},
		}, models.VisibilityDirect, "conv", "content", "https://example.com/users/bob"))
	})

	t.Run("verify collection helpers retry until data appears", func(t *testing.T) {
		hashtagAttempts := 0
		require.NoError(t, verifyHashtagIndex(context.Background(), &fakeStatusContractStatusReader{
			getStatusesByHashtagFn: func(_ context.Context, hashtag string, _ interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
				hashtagAttempts++
				if hashtagAttempts == 1 {
					return &interfaces.PaginatedResult[*models.Status]{Items: nil}, nil
				}
				return &interfaces.PaginatedResult[*models.Status]{Items: []*models.Status{{StatusID: hashtag + "-1"}}}, nil
			},
		}, "tag", "tag-1"))

		threadAttempts := 0
		require.NoError(t, verifyConversationThread(context.Background(), &fakeStatusContractStatusReader{
			getConversationThreadFn: func(_ context.Context, conversationID string, _ interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
				threadAttempts++
				if threadAttempts == 1 {
					return nil, errors.New("retry")
				}
				return &interfaces.PaginatedResult[*models.Status]{Items: []*models.Status{{StatusID: "first"}, {StatusID: "second"}}}, nil
			},
		}, "conv", "first", "second"))

		stateAttempts := 0
		require.NoError(t, verifyConversationStates(context.Background(), &fakeStatusContractConversationStateReader{
			getUserConversationStateFn: func(_ context.Context, viewerID, _ string) (*interfaces.UserConversationStateContract, error) {
				stateAttempts++
				if stateAttempts == 1 {
					return nil, errors.New("retry")
				}
				if viewerID == "sender" {
					return &interfaces.UserConversationStateContract{PreviewStatusID: "latest"}, nil
				}
				return &interfaces.UserConversationStateContract{PreviewStatusID: "latest", Unread: true}, nil
			},
		}, "sender", "recipient", "conv", "latest"))
	})

	t.Run("verifyDirectMessageResult validates send results", func(t *testing.T) {
		deps := statusContractVerifierDeps{
			statusReader: &fakeStatusContractStatusReader{
				byID: map[string]*models.Status{
					"dm-1": testVerificationStatus("dm-1", models.VisibilityDirect, "conv-1", "content", []string{"https://example.com/users/recipient"}),
				},
			},
			persistenceReader: &fakeStatusContractPersistenceReader{},
		}

		_, err := verifyDirectMessageResult(context.Background(), deps, nil, "content", "send dm", "load dm", "verify dm", "persist dm")
		require.ErrorContains(t, err, "nil message result")

		_, err = verifyDirectMessageResult(context.Background(), deps, &conversationsvc.MessageResult{
			Message:      &models.Status{StatusID: "dm-1"},
			Conversation: &models.Conversation{ID: "conv-1"},
		}, "content", "send dm", "load dm", "verify dm", "persist dm")
		require.ErrorContains(t, err, "missing to recipients")

		_, err = verifyDirectMessageResult(context.Background(), deps, &conversationsvc.MessageResult{
			Message: &models.Status{
				StatusID:     "missing",
				ToRecipients: []string{"https://example.com/users/recipient"},
			},
			Conversation: &models.Conversation{ID: "conv-1"},
		}, "content", "send dm", "load dm", "verify dm", "persist dm")
		require.ErrorContains(t, err, "load dm missing")

		_, err = verifyDirectMessageResult(context.Background(), deps, &conversationsvc.MessageResult{
			Message: &models.Status{
				StatusID:     "dm-1",
				ToRecipients: []string{"https://example.com/users/other"},
			},
			Conversation: &models.Conversation{ID: "conv-1"},
		}, "content", "send dm", "load dm", "verify dm", "persist dm")
		require.ErrorContains(t, err, "missing direct recipient")

		deps.persistenceReader = &fakeStatusContractPersistenceReader{errs: map[string]error{"dm-1": errors.New("persist boom")}}
		_, err = verifyDirectMessageResult(context.Background(), deps, &conversationsvc.MessageResult{
			Message: &models.Status{
				StatusID:     "dm-1",
				ToRecipients: []string{"https://example.com/users/recipient"},
			},
			Conversation: &models.Conversation{ID: "conv-1"},
		}, "content", "send dm", "load dm", "verify dm", "persist dm")
		require.ErrorContains(t, err, "persist boom")

		deps.persistenceReader = &fakeStatusContractPersistenceReader{}
		send, err := verifyDirectMessageResult(context.Background(), deps, &conversationsvc.MessageResult{
			Message: &models.Status{
				StatusID:     "dm-1",
				ToRecipients: []string{"https://example.com/users/recipient"},
			},
			Conversation: &models.Conversation{ID: "conv-1"},
		}, "content", "send dm", "load dm", "verify dm", "persist dm")
		require.NoError(t, err)
		require.Equal(t, "dm-1", send.statusID)
		require.Equal(t, "conv-1", send.conversationID)
	})
}

func testVerificationStatus(statusID string, visibility string, conversationID string, content string, toRecipients []string) *models.Status {
	var note *activitypub.Note
	switch visibility {
	case models.VisibilityDirect:
		note = notecontract.DirectFixtureNote()
		note.To = append([]string(nil), toRecipients...)
		note.ConversationID = conversationID
	default:
		note = notecontract.PublicFixtureNote()
	}
	note.ID = "https://example.com/users/sender/statuses/" + statusID
	note.Content = content

	return &models.Status{
		StatusID:       statusID,
		Visibility:     visibility,
		Content:        content,
		ConversationID: conversationID,
		Note:           note,
		ToRecipients:   append([]string(nil), toRecipients...),
	}
}
