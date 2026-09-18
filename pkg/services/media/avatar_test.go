package media

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type avatarS3Object struct {
	data        []byte
	contentType string
}

type fakeAvatarS3Store struct {
	objects     map[string]avatarS3Object
	uploadErr   error
	deleteErr   error
	downloadErr error
}

func newFakeAvatarS3Store() *fakeAvatarS3Store {
	return &fakeAvatarS3Store{objects: make(map[string]avatarS3Object)}
}

func (f *fakeAvatarS3Store) UploadFile(
	_ context.Context,
	bucket string,
	key string,
	data []byte,
	contentType string,
) (string, error) {
	if f.uploadErr != nil {
		return "", f.uploadErr
	}
	f.objects[bucket+"/"+key] = avatarS3Object{data: bytes.Clone(data), contentType: contentType}
	return "s3://" + bucket + "/" + key, nil
}

func (f *fakeAvatarS3Store) DeleteFile(_ context.Context, bucket, key string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.objects, bucket+"/"+key)
	return nil
}

func (f *fakeAvatarS3Store) DownloadFile(
	_ context.Context,
	bucket string,
	key string,
	maxBytes int64,
) ([]byte, string, error) {
	if f.downloadErr != nil {
		return nil, "", f.downloadErr
	}
	object, ok := f.objects[bucket+"/"+key]
	if !ok {
		return nil, "", &smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}
	}
	if int64(len(object.data)) > maxBytes {
		return nil, "", errors.New("object exceeds the declared size cap")
	}
	return bytes.Clone(object.data), object.contentType, nil
}

type writeOnlyAvatarS3Store struct{}

func (writeOnlyAvatarS3Store) UploadFile(context.Context, string, string, []byte, string) (string, error) {
	return "", nil
}

func (writeOnlyAvatarS3Store) DeleteFile(context.Context, string, string) error { return nil }

func newAvatarTestService(t *testing.T) (*Service, *fakeAvatarS3Store) {
	t.Helper()
	service, _, _, _ := createTestService(t)
	store := newFakeAvatarS3Store()
	service.SetS3Service(store)
	return service, store
}

func pngAvatarBytes() []byte {
	return append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, []byte("avatar-payload")...)
}

func TestService_StoreAvatarDownloadRoundTrip(t *testing.T) {
	service, _ := newAvatarTestService(t)
	ctx := context.Background()
	data := pngAvatarBytes()

	stored, err := service.StoreAvatar(ctx, &StoreAvatarCommand{
		UserID:      "alice",
		FileName:    "me.png",
		ContentType: "image/png",
		FileData:    data,
	})
	require.NoError(t, err)
	require.True(t, IsValidAvatarID(stored.AvatarID))
	assert.Equal(t, "image/png", stored.ContentType)
	assert.Equal(t, int64(len(data)), stored.Size)
	assert.Equal(t, AvatarS3Prefix+stored.AvatarID, stored.S3Key)

	got, contentType, err := service.GetAvatar(ctx, stored.AvatarID)
	require.NoError(t, err)
	assert.Equal(t, data, got)
	assert.Equal(t, "image/png", contentType)
}

func TestService_StoreAvatarRejectsOversize(t *testing.T) {
	service, _ := newAvatarTestService(t)

	_, err := service.StoreAvatar(context.Background(), &StoreAvatarCommand{
		UserID:      "alice",
		ContentType: "image/png",
		FileData:    make([]byte, AvatarMaxUploadBytes+1),
	})
	assert.ErrorIs(t, err, ErrAvatarTooLarge)
}

func TestService_StoreAvatarRejectsContentTypeMismatch(t *testing.T) {
	service, _ := newAvatarTestService(t)

	_, err := service.StoreAvatar(context.Background(), &StoreAvatarCommand{
		UserID:      "alice",
		ContentType: "image/png",
		FileData:    []byte("this is not an image at all"),
	})
	assert.ErrorIs(t, err, ErrAvatarContentTypeNotAllowed)
}

func TestService_StoreAvatarRejectsSVG(t *testing.T) {
	service, _ := newAvatarTestService(t)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`)

	cases := []struct {
		name        string
		contentType string
	}{
		{name: "declared svg", contentType: "image/svg+xml"},
		{name: "declared png", contentType: "image/png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.StoreAvatar(context.Background(), &StoreAvatarCommand{
				UserID:      "alice",
				ContentType: tc.contentType,
				FileData:    svg,
			})
			assert.ErrorIs(t, err, ErrAvatarContentTypeNotAllowed)
		})
	}
}

func TestService_StoreAvatarRejectsEmptyCommand(t *testing.T) {
	service, _ := newAvatarTestService(t)

	_, err := service.StoreAvatar(context.Background(), nil)
	assert.ErrorIs(t, err, ErrAvatarRequired)
}

func TestService_AvatarRejectsInvalidIDs(t *testing.T) {
	service, _ := newAvatarTestService(t)
	ctx := context.Background()
	invalid := []string{
		"",
		"../../etc/passwd",
		"ABC",
		"550e8400-e29b-41d4-a716-44665544000",
		"550E8400-E29B-41D4-A716-446655440000",
		"550e8400e29b41d4a716446655440000",
		"550e8400-e29b-41d4-a716-44665544000g",
	}

	for _, id := range invalid {
		assert.False(t, IsValidAvatarID(id), "id %q must be invalid", id)
		_, _, err := service.GetAvatar(ctx, id)
		assert.ErrorIs(t, err, ErrAvatarInvalidID)
		assert.ErrorIs(t, service.DeleteAvatar(ctx, id), ErrAvatarInvalidID)
	}
}

func TestService_GetAvatarMissingIsNotFound(t *testing.T) {
	service, _ := newAvatarTestService(t)

	_, _, err := service.GetAvatar(context.Background(), "550e8400-e29b-41d4-a716-446655440000")
	assert.ErrorIs(t, err, ErrAvatarNotFound)
}

func TestService_GetAvatarRejectsDisallowedStoredType(t *testing.T) {
	service, store := newAvatarTestService(t)
	id := "550e8400-e29b-41d4-a716-446655440000"
	store.objects["test-bucket/"+AvatarObjectKey(id)] = avatarS3Object{
		data:        []byte("<html></html>"),
		contentType: "text/html",
	}

	_, _, err := service.GetAvatar(context.Background(), id)
	assert.ErrorIs(t, err, ErrAvatarContentTypeNotAllowed)
}

func TestService_AvatarStoreUnavailable(t *testing.T) {
	service, _ := newAvatarTestService(t)
	ctx := context.Background()
	id := "550e8400-e29b-41d4-a716-446655440000"

	service.SetS3Service(nil)
	_, err := service.StoreAvatar(ctx, &StoreAvatarCommand{
		UserID:      "alice",
		ContentType: "image/png",
		FileData:    pngAvatarBytes(),
	})
	assert.ErrorIs(t, err, ErrAvatarStoreUnavailable)
	_, _, err = service.GetAvatar(ctx, id)
	assert.ErrorIs(t, err, ErrAvatarStoreUnavailable)
	assert.ErrorIs(t, service.DeleteAvatar(ctx, id), ErrAvatarStoreUnavailable)

	service.SetS3Service(writeOnlyAvatarS3Store{})
	_, _, err = service.GetAvatar(ctx, id)
	assert.ErrorIs(t, err, ErrAvatarStoreUnavailable)
}

func TestService_DeleteAvatarIsIdempotent(t *testing.T) {
	service, _ := newAvatarTestService(t)
	ctx := context.Background()

	stored, err := service.StoreAvatar(ctx, &StoreAvatarCommand{
		UserID:      "alice",
		ContentType: "image/png",
		FileData:    pngAvatarBytes(),
	})
	require.NoError(t, err)

	require.NoError(t, service.DeleteAvatar(ctx, stored.AvatarID))
	require.NoError(t, service.DeleteAvatar(ctx, stored.AvatarID))
	_, _, err = service.GetAvatar(ctx, stored.AvatarID)
	assert.ErrorIs(t, err, ErrAvatarNotFound)
}

func TestAvatarObjectKeyIsAlwaysNamespaced(t *testing.T) {
	ids := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"00000000-0000-0000-0000-000000000000",
	}
	for _, id := range ids {
		key := AvatarObjectKey(id)
		assert.Equal(t, AvatarS3Prefix+id, key)
		assert.True(t, len(key) > len(AvatarS3Prefix))
		assert.Equal(t, AvatarS3Prefix, key[:len(AvatarS3Prefix)])
	}
}
