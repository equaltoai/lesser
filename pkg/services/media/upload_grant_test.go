package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// tinyPNG is a 1x1 transparent PNG that the media processor decodes, giving a
// deterministic ready-path for internal image finalize.
const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

var tinyPNG = func() []byte {
	data, err := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if err != nil {
		panic(err)
	}
	return data
}()

type uploadGrantPresignCall struct {
	bucket        string
	key           string
	contentType   string
	contentSHA256 string
	kmsKeyID      string
	expiry        time.Duration
}

type uploadGrantObjectFake struct {
	mu          sync.Mutex
	objects     map[string][]byte
	types       map[string]string
	presigns    []uploadGrantPresignCall
	deletes     []string
	downloads   []string
	presignErr  error
	downloadErr error
	deleteErr   error
}

func newUploadGrantObjectFake() *uploadGrantObjectFake {
	return &uploadGrantObjectFake{
		objects: make(map[string][]byte),
		types:   make(map[string]string),
	}
}

func (f *uploadGrantObjectFake) UploadFile(_ context.Context, bucket, key string, data []byte, contentType string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[bucket+"/"+key] = bytes.Clone(data)
	f.types[bucket+"/"+key] = contentType
	return "s3://" + bucket + "/" + key, nil
}

func (f *uploadGrantObjectFake) DeleteFile(_ context.Context, bucket, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletes = append(f.deletes, bucket+"/"+key)
	delete(f.objects, bucket+"/"+key)
	return nil
}

// SimulateUpload PUTs the declared bytes to the grant's key, mirroring what a
// compliant presigned-companion client does.
func (f *uploadGrantObjectFake) SimulateUpload(grant *models.UploadGrant, data []byte) error {
	_, err := f.UploadFile(context.Background(), grant.S3Bucket, grant.S3Key, data, grant.ContentType)
	return err
}

func (f *uploadGrantObjectFake) PresignPutObject(_ context.Context, bucket, key, contentType, contentSHA256Hex, kmsKeyID string, expiry time.Duration) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.presignErr != nil {
		return "", f.presignErr
	}
	f.presigns = append(f.presigns, uploadGrantPresignCall{
		bucket: bucket, key: key, contentType: contentType, contentSHA256: contentSHA256Hex,
		kmsKeyID: kmsKeyID, expiry: expiry,
	})
	return "https://presigned.example.invalid/" + bucket + "/" + key + "?X-Amz-Signature=test", nil
}

func (f *uploadGrantObjectFake) HeadFile(_ context.Context, bucket, key string) (int64, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[bucket+"/"+key]
	if !ok {
		return 0, "", fmt.Errorf("object not found")
	}
	return int64(len(data)), f.types[bucket+"/"+key], nil
}

func (f *uploadGrantObjectFake) DownloadFile(_ context.Context, bucket, key string, maxBytes int64) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.downloadErr != nil {
		return nil, "", f.downloadErr
	}
	f.downloads = append(f.downloads, bucket+"/"+key)
	data, ok := f.objects[bucket+"/"+key]
	if !ok {
		return nil, "", fmt.Errorf("object not found")
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("object exceeds size cap")
	}
	return bytes.Clone(data), f.types[bucket+"/"+key], nil
}

func (f *uploadGrantObjectFake) Presigns() []uploadGrantPresignCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uploadGrantPresignCall(nil), f.presigns...)
}

func (f *uploadGrantObjectFake) Deletes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deletes...)
}

// Downloads returns the object keys whose bodies were actually fetched, so
// tests can assert the fail-closed size gate never downloaded an object.
func (f *uploadGrantObjectFake) Downloads() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.downloads...)
}

func newUploadGrantTestService(t *testing.T) (*Service, *inmemory.UploadGrantRepository, *uploadGrantObjectFake, *MockMediaRepository) {
	t.Helper()
	mediaRepo := new(MockMediaRepository)
	service := NewService(
		mediaRepo,
		nil,
		streaming.NewMockPublisher(),
		new(MockJobQueueService),
		zaptest.NewLogger(t),
		"media-private",
		"cdn.example.com",
	)
	objectStore := newUploadGrantObjectFake()
	service.SetS3Service(objectStore)
	service.SetEditorialKMSKeyID("alias/lesser-test")
	grantRepo := inmemory.NewUploadGrantRepository()
	service.SetUploadGrantRepository(grantRepo)
	return service, grantRepo, objectStore, mediaRepo
}

func uploadGrantDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestMintUploadGrant(t *testing.T) {
	service, grantRepo, objectStore, _ := newUploadGrantTestService(t)
	input := MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	}
	grant, url, err := service.MintUploadGrant(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, grant)
	require.NotEmpty(t, url)
	require.Contains(t, url, grant.S3Key)

	require.Equal(t, "alice", grant.Owner)
	require.Equal(t, "image/png", grant.ContentType)
	require.Equal(t, int64(5*1024*1024), grant.MaxSizeBytes)
	require.Equal(t, uploadGrantDigest(tinyPNG), grant.ContentSHA256)
	require.Equal(t, models.UploadGrantStatusMinted, grant.Status)
	require.Equal(t, "media-private", grant.S3Bucket)
	require.Contains(t, grant.S3Key, grant.MediaID)
	require.NotEmpty(t, grant.FileName)
	require.True(t, grant.ExpiresAt.After(time.Now().UTC()))
	require.Equal(t, grant.ExpiresAt.Unix(), grant.ExpiresAtTTL)
	require.GreaterOrEqual(t, grant.ExpiresAt.Sub(grant.GrantedAt), UploadGrantTTL-time.Second)

	// The grant row is persisted in the owner's partition.
	stored, err := grantRepo.GetUploadGrant(context.Background(), "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, grant.GrantID, stored.GrantID)
	require.Equal(t, "USER#alice#UPLOAD", stored.PK)
	require.Equal(t, "GRANT#"+grant.GrantID, stored.SK)

	// The presigned PUT was minted against the grant's exact constraints:
	// declared content type, the declared sha256 (the object store converts the
	// hex digest to the base64 x-amz-checksum-sha256 header), SSE-KMS instance
	// key, and the grant-scoped object key.
	presigns := objectStore.Presigns()
	require.Len(t, presigns, 1)
	call := presigns[0]
	require.Equal(t, "media-private", call.bucket)
	require.Equal(t, grant.S3Key, call.key)
	require.Equal(t, "image/png", call.contentType)
	require.Equal(t, grant.ContentSHA256, call.contentSHA256, "the declared digest must be bound into the presigned PUT")
	require.Equal(t, "alias/lesser-test", call.kmsKeyID)
	require.Equal(t, UploadGrantTTL, call.expiry)
}

func TestMintUploadGrantValidation(t *testing.T) {
	service, grantRepo, _, _ := newUploadGrantTestService(t)
	valid := uploadGrantDigest(tinyPNG)
	cases := []struct {
		name  string
		input MintUploadGrantInput
	}{
		{"missing owner", MintUploadGrantInput{ContentType: "image/png", MaxSizeBytes: 100, ContentSHA256: valid}},
		{"unsupported type", MintUploadGrantInput{Owner: "alice", ContentType: "text/html", MaxSizeBytes: 100, ContentSHA256: valid}},
		{"zero size", MintUploadGrantInput{Owner: "alice", ContentType: "image/png", MaxSizeBytes: 0, ContentSHA256: valid}},
		{"over cap", MintUploadGrantInput{Owner: "alice", ContentType: "image/png", MaxSizeBytes: 51 * 1024 * 1024, ContentSHA256: valid}},
		{"bad sha256", MintUploadGrantInput{Owner: "alice", ContentType: "image/png", MaxSizeBytes: 100, ContentSHA256: "not-hex"}},
		{"short sha256", MintUploadGrantInput{Owner: "alice", ContentType: "image/png", MaxSizeBytes: 100, ContentSHA256: "abcd"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grant, url, err := service.MintUploadGrant(context.Background(), tc.input)
			require.Error(t, err)
			require.Nil(t, grant)
			require.Empty(t, url)
			require.Len(t, grantRepo.Grants(), 0, "no grant row may be persisted for a rejected mint")
		})
	}
}

func TestMintUploadGrantFailsClosedWhenUnwired(t *testing.T) {
	service, _, _, _ := newUploadGrantTestService(t)
	service.SetUploadGrantRepository(nil)
	_, _, err := service.MintUploadGrant(context.Background(), MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 100, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.ErrorIs(t, err, ErrUploadGrantUnavailable)
}

func TestFinalizeUploadGrantFullFlow(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()

	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	require.NoError(t, objectStore.SimulateUpload(grant, tinyPNG))

	var created *models.Media
	mediaRepo.On("CreateMedia", ctx, mock.MatchedBy(func(media *models.Media) bool {
		created = media
		return media.IsInternalEditorial()
	})).Return(nil).Once()

	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.NotNil(t, media)
	require.Equal(t, grant.MediaID, media.MediaID)
	require.Equal(t, "alice", media.UserID)
	require.Equal(t, "sha256:"+uploadGrantDigest(tinyPNG), media.ContentHash)
	require.Equal(t, models.MediaVisibilityInternal, media.Visibility)
	require.Equal(t, int64(len(tinyPNG)), media.FileSize)
	require.Equal(t, models.StatusReady, media.Status, "verified internal images enter ready like the M0 pipeline")
	require.Equal(t, grant.S3Bucket, media.S3Bucket)
	require.Equal(t, grant.S3Key, media.S3Key)
	require.NotNil(t, media.Provenance)
	require.Equal(t, models.EditorialMediaOriginSupplied, media.Provenance.Origin)
	require.Equal(t, media.ContentHash, media.Provenance.ContentIntegrity)
	require.NotNil(t, created)

	// The grant is consumed exactly once; the verified object is not deleted.
	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusUsed, stored.Status)
	require.NotNil(t, stored.UsedAt)
	require.Len(t, objectStore.Deletes(), 0)
}

func TestFinalizeUploadGrantDigestMismatchConsumesAndDeletes(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()

	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)

	// The client PUTs different bytes than it declared.
	wrongBytes := bytes.Repeat([]byte{0xAB}, 64)
	require.NoError(t, objectStore.SimulateUpload(grant, wrongBytes))

	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.ErrorIs(t, err, ErrUploadGrantDigestMismatch)
	require.Nil(t, media)
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)

	// The grant is consumed to FAILED_DIGEST with an inspectable reason, and
	// the unverified object is deleted (quarantine-free fail-closed choice).
	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusFailedDigest, stored.Status)
	require.NotNil(t, stored.FailedAt)
	require.Contains(t, stored.FailureReason, "digest")
	require.Contains(t, objectStore.Deletes(), grant.S3Bucket+"/"+grant.S3Key)
}

func TestFinalizeUploadGrantUnsafeSVGConsumesAndDeletes(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()

	unsafeSVGs := map[string][]byte{
		"script tag":    []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"event handler": []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`),
		"external ref":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><a href="https://evil.example/x">x</a></svg>`),
	}
	for name, payload := range unsafeSVGs {
		t.Run(name, func(t *testing.T) {
			grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
				Owner: "alice", ContentType: "image/svg+xml", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(payload),
			})
			require.NoError(t, err)
			require.NoError(t, objectStore.SimulateUpload(grant, payload))

			media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
			require.ErrorIs(t, err, ErrUploadGrantDigestMismatch)
			require.Nil(t, media)
			mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)

			// The unsafe SVG is consumed to FAILED_DIGEST with an inspectable
			// reason and the object is deleted; no media record is ever created.
			stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
			require.NoError(t, err)
			require.Equal(t, models.UploadGrantStatusFailedDigest, stored.Status)
			require.Contains(t, stored.FailureReason, "SVG")
			require.Contains(t, objectStore.Deletes(), grant.S3Bucket+"/"+grant.S3Key)
		})
	}
}

func TestFinalizeUploadGrantCleanSVGPasses(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()

	cleanSVG := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10v10z"/></svg>`)
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/svg+xml", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(cleanSVG),
	})
	require.NoError(t, err)
	require.NoError(t, objectStore.SimulateUpload(grant, cleanSVG))

	mediaRepo.On("CreateMedia", ctx, mock.MatchedBy(func(media *models.Media) bool {
		return media.IsInternalEditorial() && media.ContentType == "image/svg+xml"
	})).Return(nil).Once()

	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.NotNil(t, media)
	require.Equal(t, "image/svg+xml", media.ContentType)

	// The inert SVG is admitted: grant consumed to USED, object retained (it is
	// the asset's backing bytes).
	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusUsed, stored.Status)
	require.Len(t, objectStore.Deletes(), 0)
}

func TestFinalizeUploadGrantSizeCapViolationConsumesAndDeletes(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()

	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 64, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)

	oversized := bytes.Repeat([]byte{0x01}, 4096)
	require.NoError(t, objectStore.SimulateUpload(grant, oversized))

	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.ErrorIs(t, err, ErrUploadGrantDigestMismatch)
	require.Nil(t, media)
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)

	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusFailedDigest, stored.Status)
	require.Contains(t, stored.FailureReason, "exceeds declared cap")
	require.Contains(t, objectStore.Deletes(), grant.S3Bucket+"/"+grant.S3Key)
	// The size gate rejects from HEAD metadata alone: the oversized object must
	// never be downloaded into memory.
	require.Len(t, objectStore.Downloads(), 0, "oversized object must be rejected without downloading")
}

func TestFinalizeUploadGrantSizeBoundaryExactlyAtCapPasses(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()

	// The cap is exactly the object size: the boundary must pass, not trip the
	// size gate.
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: int64(len(tinyPNG)), ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	require.NoError(t, objectStore.SimulateUpload(grant, tinyPNG))

	mediaRepo.On("CreateMedia", ctx, mock.MatchedBy(func(media *models.Media) bool {
		return media.IsInternalEditorial() && media.FileSize == int64(len(tinyPNG))
	})).Return(nil).Once()

	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.NotNil(t, media)
	require.Equal(t, models.StatusReady, media.Status)

	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusUsed, stored.Status)
	require.Len(t, objectStore.Deletes(), 0)
	require.Len(t, objectStore.Downloads(), 1, "an at-cap object is downloaded exactly once for digest verification")
}

func TestFinalizeUploadGrantExpiredFailsClosed(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()

	now := time.Now().UTC()
	expired := &models.UploadGrant{
		Owner: "alice", GrantID: "grant-expired", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024,
		ContentSHA256: uploadGrantDigest(tinyPNG), S3Bucket: "media-private", S3Key: "media/2026/08/24/expired.png",
		MediaID: "media-expired", FileName: "expired.png", Status: models.UploadGrantStatusMinted,
		GrantedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute), Version: 0,
	}
	require.NoError(t, expired.UpdateKeys())
	grantRepo.SeedUploadGrant(expired)
	require.NoError(t, objectStore.SimulateUpload(expired, tinyPNG))

	media, err := service.FinalizeUploadGrant(ctx, "alice", "grant-expired")
	require.ErrorIs(t, err, ErrUploadGrantExpired)
	require.Nil(t, media)
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)

	// The expired grant is never consumed (the row self-cleans via TTL), but its
	// object is grant-scoped and unreferencable after expiry, so finalize deletes
	// it before failing closed.
	stored, err := grantRepo.GetUploadGrant(ctx, "alice", "grant-expired")
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusMinted, stored.Status)
	require.Contains(t, objectStore.Deletes(), expired.S3Bucket+"/"+expired.S3Key)
}

func TestFinalizeUploadGrantAlreadyUsedFailsClosed(t *testing.T) {
	service, grantRepo, _, mediaRepo := newUploadGrantTestService(t)
	now := time.Now().UTC()
	used := &models.UploadGrant{
		Owner: "alice", GrantID: "grant-used", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024,
		ContentSHA256: uploadGrantDigest(tinyPNG), S3Bucket: "media-private", S3Key: "media/2026/08/24/used.png",
		MediaID: "media-used", FileName: "used.png", Status: models.UploadGrantStatusUsed,
		GrantedAt: now, ExpiresAt: now.Add(time.Hour), Version: 1,
	}
	require.NoError(t, used.UpdateKeys())
	grantRepo.SeedUploadGrant(used)
	media, err := service.FinalizeUploadGrant(context.Background(), "alice", "grant-used")
	require.ErrorIs(t, err, ErrUploadGrantNotMinted)
	require.Nil(t, media)
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)
}

func TestFinalizeUploadGrantActorIsolation(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()

	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	require.NoError(t, objectStore.SimulateUpload(grant, tinyPNG))

	// Bob cannot finalize alice's grant: the owner-scoped key construction
	// makes the grant invisible to him, and no asset may be created.
	media, err := service.FinalizeUploadGrant(ctx, "bob", grant.GrantID)
	require.ErrorIs(t, err, ErrUploadGrantNotFound)
	require.Nil(t, media)
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)

	// Alice's grant is untouched by bob's attempt.
	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusMinted, stored.Status)
	require.Len(t, objectStore.Deletes(), 0)
}

func TestFinalizeUploadGrantMissingObjectLeavesGrantMinted(t *testing.T) {
	service, grantRepo, _, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()

	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)

	// No PUT happened: finalize fails without consuming, so a transient PUT
	// failure can be retried against the same grant.
	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.ErrorIs(t, err, ErrUploadGrantObjectMissing)
	require.Nil(t, media)
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)

	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusMinted, stored.Status)
}

func TestFinalizeUploadGrantConcurrentRaceExactlyOneWins(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()

	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	require.NoError(t, objectStore.SimulateUpload(grant, tinyPNG))

	mediaRepo.On("CreateMedia", ctx, mock.MatchedBy(func(media *models.Media) bool {
		return media.IsInternalEditorial()
	})).Return(nil).Once()

	const racers = 6
	var wg sync.WaitGroup
	results := make(chan error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	raced := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		// Losers fail closed either on the atomic consume (observed MINTED but
		// lost the CAS) or on re-reading the already-consumed grant (observed
		// USED). Both preserve the single-use invariant; the CAS race itself is
		// exercised deterministically at the repository layer.
		case errors.Is(err, ErrUploadGrantAlreadyConsumed), errors.Is(err, ErrUploadGrantNotMinted):
			raced++
		default:
			t.Fatalf("unexpected finalize error: %v", err)
		}
	}
	require.Equal(t, 1, successes, "exactly one concurrent finalize wins")
	require.Equal(t, racers-1, raced, "every loser fails closed on the single-use consume")
	mediaRepo.AssertNumberOfCalls(t, "CreateMedia", 1)

	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusUsed, stored.Status)
	require.Equal(t, 1, stored.Version)
}

func TestUploadGrantQueryStates(t *testing.T) {
	service, grantRepo, objectStore, _ := newUploadGrantTestService(t)
	ctx := context.Background()

	// MINTED: returns the grant with a fresh presigned URL for a retried PUT.
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	queried, url, err := service.UploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusMinted, queried.StatusClassification(time.Now().UTC()))
	require.NotEmpty(t, url)

	// USED: no URL, terminal state.
	require.NoError(t, objectStore.SimulateUpload(grant, tinyPNG))
	mediaRepo := new(MockMediaRepository)
	service.mediaRepo = mediaRepo
	mediaRepo.On("CreateMedia", ctx, mock.MatchedBy(func(media *models.Media) bool { return true })).Return(nil).Once()
	_, err = service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	queried, url, err = service.UploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusUsed, queried.StatusClassification(time.Now().UTC()))
	require.Empty(t, url)

	// FAILED_DIGEST: reason inspectable, no URL.
	now := time.Now().UTC()
	failed := &models.UploadGrant{
		Owner: "alice", GrantID: "grant-failed", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024,
		ContentSHA256: uploadGrantDigest(tinyPNG), S3Bucket: "media-private", S3Key: "media/2026/08/24/failed.png",
		MediaID: "media-failed", FileName: "failed.png", Status: models.UploadGrantStatusFailedDigest,
		GrantedAt: now, ExpiresAt: now.Add(time.Hour), Version: 1,
	}
	require.NoError(t, failed.UpdateKeys())
	failedAt := now.Add(-time.Minute)
	failed.FailedAt = &failedAt
	failed.FailureReason = "uploaded object digest does not match the declared sha256"
	grantRepo.SeedUploadGrant(failed)
	queried, url, err = service.UploadGrant(ctx, "alice", "grant-failed")
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusFailedDigest, queried.StatusClassification(time.Now().UTC()))
	require.Empty(t, url)
	require.Contains(t, queried.FailureReason, "digest")

	// EXPIRED: derived at read time from the bounded expiry.
	expired := &models.UploadGrant{
		Owner: "alice", GrantID: "grant-expired-q", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024,
		ContentSHA256: uploadGrantDigest(tinyPNG), S3Bucket: "media-private", S3Key: "media/2026/08/24/expired-q.png",
		MediaID: "media-expired-q", FileName: "expired-q.png", Status: models.UploadGrantStatusMinted,
		GrantedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute), Version: 0,
	}
	require.NoError(t, expired.UpdateKeys())
	grantRepo.SeedUploadGrant(expired)
	queried, url, err = service.UploadGrant(ctx, "alice", "grant-expired-q")
	require.NoError(t, err)
	require.Equal(t, "EXPIRED", queried.StatusClassification(time.Now().UTC()))
	require.Empty(t, url)

	// Actor isolation on the query: bob cannot see alice's grant.
	_, _, err = service.UploadGrant(ctx, "bob", grant.GrantID)
	require.ErrorIs(t, err, ErrUploadGrantNotFound)
}

func TestVerifyUploadedObject(t *testing.T) {
	data := []byte("exact bytes")
	digest := uploadGrantDigest(data)
	grant := &models.UploadGrant{ContentSHA256: digest, MaxSizeBytes: 100, ContentType: "image/png"}

	require.Empty(t, verifyUploadedObject(grant, data, "image/png"))
	require.Contains(t, verifyUploadedObject(grant, []byte("other bytes"), "image/png"), "digest")
	require.Contains(t, verifyUploadedObject(grant, bytes.Repeat([]byte{0x01}, 101), "image/png"), "exceeds declared cap")
	require.Contains(t, verifyUploadedObject(grant, data, "image/jpeg"), "content type")
}

func TestMintUploadGrantStoreUnavailableFailsClosed(t *testing.T) {
	service, grantRepo, _, _ := newUploadGrantTestService(t)
	// A store without the presigned-PUT capability must fail closed.
	service.SetS3Service(newFakeMediaS3Service())
	_, _, err := service.MintUploadGrant(context.Background(), MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 100, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.ErrorIs(t, err, ErrUploadGrantUnavailable)
	require.Len(t, grantRepo.Grants(), 0)
}

func TestMintUploadGrantFailsClosedOnMissingBucketAndKMS(t *testing.T) {
	service, _, _, _ := newUploadGrantTestService(t)
	service.s3Bucket = ""
	_, _, err := service.MintUploadGrant(context.Background(), MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 100, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.Error(t, err)

	service.s3Bucket = "media-private"
	service.editorialKMSKeyID = ""
	_, _, err = service.MintUploadGrant(context.Background(), MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 100, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.Error(t, err)
}

func TestMintUploadGrantPresignAndPersistErrors(t *testing.T) {
	service, grantRepo, objectStore, _ := newUploadGrantTestService(t)
	valid := MintUploadGrantInput{Owner: "alice", ContentType: "image/png", MaxSizeBytes: 100, ContentSHA256: uploadGrantDigest(tinyPNG)}

	// Presign failure: nothing is persisted.
	objectStore.presignErr = fmt.Errorf("presign down")
	_, _, err := service.MintUploadGrant(context.Background(), valid)
	require.Error(t, err)
	require.Len(t, grantRepo.Grants(), 0)

	// Persist failure: the error surfaces.
	objectStore.presignErr = nil
	service.uploadGrantRepo = &failingUploadGrantRepo{createErr: fmt.Errorf("storage down")}
	_, _, err = service.MintUploadGrant(context.Background(), valid)
	require.Error(t, err)
	require.Contains(t, err.Error(), "storage down")
}

type failingUploadGrantRepo struct {
	inmemory.UploadGrantRepository
	createErr  error
	getErr     error
	consumeErr error
}

func (f *failingUploadGrantRepo) CreateUploadGrant(_ context.Context, _ *models.UploadGrant) error {
	return f.createErr
}

func (f *failingUploadGrantRepo) GetUploadGrant(_ context.Context, _, _ string) (*models.UploadGrant, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return nil, storage.ErrNotFound
}

func (f *failingUploadGrantRepo) ConsumeUploadGrant(_ context.Context, _ *models.UploadGrant, _, _ string, _ time.Time) error {
	return f.consumeErr
}

func TestFinalizeUploadGrantConsumeErrorSurfaces(t *testing.T) {
	service, _, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	require.NoError(t, objectStore.SimulateUpload(grant, tinyPNG))

	service.uploadGrantRepo = &failingUploadGrantRepo{getErr: fmt.Errorf("read failed")}
	_, err = service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read failed")
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)
}

func TestFinalizeUploadGrantNonImageStaysPendingLikeM0(t *testing.T) {
	service, _, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()
	audio := []byte{0xFF, 0xF3, 0x04, 0x01}
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "audio/mpeg", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(audio),
	})
	require.NoError(t, err)
	require.NoError(t, objectStore.SimulateUpload(grant, audio))

	var created *models.Media
	mediaRepo.On("CreateMedia", ctx, mock.MatchedBy(func(media *models.Media) bool {
		created = media
		return true
	})).Return(nil).Once()

	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.NotNil(t, media)
	require.Equal(t, models.StatusPending, media.Status, "non-image internal assets stay pending exactly as the M0 pipeline leaves them")
	require.NotNil(t, created)
	require.Equal(t, models.StatusPending, created.Status)
}

func TestFinalizeUploadGrantImageDecodeFailureMarksFailed(t *testing.T) {
	service, _, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()
	garbage := []byte("definitely not a decodable image")
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(garbage),
	})
	require.NoError(t, err)
	require.NoError(t, objectStore.SimulateUpload(grant, garbage))

	var created *models.Media
	mediaRepo.On("CreateMedia", ctx, mock.MatchedBy(func(media *models.Media) bool {
		created = media
		return true
	})).Return(nil).Once()

	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.StatusFailed, media.Status, "an image that fails dimension processing enters failed like the M0 pipeline")
	require.NotNil(t, created)
	require.Equal(t, models.StatusFailed, created.Status)
}

func TestFinalizeUploadGrantStoredTypeMismatchFailsClosed(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	// The PUT carries the right bytes but a different Content-Type than declared.
	_, err = objectStore.UploadFile(ctx, grant.S3Bucket, grant.S3Key, tinyPNG, "image/jpeg")
	require.NoError(t, err)

	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.ErrorIs(t, err, ErrUploadGrantDigestMismatch)
	require.Nil(t, media)
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)
	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusFailedDigest, stored.Status)
	require.Contains(t, stored.FailureReason, "content type")
	require.Contains(t, objectStore.Deletes(), grant.S3Bucket+"/"+grant.S3Key)
}

func TestFinalizeUploadGrantCreateMediaFailureCleansObject(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	require.NoError(t, objectStore.SimulateUpload(grant, tinyPNG))

	mediaRepo.On("CreateMedia", ctx, mock.MatchedBy(func(media *models.Media) bool { return true })).
		Return(fmt.Errorf("record write failed")).Once()

	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "record write failed")
	require.Nil(t, media)
	// The grant is consumed (single-use) and the object is removed so no
	// orphan bytes survive a failed record creation.
	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusUsed, stored.Status)
	require.Contains(t, objectStore.Deletes(), grant.S3Bucket+"/"+grant.S3Key)
}

func TestFinalizeUploadGrantDeleteFailureStillFailsClosed(t *testing.T) {
	service, grantRepo, objectStore, mediaRepo := newUploadGrantTestService(t)
	ctx := context.Background()
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	wrong := []byte("wrong bytes")
	require.NoError(t, objectStore.SimulateUpload(grant, wrong))
	// Deletion failure must not mask the digest failure or admit the asset.
	objectStore.deleteErr = fmt.Errorf("delete failed")

	media, err := service.FinalizeUploadGrant(ctx, "alice", grant.GrantID)
	require.ErrorIs(t, err, ErrUploadGrantDigestMismatch)
	require.Nil(t, media)
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)
	stored, err := grantRepo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusFailedDigest, stored.Status)
}

func TestUploadGrantQueryPresignErrorReturnsStateWithoutURL(t *testing.T) {
	service, _, objectStore, _ := newUploadGrantTestService(t)
	ctx := context.Background()
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	objectStore.presignErr = fmt.Errorf("presign down")

	queried, url, err := service.UploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusMinted, queried.StatusClassification(time.Now().UTC()))
	require.Empty(t, url, "a presign failure must not hide the grant state")
}

func TestUploadGrantQueryStoreUnavailableReturnsStateWithoutURL(t *testing.T) {
	service, _, _, _ := newUploadGrantTestService(t)
	ctx := context.Background()
	grant, _, err := service.MintUploadGrant(ctx, MintUploadGrantInput{
		Owner: "alice", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, ContentSHA256: uploadGrantDigest(tinyPNG),
	})
	require.NoError(t, err)
	service.SetS3Service(newFakeMediaS3Service())

	queried, url, err := service.UploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusMinted, queried.StatusClassification(time.Now().UTC()))
	require.Empty(t, url)
}

func TestUploadGrantQueryUnwiredFailsClosed(t *testing.T) {
	service, _, _, _ := newUploadGrantTestService(t)
	service.SetUploadGrantRepository(nil)
	_, _, err := service.UploadGrant(context.Background(), "alice", "grant-x")
	require.ErrorIs(t, err, ErrUploadGrantUnavailable)
}

func TestExtensionForContentType(t *testing.T) {
	cases := map[string]string{
		"image/jpeg": ".jpg", "image/jpg": ".jpg", "image/png": ".png", "image/gif": ".gif",
		"image/webp": ".webp", "image/svg+xml": ".svg", "image/bmp": ".bmp", "image/tiff": ".tiff",
		"video/mp4": ".mp4", "video/webm": ".webm", "video/ogg": ".ogv", "video/avi": ".avi",
		"video/x-msvideo": ".avi", "video/mov": ".mov", "video/quicktime": ".mov",
		"audio/mpeg": ".mp3", "audio/mp3": ".mp3", "audio/wav": ".wav", "audio/x-wav": ".wav",
		"audio/ogg": ".oga", "audio/aac": ".aac", "audio/flac": ".flac",
		"application/octet-stream": "",
	}
	for contentType, expected := range cases {
		require.Equal(t, expected, extensionForContentType(contentType), "extension for %q", contentType)
	}
}
