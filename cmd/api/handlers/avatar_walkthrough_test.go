package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/smithy-go"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

const (
	avatarTestBoundary  = "----WebKitFormBoundaryAvatarWalkthrough"
	avatarTestBucket    = "test-bucket"
	imagePNGContentType = "image/png"
	imageSVGContentType = "image/svg+xml"
)

// avatarTestS3Store is an in-memory media.S3Service plus the optional read
// capability the avatar serve path type-asserts.
type avatarTestS3Store struct {
	objects      map[string][]byte
	contentTypes map[string]string
}

func newAvatarTestS3Store() *avatarTestS3Store {
	return &avatarTestS3Store{
		objects:      make(map[string][]byte),
		contentTypes: make(map[string]string),
	}
}

func (s *avatarTestS3Store) UploadFile(
	_ context.Context,
	bucket, key string,
	data []byte,
	contentType string,
) (string, error) {
	s.objects[bucket+"/"+key] = bytes.Clone(data)
	s.contentTypes[bucket+"/"+key] = contentType
	return "s3://" + bucket + "/" + key, nil
}

func (s *avatarTestS3Store) DeleteFile(_ context.Context, bucket, key string) error {
	delete(s.objects, bucket+"/"+key)
	delete(s.contentTypes, bucket+"/"+key)
	return nil
}

func (s *avatarTestS3Store) DownloadFile(
	_ context.Context,
	bucket, key string,
	maxBytes int64,
) ([]byte, string, error) {
	object, ok := s.objects[bucket+"/"+key]
	if !ok {
		return nil, "", &smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}
	}
	if int64(len(object)) > maxBytes {
		return nil, "", assert.AnError
	}
	return bytes.Clone(object), s.contentTypes[bucket+"/"+key], nil
}

func (s *avatarTestS3Store) has(key string) bool {
	_, ok := s.objects[avatarTestBucket+"/"+key]
	return ok
}

func avatarTestPNGBytes() []byte {
	return append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, []byte("avatar-walkthrough-payload")...)
}

// avatarAccountsStore mirrors the account-side ownership contract of
// accounts.Service: SetAvatar records the minted avatar id on the account's own
// row alongside user.Avatar and Actor.Icon, and ClearAvatar reports only that
// recorded id, so a clear can address only the object the caller's own row
// records. The handler test harness's in-memory DynamoDB fake does not apply
// UpdateBuilder writes, so a real accounts.Service cannot observe its own writes
// there; the real service dual-write is proven by
// pkg/services/accounts/avatar_test.go.
type avatarAccountsStore struct {
	*AccountsServiceStub
	domain            string
	user              *storage.User
	actor             *activitypub.Actor
	updateProfileHits int
}

func newAvatarAccountsStore(t *testing.T, state *round10QueryState, domain string) *avatarAccountsStore {
	t.Helper()

	storedUser := state.usersByUsername["alice"]
	storedActor := state.actorsByUser["alice"]
	require.NotNil(t, storedActor.Actor)

	store := &avatarAccountsStore{
		domain: domain,
		user: &storage.User{
			ID:          "1",
			Username:    storedUser.Username,
			DisplayName: "Alice",
			URL:         "https://example.com/@alice",
		},
		actor: storedActor.Actor,
	}

	store.AccountsServiceStub = &AccountsServiceStub{
		GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
			return store.account(), nil
		},
		SetAvatarFunc:   store.setAvatar,
		ClearAvatarFunc: store.clearAvatar,
		UpdateProfileFunc: func(_ context.Context, cmd *accounts.UpdateProfileCommand) (*accounts.AccountResult, error) {
			store.updateProfileHits++
			store.user.DisplayName = cmd.DisplayName
			return &accounts.AccountResult{Account: store.account()}, nil
		},
	}
	return store
}

func (s *avatarAccountsStore) account() *storage.Account {
	return &storage.Account{User: s.user, Actor: s.actor}
}

func (s *avatarAccountsStore) servedURL(avatarID string) string {
	return "https://" + s.domain + "/api/v1/avatars/" + avatarID
}

func (s *avatarAccountsStore) setAvatar(_ context.Context, cmd *accounts.SetAvatarCommand) (*accounts.SetAvatarResult, error) {
	if !media.IsValidAvatarID(cmd.AvatarID) {
		return nil, accounts.ErrInvalidAvatarID
	}

	previous := ""
	if s.user != nil && s.user.AvatarID != cmd.AvatarID && media.IsValidAvatarID(s.user.AvatarID) {
		previous = s.user.AvatarID
	}

	served := s.servedURL(cmd.AvatarID)
	s.user.Avatar = served
	s.user.AvatarID = cmd.AvatarID
	if s.actor != nil {
		s.actor.Icon = &activitypub.Image{URL: served, MediaType: strings.TrimSpace(cmd.ContentType)}
	}
	return &accounts.SetAvatarResult{Account: s.account(), PreviousAvatarID: previous}, nil
}

func (s *avatarAccountsStore) clearAvatar(_ context.Context, _ *accounts.ClearAvatarCommand) (*accounts.ClearAvatarResult, error) {
	previous := ""
	if s.user != nil && media.IsValidAvatarID(s.user.AvatarID) {
		previous = s.user.AvatarID
	}

	s.user.Avatar = ""
	s.user.AvatarID = ""
	if s.actor != nil {
		s.actor.Icon = nil
	}
	return &accounts.ClearAvatarResult{Account: s.account(), PreviousAvatarID: previous}, nil
}

func newAvatarWalkthroughHandler(t *testing.T) (*Handler, *avatarTestS3Store, *avatarAccountsStore) {
	t.Helper()

	cfg := round11TestConfig()
	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {
				PK:        "USER#alice",
				SK:        storagemodels.SKMetadata,
				Username:  "alice",
				Role:      "user",
				Approved:  true,
				Version:   1,
				CreatedAt: now,
			},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/alice",
						Type: "Person",
					},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now,
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	store := newAvatarTestS3Store()
	mediaSvc := media.NewService(nil, nil, nil, nil, round10TestLogger(t), avatarTestBucket, "")
	mediaSvc.SetS3Service(store)

	accountsStore := newAvatarAccountsStore(t, state, cfg.Domain)

	handler.registry = &RegistryStub{AccountsSvc: accountsStore, MediaSvc: mediaSvc}
	return handler, store, accountsStore
}

func avatarUploadBody(t *testing.T, fileName string) []byte {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.SetBoundary(avatarTestBoundary))

	writeAvatarFilePart(t, writer, "file", fileName, imagePNGContentType, avatarTestPNGBytes())
	require.NoError(t, writer.Close())

	return body.Bytes()
}

// writeAvatarFilePart writes one binary form part with an explicit part
// content type; multipart.Writer.CreateFormFile always claims
// application/octet-stream, which the avatar allowlist rejects.
func writeAvatarFilePart(
	t *testing.T,
	writer *multipart.Writer,
	field, fileName, contentType string,
	data []byte,
) {
	t.Helper()

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, fileName))
	header.Set("Content-Type", contentType)

	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
}

func avatarUploadHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "multipart/form-data; boundary=" + avatarTestBoundary,
	}
}

func requireAccountAvatar(t *testing.T, resp *apptheory.Response) string {
	t.Helper()

	avatar := accountAvatarFrom(t, resp)
	require.NotEmpty(t, avatar)
	return avatar
}

func accountAvatarFrom(t *testing.T, resp *apptheory.Response) string {
	t.Helper()

	var body struct {
		Avatar       string `json:"avatar"`
		AvatarStatic string `json:"avatar_static"`
	}
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	assert.Equal(t, body.Avatar, body.AvatarStatic)
	return body.Avatar
}

// TestAvatarWalkthrough proves the upload -> public serve -> clear walkthrough
// end to end at the HTTP boundary with a real media service over an in-memory
// object store. The accounts side uses avatarAccountsStore, which mirrors the
// service's dual-write contract; see its doc comment for why.
func TestAvatarWalkthrough(t *testing.T) {
	handler, store, accountsStore := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	uploadCtx := round10NewLiftContextWithBodyBytes(
		http.MethodPost, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, avatarUploadBody(t, "me.png"),
	)
	uploadResp, err := handler.HandleUploadAvatarLift(uploadCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, uploadResp.Status)

	servedURL := requireAccountAvatar(t, uploadResp)

	avatarID := accountsStore.user.AvatarID
	require.True(t, media.IsValidAvatarID(avatarID), avatarID)
	require.Equal(t, "https://example.com/api/v1/avatars/"+avatarID, servedURL)
	require.True(t, store.has(media.AvatarObjectKey(avatarID)))

	assert.Equal(t, servedURL, accountsStore.user.Avatar)
	require.NotNil(t, accountsStore.actor)
	require.NotNil(t, accountsStore.actor.Icon, "the upload must set user.Avatar and Actor.Icon together")
	assert.Equal(t, servedURL, accountsStore.actor.Icon.URL)
	assert.Equal(t, imagePNGContentType, accountsStore.actor.Icon.MediaType)

	serveCtx := round10NewLiftContextWithBodyBytes(http.MethodGet, "/api/v1/avatars/"+avatarID, nil, nil, nil)
	serveCtx.Params["id"] = avatarID

	serveResp, err := handler.HandleGetAvatarLift(serveCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, serveResp.Status)
	assert.True(t, serveResp.IsBase64)
	assert.Equal(t, avatarTestPNGBytes(), serveResp.Body)
	assert.Equal(t, []string{"image/png"}, serveResp.Headers["content-type"])
	assert.Equal(t, []string{media.AvatarCacheControl}, serveResp.Headers["cache-control"])
	assert.Equal(t, []string{"nosniff"}, serveResp.Headers["x-content-type-options"])
	assert.Equal(t, []string{strconv.Itoa(len(avatarTestPNGBytes()))}, serveResp.Headers["content-length"])

	clearCtx := round10NewLiftContextWithBodyBytes(http.MethodDelete, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, nil)
	clearResp, err := handler.HandleClearAvatarLift(clearCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, clearResp.Status)

	clearedURL := accountAvatarFrom(t, clearResp)
	assert.Empty(t, clearedURL, "a cleared avatar is reported as absent, not as a placeholder image URL")
	assert.Empty(t, accountsStore.user.Avatar)
	assert.Nil(t, accountsStore.actor.Icon, "clear must empty Actor.Icon with user.Avatar")
	assert.False(t, store.has(media.AvatarObjectKey(avatarID)), "orphaned avatar object must be deleted")

	missingResp, err := handler.HandleGetAvatarLift(serveCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, missingResp.Status)
}

// TestUpdateCredentialsRejectsAnAvatarURL is the HTTP half of the ruling that
// update_credentials never accepts an avatar URL: the parameter is rejected with
// a 4xx that points at the upload route, and nothing reaches the profile service.
func TestUpdateCredentialsRejectsAnAvatarURL(t *testing.T) {
	handler, _, accountsStore := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	rejected := round10NewLiftContextWithBodyBytes(
		http.MethodPatch, "/api/v1/accounts/update_credentials", headers, nil,
		[]byte(`{"avatar":"https://example.com/api/v1/avatars/11111111-2222-4333-8444-555555555555"}`),
	)
	rejectedResp, err := handler.HandleUpdateCredentialsLift(rejected)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, rejectedResp.Status)
	assert.Contains(t, string(rejectedResp.Body), "POST /api/v1/accounts/avatar")
	assert.Equal(t, 0, accountsStore.updateProfileHits, "a rejected avatar parameter must not reach the profile service")
	assert.Empty(t, accountsStore.user.Avatar, "the profile URL must never take a caller-chosen value")

	// The same request without the avatar parameter is still accepted.
	accepted := round10NewLiftContextWithBodyBytes(
		http.MethodPatch, "/api/v1/accounts/update_credentials", headers, nil,
		[]byte(`{"display_name":"Alice"}`),
	)
	acceptedResp, err := handler.HandleUpdateCredentialsLift(accepted)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, acceptedResp.Status)
	assert.Equal(t, 1, accountsStore.updateProfileHits)
}

func TestUploadAvatarRejectsOversizeBody(t *testing.T) {
	handler, store, _ := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	oversize := append(
		append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0x00}, media.AvatarMaxUploadBytes)...),
		[]byte("tail")...,
	)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.SetBoundary(avatarTestBoundary))
	writeAvatarFilePart(t, writer, "file", "huge.png", imagePNGContentType, oversize)
	require.NoError(t, writer.Close())

	ctx := round10NewLiftContextWithBodyBytes(
		http.MethodPost, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, body.Bytes(),
	)
	resp, err := handler.HandleUploadAvatarLift(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.Status)
	assert.Empty(t, store.objects)
}

func TestUploadAvatarRejectsDisallowedContentType(t *testing.T) {
	handler, _, _ := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.SetBoundary(avatarTestBoundary))
	writeAvatarFilePart(t, writer, "file", "me.svg", imageSVGContentType, svg)
	require.NoError(t, writer.Close())

	ctx := round10NewLiftContextWithBodyBytes(
		http.MethodPost, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, body.Bytes(),
	)
	resp, err := handler.HandleUploadAvatarLift(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.Status)
}

func TestUploadAvatarRequiresFilePart(t *testing.T) {
	handler, _, _ := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.SetBoundary(avatarTestBoundary))
	require.NoError(t, writer.WriteField("description", "no file here"))
	require.NoError(t, writer.Close())

	ctx := round10NewLiftContextWithBodyBytes(
		http.MethodPost, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, body.Bytes(),
	)
	resp, err := handler.HandleUploadAvatarLift(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.Status)
}

func TestUploadAvatarRequiresWriteScope(t *testing.T) {
	handler, _, _ := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	ctx := round10NewLiftContextWithBodyBytes(
		http.MethodPost, "/api/v1/accounts/avatar",
		map[string]string{
			"Authorization": "Bearer " + readToken,
			"Content-Type":  "multipart/form-data; boundary=" + avatarTestBoundary,
		}, nil, avatarUploadBody(t, "me.png"),
	)
	resp, err := handler.HandleUploadAvatarLift(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.Status)

	anonCtx := round10NewLiftContextWithBodyBytes(
		http.MethodPost, "/api/v1/accounts/avatar",
		map[string]string{"Content-Type": "multipart/form-data; boundary=" + avatarTestBoundary},
		nil, avatarUploadBody(t, "me.png"),
	)
	anonResp, err := handler.HandleUploadAvatarLift(anonCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, anonResp.Status)
}

func TestGetAvatarHidesMalformedAndMissingIDs(t *testing.T) {
	handler, store, _ := newAvatarWalkthroughHandler(t)

	malformed := round10NewLiftContextWithBodyBytes(http.MethodGet, "/api/v1/avatars/not-a-uuid", nil, nil, nil)
	malformed.Params["id"] = "../../etc/passwd"
	malformedResp, err := handler.HandleGetAvatarLift(malformed)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, malformedResp.Status)

	missing := round10NewLiftContextWithBodyBytes(
		http.MethodGet, "/api/v1/avatars/550e8400-e29b-41d4-a716-446655440000", nil, nil, nil,
	)
	missing.Params["id"] = "550e8400-e29b-41d4-a716-446655440000"
	missingResp, err := handler.HandleGetAvatarLift(missing)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, missingResp.Status)
	require.Equal(t, malformedResp.Body, missingResp.Body)

	// An object whose stored content type has drifted out of the allowlist is
	// not servable at all, so it is reported exactly like a missing avatar rather
	// than surfacing a 500.
	driftedID := "11111111-2222-4333-8444-555555555555"
	_, err = store.UploadFile(
		context.Background(), avatarTestBucket, media.AvatarObjectKey(driftedID), avatarTestPNGBytes(), imageSVGContentType,
	)
	require.NoError(t, err)

	drifted := round10NewLiftContextWithBodyBytes(http.MethodGet, "/api/v1/avatars/"+driftedID, nil, nil, nil)
	drifted.Params["id"] = driftedID
	driftedResp, err := handler.HandleGetAvatarLift(drifted)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, driftedResp.Status)
	require.Equal(t, missingResp.Body, driftedResp.Body,
		"a drifted stored content type must be byte-identical to a missing avatar")
}

// TestClearAvatarNeverDeletesAnotherAccountsObject is the cross-user deletion
// regression. A row that predates the recorded avatar id can hold another
// account's served avatar URL verbatim, and a clear of that row must still delete
// nothing: the handler acts only on the id the authenticated principal's own row
// recorded, never on what a stored URL happens to contain.
func TestClearAvatarNeverDeletesAnotherAccountsObject(t *testing.T) {
	handler, store, accountsStore := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	victimID := "550e8400-e29b-41d4-a716-446655440000"
	_, err := store.UploadFile(
		context.Background(), avatarTestBucket, media.AvatarObjectKey(victimID), avatarTestPNGBytes(), imagePNGContentType,
	)
	require.NoError(t, err)

	// The victim's own served URL, well-formed and pointing at this instance —
	// exactly the shape the removed URL-parsing cleanup would have deleted.
	accountsStore.user.Avatar = accountsStore.servedURL(victimID)

	ctx := round10NewLiftContextWithBodyBytes(http.MethodDelete, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, nil)
	resp, err := handler.HandleClearAvatarLift(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
	assert.True(t, store.has(media.AvatarObjectKey(victimID)),
		"a served-avatar-shaped stored URL must never authorize deleting another account's object")
	assert.Empty(t, accountsStore.user.Avatar)
}

func TestClearAvatarSkipsNonAvatarURLs(t *testing.T) {
	handler, store, accountsStore := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	unrelatedID := "550e8400-e29b-41d4-a716-446655440000"
	_, err := store.UploadFile(
		context.Background(), avatarTestBucket, media.AvatarObjectKey(unrelatedID), avatarTestPNGBytes(), imagePNGContentType,
	)
	require.NoError(t, err)

	accountsStore.user.Avatar = "https://cdn.example/media/550e8400-e29b-41d4-a716-446655440000.png"

	ctx := round10NewLiftContextWithBodyBytes(http.MethodDelete, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, nil)
	resp, err := handler.HandleClearAvatarLift(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
	assert.True(t, store.has(media.AvatarObjectKey(unrelatedID)), "a non-avatar URL must never trigger a delete")
}

// TestClearAvatarDeletesOnlyTheOwnRecordedID covers the positive half: when the
// account's own row records an id, the clear deletes exactly that object and
// nothing else.
func TestClearAvatarDeletesOnlyTheOwnRecordedID(t *testing.T) {
	handler, store, accountsStore := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	ownID := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	otherID := "11111111-2222-4333-8444-555555555555"
	for _, id := range []string{ownID, otherID} {
		_, err := store.UploadFile(
			context.Background(), avatarTestBucket, media.AvatarObjectKey(id), avatarTestPNGBytes(), imagePNGContentType,
		)
		require.NoError(t, err)
	}

	accountsStore.user.Avatar = accountsStore.servedURL(ownID)
	accountsStore.user.AvatarID = ownID

	ctx := round10NewLiftContextWithBodyBytes(http.MethodDelete, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, nil)
	resp, err := handler.HandleClearAvatarLift(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
	assert.False(t, store.has(media.AvatarObjectKey(ownID)), "the account's own recorded object must be deleted")
	assert.True(t, store.has(media.AvatarObjectKey(otherID)), "no other object may be touched")
}

// TestUploadAvatarDeletesOnlyTheOwnPreviousObject covers the re-upload cleanup:
// the handler best-effort deletes the id the caller's own row recorded before,
// and never an object belonging to anyone else.
func TestUploadAvatarDeletesOnlyTheOwnPreviousObject(t *testing.T) {
	handler, store, accountsStore := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	previousID := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	otherID := "11111111-2222-4333-8444-555555555555"
	for _, id := range []string{previousID, otherID} {
		_, err := store.UploadFile(
			context.Background(), avatarTestBucket, media.AvatarObjectKey(id), avatarTestPNGBytes(), imagePNGContentType,
		)
		require.NoError(t, err)
	}

	accountsStore.user.Avatar = accountsStore.servedURL(previousID)
	accountsStore.user.AvatarID = previousID

	ctx := round10NewLiftContextWithBodyBytes(
		http.MethodPost, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, avatarUploadBody(t, "next.png"),
	)
	resp, err := handler.HandleUploadAvatarLift(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)

	newID := accountsStore.user.AvatarID
	require.True(t, media.IsValidAvatarID(newID))
	require.NotEqual(t, previousID, newID)
	assert.True(t, store.has(media.AvatarObjectKey(newID)), "the newly stored object must survive")
	assert.False(t, store.has(media.AvatarObjectKey(previousID)), "the caller's own previous object must be cleaned up")
	assert.True(t, store.has(media.AvatarObjectKey(otherID)), "no other object may be touched")
}
