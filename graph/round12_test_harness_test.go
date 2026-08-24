package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/equaltoai/lesser/pkg/activitypub"
	agentpkg "github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services"
	storagepkg "github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type round12TestClaims struct {
	username string
}

type round12MediaS3Service struct {
	state *round12PermissiveQueryState
}

func (round12MediaS3Service) UploadFile(
	_ context.Context,
	bucket string,
	key string,
	_ []byte,
	_ string,
) (string, error) {
	return "s3://" + bucket + "/" + key, nil
}

func (s round12MediaS3Service) UploadInternalFile(
	ctx context.Context,
	bucket string,
	key string,
	data []byte,
	contentType string,
	_ string,
) (string, error) {
	return s.UploadFile(ctx, bucket, key, data, contentType)
}

func (round12MediaS3Service) DeleteFile(context.Context, string, string) error { return nil }

func (s round12MediaS3Service) GeneratePresignedURL(_ context.Context, bucket, key string, _ time.Duration) (string, error) {
	if s.state != nil {
		s.state.presignCalls++
		if s.state.presignErr != nil {
			return "", s.state.presignErr
		}
	}
	return "https://signed.example/" + bucket + "/" + key + "?signature=review", nil
}

func (s round12MediaS3Service) CopyFileToPublished(_ context.Context, bucket, sourceKey, destinationKey, _ string) (string, error) {
	if s.state != nil {
		s.state.publishCopies = append(s.state.publishCopies, round12PublishCopy{source: sourceKey, destination: destinationKey})
	}
	return "s3://" + bucket + "/" + destinationKey, nil
}

type round12VAPIDSecretsClient struct {
	secret string
}

func newRound12VAPIDSecretsClient() *round12VAPIDSecretsClient {
	payload, _ := json.Marshal(map[string]string{
		"public_key":  "server-key",
		"private_key": "test-private-key",
		"subject":     "mailto:admin@localhost",
	})
	return &round12VAPIDSecretsClient{secret: string(payload)}
}

func (c *round12VAPIDSecretsClient) GetSecretValue(
	_ context.Context,
	_ *secretsmanager.GetSecretValueInput,
	_ ...func(*secretsmanager.Options),
) (*secretsmanager.GetSecretValueOutput, error) {
	return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(c.secret)}, nil
}

func (c *round12VAPIDSecretsClient) PutSecretValue(
	_ context.Context,
	input *secretsmanager.PutSecretValueInput,
	_ ...func(*secretsmanager.Options),
) (*secretsmanager.PutSecretValueOutput, error) {
	if input == nil || input.SecretString == nil {
		return nil, fmt.Errorf("test VAPID secret value is absent")
	}
	c.secret = aws.ToString(input.SecretString)
	return &secretsmanager.PutSecretValueOutput{}, nil
}

func (c round12TestClaims) HasScope(string) bool { return true }
func (c round12TestClaims) GetUsername() string  { return c.username }

func round12AuthContext(username string) context.Context {
	return context.WithValue(context.Background(), common.ContextKeyClaims, round12TestClaims{username: username})
}

type round12PermissiveQueryState struct {
	lastPK       string
	lastSK       string
	currentModel any

	autoPopulateScan  bool
	autoPopulateAll   bool
	autoPopulateCount int
	autoPopulateIndex int

	seededAccountUsers     map[string]*storagepkg.User
	seededGovernanceStates map[string]*storagepkg.AgentGovernanceState
	seededImportBudgets    map[string]*models.ImportBudget
	seededQuotePermissions map[string]*models.QuotePermissions
	seededAgentShareGrants map[string]*models.AgentShareGrant
	seededMedia            map[string]*models.Media
	persistMedia           bool
	presignCalls           int
	presignErr             error
	publishCopies          []round12PublishCopy
	pendingUpdateSets      map[string]any
	pendingUpdateRemovals  map[string]struct{}
}

type round12PublishCopy struct {
	source      string
	destination string
}

func setupRound12PermissiveDynamormMocks(t *testing.T) (*dynamormmocks.MockDB, *dynamormmocks.MockQuery, *round12PermissiveQueryState) {
	t.Helper()

	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockUpdateBuilder := new(dynamormmocks.MockUpdateBuilder)
	state := &round12PermissiveQueryState{}

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
		state.currentModel = args.Get(0)
	}).Return(mockQuery).Maybe()

	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		field, _ := args.Get(0).(string)
		value, _ := args.Get(2).(string)
		fieldLower := strings.ToLower(field)
		if strings.HasSuffix(fieldLower, "pk") {
			state.lastPK = value
		}
		if strings.HasSuffix(fieldLower, "sk") {
			state.lastSK = value
		}
	}).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrFilter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("FilterGroup", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrFilterGroup", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithCondition", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithConditionExpression", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Offset", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("IfNotExists").Return(mockQuery).Maybe()
	mockQuery.On("IfExists").Return(mockQuery).Maybe()
	mockQuery.On("Create").Run(func(mock.Arguments) {
		round12ApplyDirectModelCreate(state)
	}).Return(nil).Maybe()
	mockQuery.On("CreateOrUpdate").Return(nil).Maybe()
	mockQuery.On("Update").Run(func(mock.Arguments) {
		round12ApplyDirectModelUpdate(state)
	}).Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Run(func(mock.Arguments) {
		round12ApplyDirectModelUpdate(state)
	}).Return(nil).Maybe()
	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		field, _ := args.Get(0).(string)
		if state.pendingUpdateSets == nil {
			state.pendingUpdateSets = map[string]any{}
		}
		if state.pendingUpdateRemovals == nil {
			state.pendingUpdateRemovals = map[string]struct{}{}
		}
		state.pendingUpdateSets[field] = args.Get(1)
		delete(state.pendingUpdateRemovals, field)
	}).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Increment", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Decrement", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Remove", mock.Anything).Run(func(args mock.Arguments) {
		field, _ := args.Get(0).(string)
		if state.pendingUpdateSets == nil {
			state.pendingUpdateSets = map[string]any{}
		}
		if state.pendingUpdateRemovals == nil {
			state.pendingUpdateRemovals = map[string]struct{}{}
		}
		delete(state.pendingUpdateSets, field)
		state.pendingUpdateRemovals[field] = struct{}{}
	}).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Delete", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("AppendToList", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("PrependToList", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("RemoveFromListAt", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("SetListElement", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("OrCondition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ConditionExists", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ConditionNotExists", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ConditionVersion", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ReturnValues", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Execute").Run(func(mock.Arguments) {
		round12ApplyPendingUpdate(state)
	}).Return(nil).Maybe()
	mockUpdateBuilder.On("ExecuteWithResult", mock.Anything).Run(func(mock.Arguments) {
		round12ApplyPendingUpdate(state)
	}).Return(nil).Maybe()
	mockQuery.On("UpdateBuilder").Run(func(mock.Arguments) {
		state.pendingUpdateSets = map[string]any{}
		state.pendingUpdateRemovals = map[string]struct{}{}
	}).Return(mockUpdateBuilder).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if state.autoPopulateAll {
			round12PopulateSlice(args.Get(0), state)
		}
	}).Return(nil).Maybe()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		if state.autoPopulateAll {
			round12PopulateSlice(args.Get(0), state)
		}
	}).Return(&dynamormcore.PaginatedResult{}, nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		if state.autoPopulateScan {
			round12PopulateSlice(args.Get(0), state)
		}
	}).Return(nil).Maybe()
	// Default absence: treat block/mute lookups as not-found so relationship flows don't
	// incorrectly short-circuit on "blocked" when our DB mock has no backing store.
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.Block)
		return ok
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.Mute)
		return ok
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	// Follow requests are directional; default the bob->alice request to "not found" so
	// Accept/RejectFollowRequest tests cover the error path realistically.
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.RelationshipRecord)
		if !ok {
			return false
		}
		return state.lastPK == "FOLLOW#bob" && state.lastSK == "FOLLOWING#alice"
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	// Quote permissions: return "not found" for a dedicated test username so callers
	// can exercise create-then-update flows deterministically.
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.QuotePermissions)
		if !ok {
			return false
		}
		return state.lastPK == "USER#missing-perms" && state.lastSK == "QUOTE_PERMISSIONS"
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	// Emoji: allow one stable "existing" emoji for update flows, but default lookups
	// to not-found so create flows don't falsely detect duplicates.
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.EmojiModel)
		if !ok {
			return false
		}
		return state.lastPK == "EMOJI#existing" && state.lastSK == "EMOJI"
	})).Run(func(args mock.Arguments) {
		round12PopulateStruct(args.Get(0), state)
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.EmojiModel)
		return ok
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.AgentGovernanceState)
		if !ok {
			return false
		}
		username := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(state.lastPK, "USER#")))
		if username == "" {
			return false
		}
		_, exists := state.seededGovernanceStates[username]
		return exists
	})).Run(func(args mock.Arguments) {
		round12PopulateStruct(args.Get(0), state)
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.AgentGovernanceState)
		if !ok {
			return false
		}
		username := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(state.lastPK, "USER#")))
		if username == "" {
			return false
		}
		_, exists := state.seededGovernanceStates[username]
		return !exists
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	// Agent share grants: deterministic act-as authorization reads. Seeded grants
	// populate; unseeded (agent, grantee) pairs fail closed as not-found.
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*models.AgentShareGrant); !ok {
			return false
		}
		_, exists := state.seededAgentShareGrants[round12AgentShareGrantKey(state.lastPK, state.lastSK)]
		return exists
	})).Run(func(args mock.Arguments) {
		grant, ok := args.Get(0).(*models.AgentShareGrant)
		if !ok {
			return
		}
		if seeded := state.seededAgentShareGrants[round12AgentShareGrantKey(state.lastPK, state.lastSK)]; seeded != nil {
			*grant = *seeded
		}
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*models.AgentShareGrant); !ok {
			return false
		}
		_, exists := state.seededAgentShareGrants[round12AgentShareGrantKey(state.lastPK, state.lastSK)]
		return !exists
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		round12PopulateStruct(args.Get(0), state)
	}).Return(nil).Maybe()
	return mockDB, mockQuery, state
}

func round12AgentShareGrantKey(pk, sk string) string {
	agent := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(pk, "USER#")))
	grantee := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(sk, "AGENT_SHARE#GRANTEE#")))
	return agent + "|" + grantee
}

func round12PopulateStruct(dest any, state *round12PermissiveQueryState) {
	if round12PopulateAccountProjection(dest, state) {
		return
	}

	switch v := dest.(type) {
	case *models.User:
		username := strings.TrimPrefix(state.lastPK, "USER#")
		if strings.TrimSpace(username) == "" {
			username = "admin"
		}
		user := round12SeededAccountUser(state, username)
		v.Username = user.Username
		v.Email = user.Email
		v.PasswordHash = user.PasswordHash
		v.DisplayName = user.DisplayName
		v.Note = user.Note
		v.Avatar = user.Avatar
		v.Header = user.Header
		v.URL = user.URL
		v.Locked = user.Locked
		v.Discoverable = user.Discoverable
		v.AllowNSFW = user.AllowNSFW
		v.RequireNSFWWarning = user.RequireNSFWWarning
		v.Fields = append([]map[string]string(nil), user.Fields...)
		v.CreatedAt = user.CreatedAt
		v.UpdatedAt = user.UpdatedAt
		v.Approved = user.Approved
		v.Suspended = user.Suspended
		v.Silenced = user.Silenced
		v.Role = user.Role
		v.Locale = user.Locale
		v.RecoveryMethods = append([]string(nil), user.RecoveryMethods...)
		v.Metadata = round12CloneMetadata(user.Metadata)
		v.IsAgent = user.IsAgent
		v.AgentType = user.AgentType
		v.AgentCapabilities = round12CloneCapabilities(user.AgentCapabilities)
		v.AgentVersion = user.AgentVersion
		v.AgentOwner = user.AgentOwner
		v.AgentCreatedBy = user.AgentCreatedBy
		v.AgentPublicKey = user.AgentPublicKey
		v.AgentKeyType = user.AgentKeyType
		v.Version = user.Version
		v.PK = fmt.Sprintf("USER#%s", user.Username)
		v.SK = models.SKMetadata
		return
	case *models.Actor:
		username := strings.TrimPrefix(state.lastPK, "ACTOR#")
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		v.Username = username
		v.Actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   config.Get().ActorURL(username),
				Type: activitypub.PersonType,
			},
			PreferredUsername: username,
		}
		v.NumericID = common.GenerateNumericID(username)
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.WalletCredential:
		username := strings.TrimPrefix(state.lastPK, "USER#")
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		addresses := []string{"0x1111111111111111111111111111111111111111", "0x2222222222222222222222222222222222222222"}
		index := state.autoPopulateIndex
		if index < 0 || index >= len(addresses) {
			index = 0
		}
		v.PK = fmt.Sprintf("USER#%s", username)
		v.SK = fmt.Sprintf("WALLET#%s", addresses[index])
		v.Username = username
		v.Address = addresses[index]
		v.ChainID = 1
		v.Type = "ethereum"
		v.LinkedAt = time.Now().Add(-2 * time.Hour)
		v.LastUsed = time.Now().Add(time.Duration(index) * time.Minute)
		return
	case *models.RelationshipRecord:
		follower := strings.TrimPrefix(state.lastPK, "FOLLOW#")
		if strings.TrimSpace(follower) == "" {
			follower = "alice"
		}
		following := strings.TrimPrefix(state.lastSK, "FOLLOWING#")
		if strings.TrimSpace(following) == "" {
			following = "bob"
		}
		v.PK = fmt.Sprintf("FOLLOW#%s", follower)
		v.SK = fmt.Sprintf("FOLLOWING#%s", following)
		v.GSI1PK = fmt.Sprintf("FOLLOW#%s", following)
		v.GSI1SK = fmt.Sprintf("FOLLOWER#%s", follower)
		v.ActivityID = "activity-1"
		v.State = models.RelationshipAccepted
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.QuotePermissions:
		username := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(state.lastPK, "USER#")))
		if permissions := state.seededQuotePermissions[username]; permissions != nil {
			*v = *permissions
			v.BlockList = append([]string(nil), permissions.BlockList...)
			return
		}
		v.Username = username
		return
	case *models.Export:
		exportID := "export-1"
		if strings.HasPrefix(state.lastPK, "EXPORT#") {
			exportID = strings.TrimPrefix(state.lastPK, "EXPORT#")
			if strings.TrimSpace(exportID) == "" {
				exportID = "export-1"
			}
		}
		v.ID = exportID
		username := "alice"
		if strings.HasPrefix(state.lastPK, "USER#") {
			username = strings.TrimPrefix(state.lastPK, "USER#")
		}
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		v.Username = username
		v.Type = "archive"
		v.Format = "csv"
		v.Status = "completed"
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.Import:
		importID := "import-1"
		if strings.HasPrefix(state.lastPK, "IMPORT#") {
			importID = strings.TrimPrefix(state.lastPK, "IMPORT#")
			if strings.TrimSpace(importID) == "" {
				importID = "import-1"
			}
		}
		v.ID = importID
		username := "alice"
		if strings.HasPrefix(state.lastPK, "USER#") {
			username = strings.TrimPrefix(state.lastPK, "USER#")
		}
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		v.Username = username
		v.Type = "followers"
		switch importID {
		case "processing":
			v.Status = "processing"
		default:
			v.Status = "completed"
		}
		v.Progress = 1.0
		v.Total = 1
		v.SuccessCount = 1
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.ImportBudget:
		if state != nil && state.seededImportBudgets != nil {
			key := state.lastPK + "#" + state.lastSK
			if budget := state.seededImportBudgets[key]; budget != nil {
				*v = *budget
				return
			}
		}
		v.PK = state.lastPK
		v.SK = state.lastSK
		v.IsActive = false
		return
	case *models.Media:
		mediaID := strings.TrimPrefix(state.lastPK, "media#")
		if strings.TrimSpace(mediaID) == "" {
			mediaID = "m1"
		}
		if state != nil && state.seededMedia != nil {
			if seeded := state.seededMedia[mediaID]; seeded != nil {
				*v = *round12CloneMedia(seeded)
				return
			}
		}
		v.MediaID = mediaID
		v.UserID = "alice"
		v.ContentType = "image/jpeg"
		v.FileName = mediaID + ".jpg"
		v.FileSize = 1024
		v.Version = "original"
		v.UploadedAt = time.Now().Add(-time.Hour)
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		v.Status = "ready"
		v.Duration = 120
		v.CDNUrl = "https://cdn.local/media/" + mediaID + ".m3u8"
		v.Variants = map[string]models.MediaVariant{
			"480p":  {CDNUrl: "https://cdn.local/media/" + mediaID + "/480.m3u8", ContentType: "application/x-mpegURL", FileSize: 123},
			"1080p": {CDNUrl: "https://cdn.local/media/" + mediaID + "/1080.m3u8", ContentType: "application/x-mpegURL", FileSize: 456},
		}
		return
	case *models.CommunityNote:
		noteID := strings.TrimPrefix(state.lastPK, "NOTE#")
		if strings.TrimSpace(noteID) == "" {
			noteID = "note-1"
		}
		v.ID = noteID
		v.ObjectID = "status-1"
		v.ObjectType = "status"
		v.AuthorID = config.Get().ActorURL("bob")
		v.Content = `<script>alert(1)</script><b>legacy note</b>`
		v.Language = "en"
		v.VisibilityStatus = "pending"
		v.Score = 0
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.PushSubscription:
		username := strings.TrimPrefix(state.lastPK, "PUSH#")
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		id := "sub-1"
		if strings.HasPrefix(state.lastSK, "SUB#") {
			id = strings.TrimPrefix(state.lastSK, "SUB#")
			if strings.TrimSpace(id) == "" {
				id = "sub-1"
			}
		}
		v.Username = username
		v.ID = id
		v.Endpoint = "https://push.example/endpoint/" + id
		v.Auth = "auth-" + id
		v.P256dh = "p256dh-" + id
		v.Policy = "all"
		v.Alerts = models.PushSubscriptionAlerts{
			Follow:        true,
			Favourite:     false,
			Reblog:        true,
			Mention:       true,
			Poll:          false,
			FollowRequest: true,
			Status:        true,
			Update:        false,
			AdminSignUp:   false,
			AdminReport:   false,
		}
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.VAPIDKeyRecord:
		v.PK = "INSTANCE#CONFIG"
		v.SK = "VAPID_KEYS"
		createdAt := time.Now().Add(-time.Hour).UTC()
		updatedAt := time.Now().Add(-time.Minute).UTC()
		v.Data = map[string]any{
			"public_key":  "test-public-key",
			"private_key": "test-private-key",
			"subject":     "mailto:admin@localhost",
			"created_at":  createdAt.Format(time.RFC3339Nano),
			"updated_at":  updatedAt.Format(time.RFC3339Nano),
		}
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.EmojiModel:
		shortcode := strings.TrimPrefix(state.lastPK, "EMOJI#")
		shortcode = strings.TrimSpace(strings.TrimSuffix(shortcode, "@localhost"))
		if strings.TrimSpace(shortcode) == "" {
			shortcode = "existing"
		}
		v.Shortcode = shortcode
		v.URL = "https://cdn.local/emoji/" + shortcode + ".png"
		v.StaticURL = v.URL
		v.VisibleInPicker = true
		v.Category = "custom"
		v.Domain = ""
		v.Disabled = false
		v.CreatedAt = time.Now().Add(-24 * time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Hour)
		_ = v.UpdateKeys()
		return
	case *models.ScheduledStatus:
		username := "alice"
		if strings.HasPrefix(state.lastPK, "USER#") {
			trimmed := strings.TrimPrefix(state.lastPK, "USER#")
			if idx := strings.Index(trimmed, "#"); idx >= 0 {
				if candidate := strings.TrimSpace(trimmed[:idx]); candidate != "" {
					username = candidate
				}
			} else if candidate := strings.TrimSpace(trimmed); candidate != "" {
				username = candidate
			}
		}

		id := strings.TrimPrefix(state.lastSK, "ID#")
		if strings.TrimSpace(id) == "" {
			id = fmt.Sprintf("sched_%d", state.autoPopulateIndex+1)
		}

		v.ID = id
		v.Username = username
		v.Status = "scheduled status"
		v.Visibility = "public"
		v.Language = "en"
		v.Sensitive = false
		v.ScheduledAt = time.Now().Add(time.Duration(state.autoPopulateIndex+1) * time.Hour)
		v.CreatedAt = time.Now().Add(-24 * time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Hour)
		v.Published = false
		_ = v.UpdateKeys()
		return
	case *models.List:
		username := "alice"
		if strings.HasPrefix(state.lastPK, "USER_LISTS#") {
			if candidate := strings.TrimPrefix(state.lastPK, "USER_LISTS#"); strings.TrimSpace(candidate) != "" {
				username = candidate
			}
		}

		v.Username = username
		if strings.HasPrefix(state.lastPK, "LIST#") {
			v.ID = strings.TrimPrefix(state.lastPK, "LIST#")
		}
		if strings.TrimSpace(v.ID) == "" {
			v.ID = fmt.Sprintf("list_%d", state.autoPopulateIndex+1)
		}
		v.Title = "Test List"
		if strings.TrimSpace(v.RepliesPolicy) == "" {
			v.RepliesPolicy = "list"
		}
		v.Exclusive = false
		if v.CreatedAt.IsZero() {
			v.CreatedAt = time.Now().Add(-time.Hour)
		}
		v.UpdatedAt = time.Now()
		_ = v.UpdateKeys()
		return
	case *models.Conversation:
		username := "alice"
		if strings.HasPrefix(state.lastPK, "USER_CONVERSATIONS#") {
			if candidate := strings.TrimPrefix(state.lastPK, "USER_CONVERSATIONS#"); strings.TrimSpace(candidate) != "" {
				username = candidate
			}
		}

		if strings.HasPrefix(state.lastPK, "CONVERSATION#") {
			v.ID = strings.TrimPrefix(state.lastPK, "CONVERSATION#")
		}
		if strings.TrimSpace(v.ID) == "" {
			v.ID = fmt.Sprintf("conv_%d", state.autoPopulateIndex+1)
		}
		v.Participants = []string{username, "bob"}
		v.LastStatusID = "status-1"
		v.Unread = true
		if v.CreatedAt.IsZero() {
			v.CreatedAt = time.Now().Add(-time.Hour)
		}
		v.UpdatedAt = time.Now()
		v.LastMessageTime = v.UpdatedAt
		v.TotalMessageCount = 1
		_ = v.UpdateKeys()
		return
	case *models.ConversationParticipantRecord:
		if v.Conversation == nil {
			v.Conversation = &models.Conversation{}
		}
		round12PopulateStruct(v.Conversation, state)
		if strings.TrimSpace(v.ViewerID) == "" {
			v.ViewerID = "alice"
		}
		if strings.TrimSpace(v.ConversationID) == "" {
			v.ConversationID = v.Conversation.ID
		}
		if v.Folder == "" {
			v.Folder = models.UserConversationFolderInbox
		}
		v.Unread = true
		return
	case *models.UserConversationState:
		viewerID := "alice"
		if strings.Contains(state.lastPK, "USER_CONVERSATION_STATE#") {
			if candidate := strings.TrimPrefix(state.lastPK, "USER_CONVERSATION_STATE#"); strings.TrimSpace(candidate) != "" {
				viewerID = candidate
			}
		}
		v.ViewerID = viewerID
		if strings.HasPrefix(state.lastSK, "CONVERSATION#") {
			v.ConversationID = strings.TrimPrefix(state.lastSK, "CONVERSATION#")
		}
		if strings.TrimSpace(v.ConversationID) == "" {
			v.ConversationID = fmt.Sprintf("conv_%d", state.autoPopulateIndex+1)
		}
		v.CounterpartID = "bob"
		v.Folder = models.UserConversationFolderInbox
		v.RequestState = models.DmRequestStateAccepted
		v.Unread = true
		if v.CreatedAt.IsZero() {
			v.CreatedAt = time.Now().Add(-time.Hour)
		}
		v.UpdatedAt = time.Now()
		if v.SortAt.IsZero() {
			v.SortAt = v.UpdatedAt
		}
		_ = v.UpdateKeys()
		return
	case *models.Announcement:
		id := fmt.Sprintf("announcement-%d", state.autoPopulateIndex+1)
		v.ID = id
		v.Content = "<p>Hello</p>"
		v.Text = "Hello"
		v.PublishedAt = time.Now().Add(-time.Duration(state.autoPopulateIndex) * time.Hour)
		v.UpdatedAt = v.PublishedAt.Add(5 * time.Minute)
		v.AllDay = false
		if state.autoPopulateIndex%2 == 0 {
			start := time.Now().Add(10 * time.Minute)
			end := start.Add(1 * time.Hour)
			v.StartsAt = &start
			v.EndsAt = &end
		}
		// Provide a stable reaction list to exercise "available reactions" paths.
		v.Reactions = []models.Reaction{
			{Name: "👍"},
			{Name: ":custom:", URL: "https://cdn.local/custom.png", StaticURL: "https://cdn.local/custom.png"},
		}
		v.CreatedBy = "admin"
		_ = v.UpdateKeys()
		return
	case *models.AnnouncementDismissal:
		username := strings.TrimPrefix(state.lastPK, "USER#")
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		v.Username = username
		// Dismiss the first auto-populated announcement by default.
		v.AnnouncementID = "announcement-1"
		v.DismissedAt = time.Now().Add(-time.Minute)
		_ = v.UpdateKeys()
		return
	case *models.AnnouncementReaction:
		announcementID := strings.TrimPrefix(state.lastPK, "ANNOUNCEMENT_REACTION#")
		if strings.TrimSpace(announcementID) == "" {
			announcementID = "announcement-1"
		}
		v.AnnouncementID = announcementID
		v.Username = "alice"
		v.EmojiName = "👍"
		v.ReactedAt = time.Now().Add(-time.Minute)
		_ = v.UpdateKeys()
		return
	case *models.AgentGovernanceState:
		username := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(state.lastPK, "USER#")))
		seeded := state.seededGovernanceStates[username]
		if seeded == nil {
			return
		}
		v.Username = seeded.Username
		v.QuarantineStatus = seeded.QuarantineStatus
		v.QuarantineStart = round12CloneTime(seeded.QuarantineStart)
		v.QuarantineEnd = round12CloneTime(seeded.QuarantineEnd)
		v.QuarantineApprovedBy = seeded.QuarantineApprovedBy
		v.QuarantineApprovedAt = round12CloneTime(seeded.QuarantineApprovedAt)
		v.DelegatedScopes = append([]string(nil), seeded.DelegatedScopes...)
		v.SelfScopes = append([]string(nil), seeded.SelfScopes...)
		v.SelfSovereign = seeded.SelfSovereign
		v.Verified = seeded.Verified
		v.VerifiedAt = round12CloneTime(seeded.VerifiedAt)
		v.VerifiedBy = seeded.VerifiedBy
		v.VerifiedReason = seeded.VerifiedReason
		v.UnverifiedAt = round12CloneTime(seeded.UnverifiedAt)
		v.UnverifiedBy = seeded.UnverifiedBy
		v.UnverifiedReason = seeded.UnverifiedReason
		v.KeyRotatedAt = round12CloneTime(seeded.KeyRotatedAt)
		v.CreatedAt = seeded.CreatedAt
		v.UpdatedAt = seeded.UpdatedAt
		v.Version = seeded.Version
		_ = v.UpdateKeys()
		return
	default:
		// Best-effort generic population for pointer-to-struct destinations.
		rv := reflect.ValueOf(dest)
		if rv.Kind() != reflect.Ptr || rv.IsNil() {
			return
		}
		// Support ORM-style pointer-to-pointer destinations (e.g. **T), allocating the inner value
		// so downstream code doesn't panic when a mock "First" returns nil error.
		if rv.Elem().Kind() == reflect.Ptr {
			if rv.Elem().IsNil() && rv.Elem().Type().Elem().Kind() == reflect.Struct {
				rv.Elem().Set(reflect.New(rv.Elem().Type().Elem()))
			}
			return
		}
		if rv.Elem().Kind() != reflect.Struct {
			return
		}
	}
}

func round12PopulateAccountProjection(dest any, state *round12PermissiveQueryState) bool {
	typeName := reflect.TypeOf(dest).String()
	if typeName != "*repositories.userCoreProjection" && typeName != "*repositories.userMetadataProjection" {
		return false
	}

	username := strings.TrimPrefix(state.lastPK, "USER#")
	if strings.TrimSpace(username) == "" {
		username = "admin"
	}
	user := round12SeededAccountUser(state, username)

	value := reflect.ValueOf(dest).Elem()
	round12SetField(value, "Table", "test-table")
	round12SetField(value, "PK", "USER#"+user.Username)
	round12SetField(value, "SK", models.SKMetadata)

	if typeName == "*repositories.userMetadataProjection" {
		round12SetField(value, "Metadata", user.Metadata)
		return true
	}

	round12SetField(value, "Username", user.Username)
	round12SetField(value, "Email", user.Email)
	round12SetField(value, "PasswordHash", user.PasswordHash)
	round12SetField(value, "DisplayName", user.DisplayName)
	round12SetField(value, "Note", user.Note)
	round12SetField(value, "Avatar", user.Avatar)
	round12SetField(value, "Header", user.Header)
	round12SetField(value, "URL", user.URL)
	round12SetField(value, "Locked", user.Locked)
	round12SetField(value, "Discoverable", user.Discoverable)
	round12SetField(value, "Fields", user.Fields)
	round12SetField(value, "CreatedAt", user.CreatedAt)
	round12SetField(value, "UpdatedAt", user.UpdatedAt)
	round12SetField(value, "Approved", user.Approved)
	round12SetField(value, "Suspended", user.Suspended)
	round12SetField(value, "Silenced", user.Silenced)
	round12SetField(value, "Role", user.Role)
	round12SetField(value, "Locale", user.Locale)
	round12SetField(value, "RecoveryMethods", user.RecoveryMethods)
	round12SetField(value, "AllowNSFW", user.AllowNSFW)
	round12SetField(value, "RequireNSFWWarning", user.RequireNSFWWarning)
	round12SetField(value, "IsAgent", user.IsAgent)
	round12SetField(value, "AgentType", user.AgentType)
	round12SetField(value, "AgentCapabilities", user.AgentCapabilities)
	round12SetField(value, "AgentVersion", user.AgentVersion)
	round12SetField(value, "AgentOwner", user.AgentOwner)
	round12SetField(value, "AgentCreatedBy", user.AgentCreatedBy)
	round12SetField(value, "AgentPublicKey", user.AgentPublicKey)
	round12SetField(value, "AgentKeyType", user.AgentKeyType)
	round12SetField(value, "Version", user.Version)
	return true
}

func round12SeededAccountUser(state *round12PermissiveQueryState, username string) storagepkg.User {
	username = strings.ToLower(strings.TrimSpace(username))
	if state != nil && state.seededAccountUsers != nil {
		if seeded := state.seededAccountUsers[username]; seeded != nil {
			return *round12CloneStorageUser(seeded)
		}
	}

	role := adminRoleUser
	if strings.EqualFold(username, "admin") {
		role = adminRoleAdmin
	}
	now := time.Now()
	return storagepkg.User{
		Username:  username,
		Role:      role,
		Approved:  true,
		Version:   1,
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
}

func round12ApplyPendingUpdate(state *round12PermissiveQueryState) {
	if state == nil {
		return
	}
	defer func() {
		state.pendingUpdateSets = nil
		state.pendingUpdateRemovals = nil
	}()

	// Media rows use media#<id> partition keys; apply field-scoped writer
	// updates to the seeded map so the double models the write semantics.
	if mediaID := strings.TrimPrefix(state.lastPK, "media#"); mediaID != state.lastPK && strings.TrimSpace(mediaID) != "" {
		if seeded := state.seededMedia[mediaID]; seeded != nil {
			updated := round12CloneMedia(seeded)
			for field, value := range state.pendingUpdateSets {
				round12ApplyMediaField(updated, field, value)
			}
			for field := range state.pendingUpdateRemovals {
				round12RemoveMediaField(updated, field)
			}
			state.seededMedia[mediaID] = updated
		}
		return
	}

	username := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(state.lastPK, "USER#")))
	if username == "" {
		return
	}

	switch state.lastSK {
	case models.SKMetadata:
		user := round12SeededAccountUser(state, username)
		round12ApplyUserUpdate(&user, state.pendingUpdateSets, state.pendingUpdateRemovals)
		if state.seededAccountUsers == nil {
			state.seededAccountUsers = map[string]*storagepkg.User{}
		}
		state.seededAccountUsers[username] = round12CloneStorageUser(&user)
	case models.SKAgentGovernance:
		governance := round12SeededGovernanceState(state, username)
		round12ApplyGovernanceUpdate(governance, state.pendingUpdateSets, state.pendingUpdateRemovals)
		if state.seededGovernanceStates == nil {
			state.seededGovernanceStates = map[string]*storagepkg.AgentGovernanceState{}
		}
		state.seededGovernanceStates[username] = governance.Clone()
	}
}

func round12ApplyMediaField(media *models.Media, field string, value any) {
	switch field {
	case "EditorialState":
		if state, ok := value.(models.EditorialLifecycle); ok {
			media.EditorialState = state
		}
	case "SupersededByMediaID":
		if id, ok := value.(string); ok {
			media.SupersededByMediaID = id
		}
	case "PublishedS3Key":
		if key, ok := value.(string); ok {
			media.PublishedS3Key = key
		}
	case "PublishedURL":
		if url, ok := value.(string); ok {
			media.PublishedURL = url
		}
	case "PublishedAt":
		if at, ok := value.(time.Time); ok {
			media.PublishedAt = &at
		}
	}
}

func round12RemoveMediaField(media *models.Media, field string) {
	switch field {
	case "EditorialState":
		media.EditorialState = ""
	case "SupersededByMediaID":
		media.SupersededByMediaID = ""
	}
}

func round12ApplyDirectModelUpdate(state *round12PermissiveQueryState) {
	if state == nil {
		return
	}

	switch model := state.currentModel.(type) {
	case *models.User:
		round12StoreUpdatedUserModel(state, model)
	case models.User:
		modelCopy := model
		round12StoreUpdatedUserModel(state, &modelCopy)
	case *models.Media:
		round12StoreMediaModel(state, model)
	case models.Media:
		modelCopy := model
		round12StoreMediaModel(state, &modelCopy)
	}
}

func round12ApplyDirectModelCreate(state *round12PermissiveQueryState) {
	if state == nil {
		return
	}
	switch model := state.currentModel.(type) {
	case *models.Media:
		round12StoreMediaModel(state, model)
	case models.Media:
		modelCopy := model
		round12StoreMediaModel(state, &modelCopy)
	}
}

func round12StoreMediaModel(state *round12PermissiveQueryState, media *models.Media) {
	if state == nil || !state.persistMedia || media == nil || strings.TrimSpace(media.MediaID) == "" {
		return
	}
	if state.seededMedia == nil {
		state.seededMedia = make(map[string]*models.Media)
	}
	state.seededMedia[media.MediaID] = round12CloneMedia(media)
}

func round12CloneMedia(media *models.Media) *models.Media {
	if media == nil {
		return nil
	}
	clone := *media
	clone.Variants = make(map[string]models.MediaVariant, len(media.Variants))
	for name, variant := range media.Variants {
		clone.Variants[name] = variant
	}
	if media.Provenance != nil {
		provenance := *media.Provenance
		provenance.SourceReferences = append([]string(nil), media.Provenance.SourceReferences...)
		clone.Provenance = &provenance
	}
	return &clone
}

func round12StoreUpdatedUserModel(state *round12PermissiveQueryState, model *models.User) {
	if state == nil || model == nil {
		return
	}

	username := strings.ToLower(strings.TrimSpace(model.Username))
	if username == "" {
		username = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(model.PK, "USER#")))
	}
	if username == "" {
		return
	}

	if state.seededAccountUsers == nil {
		state.seededAccountUsers = map[string]*storagepkg.User{}
	}
	state.seededAccountUsers[username] = &storagepkg.User{
		Username:           username,
		Email:              model.Email,
		PasswordHash:       model.PasswordHash,
		DisplayName:        model.DisplayName,
		Note:               model.Note,
		Avatar:             model.Avatar,
		Header:             model.Header,
		URL:                model.URL,
		Locked:             model.Locked,
		Discoverable:       model.Discoverable,
		AllowNSFW:          model.AllowNSFW,
		RequireNSFWWarning: model.RequireNSFWWarning,
		Fields:             append([]map[string]string(nil), model.Fields...),
		CreatedAt:          model.CreatedAt,
		UpdatedAt:          model.UpdatedAt,
		Approved:           model.Approved,
		Suspended:          model.Suspended,
		Silenced:           model.Silenced,
		Role:               model.Role,
		Locale:             model.Locale,
		RecoveryMethods:    append([]string(nil), model.RecoveryMethods...),
		Metadata:           round12CloneMetadata(model.Metadata),
		IsAgent:            model.IsAgent,
		AgentType:          model.AgentType,
		AgentCapabilities:  round12CloneCapabilities(model.AgentCapabilities),
		AgentVersion:       model.AgentVersion,
		AgentOwner:         model.AgentOwner,
		AgentCreatedBy:     model.AgentCreatedBy,
		AgentPublicKey:     model.AgentPublicKey,
		AgentKeyType:       model.AgentKeyType,
		Version:            model.Version,
	}
}

func round12SeededGovernanceState(state *round12PermissiveQueryState, username string) *storagepkg.AgentGovernanceState {
	username = strings.ToLower(strings.TrimSpace(username))
	if state != nil && state.seededGovernanceStates != nil {
		if seeded := state.seededGovernanceStates[username]; seeded != nil {
			return seeded.Clone()
		}
	}
	now := time.Now().UTC()
	return &storagepkg.AgentGovernanceState{
		Username:  username,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func round12ApplyUserUpdate(user *storagepkg.User, sets map[string]any, removals map[string]struct{}) {
	if user == nil {
		return
	}
	for field, value := range sets {
		switch field {
		case "DisplayName":
			user.DisplayName, _ = value.(string)
		case "Note":
			user.Note, _ = value.(string)
		case "Avatar":
			user.Avatar, _ = value.(string)
		case "Header":
			user.Header, _ = value.(string)
		case "URL":
			user.URL, _ = value.(string)
		case "Locked":
			user.Locked, _ = value.(bool)
		case "Discoverable":
			user.Discoverable, _ = value.(bool)
		case "Fields":
			if typed, ok := value.([]map[string]string); ok {
				user.Fields = append([]map[string]string(nil), typed...)
			}
		case "Email":
			user.Email, _ = value.(string)
		case "Locale":
			user.Locale, _ = value.(string)
		case "IsAgent":
			user.IsAgent, _ = value.(bool)
		case "AgentType":
			user.AgentType, _ = value.(string)
		case "AgentCapabilities":
			if typed, ok := value.(*agentpkg.Capabilities); ok {
				user.AgentCapabilities = typed
			}
		case "AgentVersion":
			user.AgentVersion, _ = value.(string)
		case "AgentOwner":
			user.AgentOwner, _ = value.(string)
		case "AgentCreatedBy":
			user.AgentCreatedBy, _ = value.(string)
		case "AgentPublicKey":
			user.AgentPublicKey, _ = value.(string)
		case "AgentKeyType":
			user.AgentKeyType, _ = value.(string)
		case "Approved":
			user.Approved, _ = value.(bool)
		case "Suspended":
			user.Suspended, _ = value.(bool)
		case "Silenced":
			user.Silenced, _ = value.(bool)
		case "Role":
			user.Role, _ = value.(string)
		case "RecoveryMethods":
			if typed, ok := value.([]string); ok {
				user.RecoveryMethods = append([]string(nil), typed...)
			}
		case "AllowNSFW":
			user.AllowNSFW, _ = value.(bool)
		case "RequireNSFWWarning":
			user.RequireNSFWWarning, _ = value.(bool)
		case "Metadata":
			if typed, ok := value.(map[string]interface{}); ok {
				user.Metadata = round12CloneMetadata(typed)
			}
		case "UpdatedAt":
			if typed, ok := value.(time.Time); ok {
				user.UpdatedAt = typed
			}
		case "Version":
			switch typed := value.(type) {
			case int:
				user.Version = typed
			case int64:
				user.Version = int(typed)
			}
		}
	}

	for field := range removals {
		if field == "Metadata" {
			user.Metadata = nil
		}
	}
}

func round12ApplyGovernanceUpdate(
	state *storagepkg.AgentGovernanceState,
	sets map[string]any,
	removals map[string]struct{},
) {
	if state == nil {
		return
	}
	for field, value := range sets {
		switch field {
		case "Username":
			state.Username, _ = value.(string)
		case "CreatedAt":
			if typed, ok := value.(time.Time); ok {
				state.CreatedAt = typed
			}
		case "UpdatedAt":
			if typed, ok := value.(time.Time); ok {
				state.UpdatedAt = typed
			}
		case "QuarantineStatus":
			state.QuarantineStatus, _ = value.(string)
		case "QuarantineStart":
			state.QuarantineStart = round12TimeValuePtr(value)
		case "QuarantineEnd":
			state.QuarantineEnd = round12TimeValuePtr(value)
		case "QuarantineApprovedBy":
			state.QuarantineApprovedBy, _ = value.(string)
		case "QuarantineApprovedAt":
			state.QuarantineApprovedAt = round12TimeValuePtr(value)
		case "DelegatedScopes":
			if typed, ok := value.([]string); ok {
				state.DelegatedScopes = append([]string(nil), typed...)
			}
		case "SelfScopes":
			if typed, ok := value.([]string); ok {
				state.SelfScopes = append([]string(nil), typed...)
			}
		case "SelfSovereign":
			state.SelfSovereign, _ = value.(bool)
		case "Verified":
			state.Verified, _ = value.(bool)
		case "VerifiedAt":
			state.VerifiedAt = round12TimeValuePtr(value)
		case "VerifiedBy":
			state.VerifiedBy, _ = value.(string)
		case "VerifiedReason":
			state.VerifiedReason, _ = value.(string)
		case "UnverifiedAt":
			state.UnverifiedAt = round12TimeValuePtr(value)
		case "UnverifiedBy":
			state.UnverifiedBy, _ = value.(string)
		case "UnverifiedReason":
			state.UnverifiedReason, _ = value.(string)
		case "KeyRotatedAt":
			state.KeyRotatedAt = round12TimeValuePtr(value)
		case "Version":
			switch typed := value.(type) {
			case int:
				state.Version = typed
			case int64:
				state.Version = int(typed)
			}
		}
	}

	for field := range removals {
		switch field {
		case "QuarantineStatus":
			state.QuarantineStatus = ""
		case "QuarantineStart":
			state.QuarantineStart = nil
		case "QuarantineEnd":
			state.QuarantineEnd = nil
		case "QuarantineApprovedBy":
			state.QuarantineApprovedBy = ""
		case "QuarantineApprovedAt":
			state.QuarantineApprovedAt = nil
		case "DelegatedScopes":
			state.DelegatedScopes = nil
		case "SelfScopes":
			state.SelfScopes = nil
		case "VerifiedAt":
			state.VerifiedAt = nil
		case "VerifiedBy":
			state.VerifiedBy = ""
		case "VerifiedReason":
			state.VerifiedReason = ""
		case "UnverifiedAt":
			state.UnverifiedAt = nil
		case "UnverifiedBy":
			state.UnverifiedBy = ""
		case "UnverifiedReason":
			state.UnverifiedReason = ""
		case "KeyRotatedAt":
			state.KeyRotatedAt = nil
		}
	}
}

func round12CloneCapabilities(value *agentpkg.Capabilities) *agentpkg.Capabilities {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func round12CloneStorageUser(user *storagepkg.User) *storagepkg.User {
	if user == nil {
		return nil
	}
	cloned := *user
	cloned.Fields = append([]map[string]string(nil), user.Fields...)
	cloned.RecoveryMethods = append([]string(nil), user.RecoveryMethods...)
	if user.AgentCapabilities != nil {
		caps := *user.AgentCapabilities
		cloned.AgentCapabilities = &caps
	}
	cloned.Metadata = round12CloneMetadata(user.Metadata)
	return &cloned
}

func round12CloneMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	bytes, err := json.Marshal(metadata)
	if err != nil {
		cloned := make(map[string]interface{}, len(metadata))
		for key, value := range metadata {
			cloned[key] = value
		}
		return cloned
	}
	var cloned map[string]interface{}
	if err := json.Unmarshal(bytes, &cloned); err != nil {
		fallback := make(map[string]interface{}, len(metadata))
		for key, value := range metadata {
			fallback[key] = value
		}
		return fallback
	}
	return cloned
}

func round12CloneTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func round12TimeValuePtr(value any) *time.Time {
	switch typed := value.(type) {
	case time.Time:
		cloned := typed.UTC()
		return &cloned
	case *time.Time:
		return round12CloneTime(typed)
	default:
		return nil
	}
}

func round12SetField(target reflect.Value, name string, value any) {
	field := target.FieldByName(name)
	if !field.IsValid() || !field.CanSet() {
		return
	}
	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return
	}
	incoming := reflect.ValueOf(value)
	if incoming.Type().AssignableTo(field.Type()) {
		field.Set(incoming)
		return
	}
	if incoming.Type().ConvertibleTo(field.Type()) {
		field.Set(incoming.Convert(field.Type()))
	}
}

func round12PopulateSlice(dest any, state *round12PermissiveQueryState) {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}
	slice := rv.Elem()
	if slice.Kind() != reflect.Slice {
		return
	}
	if slice.Len() > 0 {
		return
	}

	count := state.autoPopulateCount
	if count <= 0 {
		count = 2
	}

	out := reflect.MakeSlice(slice.Type(), 0, count)
	for i := 0; i < count; i++ {
		elemType := slice.Type().Elem()
		switch elemType.Kind() {
		case reflect.Ptr:
			state.autoPopulateIndex = i
			elem := reflect.New(elemType.Elem())
			round12PopulateStruct(elem.Interface(), state)
			// Apply stable ordering differences across list items.
			if i > 0 {
				if createdAtField := elem.Elem().FieldByName("CreatedAt"); createdAtField.IsValid() && createdAtField.CanSet() && createdAtField.Type() == reflect.TypeOf(time.Time{}) {
					createdAtField.Set(reflect.ValueOf(time.Now().Add(-time.Duration(i) * time.Hour)))
				}
			}
			out = reflect.Append(out, elem)
		case reflect.Struct:
			state.autoPopulateIndex = i
			elem := reflect.New(elemType)
			round12PopulateStruct(elem.Interface(), state)
			// Apply stable ordering differences across list items.
			if i > 0 {
				if createdAtField := elem.Elem().FieldByName("CreatedAt"); createdAtField.IsValid() && createdAtField.CanSet() && createdAtField.Type() == reflect.TypeOf(time.Time{}) {
					createdAtField.Set(reflect.ValueOf(time.Now().Add(-time.Duration(i) * time.Hour)))
				}
			}
			out = reflect.Append(out, elem.Elem())
		case reflect.String:
			out = reflect.Append(out, reflect.ValueOf(fmt.Sprintf("item-%d", i+1)))
		default:
			// Unsupported slice element kinds; leave empty.
			return
		}
	}

	slice.Set(out)
}

type round12ArticleRepoWithDB struct {
	*inmemory.ArticleRepository
	db dynamormcore.DB
}

func (r *round12ArticleRepoWithDB) GetDB() dynamormcore.DB { return r.db }

type round12CategoryRepoWithDB struct {
	*inmemory.CategoryRepository
	db dynamormcore.DB
}

func (r *round12CategoryRepoWithDB) GetDB() dynamormcore.DB { return r.db }

type round12PublicationRepoWithDB struct {
	*inmemory.PublicationRepository
	db dynamormcore.DB
}

func (r *round12PublicationRepoWithDB) GetDB() dynamormcore.DB { return r.db }

type round12GraphStorage struct {
	*pkgtesting.MockRepositoryStorage

	db           dynamormcore.DB
	queryState   *round12PermissiveQueryState
	accountRepo  *repositories.AccountRepository
	bookmarkRepo *repositories.BookmarkRepository
	mediaRepo    *repositories.MediaRepository

	markerRepo *repositories.MarkerRepository

	communityNoteRepo *repositories.CommunityNoteRepository
	costRepo          *repositories.TrackingRepository
	analyticsRepo     *repositories.TrendingRepository
	auditRepo         *repositories.AuditRepository

	notificationRepo     interfaces.NotificationRepository
	pushSubscriptionRepo *repositories.PushSubscriptionRepository
	pollRepo             *repositories.PollRepository
	scheduledStatusRepo  *repositories.ScheduledStatusRepository

	threadRepo *repositories.ThreadRepository

	domainBlockRepo  *repositories.DomainBlockRepository
	federationRepo   *repositories.FederationRepository
	filterRepo       *repositories.FilterRepository
	importRepo       *repositories.ImportRepository
	exportRepo       *repositories.ExportRepository
	instanceRepo     *repositories.InstanceRepository
	listRepo         *repositories.ListRepository
	quoteRepo        *repositories.QuoteRepository
	searchRepo       *repositories.SearchRepository
	hashtagRepo      *repositories.HashtagRepository
	emojiRepo        *repositories.EmojiRepository
	socialRepo       *repositories.SocialRepository
	conversationRepo *repositories.ConversationRepository

	relationshipRepo interfaces.ConcreteRelationshipRepository
	announcementRepo *repositories.AnnouncementRepository
	likeRepo         *repositories.LikeRepository
}

type round12MirroredStatusRepo struct {
	interfaces.StatusRepository
	mirror interfaces.StatusRepository
}

func (r *round12MirroredStatusRepo) CreateStatus(ctx context.Context, status *models.Status) error {
	if r.mirror != nil {
		mirrored := *status
		if err := r.mirror.CreateStatus(ctx, &mirrored); err != nil {
			return err
		}
	}
	return r.StatusRepository.CreateStatus(ctx, status)
}

func (r *round12MirroredStatusRepo) CreateBoostStatus(ctx context.Context, status *models.Status) error {
	if r.mirror != nil {
		mirrored := *status
		if err := r.mirror.CreateBoostStatus(ctx, &mirrored); err != nil {
			return err
		}
	}
	return r.StatusRepository.CreateBoostStatus(ctx, status)
}

func (s *round12GraphStorage) GetDB() dynamormcore.DB { return s.db }
func (s *round12GraphStorage) Account() *repositories.AccountRepository {
	return s.accountRepo
}
func (s *round12GraphStorage) Notification() interfaces.NotificationRepository {
	return s.notificationRepo
}
func (s *round12GraphStorage) Bookmark() *repositories.BookmarkRepository {
	return s.bookmarkRepo
}
func (s *round12GraphStorage) Media() *repositories.MediaRepository {
	return s.mediaRepo
}
func (s *round12GraphStorage) PushSubscription() *repositories.PushSubscriptionRepository {
	return s.pushSubscriptionRepo
}
func (s *round12GraphStorage) Poll() *repositories.PollRepository {
	return s.pollRepo
}
func (s *round12GraphStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return s.scheduledStatusRepo
}
func (s *round12GraphStorage) Thread() *repositories.ThreadRepository {
	return s.threadRepo
}
func (s *round12GraphStorage) Marker() *repositories.MarkerRepository {
	return s.markerRepo
}
func (s *round12GraphStorage) CommunityNote() *repositories.CommunityNoteRepository {
	return s.communityNoteRepo
}
func (s *round12GraphStorage) Cost() *repositories.TrackingRepository {
	return s.costRepo
}
func (s *round12GraphStorage) Analytics() *repositories.TrendingRepository {
	return s.analyticsRepo
}
func (s *round12GraphStorage) Audit() *repositories.AuditRepository {
	return s.auditRepo
}
func (s *round12GraphStorage) DomainBlock() *repositories.DomainBlockRepository {
	return s.domainBlockRepo
}
func (s *round12GraphStorage) Federation() *repositories.FederationRepository {
	return s.federationRepo
}
func (s *round12GraphStorage) Filter() *repositories.FilterRepository {
	return s.filterRepo
}
func (s *round12GraphStorage) Import() *repositories.ImportRepository {
	return s.importRepo
}
func (s *round12GraphStorage) Export() *repositories.ExportRepository {
	return s.exportRepo
}
func (s *round12GraphStorage) Instance() *repositories.InstanceRepository {
	return s.instanceRepo
}
func (s *round12GraphStorage) List() *repositories.ListRepository {
	return s.listRepo
}
func (s *round12GraphStorage) Quote() *repositories.QuoteRepository {
	return s.quoteRepo
}
func (s *round12GraphStorage) Search() *repositories.SearchRepository {
	return s.searchRepo
}
func (s *round12GraphStorage) Hashtag() *repositories.HashtagRepository {
	return s.hashtagRepo
}
func (s *round12GraphStorage) Emoji() *repositories.EmojiRepository {
	return s.emojiRepo
}
func (s *round12GraphStorage) Social() *repositories.SocialRepository {
	return s.socialRepo
}
func (s *round12GraphStorage) Conversation() *repositories.ConversationRepository {
	return s.conversationRepo
}
func (s *round12GraphStorage) Relationship() interfaces.ConcreteRelationshipRepository {
	return s.relationshipRepo
}
func (s *round12GraphStorage) Announcement() *repositories.AnnouncementRepository {
	return s.announcementRepo
}
func (s *round12GraphStorage) Like() *repositories.LikeRepository {
	return s.likeRepo
}

type round12SearchDeps struct {
	relationships interfaces.ConcreteRelationshipRepository
}

func (d *round12SearchDeps) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return d.relationships.GetFollowing(ctx, username, limit, cursor)
}

func (d *round12SearchDeps) IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error) {
	return d.relationships.IsBlocked(ctx, blockerActor, blockedActor)
}

func (d *round12SearchDeps) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	return d.relationships.IsBlockedBidirectional(ctx, actor1, actor2)
}

func (d *round12SearchDeps) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return d.relationships.GetFollowers(ctx, username, limit, cursor)
}

func newRound12GraphResolverWithMocks(t *testing.T) (*Resolver, *round12GraphStorage, *dynamormmocks.MockDB, *dynamormmocks.MockQuery, *round12PermissiveQueryState) {
	t.Helper()

	mockDB, mockQuery, state := setupRound12PermissiveDynamormMocks(t)

	cfg := &config.Config{
		Domain:                        "localhost",
		InstanceMode:                  config.InstanceModeHybrid,
		IntegrationTestMode:           true,
		CMSLongFormPublishingEnabled:  true,
		CMSDraftSystemEnabled:         true,
		CMSRevisionHistoryEnabled:     true,
		CMSScheduledPublishingEnabled: true,
		CMSSeriesEnabled:              true,
		CMSCategoriesEnabled:          true,
		KMSKeyID:                      "alias/lesser-test",
	}

	articleRepo := &round12ArticleRepoWithDB{ArticleRepository: inmemory.NewArticleRepository(), db: mockDB}
	categoryRepo := &round12CategoryRepoWithDB{CategoryRepository: inmemory.NewCategoryRepository(), db: mockDB}
	publicationRepo := &round12PublicationRepoWithDB{PublicationRepository: inmemory.NewPublicationRepository(), db: mockDB}

	tableName := "test-table"
	statusRepo := &round12MirroredStatusRepo{
		StatusRepository: inmemory.NewStatusRepository(),
		mirror:           repositories.NewStatusRepository(mockDB, tableName, zap.NewNop(), nil),
	}

	base := pkgtesting.NewMockRepositoryStorage(
		pkgtesting.WithArticleRepository(articleRepo),
		pkgtesting.WithCategoryRepository(categoryRepo),
		pkgtesting.WithPublicationRepository(publicationRepo),
		pkgtesting.WithStatusRepository(statusRepo),
		pkgtesting.WithTableName(tableName),
		pkgtesting.WithLogger(zap.NewNop()),
	)

	tableName = base.GetTableName()
	accountRepo := repositories.NewAccountRepository(mockDB, tableName, "localhost", zap.NewNop())
	notificationRepo := inmemory.NewNotificationRepository()
	bookmarkRepo := repositories.NewBookmarkRepository(mockDB, tableName, zap.NewNop())
	mediaRepo := repositories.NewMediaRepository(mockDB, tableName, zap.NewNop(), nil)
	pollRepo := repositories.NewPollRepository(mockDB, tableName, zap.NewNop(), nil)
	scheduledStatusRepo := repositories.NewScheduledStatusRepository(mockDB, tableName, zap.NewNop(), nil)
	scheduledStatusRepo.SetMediaRepository(mediaRepo)
	pushSubscriptionRepo := repositories.NewPushSubscriptionRepository(
		mockDB,
		tableName,
		zap.NewNop(),
		nil,
		newRound12VAPIDSecretsClient(),
		"test-vapid-secret",
		"mailto:admin@localhost",
	)
	markerRepo := repositories.NewMarkerRepository(mockDB, tableName, zap.NewNop(), nil)
	communityNoteRepo := repositories.NewCommunityNoteRepository(mockDB, tableName, zap.NewNop(), nil)
	costRepo := repositories.NewTrackingRepository(mockDB, tableName, zap.NewNop(), nil)
	analyticsRepo := repositories.NewTrendingRepository(mockDB, zap.NewNop(), nil)
	auditRepo := repositories.NewAuditRepository(mockDB, tableName, zap.NewNop(), nil)
	threadRepo := repositories.NewThreadRepository(mockDB, zap.NewNop())

	relationshipRepo := repositories.NewRelationshipRepository(mockDB, tableName, zap.NewNop())
	searchRepo := repositories.NewSearchRepository(mockDB, tableName, zap.NewNop(), nil)
	searchRepo.SetDependencies(&round12SearchDeps{relationships: relationshipRepo})

	storage := &round12GraphStorage{
		MockRepositoryStorage: base,
		db:                    mockDB,
		queryState:            state,
		accountRepo:           accountRepo,
		notificationRepo:      notificationRepo,
		bookmarkRepo:          bookmarkRepo,
		mediaRepo:             mediaRepo,
		pollRepo:              pollRepo,
		scheduledStatusRepo:   scheduledStatusRepo,
		threadRepo:            threadRepo,
		pushSubscriptionRepo:  pushSubscriptionRepo,
		markerRepo:            markerRepo,
		communityNoteRepo:     communityNoteRepo,
		costRepo:              costRepo,
		analyticsRepo:         analyticsRepo,
		auditRepo:             auditRepo,
		announcementRepo:      repositories.NewAnnouncementRepository(mockDB, tableName, zap.NewNop()),
		likeRepo:              repositories.NewLikeRepository(mockDB, tableName, zap.NewNop()),
		conversationRepo:      repositories.NewConversationRepository(mockDB, tableName, zap.NewNop(), nil),
		socialRepo:            repositories.NewSocialRepository(mockDB, tableName, zap.NewNop(), nil),
		domainBlockRepo:       repositories.NewDomainBlockRepository(mockDB, tableName, zap.NewNop()),
		federationRepo:        repositories.NewFederationRepository(mockDB, tableName, zap.NewNop(), nil, cfg),
		filterRepo:            repositories.NewFilterRepository(mockDB, tableName, zap.NewNop(), nil),
		importRepo:            repositories.NewImportRepository(mockDB, tableName, zap.NewNop()),
		exportRepo:            repositories.NewExportRepository(mockDB, tableName, zap.NewNop()),
		instanceRepo:          repositories.NewInstanceRepository(mockDB, tableName, zap.NewNop()),
		listRepo:              repositories.NewListRepository(mockDB, tableName, zap.NewNop(), nil),
		quoteRepo:             repositories.NewQuoteRepository(mockDB, tableName, zap.NewNop(), nil),
		hashtagRepo:           repositories.NewHashtagRepository(mockDB, tableName, zap.NewNop(), "localhost"),
		emojiRepo:             repositories.NewEmojiRepository(mockDB, tableName, zap.NewNop(), nil),
		searchRepo:            searchRepo,
		relationshipRepo:      relationshipRepo,
	}

	registry, err := services.NewRegistry(
		services.WithStorage(storage),
		services.WithPublisher(streaming.NewMockPublisher()),
		services.WithLogger(zap.NewNop()),
		services.WithMediaS3Service(round12MediaS3Service{state: state}),
		services.WithConfig(&services.ServiceConfig{
			BaseURL:   "https://localhost",
			JWTSecret: strings.Repeat("x", 32),
			Config:    cfg,
		}),
	)
	require.NoError(t, err)

	resolver := &Resolver{
		Registry: registry,
		Config:   cfg,
		Storage:  storage,
		Logger:   zap.NewNop(),
	}

	return resolver, storage, mockDB, mockQuery, state
}

func (s *round12GraphStorage) SeedAccountUser(user *storagepkg.User) {
	if s == nil || s.queryState == nil || user == nil {
		return
	}
	username := strings.ToLower(strings.TrimSpace(user.Username))
	if username == "" {
		return
	}
	if s.queryState.seededAccountUsers == nil {
		s.queryState.seededAccountUsers = map[string]*storagepkg.User{}
	}
	cloned := round12CloneStorageUser(user)
	cloned.Username = username
	s.queryState.seededAccountUsers[username] = cloned
}

func (s *round12GraphStorage) SeedAgentShareGrant(grant *models.AgentShareGrant) {
	if s == nil || s.queryState == nil || grant == nil {
		return
	}
	agent := strings.ToLower(strings.TrimSpace(grant.AgentUsername))
	grantee := strings.ToLower(strings.TrimSpace(grant.GranteeUsername))
	if agent == "" || grantee == "" {
		return
	}
	if s.queryState.seededAgentShareGrants == nil {
		s.queryState.seededAgentShareGrants = map[string]*models.AgentShareGrant{}
	}
	cloned := *grant
	cloned.AgentUsername = agent
	cloned.GranteeUsername = grantee
	s.queryState.seededAgentShareGrants[agent+"|"+grantee] = &cloned
}

func (s *round12GraphStorage) SeedAgentGovernanceState(state *storagepkg.AgentGovernanceState) {
	if s == nil || s.queryState == nil || state == nil {
		return
	}
	username := strings.ToLower(strings.TrimSpace(state.Username))
	if username == "" {
		return
	}
	if s.queryState.seededGovernanceStates == nil {
		s.queryState.seededGovernanceStates = map[string]*storagepkg.AgentGovernanceState{}
	}
	cloned := state.Clone()
	cloned.Username = username
	s.queryState.seededGovernanceStates[username] = cloned
}

func (s *round12GraphStorage) SeedQuotePermissions(permissions *models.QuotePermissions) {
	if s == nil || s.queryState == nil || permissions == nil {
		return
	}
	username := strings.ToLower(strings.TrimSpace(permissions.Username))
	if username == "" {
		return
	}
	if s.queryState.seededQuotePermissions == nil {
		s.queryState.seededQuotePermissions = map[string]*models.QuotePermissions{}
	}
	cloned := *permissions
	cloned.Username = username
	cloned.BlockList = append([]string(nil), permissions.BlockList...)
	s.queryState.seededQuotePermissions[username] = &cloned
}

func newRound12GraphResolver(t *testing.T) (*Resolver, *round12GraphStorage) {
	t.Helper()

	resolver, storage, _, _, _ := newRound12GraphResolverWithMocks(t)
	return resolver, storage
}
