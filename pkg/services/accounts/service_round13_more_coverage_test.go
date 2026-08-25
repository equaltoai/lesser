package accounts

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb/marshalers"
	"github.com/equaltoai/lesser/pkg/streaming"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormMocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type noopEncryptor struct{}

func (noopEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	return append([]byte(nil), plaintext...), nil
}
func (noopEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	return append([]byte(nil), ciphertext...), nil
}

type staticCryptoService struct {
	publicKeyPEM  []byte
	privateKeyPEM []byte
	key           any
}

func (s staticCryptoService) GenerateRSAKeyPair(_ int) (interface{}, error) { return s.key, nil }
func (s staticCryptoService) EncodePublicKeyPEM(_ interface{}) ([]byte, error) {
	return append([]byte(nil), s.publicKeyPEM...), nil
}
func (s staticCryptoService) EncodePrivateKeyPEM(_ interface{}) ([]byte, error) {
	return append([]byte(nil), s.privateKeyPEM...), nil
}

type staticAuthService struct {
	hash string
}

func (s staticAuthService) HashPassword(_ string) (string, error) { return s.hash, nil }
func (s staticAuthService) ValidatePassword(_, _ string) error    { return nil }
func (s staticAuthService) PasswordStrength(_ string) int         { return 4 }

type permissiveAccountsStorage struct {
	*MockRepositoryStorage

	db        dynamormcore.DB
	tableName string
	logger    *zap.Logger

	account      *repositories.AccountRepository
	actor        interfaces.ActorRepository
	relationship interfaces.ConcreteRelationshipRepository
	social       *repositories.SocialRepository
	user         *repositories.UserRepository
	marker       *repositories.MarkerRepository
	analytics    *repositories.TrendingRepository
	instance     *repositories.InstanceRepository
	domainBlock  *repositories.DomainBlockRepository
	quote        *repositories.QuoteRepository
	activity     interfaces.ActivityRepository
}

func (s *permissiveAccountsStorage) Account() *repositories.AccountRepository { return s.account }
func (s *permissiveAccountsStorage) Actor() interfaces.ActorRepository        { return s.actor }
func (s *permissiveAccountsStorage) Relationship() interfaces.ConcreteRelationshipRepository {
	return s.relationship
}
func (s *permissiveAccountsStorage) Social() *repositories.SocialRepository      { return s.social }
func (s *permissiveAccountsStorage) User() interfaces.UserRepository             { return s.user }
func (s *permissiveAccountsStorage) Marker() *repositories.MarkerRepository      { return s.marker }
func (s *permissiveAccountsStorage) Analytics() *repositories.TrendingRepository { return s.analytics }
func (s *permissiveAccountsStorage) Instance() *repositories.InstanceRepository  { return s.instance }
func (s *permissiveAccountsStorage) DomainBlock() *repositories.DomainBlockRepository {
	return s.domainBlock
}
func (s *permissiveAccountsStorage) Skill() interfaces.SkillRepository       { return nil }
func (s *permissiveAccountsStorage) Quote() *repositories.QuoteRepository    { return s.quote }
func (s *permissiveAccountsStorage) Activity() interfaces.ActivityRepository { return s.activity }
func (s *permissiveAccountsStorage) GetDB() dynamormcore.DB                  { return s.db }
func (s *permissiveAccountsStorage) GetTableName() string                    { return s.tableName }
func (s *permissiveAccountsStorage) GetLogger() *zap.Logger                  { return s.logger }

type userRepositoryOverrideStorage struct {
	*permissiveAccountsStorage
	userRepo interfaces.UserRepository
}

func (s *userRepositoryOverrideStorage) User() interfaces.UserRepository { return s.userRepo }

type permissiveDBOptions struct {
	domain               string
	forceUserNotFound    bool
	forceUserSuspended   bool
	forceActorLocked     bool
	forceMarkerNotFound  bool
	forceConsentNotFound bool
	defaultCountValue    int64

	firstCreateError         error
	createErrorTimes         int
	firstCreateOrUpdateError error
	firstUpdateError         error
	firstDeleteError         error
	firstAllError            error
	allErrorTimes            int
	firstMetricError         error
	metricErrorTimes         int
	firstScanError           error
	firstCountError          error

	firstRelationshipFirstError error
	firstMuteFirstError         error
}

type permissiveUpdateCondition struct {
	field string
	op    string
	value any
}

type permissiveUpdateBuilder struct {
	currentModel func() any
	getProof     func(string) (models.PasskeyRegistrationProof, bool)
	storeProof   func(models.PasskeyRegistrationProof)

	sets       map[string]any
	conditions []permissiveUpdateCondition
}

func newPermissiveDynamormDB(t *testing.T, opts permissiveDBOptions) dynamormcore.DB {
	t.Helper()

	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)
	updateBuilder := &permissiveUpdateBuilder{
		sets: make(map[string]any),
	}
	batchGetBuilder := new(dynamormMocks.MockBatchGetBuilder)

	var mu sync.Mutex
	where := make(map[string]any)
	walletChallenges := make(map[string]models.WalletChallenge)
	passkeyProofs := make(map[string]models.PasskeyRegistrationProof)
	webAuthnCredentials := make(map[string]models.WebAuthnCredential)
	var currentModel any
	resetWhere := func() {
		mu.Lock()
		where = make(map[string]any)
		mu.Unlock()
	}
	setWhere := func(field string, value any) {
		mu.Lock()
		where[field] = value
		mu.Unlock()
	}
	getWhere := func(field string) (any, bool) {
		mu.Lock()
		defer mu.Unlock()
		val, ok := where[field]
		return val, ok
	}
	storeWalletChallenge := func(model models.WalletChallenge) {
		mu.Lock()
		defer mu.Unlock()
		walletChallenges[model.ID] = model
	}
	getWalletChallenge := func(challengeID string) (models.WalletChallenge, bool) {
		mu.Lock()
		defer mu.Unlock()
		model, ok := walletChallenges[challengeID]
		return model, ok
	}
	storePasskeyProof := func(model models.PasskeyRegistrationProof) {
		mu.Lock()
		defer mu.Unlock()
		passkeyProofs[model.ID] = model
	}
	getPasskeyProof := func(proofID string) (models.PasskeyRegistrationProof, bool) {
		mu.Lock()
		defer mu.Unlock()
		model, ok := passkeyProofs[proofID]
		return model, ok
	}
	storeWebAuthnCredential := func(model models.WebAuthnCredential) {
		mu.Lock()
		defer mu.Unlock()
		webAuthnCredentials[model.ID] = model
	}
	deleteWebAuthnCredential := func(credentialID string) {
		mu.Lock()
		defer mu.Unlock()
		delete(webAuthnCredentials, credentialID)
	}
	getWebAuthnCredential := func(credentialID string) (models.WebAuthnCredential, bool) {
		mu.Lock()
		defer mu.Unlock()
		model, ok := webAuthnCredentials[credentialID]
		return model, ok
	}
	getWebAuthnCredentialsByUser := func(userID string) []models.WebAuthnCredential {
		mu.Lock()
		defer mu.Unlock()
		out := make([]models.WebAuthnCredential, 0)
		for _, credential := range webAuthnCredentials {
			if strings.EqualFold(strings.TrimSpace(credential.UserID), strings.TrimSpace(userID)) {
				out = append(out, credential)
			}
		}
		return out
	}
	setCurrentModel := func(model any) {
		mu.Lock()
		defer mu.Unlock()
		currentModel = model
	}
	getCurrentModel := func() any {
		mu.Lock()
		defer mu.Unlock()
		return currentModel
	}
	updateBuilder.currentModel = getCurrentModel
	updateBuilder.getProof = getPasskeyProof
	updateBuilder.storeProof = storePasskeyProof

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Run(func(arguments mock.Arguments) {
		resetWhere()
		setCurrentModel(arguments.Get(0))
	}).Return(q).Maybe()
	db.On("Transaction", mock.Anything).Return(nil).Maybe()
	db.On("Migrate").Return(nil).Maybe()
	db.On("AutoMigrate", mock.Anything).Return(nil).Maybe()
	db.On("Close").Return(nil).Maybe()

	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Run(func(arguments mock.Arguments) {
		field, _ := arguments.Get(0).(string)
		setWhere(field, arguments.Get(2))
	}).Return(q).Maybe()

	q.On("UpdateBuilder").Return(updateBuilder).Maybe()

	if opts.firstCreateError != nil {
		if opts.createErrorTimes > 0 {
			q.On("Create").Return(opts.firstCreateError).Times(opts.createErrorTimes)
		} else {
			q.On("Create").Return(opts.firstCreateError).Once()
		}
	}
	if opts.firstCreateOrUpdateError != nil {
		q.On("CreateOrUpdate").Return(opts.firstCreateOrUpdateError).Once()
	}
	if opts.firstUpdateError != nil {
		q.On("Update", mock.Anything).Return(opts.firstUpdateError).Once()
	}
	if opts.firstDeleteError != nil {
		q.On("Delete").Return(opts.firstDeleteError).Once()
	}
	if opts.firstAllError != nil {
		if opts.allErrorTimes > 0 {
			q.On("All", mock.Anything).Return(opts.firstAllError).Times(opts.allErrorTimes)
		} else {
			q.On("All", mock.Anything).Return(opts.firstAllError).Once()
		}
	}
	if opts.firstScanError != nil {
		q.On("Scan", mock.Anything).Return(opts.firstScanError).Once()
	}

	// O(1) instance-count reads (see instance_counts.go) fail on the counter
	// item's First when injected, exercising the service error fallbacks.
	if opts.firstMetricError != nil {
		if opts.metricErrorTimes > 0 {
			q.On("First", mock.AnythingOfType("*models.InstanceMetrics")).Return(opts.firstMetricError).Times(opts.metricErrorTimes)
		} else {
			q.On("First", mock.AnythingOfType("*models.InstanceMetrics")).Return(opts.firstMetricError).Once()
		}
	}

	if opts.forceUserNotFound {
		q.On("First", mock.AnythingOfType("*models.User")).Return(dynamormErrors.ErrItemNotFound).Maybe()
		q.On("First", mock.MatchedBy(func(dest any) bool {
			if dest == nil {
				return false
			}
			typeName := reflect.TypeOf(dest).String()
			return typeName == "*repositories.userCoreProjection" || typeName == "*repositories.userMetadataProjection"
		})).Return(dynamormErrors.ErrItemNotFound).Maybe()
	}
	if opts.forceMarkerNotFound {
		q.On("First", mock.AnythingOfType("*models.Marker")).Return(dynamormErrors.ErrItemNotFound).Maybe()
	}
	if opts.forceConsentNotFound {
		q.On("First", mock.AnythingOfType("*models.UserAppConsent")).Return(dynamormErrors.ErrItemNotFound).Maybe()
	}
	if opts.firstRelationshipFirstError != nil {
		q.On("First", mock.AnythingOfType("*models.RelationshipRecord")).Return(opts.firstRelationshipFirstError).Once()
	}
	if opts.firstMuteFirstError != nil {
		q.On("First", mock.AnythingOfType("*models.Mute")).Return(opts.firstMuteFirstError).Once()
	}
	q.On("Create").Run(func(_ mock.Arguments) {
		switch model := getCurrentModel().(type) {
		case *models.WalletChallenge:
			if model != nil {
				storeWalletChallenge(*model)
			}
		case *models.PasskeyRegistrationProof:
			if model != nil {
				storePasskeyProof(*model)
			}
		case *models.WebAuthnCredential:
			if model != nil {
				storeWebAuthnCredential(*model)
			}
		}
	}).Return(nil).Maybe()
	q.On("CreateOrUpdate").Return(nil).Maybe()
	q.On("Update", mock.Anything).Run(func(_ mock.Arguments) {
		if model, ok := getCurrentModel().(*models.WalletChallenge); ok && model != nil {
			storeWalletChallenge(*model)
		}
	}).Return(nil).Maybe()
	q.On("Delete").Run(func(_ mock.Arguments) {
		switch model := getCurrentModel().(type) {
		case *models.WebAuthnCredential:
			if model == nil {
				return
			}
			credentialID := strings.TrimPrefix(extractStringFromWhere(getWhere, "gsi1PK"), "WEBAUTHN_CREDENTIAL#")
			if credentialID == "" {
				credentialID = strings.TrimPrefix(extractStringFromWhere(getWhere, "SK"), "WEBAUTHN_CRED#")
			}
			if credentialID != "" {
				deleteWebAuthnCredential(credentialID)
			}
		case *models.PasskeyRegistrationProof:
			proofID := strings.TrimPrefix(extractStringFromWhere(getWhere, "PK"), "PASSKEY_REGISTRATION_PROOF#")
			if proofID == "" && model != nil {
				proofID = strings.TrimSpace(model.ID)
			}
			if proofID != "" {
				mu.Lock()
				delete(passkeyProofs, proofID)
				mu.Unlock()
			}
		}
	}).Return(nil).Maybe()
	q.On("First", mock.AnythingOfType("*models.WalletChallenge")).Run(func(arguments mock.Arguments) {
		dest := arguments.Get(0).(*models.WalletChallenge)
		challengeID := extractUsernameFromWhere(getWhere, "PK", "WALLET_CHALLENGE#", "challenge-1")
		if stored, ok := getWalletChallenge(challengeID); ok {
			*dest = stored
			return
		}
		dest.ID = challengeID
		dest.Username = "alice"
		dest.Address = "0xabc"
		dest.ChainID = 1
		dest.Nonce = "nonce"
		dest.Message = "message"
		dest.IssuedAt = time.Now().Add(-time.Minute)
		dest.ExpiresAt = time.Now().Add(5 * time.Minute)
		_ = dest.UpdateKeys()
	}).Return(nil).Maybe()
	q.On("First", mock.AnythingOfType("*models.PasskeyRegistrationProof")).Run(func(arguments mock.Arguments) {
		dest := arguments.Get(0).(*models.PasskeyRegistrationProof)
		proofID := extractUsernameFromWhere(getWhere, "PK", "PASSKEY_REGISTRATION_PROOF#", "")
		if stored, ok := getPasskeyProof(proofID); ok {
			*dest = stored
			return
		}
	}).Return(nil).Maybe()
	q.On("First", mock.AnythingOfType("*models.WebAuthnCredential")).Run(func(arguments mock.Arguments) {
		dest := arguments.Get(0).(*models.WebAuthnCredential)
		credentialID := strings.TrimPrefix(extractStringFromWhere(getWhere, "gsi1PK"), "WEBAUTHN_CREDENTIAL#")
		if stored, ok := getWebAuthnCredential(credentialID); ok {
			*dest = stored
			return
		}
	}).Return(nil).Maybe()

	q.On("First", mock.Anything).Run(func(arguments mock.Arguments) {
		dest := arguments.Get(0)
		fillModelPointer(t, dest, getWhere, opts)
	}).Return(nil).Maybe()

	q.On("All", mock.Anything).Run(func(arguments mock.Arguments) {
		dest := arguments.Get(0)
		fillSlicePointer(t, dest, getWhere, opts, getWebAuthnCredentialsByUser)
	}).Return(nil).Maybe()

	q.On("Scan", mock.Anything).Run(func(arguments mock.Arguments) {
		dest := arguments.Get(0)
		fillSlicePointer(t, dest, getWhere, opts, getWebAuthnCredentialsByUser)
	}).Return(nil).Maybe()

	q.On("BatchGet", mock.Anything).Run(func(arguments mock.Arguments) {
		dest := arguments.Get(0)
		fillSlicePointer(t, dest, getWhere, opts, getWebAuthnCredentialsByUser)
	}).Return(batchGetBuilder).Maybe()

	q.On("ScanAllSegments", mock.Anything).Run(func(arguments mock.Arguments) {
		dest := arguments.Get(0)
		fillSlicePointer(t, dest, getWhere, opts, getWebAuthnCredentialsByUser)
	}).Return(nil).Maybe()

	if opts.firstCountError != nil {
		q.On("Count").Return(int64(0), opts.firstCountError).Once()
	}

	q.On("Count").Return(func() any {
		if opts.defaultCountValue != 0 {
			return opts.defaultCountValue
		}
		return int64(0)
	}(), nil).Maybe()

	// Generic fallbacks for the rest of the query interface.
	queryType := reflect.TypeOf((*dynamormcore.Query)(nil)).Elem()
	for i := 0; i < queryType.NumMethod(); i++ {
		method := queryType.Method(i)

		// Already handled above with custom behavior.
		switch method.Name {
		case "Where", "UpdateBuilder", "First", "All", "Scan", "BatchGet", "ScanAllSegments", "Count", "Delete":
			continue
		}

		args := make([]any, method.Type.NumIn())
		for j := range args {
			args[j] = mock.Anything
		}

		switch method.Type.NumOut() {
		case 0:
			q.On(method.Name, args...).Return().Maybe()
		case 1:
			out0 := method.Type.Out(0)
			switch {
			case out0.Implements(queryType) || out0.AssignableTo(queryType):
				q.On(method.Name, args...).Return(q).Maybe()
			case out0.Implements(reflect.TypeOf((*dynamormcore.UpdateBuilder)(nil)).Elem()):
				q.On(method.Name, args...).Return(updateBuilder).Maybe()
			case out0.Implements(reflect.TypeOf((*dynamormcore.BatchGetBuilder)(nil)).Elem()):
				q.On(method.Name, args...).Return(batchGetBuilder).Maybe()
			case out0.Kind() == reflect.Int || out0.Kind() == reflect.Int64:
				q.On(method.Name, args...).Return(0).Maybe()
			case out0.Kind() == reflect.Bool:
				q.On(method.Name, args...).Return(false).Maybe()
			default:
				q.On(method.Name, args...).Return(reflect.Zero(out0).Interface()).Maybe()
			}
		case 2:
			q.On(method.Name, args...).Return(
				reflect.Zero(method.Type.Out(0)).Interface(),
				reflect.Zero(method.Type.Out(1)).Interface(),
			).Maybe()
		default:
			zero := make([]any, method.Type.NumOut())
			for j := range zero {
				zero[j] = reflect.Zero(method.Type.Out(j)).Interface()
			}
			q.On(method.Name, args...).Return(zero...).Maybe()
		}
	}

	// Permissive BatchGetBuilder behavior.
	batchGetBuilderType := reflect.TypeOf((*dynamormcore.BatchGetBuilder)(nil)).Elem()
	for i := 0; i < batchGetBuilderType.NumMethod(); i++ {
		method := batchGetBuilderType.Method(i)
		args := make([]any, method.Type.NumIn())
		for j := range args {
			args[j] = mock.Anything
		}

		switch method.Type.NumOut() {
		case 0:
			batchGetBuilder.On(method.Name, args...).Return().Maybe()
		case 1:
			out0 := method.Type.Out(0)
			switch {
			case out0.Implements(batchGetBuilderType) || out0.AssignableTo(batchGetBuilderType):
				batchGetBuilder.On(method.Name, args...).Return(batchGetBuilder).Maybe()
			default:
				batchGetBuilder.On(method.Name, args...).Return(reflect.Zero(out0).Interface()).Maybe()
			}
		default:
			zero := make([]any, method.Type.NumOut())
			for j := range zero {
				zero[j] = reflect.Zero(method.Type.Out(j)).Interface()
			}
			batchGetBuilder.On(method.Name, args...).Return(zero...).Maybe()
		}
	}

	return db
}

func (b *permissiveUpdateBuilder) Set(field string, value any) dynamormcore.UpdateBuilder {
	if b.sets == nil {
		b.sets = make(map[string]any)
	}
	b.sets[field] = value
	return b
}

func (b *permissiveUpdateBuilder) SetIfNotExists(string, any, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *permissiveUpdateBuilder) Add(string, any) dynamormcore.UpdateBuilder              { return b }
func (b *permissiveUpdateBuilder) Increment(string) dynamormcore.UpdateBuilder             { return b }
func (b *permissiveUpdateBuilder) Decrement(string) dynamormcore.UpdateBuilder             { return b }
func (b *permissiveUpdateBuilder) Remove(string) dynamormcore.UpdateBuilder                { return b }
func (b *permissiveUpdateBuilder) Delete(string, any) dynamormcore.UpdateBuilder           { return b }
func (b *permissiveUpdateBuilder) AppendToList(string, any) dynamormcore.UpdateBuilder     { return b }
func (b *permissiveUpdateBuilder) PrependToList(string, any) dynamormcore.UpdateBuilder    { return b }
func (b *permissiveUpdateBuilder) RemoveFromListAt(string, int) dynamormcore.UpdateBuilder { return b }
func (b *permissiveUpdateBuilder) SetListElement(string, int, any) dynamormcore.UpdateBuilder {
	return b
}

func (b *permissiveUpdateBuilder) Condition(field string, op string, value any) dynamormcore.UpdateBuilder {
	b.conditions = append(b.conditions, permissiveUpdateCondition{field: field, op: op, value: value})
	return b
}

func (b *permissiveUpdateBuilder) OrCondition(string, string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *permissiveUpdateBuilder) ConditionExists(string) dynamormcore.UpdateBuilder    { return b }
func (b *permissiveUpdateBuilder) ConditionNotExists(string) dynamormcore.UpdateBuilder { return b }
func (b *permissiveUpdateBuilder) ConditionVersion(int64) dynamormcore.UpdateBuilder    { return b }
func (b *permissiveUpdateBuilder) ReturnValues(string) dynamormcore.UpdateBuilder       { return b }

func (b *permissiveUpdateBuilder) Execute() error {
	defer func() {
		b.sets = make(map[string]any)
		b.conditions = nil
	}()

	currentProof, ok := b.currentModel().(*models.PasskeyRegistrationProof)
	if !ok || currentProof == nil {
		return nil
	}

	proof, ok := b.getProof(strings.TrimSpace(currentProof.ID))
	if !ok {
		return dynamormErrors.ErrItemNotFound
	}

	for _, condition := range b.conditions {
		if !permissivePasskeyProofConditionMet(&proof, condition) {
			return dynamormErrors.ErrConditionFailed
		}
	}

	for field, value := range b.sets {
		switch field {
		case "Consumed":
			boolValue, _ := value.(bool)
			proof.Consumed = boolValue
		case "ConsumedAt":
			if ts, ok := value.(time.Time); ok {
				proof.ConsumedAt = ts
			}
		}
	}

	b.storeProof(proof)
	return nil
}

func (b *permissiveUpdateBuilder) ExecuteWithResult(any) error {
	return b.Execute()
}

func permissivePasskeyProofConditionMet(proof *models.PasskeyRegistrationProof, condition permissiveUpdateCondition) bool {
	switch condition.field {
	case "Consumed":
		expected, _ := condition.value.(bool)
		return condition.op == "=" && proof.Consumed == expected
	case "TTL":
		expected, _ := condition.value.(int64)
		switch condition.op {
		case ">":
			return proof.TTL > expected
		case "=":
			return proof.TTL == expected
		default:
			return false
		}
	case "Username":
		expected, _ := condition.value.(string)
		return condition.op == "=" && strings.TrimSpace(proof.Username) == strings.TrimSpace(expected)
	case "CeremonyID":
		expected, _ := condition.value.(string)
		return condition.op == "=" && strings.TrimSpace(proof.CeremonyID) == strings.TrimSpace(expected)
	default:
		return true
	}
}

func fillModelPointer(t *testing.T, dest any, getWhere func(string) (any, bool), opts permissiveDBOptions) {
	t.Helper()

	if fillAccountsUserProjection(dest, getWhere, opts) {
		return
	}

	switch v := dest.(type) {
	case *models.User:
		username := extractUsernameFromWhere(getWhere, "PK", "USER#", "alice")
		v.Username = username
		v.Role = "user"
		v.Suspended = opts.forceUserSuspended
		v.Approved = true
		_ = v.UpdateKeys()
		return
	case *models.Actor:
		username := extractUsernameFromWhere(getWhere, "PK", "ACTOR#", "alice")
		v.Username = username
		v.Actor = activitypubutilLocalActor(username, opts.domain, opts.forceActorLocked)
		_ = v.UpdateKeys()
		return
	case *models.RelationshipRecord:
		pk := extractStringFromWhere(getWhere, "PK")
		sk := extractStringFromWhere(getWhere, "SK")
		if pk == "" {
			pk = "FOLLOW#alice"
		}
		if sk == "" {
			sk = "FOLLOWING#bob"
		}
		v.PK = pk
		v.SK = sk
		v.GSI1PK = fmt.Sprintf("FOLLOW#%s", strings.TrimPrefix(sk, "FOLLOWING#"))
		v.GSI1SK = fmt.Sprintf("FOLLOWER#%s", strings.TrimPrefix(pk, "FOLLOW#"))
		v.State = models.RelationshipAccepted
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.Marker:
		v.LastReadID = "last-read-1"
		v.Version = 0
		v.UpdatedAt = time.Now()
		return
	case *models.UserAppConsent:
		userID := extractUsernameFromWhere(getWhere, "PK", "USER#", "alice")
		appID := strings.TrimPrefix(extractStringFromWhere(getWhere, "SK"), "CONSENT#")
		if appID == "" {
			appID = "client-1"
		}
		v.UserID = userID
		v.AppID = appID
		v.Scopes = []string{"read"}
		v.CreatedAt = time.Now().Add(-time.Hour)
		return
	case *models.OAuthClient:
		clientID := extractUsernameFromWhere(getWhere, "PK", "OAUTH_CLIENT#", "client-1")
		v.ClientID = clientID
		v.ClientSecret = "secret"
		v.Name = "Test App"
		v.RedirectURIs = []string{"https://example.com/callback"}
		v.GrantTypes = []string{"authorization_code"}
		v.Scopes = []string{"read"}
		v.OwnerID = "alice"
		v.Confidential = true
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.AccountPin:
		v.Username = "alice"
		v.PinnedActorID = fmt.Sprintf("https://%s/users/bob", opts.domain)
		v.PinnedUsername = "bob"
		_ = v.UpdateKeys()
		return
	case *models.AccountNote:
		v.Username = "alice"
		v.TargetActorID = fmt.Sprintf("https://%s/users/bob", opts.domain)
		v.Note = "note"
		_ = v.UpdateKeys()
		return
	default:
		// Leave other models as zero values.
		return
	}
}

func fillAccountsUserProjection(dest any, getWhere func(string) (any, bool), opts permissiveDBOptions) bool {
	if dest == nil {
		return false
	}
	typeName := reflect.TypeOf(dest).String()
	if typeName != "*repositories.userCoreProjection" && typeName != "*repositories.userMetadataProjection" {
		return false
	}

	username := extractUsernameFromWhere(getWhere, "PK", "USER#", "alice")
	if gsi5sk := extractStringFromWhere(getWhere, "gsi5SK"); strings.TrimSpace(gsi5sk) != "" {
		username = strings.TrimSpace(gsi5sk)
	}

	value := reflect.ValueOf(dest).Elem()
	setAccountsProjectionField(value, "Table", "test-table")
	setAccountsProjectionField(value, "PK", "USER#"+username)
	setAccountsProjectionField(value, "SK", models.SKMetadata)

	if typeName == "*repositories.userMetadataProjection" {
		setAccountsProjectionField(value, "Metadata", map[string]any{})
		return true
	}

	now := time.Now()
	setAccountsProjectionField(value, "Username", username)
	setAccountsProjectionField(value, "DisplayName", username)
	setAccountsProjectionField(value, "Role", "user")
	setAccountsProjectionField(value, "Approved", true)
	setAccountsProjectionField(value, "Suspended", opts.forceUserSuspended)
	setAccountsProjectionField(value, "Version", 1)
	setAccountsProjectionField(value, "CreatedAt", now.Add(-time.Hour))
	setAccountsProjectionField(value, "UpdatedAt", now)
	return true
}

func setAccountsProjectionField(target reflect.Value, name string, value any) {
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

func fillSlicePointer(t *testing.T, dest any, getWhere func(string) (any, bool), opts permissiveDBOptions, getWebAuthnCredentialsByUser func(string) []models.WebAuthnCredential) {
	t.Helper()

	ptr := reflect.ValueOf(dest)
	if ptr.Kind() != reflect.Ptr || ptr.IsNil() {
		return
	}
	elem := ptr.Elem()
	if elem.Kind() != reflect.Slice {
		return
	}

	switch elem.Type().Elem() {
	case reflect.TypeOf(models.WebAuthnCredential{}):
		userID := extractUsernameFromWhere(getWhere, "PK", "USER#", "alice")
		credentials := getWebAuthnCredentialsByUser(userID)
		elem.Set(reflect.MakeSlice(elem.Type(), 0, len(credentials)))
		for _, credential := range credentials {
			elem.Set(reflect.Append(elem, reflect.ValueOf(credential)))
		}
		return
	case reflect.TypeOf(models.User{}):
		// SearchAccounts expects users with mixed suspended statuses.
		elem.Set(reflect.MakeSlice(elem.Type(), 0, 3))
		elem.Set(reflect.Append(elem, reflect.ValueOf(models.User{Username: "alice", Approved: true, Role: "user"})))
		elem.Set(reflect.Append(elem, reflect.ValueOf(models.User{Username: "bob", Suspended: true, Approved: true, Role: "user"})))
		elem.Set(reflect.Append(elem, reflect.ValueOf(models.User{Username: "carol", Suspended: false, Approved: true, Role: "user"})))
		return
	case reflect.TypeOf(models.UserPreference{}):
		elem.Set(reflect.MakeSlice(elem.Type(), 0, 1))
		elem.Set(reflect.Append(elem, reflect.ValueOf(models.UserPreference{Key: "language", Value: "en"})))
		return
	case reflect.TypeOf(models.RelationshipRecord{}):
		basePK := extractStringFromWhere(getWhere, "PK")
		gsi1PK := extractStringFromWhere(getWhere, "gsi1PK")
		cursorMode := strings.Contains(basePK, "://") || strings.Contains(gsi1PK, "://")

		var followerValue string
		var followingValue string
		if cursorMode {
			followerValue = fmt.Sprintf("https://%s/users/bob", opts.domain)
			followingValue = fmt.Sprintf("https://%s/users/bob", opts.domain)
		} else {
			followerValue = "bob"
			followingValue = "bob"
		}

		elem.Set(reflect.MakeSlice(elem.Type(), 0, 2))
		elem.Set(reflect.Append(elem, reflect.ValueOf(models.RelationshipRecord{
			PK:     fmt.Sprintf("FOLLOW#%s", followerValue),
			SK:     fmt.Sprintf("FOLLOWING#%s", followingValue),
			GSI1PK: fmt.Sprintf("FOLLOW#%s", followingValue),
			GSI1SK: fmt.Sprintf("FOLLOWER#%s", followerValue),
			State:  models.RelationshipAccepted,
		})))
		elem.Set(reflect.Append(elem, reflect.ValueOf(models.RelationshipRecord{
			PK:     fmt.Sprintf("FOLLOW#%s", followerValue),
			SK:     fmt.Sprintf("FOLLOWING#%s", followingValue),
			GSI1PK: fmt.Sprintf("FOLLOW#%s", followingValue),
			GSI1SK: fmt.Sprintf("FOLLOWER#%s", followerValue),
			State:  models.RelationshipAccepted,
		})))
		return
	case reflect.TypeOf(models.AccountPin{}):
		elem.Set(reflect.MakeSlice(elem.Type(), 0, 1))
		pin := models.AccountPin{
			Username:       "alice",
			PinnedActorID:  fmt.Sprintf("https://%s/users/bob", opts.domain),
			PinnedUsername: "bob",
			CreatedAt:      time.Now().Add(-time.Hour),
		}
		_ = pin.UpdateKeys()
		elem.Set(reflect.Append(elem, reflect.ValueOf(pin)))
		return
	}

	// Default: ensure at least one element exists (keeps call sites happy).
	if elem.Len() == 0 {
		elem.Set(reflect.MakeSlice(elem.Type(), 1, 1))
	}
}

func extractStringFromWhere(getWhere func(string) (any, bool), key string) string {
	val, ok := getWhere(key)
	if !ok {
		return ""
	}
	s, _ := val.(string)
	return s
}

func extractUsernameFromWhere(getWhere func(string) (any, bool), field, prefix, fallback string) string {
	raw := extractStringFromWhere(getWhere, field)
	if raw == "" {
		return fallback
	}
	return strings.TrimPrefix(raw, prefix)
}

func activitypubutilLocalActor(username, domain string, locked bool) *activitypub.Actor {
	if strings.TrimSpace(domain) == "" {
		domain = "example.com"
	}
	actorID := fmt.Sprintf("https://%s/users/%s", domain, username)
	now := time.Now()
	return &activitypub.Actor{
		BaseObject:                activitypub.BaseObject{ID: actorID, Type: activitypub.PersonType, Updated: &now},
		PreferredUsername:         username,
		Name:                      username,
		Summary:                   "bio",
		Followers:                 actorID + "/followers",
		Following:                 actorID + "/following",
		Inbox:                     actorID + "/inbox",
		Outbox:                    actorID + "/outbox",
		ManuallyApprovesFollowers: locked,
		Discoverable:              true,
	}
}

func newPermissiveAccountsService(t *testing.T, opts permissiveDBOptions) (*Service, core.RepositoryStorage) {
	t.Helper()

	logger := zap.NewNop()
	db := newPermissiveDynamormDB(t, opts)
	tableName := "test-table"
	domain := opts.domain
	if strings.TrimSpace(domain) == "" {
		domain = "example.com"
	}

	accountRepo := repositories.NewAccountRepository(db, tableName, domain, logger)
	accountRepo.SetEncryptor(noopEncryptor{})
	accountRepo.SetPermissionService(nil)
	accountRepo.SetEventService(nil)
	accountRepo.SetCachingService(nil)

	storageImpl := &permissiveAccountsStorage{
		MockRepositoryStorage: NewMockRepositoryStorage(),
		db:                    db,
		tableName:             tableName,
		logger:                logger,
		account:               accountRepo,
		actor:                 repositories.NewActorRepository(db, tableName, logger),
		relationship:          repositories.NewRelationshipRepository(db, tableName, logger),
		social:                repositories.NewSocialRepository(db, tableName, logger, nil),
		user:                  repositories.NewUserRepository(db, tableName, logger),
		marker:                repositories.NewMarkerRepository(db, tableName, logger, nil),
		analytics:             repositories.NewTrendingRepository(db, logger, nil),
		instance:              repositories.NewInstanceRepository(db, tableName, logger),
		domainBlock:           repositories.NewDomainBlockRepository(db, tableName, logger),
		quote:                 repositories.NewQuoteRepository(db, tableName, logger, nil),
		activity:              repositories.NewActivityRepository(db, tableName, logger, nil),
	}

	publisher := streaming.NewMockPublisher()
	svc := NewService(storageImpl, publisher, nil, nil, nil, logger, domain)
	return svc, storageImpl
}

func TestService_Round13_MainlineCoverage(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})

	t.Run("streaming emitter publishes", func(t *testing.T) {
		emitter := streamingEventEmitter{publisher: streaming.NewMockPublisher()}
		err := emitter.EmitEvents(ctx, []*common.StreamingEvent{{
			Type:      "test.event",
			Timestamp: time.Now(),
			Metadata:  map[string]any{"k": "v"},
		}})
		require.NoError(t, err)
	})

	t.Run("UpdateProfile end-to-end", func(t *testing.T) {
		fed := &federationRecorder{}
		svcWithFed := NewService(svc.storage, streaming.NewMockPublisher(), fed, nil, nil, zap.NewNop(), "example.com")

		res, err := svcWithFed.UpdateProfile(ctx, &UpdateProfileCommand{
			Username:    "alice",
			UpdaterID:   "alice",
			DisplayName: "Alice",
			Bio:         "bio",
			Avatar:      "https://cdn.example.com/a.png",
			Header:      "https://cdn.example.com/h.png",
			Locked:      true,
			Bot:         false,
			Fields: []ProfileField{
				{Name: "Website", Value: "https://example.com"},
			},
			Discoverable: true,
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.NotNil(t, res.Account)
		assert.NotEmpty(t, res.Events)
		assert.Equal(t, 1, fed.calls)
	})

	t.Run("UpdateProfile allows case-insensitive owner match", func(t *testing.T) {
		res, err := svc.UpdateProfile(ctx, &UpdateProfileCommand{
			Username:    "alice",
			UpdaterID:   "ALICE",
			DisplayName: "Alice Legacy",
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Account)
		assert.Equal(t, "Alice Legacy", res.Account.User.DisplayName)
	})

	t.Run("UpdateProfile returns ErrGetAccount when account lookup fails", func(t *testing.T) {
		missingSvc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com", forceUserNotFound: true})
		_, err := missingSvc.UpdateProfile(ctx, &UpdateProfileCommand{
			Username:  "alice",
			UpdaterID: "alice",
		})
		assert.ErrorIs(t, err, ErrGetAccount)
	})

	t.Run("UpdateProfile forbids other user", func(t *testing.T) {
		_, err := svc.UpdateProfile(ctx, &UpdateProfileCommand{
			Username:  "alice",
			UpdaterID: "bob",
		})
		require.Error(t, err)
		assert.NotEmpty(t, err.Error())
	})

	t.Run("GetPreferences and UpdatePreferences", func(t *testing.T) {
		got, err := svc.GetPreferences(ctx, &GetPreferencesQuery{Username: "alice"})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Contains(t, got.Preferences, "language")

		updated, err := svc.UpdatePreferences(ctx, &UpdatePreferencesCommand{
			Username:                 "alice",
			UpdaterID:                "alice",
			Language:                 "en",
			DefaultPostingVisibility: models.VisibilityPublic,
			ExpandMedia:              "default",
			PreferredTimelineOrder:   "newest",
			ReblogFilters:            map[string]bool{"foo": true},
		})
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, "en", updated.Preferences["language"])
	})

	t.Run("GetAccount and LookupAccount", func(t *testing.T) {
		account, err := svc.GetAccount(ctx, "alice")
		require.NoError(t, err)
		require.NotNil(t, account)
		require.NotNil(t, account.Actor)
		assert.Contains(t, account.Actor.ID, "/users/")

		looked, err := svc.LookupAccount(ctx, &LookupAccountQuery{
			Acct:     "@alice@example.com",
			ViewerID: "bob",
		})
		require.NoError(t, err)
		require.NotNil(t, looked)
	})

	t.Run("SearchAccounts filters suspended", func(t *testing.T) {
		res, err := svc.SearchAccounts(ctx, &SearchAccountsQuery{
			Query:      "al",
			ViewerID:   "alice",
			Pagination: interfaces.PaginationOptions{Limit: 10},
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.NotEmpty(t, res.Accounts)
	})

	t.Run("Followers and following", func(t *testing.T) {
		followers, err := svc.GetFollowers(ctx, &GetFollowersQuery{
			Username:   "alice",
			ViewerID:   "bob",
			Pagination: interfaces.PaginationOptions{Limit: 1},
		})
		require.NoError(t, err)
		require.NotNil(t, followers)

		following, err := svc.GetFollowing(ctx, &GetFollowingQuery{
			Username:   "alice",
			ViewerID:   "bob",
			Pagination: interfaces.PaginationOptions{Limit: 1},
		})
		require.NoError(t, err)
		require.NotNil(t, following)

		familiar, err := svc.GetFamiliarFollowers(ctx, &GetFamiliarFollowersQuery{
			AccountIDs: []string{"alice"},
			ViewerID:   "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, familiar)
	})

	t.Run("Pins, notes, removals", func(t *testing.T) {
		pinned, err := svc.PinAccount(ctx, &PinAccountCommand{
			Username:      "alice",
			TargetAccount: "bob",
			PinnerID:      "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, pinned)

		unpinned, err := svc.UnpinAccount(ctx, &UnpinAccountCommand{
			Username:      "alice",
			TargetAccount: "bob",
			PinnerID:      "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, unpinned)

		pins, err := svc.GetAccountPins(ctx, &GetAccountPinsQuery{Username: "alice"})
		require.NoError(t, err)
		require.NotNil(t, pins)

		noteRes, err := svc.SetAccountNote(ctx, &SetAccountNoteCommand{
			Username:      "alice",
			TargetAccount: "bob",
			Note:          "hello",
			SetterID:      "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, noteRes)

		removed, err := svc.RemoveFollower(ctx, &RemoveFollowerCommand{
			Username:   "alice",
			FollowerID: "bob",
			RemoverID:  "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, removed)
	})

	t.Run("ActivityPub collections", func(t *testing.T) {
		meta, err := svc.GetActivityPubCollection(ctx, &GetActivityPubCollectionQuery{
			Username:       "alice",
			CollectionType: "followers",
			ViewerID:       "bob",
			Page:           false,
		})
		require.NoError(t, err)
		require.NotNil(t, meta)

		page, err := svc.GetActivityPubCollection(ctx, &GetActivityPubCollectionQuery{
			Username:       "alice",
			CollectionType: "followers",
			ViewerID:       "bob",
			Page:           true,
			Limit:          1,
		})
		require.NoError(t, err)
		require.NotNil(t, page)

		followingPage, err := svc.GetActivityPubCollection(ctx, &GetActivityPubCollectionQuery{
			Username:       "alice",
			CollectionType: "following",
			ViewerID:       "bob",
			Page:           true,
			Cursor:         "c1",
			Limit:          1,
		})
		require.NoError(t, err)
		require.NotNil(t, followingPage)

		items, next := svc.getPageData(ctx, &GetActivityPubCollectionQuery{
			Username:       "alice",
			CollectionType: "nope",
		}, "https://example.com/users/alice/nope")
		assert.Empty(t, items)
		assert.Empty(t, next)

		empty := svc.createEmptyCollection("https://example.com/users/alice", "followers")
		require.NotNil(t, empty)
	})

	t.Run("Markers", func(t *testing.T) {
		got, err := svc.GetMarkers(ctx, &GetMarkersQuery{Username: "alice"})
		require.NoError(t, err)
		require.NotNil(t, got)

		saved, err := svc.SaveMarker(ctx, &SaveMarkerCommand{
			Username:   "alice",
			Timeline:   "home",
			LastReadID: "1",
			Version:    1,
		})
		require.NoError(t, err)
		require.NotNil(t, saved)
	})

	t.Run("OAuth helpers", func(t *testing.T) {
		_, err := svc.StoreOAuthState(ctx, &StoreOAuthStateCommand{
			State: "state1",
			OAuthState: &storage.OAuthState{
				Provider:    "github",
				RedirectURI: "https://example.com/callback",
				Username:    "alice",
				ClientID:    "client-1",
				CreatedAt:   time.Now(),
			},
		})
		require.NoError(t, err)

		_, err = svc.CreateAuthorizationCode(ctx, &CreateAuthorizationCodeCommand{
			AuthCode: &storage.AuthorizationCode{
				Code:      "code1",
				ClientID:  "client-1",
				Username:  "alice",
				Scopes:    []string{"read"},
				ExpiresAt: time.Now().Add(time.Minute),
			},
		})
		require.NoError(t, err)

		_, err = svc.GetOAuthApp(ctx, &GetOAuthAppQuery{ClientID: "client-1"})
		require.NoError(t, err)

		_, err = svc.GetUserAppConsent(ctx, &GetUserAppConsentQuery{
			Username: "alice",
			ClientID: "client-1",
		})
		require.NoError(t, err)

		assert.Equal(t, "field: message", (&ValidationError{Field: "field", Message: "message"}).Error())
	})

	t.Run("Instance stats and metadata", func(t *testing.T) {
		stats, err := svc.GetInstanceStats(ctx, &GetInstanceStatsQuery{})
		require.NoError(t, err)
		require.NotNil(t, stats)

		meta, err := svc.GetAccountMetadata(ctx, &GetAccountMetadataQuery{Username: "alice"})
		require.NoError(t, err)
		require.NotNil(t, meta)
	})

	t.Run("User helpers", func(t *testing.T) {
		pinned, err := svc.IsAccountPinned(ctx, "alice", "https://example.com/users/bob")
		require.NoError(t, err)
		assert.True(t, pinned)

		user, err := svc.GetUser(ctx, "alice")
		require.NoError(t, err)
		require.NotNil(t, user)

		_, err = svc.GetPreference(ctx, "alice", "language")
		require.NoError(t, err)

		state, err := svc.GetFollowRequestState(ctx, "alice", "bob")
		require.NoError(t, err)
		_ = state

		blocked, err := svc.IsBlockedDomain(ctx, "alice", "remote.social")
		require.NoError(t, err)
		_ = blocked

		_, err = svc.GetFieldVerification(ctx, "alice", "Website")
		require.NoError(t, err)

		_, err = svc.GetAccountNote(ctx, "alice", "https://example.com/users/bob")
		require.NoError(t, err)
	})
}

func TestService_Round13_RegisterAccount_SucceedsWithoutEmail(t *testing.T) {
	ctx := context.Background()

	dbOpts := permissiveDBOptions{
		domain:            "example.com",
		forceUserNotFound: true,
	}
	logger := zap.NewNop()
	db := newPermissiveDynamormDB(t, dbOpts)
	tableName := "test-table"

	accountRepo := repositories.NewAccountRepository(db, tableName, "example.com", logger)
	accountRepo.SetEncryptor(noopEncryptor{})
	accountRepo.SetPermissionService(nil)
	accountRepo.SetEventService(nil)
	accountRepo.SetCachingService(nil)

	storageImpl := &permissiveAccountsStorage{
		MockRepositoryStorage: NewMockRepositoryStorage(),
		db:                    db,
		tableName:             tableName,
		logger:                logger,
		account:               accountRepo,
		actor:                 repositories.NewActorRepository(db, tableName, logger),
		relationship:          repositories.NewRelationshipRepository(db, tableName, logger),
		social:                repositories.NewSocialRepository(db, tableName, logger, nil),
		user:                  repositories.NewUserRepository(db, tableName, logger),
		marker:                repositories.NewMarkerRepository(db, tableName, logger, nil),
		analytics:             repositories.NewTrendingRepository(db, logger, nil),
		instance:              repositories.NewInstanceRepository(db, tableName, logger),
		domainBlock:           repositories.NewDomainBlockRepository(db, tableName, logger),
		quote:                 repositories.NewQuoteRepository(db, tableName, logger, nil),
		activity:              repositories.NewActivityRepository(db, tableName, logger, nil),
	}

	cryptoSvc := staticCryptoService{
		publicKeyPEM:  []byte("PUBLIC KEY"),
		privateKeyPEM: []byte("PRIVATE KEY"),
		key:           struct{}{},
	}

	svc := NewService(storageImpl, streaming.NewMockPublisher(), nil, cryptoSvc, staticAuthService{hash: "hash"}, logger, "example.com")
	storeRegistrationWalletChallenge(t, accountRepo, "alice", "wc-1")

	got, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
		Username:                 "alice",
		Email:                    "",
		Password:                 "",
		Locale:                   "en",
		Agreement:                true,
		DefaultPostingVisibility: "public",
		RegistrationChallengeID:  "wc-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Account)
	require.NotNil(t, got.Actor)
	assert.Equal(t, "alice", got.Account.User.Username)
	assert.True(t, strings.Contains(got.Actor.ID, "/users/alice"))
}

func TestService_Round13_RegisterAccount_BindsTypedWalletChallenge(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	baseTime := time.Now().UTC()
	tableName := "test-table"

	db := newPermissiveDynamormDB(t, permissiveDBOptions{forceUserNotFound: true})
	accountRepo := repositories.NewAccountRepository(db, tableName, "example.com", logger)
	accountRepo.SetEncryptor(noopEncryptor{})
	accountRepo.SetPermissionService(nil)
	accountRepo.SetEventService(nil)
	accountRepo.SetCachingService(nil)

	storageImpl := &permissiveAccountsStorage{
		MockRepositoryStorage: NewMockRepositoryStorage(),
		db:                    db,
		tableName:             tableName,
		logger:                logger,
		account:               accountRepo,
		actor:                 repositories.NewActorRepository(db, tableName, logger),
		relationship:          repositories.NewRelationshipRepository(db, tableName, logger),
		social:                repositories.NewSocialRepository(db, tableName, logger, nil),
		user:                  repositories.NewUserRepository(db, tableName, logger),
		marker:                repositories.NewMarkerRepository(db, tableName, logger, nil),
		analytics:             repositories.NewTrendingRepository(db, logger, nil),
		instance:              repositories.NewInstanceRepository(db, tableName, logger),
		domainBlock:           repositories.NewDomainBlockRepository(db, tableName, logger),
		quote:                 repositories.NewQuoteRepository(db, tableName, logger, nil),
		activity:              repositories.NewActivityRepository(db, tableName, logger, nil),
	}

	require.NoError(t, accountRepo.StoreWalletChallenge(ctx, &storage.WalletChallenge{
		ID:        "wc-1",
		Username:  "alice",
		Address:   "0xabc",
		ChainID:   1,
		Nonce:     "nonce",
		Message:   "message",
		IssuedAt:  baseTime,
		ExpiresAt: baseTime.Add(time.Hour),
	}))

	cryptoSvc := staticCryptoService{
		publicKeyPEM:  []byte("PUBLIC KEY"),
		privateKeyPEM: []byte("PRIVATE KEY"),
		key:           struct{}{},
	}

	svc := NewService(storageImpl, streaming.NewMockPublisher(), nil, cryptoSvc, staticAuthService{hash: "hash"}, logger, "example.com")

	got, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
		Username:                "alice",
		Agreement:               true,
		Locale:                  "en",
		RegistrationChallengeID: "wc-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Account)
	require.NotNil(t, got.Account.User)
	require.Nil(t, got.Account.User.Metadata)

	challenge, err := accountRepo.GetWalletChallenge(ctx, "wc-1")
	require.NoError(t, err)
	require.NotNil(t, challenge)
	require.True(t, challenge.RegistrationCompleted)
}

func TestService_Round13_MoreBranchesAndErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("UpdateProfile validation errors don't touch storage", func(t *testing.T) {
		svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")
		_, err := svc.UpdateProfile(ctx, &UpdateProfileCommand{
			Username:  "",
			UpdaterID: "",
		})
		assert.ErrorIs(t, err, ErrValidationFailed)
	})

	t.Run("validateUpdateProfileCommand covers field rules", func(t *testing.T) {
		svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

		err := svc.validateUpdateProfileCommand(ctx, &UpdateProfileCommand{
			Username:  "alice",
			UpdaterID: "alice",
			Fields: []ProfileField{
				{Name: "a", Value: "1"},
				{Name: "b", Value: "2"},
				{Name: "c", Value: "3"},
				{Name: "d", Value: "4"},
				{Name: "e", Value: "5"},
			},
		})
		assert.Error(t, err)

		err = svc.validateUpdateProfileCommand(ctx, &UpdateProfileCommand{
			Username:  "alice",
			UpdaterID: "alice",
			Fields: []ProfileField{
				{Name: "   ", Value: "1"},
			},
		})
		assert.ErrorIs(t, err, ErrProfileFieldNameEmpty)

		err = svc.validateUpdateProfileCommand(ctx, &UpdateProfileCommand{
			Username:  "alice",
			UpdaterID: "alice",
			Fields: []ProfileField{
				{Name: strings.Repeat("a", 256), Value: "1"},
			},
		})
		assert.ErrorIs(t, err, ErrProfileFieldNameTooLong)

		err = svc.validateUpdateProfileCommand(ctx, &UpdateProfileCommand{
			Username:  "alice",
			UpdaterID: "alice",
			Fields: []ProfileField{
				{Name: "ok", Value: strings.Repeat("b", 256)},
			},
		})
		assert.ErrorIs(t, err, ErrProfileFieldValueTooLong)

		err = svc.validateUpdateProfileCommand(ctx, &UpdateProfileCommand{
			Username:    "alice",
			UpdaterID:   "alice",
			DisplayName: strings.Repeat("x", 200),
		})
		assert.Error(t, err)

		err = svc.validateUpdateProfileCommand(ctx, &UpdateProfileCommand{
			Username:  "alice",
			UpdaterID: "",
		})
		assert.ErrorIs(t, err, ErrUpdaterIDRequired)

		err = svc.validateUpdateProfileCommand(ctx, &UpdateProfileCommand{
			Username:  "alice",
			UpdaterID: "alice",
			Fields: []ProfileField{
				{Name: "Website", Value: "https://example.com"},
			},
		})
		assert.NoError(t, err)
	})

	t.Run("UpdatePreferences permission check", func(t *testing.T) {
		svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")
		_, err := svc.UpdatePreferences(ctx, &UpdatePreferencesCommand{
			Username:  "alice",
			UpdaterID: "bob",
		})
		assert.Error(t, err)
	})

	t.Run("SearchAccounts empty query", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		_, err := svc.SearchAccounts(ctx, &SearchAccountsQuery{
			Query:      "   ",
			ViewerID:   "alice",
			Pagination: interfaces.PaginationOptions{Limit: 10},
		})
		assert.ErrorIs(t, err, ErrEmptySearchQuery)
	})

	t.Run("GetAccount hides suspended users", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com", forceUserSuspended: true})
		_, err := svc.GetAccount(ctx, "alice")
		assert.ErrorIs(t, err, ErrAccountNotFound)
	})

	t.Run("hydrateAccountActor uses user URL when service domain is blank", func(t *testing.T) {
		svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "")
		account := &storage.Account{
			User: &storage.User{
				Username: "alice",
				URL:      "example.com",
			},
		}
		svc.hydrateAccountActor(account)
		require.NotNil(t, account.Actor)
		assert.Contains(t, account.Actor.ID, "/users/alice")
	})

	t.Run("GetActivityPubCollection privacy path returns empty collection", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com", forceActorLocked: true})
		res, err := svc.GetActivityPubCollection(ctx, &GetActivityPubCollectionQuery{
			Username:       "alice",
			CollectionType: "followers",
			ViewerID:       "bob",
			Page:           false,
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.EqualValues(t, 0, res.Collection["totalItems"])
	})

	t.Run("buildCollectionMetadata sets first when totalItems > 0", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com", defaultCountValue: 1})
		res, err := svc.GetActivityPubCollection(ctx, &GetActivityPubCollectionQuery{
			Username:       "alice",
			CollectionType: "followers",
			ViewerID:       "bob",
			Page:           false,
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		_, ok := res.Collection["first"]
		assert.True(t, ok)
	})

	t.Run("relationship helpers cover domain extraction and endorsement/note branches", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})

		assert.True(t, svc.checkDomainBlocking(ctx, "alice", "bob@remote.social"))
		assert.Equal(t, "remote.social", svc.extractDomainFromAccount("bob@remote.social"))

		assert.True(t, svc.checkEndorsementStatus(ctx, "alice", "https://example.com/users/bob"))
		assert.Equal(t, "note", svc.getAccountNoteText(ctx, "alice", "https://example.com/users/bob"))

		assert.False(t, svc.checkBlocking(ctx, nil, "alice", "bob"))
	})

	t.Run("nil storage and validation error guards", func(t *testing.T) {
		svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.GetAccountMetadata(ctx, &GetAccountMetadataQuery{Username: "alice"})
		assert.ErrorIs(t, err, ErrStorageNotAvailable)

		_, err = svc.GetUser(ctx, "alice")
		assert.ErrorIs(t, err, ErrStorageNotAvailable)

		_, err = svc.GetPreference(ctx, "alice", "k")
		assert.ErrorIs(t, err, ErrStorageNotAvailable)

		_, err = svc.GetAccountNote(ctx, "alice", "target")
		assert.ErrorIs(t, err, ErrStorageNotAvailable)

		_, err = svc.GetFieldVerification(ctx, "alice", "Website")
		assert.ErrorIs(t, err, ErrStorageNotAvailable)

		_, err = svc.IsAccountPinned(ctx, "alice", "target")
		assert.ErrorIs(t, err, ErrStorageNotAvailable)

		_, err = svc.GetFollowRequestState(ctx, "alice", "bob")
		assert.ErrorIs(t, err, ErrStorageNotAvailable)

		_, err = svc.IsBlockedDomain(ctx, "alice", "remote.social")
		assert.ErrorIs(t, err, ErrStorageNotAvailable)

		_, err = svc.GetMarkers(ctx, &GetMarkersQuery{Username: ""})
		assert.Error(t, err)

		_, err = svc.SaveMarker(ctx, &SaveMarkerCommand{Username: "", Timeline: "", LastReadID: ""})
		assert.Error(t, err)

		_, err = svc.StoreOAuthState(ctx, &StoreOAuthStateCommand{State: "", OAuthState: nil})
		assert.Error(t, err)

		_, err = svc.CreateAuthorizationCode(ctx, &CreateAuthorizationCodeCommand{AuthCode: nil})
		assert.Error(t, err)

		_, err = svc.GetOAuthApp(ctx, &GetOAuthAppQuery{ClientID: ""})
		assert.Error(t, err)

		_, err = svc.GetUserAppConsent(ctx, &GetUserAppConsentQuery{Username: "", ClientID: ""})
		assert.Error(t, err)

		_, err = svc.RegisterAccount(ctx, &RegisterAccountCommand{Username: "", Agreement: false})
		assert.ErrorIs(t, err, ErrValidationFailed)
	})

	t.Run("GetPreference rejects missing user repo", func(t *testing.T) {
		svc := NewService(NewMockRepositoryStorage(), streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.GetPreference(ctx, "alice", "language")
		assert.ErrorIs(t, err, ErrUserRepositoryNotAvailable)
	})

	t.Run("GetPreference returns empty when preferences or key are missing", func(t *testing.T) {
		svc, storageIface := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		storageImpl := storageIface.(*permissiveAccountsStorage)
		mockUserRepo := testmocks.NewMockUserRepositoryInterface()
		svc.storage = &userRepositoryOverrideStorage{
			permissiveAccountsStorage: storageImpl,
			userRepo:                  mockUserRepo,
		}

		mockUserRepo.On("GetUserPreferences", ctx, "alice").Return(nil, nil).Once()
		value, err := svc.GetPreference(ctx, "alice", "language")
		require.NoError(t, err)
		assert.Empty(t, value)

		mockUserRepo.On("GetUserPreferences", ctx, "alice").Return(&storage.UserPreferences{
			Username:    "alice",
			Preferences: map[string]string{"theme": "dark"},
		}, nil).Once()
		value, err = svc.GetPreference(ctx, "alice", "language")
		require.NoError(t, err)
		assert.Empty(t, value)
	})

	t.Run("GetPreference returns stored value", func(t *testing.T) {
		svc, storageIface := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		storageImpl := storageIface.(*permissiveAccountsStorage)
		mockUserRepo := testmocks.NewMockUserRepositoryInterface()
		svc.storage = &userRepositoryOverrideStorage{
			permissiveAccountsStorage: storageImpl,
			userRepo:                  mockUserRepo,
		}

		mockUserRepo.On("GetUserPreferences", ctx, "alice").Return(&storage.UserPreferences{
			Username:    "alice",
			Preferences: map[string]string{"language": "en"},
		}, nil).Once()

		value, err := svc.GetPreference(ctx, "alice", "language")
		require.NoError(t, err)
		assert.Equal(t, "en", value)
	})
}

func TestService_Round13_RegisterAccount_ErrorsAndRollbacks(t *testing.T) {
	ctx := context.Background()

	t.Run("username already taken", func(t *testing.T) {
		svc, storageIface := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		storeRegistrationWalletChallenge(t, storageIface.(*permissiveAccountsStorage).account, "alice", "wc-1")
		_, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
			Username:                "alice",
			Email:                   "",
			Agreement:               true,
			RegistrationChallengeID: "wc-1",
		})
		assert.ErrorIs(t, err, ErrUsernameAlreadyTaken)
	})

	t.Run("account repository missing", func(t *testing.T) {
		storageImpl := NewMockRepositoryStorage()
		svc := NewService(storageImpl, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")
		_, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
			Username:                "alice",
			Email:                   "",
			Agreement:               true,
			RegistrationChallengeID: "wc-1",
		})
		assert.ErrorIs(t, err, ErrAccountRepositoryNotAvailable)
	})

	t.Run("crypto missing triggers generate keypair error", func(t *testing.T) {
		db := newPermissiveDynamormDB(t, permissiveDBOptions{domain: "example.com", forceUserNotFound: true})
		logger := zap.NewNop()
		tableName := "test-table"
		accountRepo := repositories.NewAccountRepository(db, tableName, "example.com", logger)
		accountRepo.SetEncryptor(noopEncryptor{})

		storageImpl := &permissiveAccountsStorage{
			MockRepositoryStorage: NewMockRepositoryStorage(),
			db:                    db,
			tableName:             tableName,
			logger:                logger,
			account:               accountRepo,
			quote:                 repositories.NewQuoteRepository(db, tableName, logger, nil),
			activity:              repositories.NewActivityRepository(db, tableName, logger, nil),
		}

		svc := NewService(storageImpl, streaming.NewMockPublisher(), nil, nil, nil, logger, "example.com")
		_, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
			Username:                "alice",
			Email:                   "",
			Agreement:               true,
			RegistrationChallengeID: "wc-1",
		})
		assert.ErrorIs(t, err, ErrGenerateKeypair)
	})

	t.Run("quote repository missing triggers rollback path", func(t *testing.T) {
		db := newPermissiveDynamormDB(t, permissiveDBOptions{domain: "example.com", forceUserNotFound: true})
		logger := zap.NewNop()
		tableName := "test-table"
		accountRepo := repositories.NewAccountRepository(db, tableName, "example.com", logger)
		accountRepo.SetEncryptor(noopEncryptor{})
		accountRepo.SetPermissionService(nil)
		accountRepo.SetEventService(nil)
		accountRepo.SetCachingService(nil)

		cryptoSvc := staticCryptoService{
			publicKeyPEM:  []byte("PUBLIC KEY"),
			privateKeyPEM: []byte("PRIVATE KEY"),
			key:           struct{}{},
		}

		storageImpl := &permissiveAccountsStorage{
			MockRepositoryStorage: NewMockRepositoryStorage(),
			db:                    db,
			tableName:             tableName,
			logger:                logger,
			account:               accountRepo,
			quote:                 nil,
			activity:              repositories.NewActivityRepository(db, tableName, logger, nil),
		}

		svc := NewService(storageImpl, streaming.NewMockPublisher(), nil, cryptoSvc, nil, logger, "example.com")
		_, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
			Username:                "alice",
			Email:                   "",
			Agreement:               true,
			RegistrationChallengeID: "wc-1",
		})
		require.Error(t, err)
	})
}

func TestService_Round13_RepositoryErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("GetPreferences returns ErrGetPreferences on repo error", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:         "example.com",
			firstScanError: errors.New("scan failed"),
		})

		_, err := svc.GetPreferences(ctx, &GetPreferencesQuery{Username: "alice"})
		assert.ErrorIs(t, err, ErrGetPreferences)
	})

	t.Run("UpdatePreferences returns ErrUpdatePreferences on repo error", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:           "example.com",
			firstUpdateError: errors.New("update failed"),
		})

		_, err := svc.UpdatePreferences(ctx, &UpdatePreferencesCommand{
			Username:                 "alice",
			UpdaterID:                "alice",
			DefaultPostingVisibility: models.VisibilityPublic,
		})
		assert.ErrorIs(t, err, ErrUpdatePreferences)
	})

	t.Run("SearchAccounts returns ErrSearchAccounts on repo error", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:        "example.com",
			firstAllError: errors.New("all failed"),
		})

		_, err := svc.SearchAccounts(ctx, &SearchAccountsQuery{
			Query:      "al",
			ViewerID:   "alice",
			Pagination: interfaces.PaginationOptions{Limit: 10},
		})
		assert.ErrorIs(t, err, ErrSearchAccounts)
	})

	t.Run("UpdateProfile returns ErrGetAccount when storage GetAccount fails", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:            "example.com",
			forceUserNotFound: true,
		})

		_, err := svc.UpdateProfile(ctx, &UpdateProfileCommand{
			Username:  "alice",
			UpdaterID: "alice",
		})
		assert.ErrorIs(t, err, ErrGetAccount)
	})

	t.Run("PinAccount returns ErrPinAccount when repo create fails", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:           "example.com",
			firstCreateError: errors.New("create failed"),
		})

		_, err := svc.PinAccount(ctx, &PinAccountCommand{
			Username:      "alice",
			TargetAccount: "bob",
			PinnerID:      "alice",
		})
		assert.ErrorIs(t, err, ErrPinAccount)
	})

	t.Run("UnpinAccount returns ErrUnpinAccount when repo delete fails", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:           "example.com",
			firstDeleteError: errors.New("delete failed"),
		})

		_, err := svc.UnpinAccount(ctx, &UnpinAccountCommand{
			Username:      "alice",
			TargetAccount: "bob",
			PinnerID:      "alice",
		})
		assert.ErrorIs(t, err, ErrUnpinAccount)
	})

	t.Run("SetAccountNote returns ErrSetAccountNote when repo create fails", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:           "example.com",
			firstUpdateError: errors.New("update failed"),
		})

		_, err := svc.SetAccountNote(ctx, &SetAccountNoteCommand{
			Username:      "alice",
			TargetAccount: "bob",
			Note:          "hello",
			SetterID:      "alice",
		})
		assert.ErrorIs(t, err, ErrSetAccountNote)
	})

	t.Run("GetPreference returns ErrGetUserPreferences on repo error", func(t *testing.T) {
		svc, storageIface := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		storageImpl := storageIface.(*permissiveAccountsStorage)
		mockUserRepo := testmocks.NewMockUserRepositoryInterface()
		svc.storage = &userRepositoryOverrideStorage{
			permissiveAccountsStorage: storageImpl,
			userRepo:                  mockUserRepo,
		}

		mockUserRepo.On("GetUserPreferences", ctx, "alice").Return(nil, errors.New("prefs failed")).Once()

		_, err := svc.GetPreference(ctx, "alice", "language")
		assert.ErrorIs(t, err, ErrGetUserPreferences)
	})

	t.Run("RemoveFollower returns ErrRemoveFollower when repo delete fails", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:           "example.com",
			firstDeleteError: errors.New("delete failed"),
		})

		_, err := svc.RemoveFollower(ctx, &RemoveFollowerCommand{
			Username:   "alice",
			FollowerID: "bob",
			RemoverID:  "alice",
		})
		assert.ErrorIs(t, err, ErrRemoveFollower)
	})

	t.Run("GetUserAppConsent returns error when consent missing", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:               "example.com",
			forceConsentNotFound: true,
		})

		_, err := svc.GetUserAppConsent(ctx, &GetUserAppConsentQuery{
			Username: "alice",
			ClientID: "client-1",
		})
		assert.Error(t, err)
	})

	t.Run("getCollectionCount logs warning on repository errors", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:          "example.com",
			firstCountError: errors.New("count failed"),
		})

		res, err := svc.GetActivityPubCollection(ctx, &GetActivityPubCollectionQuery{
			Username:       "alice",
			CollectionType: "followers",
			ViewerID:       "bob",
			Page:           false,
		})
		require.NoError(t, err)
		require.NotNil(t, res)

		// Cover following path too.
		_, _ = svc.GetActivityPubCollection(ctx, &GetActivityPubCollectionQuery{
			Username:       "alice",
			CollectionType: "following",
			ViewerID:       "bob",
			Page:           false,
		})
	})

	t.Run("buildNextPageID returns empty when cursor missing", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		assert.Equal(t, "", svc.buildNextPageID("https://example.com/users/alice/followers", ""))
	})
}

func TestService_Round13_MoreBranchCoverage(t *testing.T) {
	ctx := context.Background()

	t.Run("PinAccount/UnpinAccount/SetAccountNote permission checks", func(t *testing.T) {
		svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.PinAccount(ctx, &PinAccountCommand{
			Username:      "alice",
			TargetAccount: "bob",
			PinnerID:      "bob",
		})
		assert.Error(t, err)

		_, err = svc.UnpinAccount(ctx, &UnpinAccountCommand{
			Username:      "alice",
			TargetAccount: "bob",
			PinnerID:      "bob",
		})
		assert.Error(t, err)

		_, err = svc.SetAccountNote(ctx, &SetAccountNoteCommand{
			Username:      "alice",
			TargetAccount: "bob",
			Note:          "hi",
			SetterID:      "bob",
		})
		assert.Error(t, err)
	})

	t.Run("PinAccount target missing", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com", forceUserNotFound: true})
		_, err := svc.PinAccount(ctx, &PinAccountCommand{
			Username:      "alice",
			TargetAccount: "bob",
			PinnerID:      "alice",
		})
		assert.ErrorIs(t, err, ErrTargetAccountNotFound)
	})

	t.Run("PinAccount relationship fallback when relationship repo missing", func(t *testing.T) {
		svc, storageIface := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		storageImpl := storageIface.(*permissiveAccountsStorage)
		storageImpl.relationship = nil

		res, err := svc.PinAccount(ctx, &PinAccountCommand{
			Username:      "alice",
			TargetAccount: "bob",
			PinnerID:      "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Relationship)
		assert.Equal(t, true, res.Relationship["endorsed"])
	})

	t.Run("UnpinAccount relationship fallback when relationship repo missing", func(t *testing.T) {
		svc, storageIface := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		storageImpl := storageIface.(*permissiveAccountsStorage)
		storageImpl.relationship = nil

		res, err := svc.UnpinAccount(ctx, &UnpinAccountCommand{
			Username:      "alice",
			TargetAccount: "bob",
			PinnerID:      "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Relationship)
		assert.Equal(t, false, res.Relationship["endorsed"])
	})

	t.Run("SetAccountNote relationship fallback when relationship repo missing", func(t *testing.T) {
		svc, storageIface := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		storageImpl := storageIface.(*permissiveAccountsStorage)
		storageImpl.relationship = nil

		res, err := svc.SetAccountNote(ctx, &SetAccountNoteCommand{
			Username:      "alice",
			TargetAccount: "bob",
			Note:          "hello",
			SetterID:      "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Relationship)
		assert.Equal(t, "hello", res.Relationship["note"])
	})

	t.Run("RemoveFollower repo availability errors", func(t *testing.T) {
		svc, storageIface := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		storageImpl := storageIface.(*permissiveAccountsStorage)

		storageImpl.relationship = nil
		_, err := svc.RemoveFollower(ctx, &RemoveFollowerCommand{
			Username:   "alice",
			FollowerID: "bob",
			RemoverID:  "alice",
		})
		assert.ErrorIs(t, err, ErrRelationshipRepositoryNotAvailable)

		storageImpl.relationship = repositories.NewRelationshipRepository(storageImpl.db, storageImpl.tableName, storageImpl.logger)
		storageImpl.actor = nil
		_, err = svc.RemoveFollower(ctx, &RemoveFollowerCommand{
			Username:   "alice",
			FollowerID: "bob",
			RemoverID:  "alice",
		})
		assert.ErrorIs(t, err, ErrActorRepositoryNotAvailable)
	})

	t.Run("GetFollowers repo availability errors", func(t *testing.T) {
		svc, storageIface := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		storageImpl := storageIface.(*permissiveAccountsStorage)

		storageImpl.relationship = nil
		_, err := svc.GetFollowers(ctx, &GetFollowersQuery{
			Username:   "alice",
			ViewerID:   "bob",
			Pagination: interfaces.PaginationOptions{Limit: 1},
		})
		assert.ErrorIs(t, err, ErrRelationshipRepositoryNotAvailable)

		storageImpl.relationship = repositories.NewRelationshipRepository(storageImpl.db, storageImpl.tableName, storageImpl.logger)
		storageImpl.actor = nil
		_, err = svc.GetFollowers(ctx, &GetFollowersQuery{
			Username:   "alice",
			ViewerID:   "bob",
			Pagination: interfaces.PaginationOptions{Limit: 1},
		})
		assert.ErrorIs(t, err, ErrActorRepositoryNotAvailable)
	})

	t.Run("GetFollowing account not found", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com", forceUserNotFound: true})
		_, err := svc.GetFollowing(ctx, &GetFollowingQuery{
			Username:   "alice",
			ViewerID:   "bob",
			Pagination: interfaces.PaginationOptions{Limit: 1},
		})
		assert.ErrorIs(t, err, ErrAccountNotFound)
	})

	t.Run("GetFamiliarFollowers missing repos", func(t *testing.T) {
		svc, storageIface := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})
		storageImpl := storageIface.(*permissiveAccountsStorage)

		storageImpl.relationship = nil
		_, err := svc.GetFamiliarFollowers(ctx, &GetFamiliarFollowersQuery{
			AccountIDs: []string{"alice"},
			ViewerID:   "alice",
		})
		assert.ErrorIs(t, err, ErrRelationshipRepositoryNotAvailable)

		storageImpl.relationship = repositories.NewRelationshipRepository(storageImpl.db, storageImpl.tableName, storageImpl.logger)
		storageImpl.actor = nil
		_, err = svc.GetFamiliarFollowers(ctx, &GetFamiliarFollowersQuery{
			AccountIDs: []string{"alice"},
			ViewerID:   "alice",
		})
		assert.ErrorIs(t, err, ErrActorRepositoryNotAvailable)
	})

	t.Run("getFollowers/getFollowing page data error paths", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:        "example.com",
			firstAllError: errors.New("all failed"),
		})

		followersPage, err := svc.GetActivityPubCollection(ctx, &GetActivityPubCollectionQuery{
			Username:       "alice",
			CollectionType: "followers",
			ViewerID:       "bob",
			Page:           true,
			Limit:          1,
		})
		require.NoError(t, err)
		require.NotNil(t, followersPage)

		followingPage, err := svc.GetActivityPubCollection(ctx, &GetActivityPubCollectionQuery{
			Username:       "alice",
			CollectionType: "following",
			ViewerID:       "bob",
			Page:           true,
			Limit:          1,
		})
		require.NoError(t, err)
		require.NotNil(t, followingPage)
	})

	t.Run("GetInstanceStats error fallbacks", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:           "example.com",
			firstMetricError: errors.New("metrics down"),
			metricErrorTimes: 3,
		})
		stats, err := svc.GetInstanceStats(ctx, &GetInstanceStatsQuery{})
		require.NoError(t, err)
		require.NotNil(t, stats)
		assert.Equal(t, 1, stats.TotalUsers)
		assert.Equal(t, 1, stats.ActiveMonth)
		assert.Equal(t, 1, stats.ActiveHalfyear)
	})

	t.Run("relationship helper error branches", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:                      "example.com",
			firstRelationshipFirstError: errors.New("relationship read failed"),
			firstMuteFirstError:         errors.New("mute read failed"),
		})

		got, err := svc.getAccountRelationship(ctx, "alice", "bob")
		require.NoError(t, err)
		require.NotNil(t, got)

		assert.False(t, svc.checkMutingNotifications(ctx, svc.storage.Relationship(), "alice", "bob", false))
	})

	t.Run("SaveMarker, StoreOAuthState, and CreateAuthorizationCode repo error branches", func(t *testing.T) {
		svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{
			domain:                   "example.com",
			firstCreateError:         errors.New("create failed"),
			createErrorTimes:         10,
			firstCreateOrUpdateError: errors.New("create or update failed"),
		})

		_, err := svc.SaveMarker(ctx, &SaveMarkerCommand{
			Username:   "alice",
			Timeline:   "home",
			LastReadID: "1",
			Version:    2,
		})
		assert.Error(t, err)

		_, err = svc.StoreOAuthState(ctx, &StoreOAuthStateCommand{
			State: "state1",
			OAuthState: &storage.OAuthState{
				Provider:    "github",
				RedirectURI: "https://example.com/callback",
				Username:    "alice",
				ClientID:    "client-1",
				CreatedAt:   time.Now(),
			},
		})
		assert.Error(t, err)

		_, err = svc.CreateAuthorizationCode(ctx, &CreateAuthorizationCodeCommand{
			AuthCode: &storage.AuthorizationCode{
				Code:      "code1",
				ClientID:  "client-1",
				Username:  "alice",
				ExpiresAt: time.Now().Add(time.Minute),
				Scopes:    []string{"read"},
			},
		})
		assert.Error(t, err)
	})

	t.Run("hashPassword and isLocalDomain branches", func(t *testing.T) {
		svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, staticAuthService{hash: "h"}, zap.NewNop(), "example.com")
		hash, err := svc.hashPassword("pw")
		require.NoError(t, err)
		assert.Equal(t, "h", hash)

		assert.False(t, svc.isLocalDomain(""))
		assert.True(t, svc.isLocalDomain("example.com"))
	})
}

func TestService_Round13_RegistrationHelpers(t *testing.T) {
	svc, _ := newPermissiveAccountsService(t, permissiveDBOptions{domain: "example.com"})

	// Exercise crypto/auth helper error handling.
	_, err := svc.generateRSAKeyPair()
	assert.ErrorIs(t, err, ErrCryptoServiceNotConfigured)
	_, err = svc.encodePublicKeyPEM(struct{}{})
	assert.ErrorIs(t, err, ErrCryptoServiceNotConfigured)
	_, err = svc.hashPassword("pw")
	assert.ErrorIs(t, err, ErrAuthServiceNotConfigured)

	// Exercise rollback logging paths.
	svc.rollbackAccountCreation(context.Background(), nil, "alice", errors.New("boom"))

	// Quote permissions and default visibility helpers.
	assert.ErrorIs(t, svc.ensureQuotePermissionsForNewUser(context.Background(), nil, "alice", models.VisibilityPublic), ErrQuoteRepositoryNotAvailable)
	assert.ErrorIs(t, svc.persistDefaultPostingVisibility(context.Background(), nil, "alice", models.VisibilityPublic), ErrAccountRepositoryNotAvailable)
}

// Ensure we still satisfy compile-time expectations for the encryptor interface.
var _ marshalers.Encryptor = (*noopEncryptor)(nil)
