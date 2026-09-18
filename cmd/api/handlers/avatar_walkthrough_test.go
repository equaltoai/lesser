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

// avatarAccountsStore mirrors the account-side dual-write contract of
// accounts.Service: SetAvatar writes user.Avatar and Actor.Icon together from
// the served /api/v1/avatars/<id> URL, and ClearAvatar empties both. The
// handler test harness's in-memory DynamoDB fake does not apply UpdateBuilder
// writes, so a real accounts.Service cannot observe its own writes there; the
// real service dual-write is proven by pkg/services/accounts/avatar_test.go.
type avatarAccountsStore struct {
	*AccountsServiceStub
	domain string
	user   *storage.User
	actor  *activitypub.Actor
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
	}
	return store
}

func (s *avatarAccountsStore) account() *storage.Account {
	return &storage.Account{User: s.user, Actor: s.actor}
}

func (s *avatarAccountsStore) servedURL(avatarID string) string {
	return "https://" + s.domain + "/api/v1/avatars/" + avatarID
}

func (s *avatarAccountsStore) setAvatar(_ context.Context, cmd *accounts.SetAvatarCommand) (*accounts.AccountResult, error) {
	if !media.IsValidAvatarID(cmd.AvatarID) {
		return nil, accounts.ErrInvalidAvatarID
	}

	served := s.servedURL(cmd.AvatarID)
	s.user.Avatar = served
	if s.actor != nil {
		s.actor.Icon = &activitypub.Image{URL: served, MediaType: strings.TrimSpace(cmd.ContentType)}
	}
	return &accounts.AccountResult{Account: s.account()}, nil
}

func (s *avatarAccountsStore) clearAvatar(_ context.Context, _ *accounts.ClearAvatarCommand) (*accounts.ClearAvatarResult, error) {
	previous := ""
	if s.user != nil {
		previous = strings.TrimSpace(s.user.Avatar)
	}

	s.user.Avatar = ""
	if s.actor != nil {
		s.actor.Icon = nil
	}
	return &accounts.ClearAvatarResult{Account: s.account(), PreviousAvatarURL: previous}, nil
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

	var body struct {
		Avatar       string `json:"avatar"`
		AvatarStatic string `json:"avatar_static"`
	}
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.NotEmpty(t, body.Avatar)
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

	avatarID, ok := media.AvatarIDFromURL(servedURL)
	require.True(t, ok, servedURL)
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

	clearedURL := requireAccountAvatar(t, clearResp)
	assert.Equal(t, "https://example.com/avatars/original/missing.png", clearedURL)
	assert.Empty(t, accountsStore.user.Avatar)
	assert.Nil(t, accountsStore.actor.Icon, "clear must empty Actor.Icon with user.Avatar")
	assert.False(t, store.has(media.AvatarObjectKey(avatarID)), "orphaned avatar object must be deleted")

	missingResp, err := handler.HandleGetAvatarLift(serveCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, missingResp.Status)
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
	handler, _, _ := newAvatarWalkthroughHandler(t)

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

func TestAvatarIDFromURL(t *testing.T) {
	id := "550e8400-e29b-41d4-a716-446655440000"

	cases := map[string]bool{
		"https://example.com/api/v1/avatars/" + id:                  true,
		"http://localhost:4000/api/v1/avatars/" + id:                true,
		"/api/v1/avatars/" + id:                                     true,
		"https://example.com/api/v1/avatars/" + id + "?v=1":         true,
		"https://example.com/api/v1/avatars/" + strings.ToUpper(id): false,
		"https://example.com/api/v1/avatars/" + id + "/extra":       false,
		"https://example.com/avatars/original/missing.png":          false,
		"https://example.com/api/v1/avatars/not-a-uuid":             false,
		"": false,
	}

	for rawURL, expected := range cases {
		got, ok := media.AvatarIDFromURL(rawURL)
		assert.Equal(t, expected, ok, rawURL)
		if expected {
			assert.Equal(t, id, got, rawURL)
		}
	}
}
