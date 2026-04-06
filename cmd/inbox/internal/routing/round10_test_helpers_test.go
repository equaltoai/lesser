package routing

import (
	"context"
	"crypto/rsa"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	pkgtypes "github.com/theory-cloud/tabletheory/pkg/types"
	"go.uber.org/zap"
)

type inboxTestEnv struct {
	handler          *InboxHandler
	mockDB           *extendedMockDB
	mockQuery        *mocks.MockQuery
	cfg              *config.Config
	logger           *zap.Logger
	local            *activitypub.Actor
	remoteActorID    string
	remoteKeyID      string
	remotePrivateKey *rsa.PrivateKey
	remotePublicPEM  []byte
}

type extendedMockDB struct {
	inner *mocks.MockDB
}

var _ dynamormCore.ExtendedDB = (*extendedMockDB)(nil)

func (db *extendedMockDB) Model(model any) dynamormCore.Query {
	return db.inner.Model(model)
}

func (db *extendedMockDB) Transaction(fn func(tx *dynamormCore.Tx) error) error {
	// Transaction behavior isn't relevant for these tests.
	return fn(nil)
}

func (db *extendedMockDB) Migrate() error { return nil }

func (db *extendedMockDB) AutoMigrate(models ...any) error { return nil }

func (db *extendedMockDB) Close() error { return nil }

func (db *extendedMockDB) WithContext(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) AutoMigrateWithOptions(_ any, _ ...any) error { return nil }

func (db *extendedMockDB) RegisterTypeConverter(_ reflect.Type, _ pkgtypes.CustomConverter) error {
	return nil
}

func (db *extendedMockDB) CreateTable(_ any, _ ...any) error { return nil }

func (db *extendedMockDB) EnsureTable(_ any) error { return nil }

func (db *extendedMockDB) DeleteTable(_ any) error { return nil }

func (db *extendedMockDB) DescribeTable(_ any) (any, error) { return nil, nil }

func (db *extendedMockDB) WithLambdaTimeout(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) WithLambdaTimeoutBuffer(_ time.Duration) dynamormCore.DB { return db }

func (db *extendedMockDB) TransactionFunc(fn func(tx any) error) error { return fn(nil) }

func (db *extendedMockDB) Transact() dynamormCore.TransactionBuilder { return nil }

func (db *extendedMockDB) TransactWrite(_ context.Context, fn func(dynamormCore.TransactionBuilder) error) error {
	return fn(nil)
}

func newInboxTestEnv(t *testing.T) *inboxTestEnv {
	t.Helper()

	baseTime := time.Date(2025, 12, 28, 12, 0, 0, 0, time.UTC)
	logger := zap.NewNop()

	cfg := config.Get()
	cfg.DynamoTableName = "test-table"
	if cfg.Domain == "" {
		cfg.Domain = "localhost"
	}

	remoteActorID := "https://remote.example/users/bob"
	remotePrivateKey, err := federation.GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	remotePublicPEM, err := federation.EncodePublicKeyPEM(&remotePrivateKey.PublicKey)
	require.NoError(t, err)
	remoteKeyID := remoteActorID + "#main-key"

	local := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   cfg.ActorURL("alice"),
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             cfg.ActorURL("alice") + "/inbox",
		Outbox:            cfg.ActorURL("alice") + "/outbox",
		PublicKey: &activitypub.PublicKey{
			ID:           cfg.ActorURL("alice") + "#main-key",
			Owner:        cfg.ActorURL("alice"),
			PublicKeyPem: string(remotePublicPEM),
		},
	}

	innerDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	mockDB := &extendedMockDB{inner: innerDB}

	innerDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	var lastActivitySearchID string

	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			field, _ := args.Get(0).(string)
			op, _ := args.Get(1).(string)
			value := args.Get(2)
			if id, ok := value.(string); ok {
				switch {
				case field == "SK" && op == "CONTAINS":
					// Legacy access pattern for ActivityRepository.GetActivity.
					lastActivitySearchID = id
				case field == "gsi2PK" && op == "=" && strings.HasPrefix(id, "ACTIVITYID#"):
					// Current access pattern: ActivityRepository.GetActivity queries gsi2PK.
					lastActivitySearchID = strings.TrimPrefix(id, "ACTIVITYID#")
				}
			}
		}).
		Return(mockQuery).
		Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrFilter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("FilterGroup", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrFilterGroup", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Offset", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("SetCursor", mock.Anything).Return(nil).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("WithRetry", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("IfNotExists").Return(mockQuery).Maybe()
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Maybe()

	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ConditionExists", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ConditionNotExists", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ReturnValues", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Execute").Return(nil).Maybe()

	// Avoid accidental federation deliveries from inbox flow; treat remote actor cache as missing.
	mockQuery.On("First", mock.AnythingOfType("*models.RemoteActor")).Return(dynamormErrors.ErrItemNotFound).Maybe()
	// Default: no blocks and no instance domain blocks in tests unless explicitly overridden.
	mockQuery.On("First", mock.AnythingOfType("*models.Block")).Return(dynamormErrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(dynamormErrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.AnythingOfType("*models.RateLimitLockout")).Return(dynamormErrors.ErrItemNotFound).Maybe()

	mockQuery.
		On("First", mock.Anything).
		Run(func(args mock.Arguments) {
			switch out := args.Get(0).(type) {
			case *models.Actor:
				out.Username = "alice"
				out.Actor = local
				out.CreatedAt = baseTime
				out.UpdatedAt = baseTime
			case *models.InstanceState:
				out.Locked = false
				out.BootstrapUsername = models.DefaultBootstrapUsername
				out.CreatedAt = baseTime
				out.UpdatedAt = baseTime
			case *models.PublicKeyCache:
				now := time.Now().UTC()
				out.ActorURL = remoteActorID
				out.KeyID = remoteKeyID
				out.PublicKeyPEM = string(remotePublicPEM)
				out.Algorithm = federation.AlgorithmRSASHA256
				out.FetchedAt = now
				out.TTL = now.Add(24 * time.Hour).Unix()
				_ = out.UpdateKeys()
			case *models.Object:
				out.ID = cfg.BaseURL() + "/objects/1"
				out.Type = activitypub.NoteType
				out.AttributedTo = remoteActorID
				out.Content = "hello from remote"
				out.Published = baseTime
				out.Updated = baseTime
			case *models.Activity:
				now := baseTime
				out.Activity = &activitypub.Activity{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						Type:    activitypub.CreateType,
						ID:      cfg.BaseURL() + "/activities/1",
						To:      []string{local.ID},
					},
					Actor:  remoteActorID,
					Object: cfg.BaseURL() + "/objects/1",
				}
				out.CreatedAt = now
			default:
			}
		}).
		Return(nil).
		Maybe()

	mockQuery.
		On("All", mock.Anything).
		Run(func(args mock.Arguments) {
			switch out := args.Get(0).(type) {
			case *[]*models.Activity:
				id := lastActivitySearchID
				if id == "" {
					id = cfg.BaseURL() + "/activities/a2"
				}
				storedActivity := storedActivityForID(id, cfg, local, remoteActorID)
				*out = append(*out, &models.Activity{
					Activity:  storedActivity,
					CreatedAt: baseTime,
				})
			case *[]models.Activity:
				*out = append(*out,
					models.Activity{
						Activity: &activitypub.Activity{
							BaseObject: activitypub.BaseObject{
								Context: activitypub.Context,
								Type:    activitypub.CreateType,
								ID:      cfg.BaseURL() + "/activities/a1",
								To:      []string{local.ID},
							},
							Actor:  remoteActorID,
							Object: cfg.BaseURL() + "/objects/1",
						},
						CreatedAt: baseTime,
					},
					models.Activity{
						Activity: &activitypub.Activity{
							BaseObject: activitypub.BaseObject{
								Context: activitypub.Context,
								Type:    activitypub.FollowType,
								ID:      cfg.BaseURL() + "/activities/a2",
								To:      []string{local.ID},
							},
							Actor:  remoteActorID,
							Object: local.ID,
						},
						CreatedAt: baseTime.Add(time.Minute),
					},
				)
			case *[]models.RelationshipRecord:
				*out = append(*out,
					models.RelationshipRecord{
						PK:     "FOLLOW#@bob@localhost",
						SK:     "FOLLOWING#@alice@localhost",
						GSI1SK: "FOLLOWER#@bob@localhost",
					},
					models.RelationshipRecord{
						PK:     "FOLLOW#@carol@remote.example",
						SK:     "FOLLOWING#@alice@localhost",
						GSI1SK: "FOLLOWER#@carol@remote.example",
					},
				)
			case *[]*models.Like:
				*out = append(*out, &models.Like{
					Actor: remoteActorID,
				})
			case *[]models.Object:
				*out = append(*out, models.Object{
					ID:   cfg.BaseURL() + "/objects/reply-1",
					Type: "Article",
				})
			default:
			}
		}).
		Return(nil).
		Maybe()

	mockQuery.On("Scan", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("CreateOrUpdate").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()
	mockQuery.On("BatchCreate", mock.Anything).Return(nil).Maybe()
	mockQuery.On("BatchDelete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockQuery.On("BatchWrite", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockQuery.On("BatchUpdateWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	repoFactory, err := factory.NewRepositoryFactory(mockDB, cfg.DynamoTableName, logger)
	require.NoError(t, err)

	lambdaCtx := &common.LambdaContext{
		Config:         cfg,
		Logger:         logger,
		DynamoDB:       mockDB,
		Repos:          repoFactory,
		AuthMiddleware: auth.NewMiddleware(),
	}

	handler, err := NewInboxHandler(lambdaCtx)
	require.NoError(t, err)

	return &inboxTestEnv{
		handler:          handler,
		mockDB:           mockDB,
		mockQuery:        mockQuery,
		cfg:              cfg,
		logger:           logger,
		local:            local,
		remoteActorID:    remoteActorID,
		remoteKeyID:      remoteKeyID,
		remotePrivateKey: remotePrivateKey,
		remotePublicPEM:  remotePublicPEM,
	}
}

func storedActivityForID(id string, cfg *config.Config, local *activitypub.Actor, remoteActorID string) *activitypub.Activity {
	activityType := activitypub.FollowType
	object := any(local.ID)
	target := ""

	switch {
	case strings.Contains(id, "like"):
		activityType = activitypub.LikeType
		object = cfg.BaseURL() + "/objects/1"
	case strings.Contains(id, "announce"):
		activityType = activitypub.AnnounceType
		object = cfg.BaseURL() + "/objects/1"
	case strings.Contains(id, "create"):
		activityType = activitypub.CreateType
		object = cfg.BaseURL() + "/objects/1"
	case strings.Contains(id, "update"):
		activityType = activitypub.UpdateType
		object = cfg.BaseURL() + "/objects/1"
	case strings.Contains(id, "delete"):
		activityType = activitypub.DeleteType
		object = cfg.BaseURL() + "/objects/1"
	case strings.Contains(id, "accept"):
		activityType = activitypub.AcceptType
		object = cfg.BaseURL() + "/activities/follow-1"
	case strings.Contains(id, "add"):
		activityType = activitypub.AddType
		object = cfg.BaseURL() + "/objects/1"
		target = local.ID + "/featured"
	case strings.Contains(id, "remove"):
		activityType = activitypub.RemoveType
		object = cfg.BaseURL() + "/objects/1"
		target = local.ID + "/featured"
	case strings.Contains(id, "flag"):
		activityType = activitypub.FlagType
		object = cfg.BaseURL() + "/objects/1"
	case strings.Contains(id, "move"):
		activityType = activitypub.MoveType
		object = cfg.BaseURL() + "/objects/1"
		target = cfg.ActorURL(local.PreferredUsername)
	case strings.Contains(id, "block"):
		activityType = activitypub.BlockType
		object = cfg.ActorURL("blocked")
	case strings.Contains(id, "unsupported"):
		activityType = "UnsupportedType"
		object = cfg.BaseURL() + "/objects/1"
	case strings.Contains(id, "follow"):
		activityType = activitypub.FollowType
		object = local.ID
	default:
		activityType = activitypub.FollowType
		object = local.ID
	}

	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activityType,
			ID:      id,
			To:      []string{local.ID},
		},
		Actor:  remoteActorID,
		Object: object,
		Target: target,
	}
}

func newAppTheoryContext(method, path string, headers map[string]string, query map[string]string, body []byte) *apptheory.Context {
	canonicalHeaders := make(map[string][]string, len(headers))
	for k, v := range headers {
		canonicalHeaders[strings.ToLower(k)] = []string{v}
	}

	canonicalQuery := make(map[string][]string, len(query))
	for k, v := range query {
		canonicalQuery[k] = []string{v}
	}

	return &apptheory.Context{
		Request: apptheory.Request{
			Method:  method,
			Path:    path,
			Headers: canonicalHeaders,
			Query:   canonicalQuery,
			Body:    body,
		},
		Params: map[string]string{},
	}
}
