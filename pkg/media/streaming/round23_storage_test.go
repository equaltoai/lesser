package streaming

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestS3MediaStorage_Paths(t *testing.T) {
	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	st := NewS3MediaStorage(newTestS3Client(srv.URL), "test-bucket", "us-east-1", db)

	assert.Equal(t, "media/m1/master.m3u8", st.GetManifestPath("m1", FormatHLS, ""))
	assert.Equal(t, "media/m1/720p/playlist.m3u8", st.GetManifestPath("m1", FormatHLS, Quality720p))
	assert.Equal(t, "media/m1/manifest.mpd", st.GetManifestPath("m1", FormatDASH, ""))
	assert.Equal(t, "", st.GetManifestPath("m1", "unknown", ""))

	assert.Equal(t, "media/m1/1080p/segment007.ts", st.GetSegmentPath("m1", Quality1080p, 7))
}

func TestS3MediaStorage_GetMediaMetadata_CacheAndErrors(t *testing.T) {
	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	st := NewS3MediaStorage(newTestS3Client(srv.URL), "test-bucket", "us-east-1", db)

	model := &models.MediaMetadata{
		MediaID:            "m1",
		OriginalURL:        "https://origin.example/video.mp4",
		Duration:           12.34,
		Width:              1920,
		Height:             1080,
		Bitrate:            4500,
		FileSize:           123,
		ProcessedAt:        time.Now(),
		AvailableQualities: []string{"720p", "1080p"},
		Status:             string(StatusComplete),
		VideoCodec:         "avc1.640028",
		AudioCodec:         "mp4a.40.2",
		QualitySettings: map[string]models.QualityCodecInfo{
			"1080p": {VideoCodec: "avc1.640028", AudioCodec: "mp4a.40.2", Bandwidth: 9000000, Width: 1920, Height: 1080},
		},
	}
	require.NoError(t, model.UpdateKeys())
	require.NoError(t, db.Model(model).Create())

	got1, err := st.GetMediaMetadata("m1")
	require.NoError(t, err)
	assert.Equal(t, "m1", got1.MediaID)
	assert.Equal(t, StatusComplete, got1.Status)
	assert.Equal(t, []Quality{Quality720p, Quality1080p}, got1.AvailableQualities)
	require.NotNil(t, got1.QualitySettings)
	assert.Equal(t, 9000000, got1.QualitySettings[Quality1080p].Bandwidth)

	// Change stored value; cache should still serve old result.
	model.Status = string(StatusFailed)
	require.NoError(t, db.Model(model).Create()) // Overwrite in DB map
	got2, err := st.GetMediaMetadata("m1")
	require.NoError(t, err)
	assert.Equal(t, StatusComplete, got2.Status)

	// Expire cache manually and confirm fresh read.
	st.metaCache.Store("m1", &cachedMetadata{metadata: got2, cachedAt: time.Now().Add(-10 * time.Minute)})
	got3, err := st.GetMediaMetadata("m1")
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, got3.Status)

	_, err = st.GetMediaMetadata("missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMediaMetadataNotFound)

	st.metaCache.Delete("m1")
	db.forceFirstErr = errors.New("boom")
	_, err = st.GetMediaMetadata("m1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrGetMetadataFromDynamoDB)
}

func TestS3MediaStorage_S3Operations(t *testing.T) {
	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	st := NewS3MediaStorage(newTestS3Client(srv.URL), "test-bucket", "us-east-1", db)

	require.NoError(t, st.SaveManifest("m1", FormatHLS, Quality(""), "#EXTM3U\n#EXT-X-VERSION:6\n"))
	ok, err := st.ManifestExists("m1", FormatHLS)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = st.ManifestExists("does-not-exist", FormatHLS)
	require.NoError(t, err)
	assert.False(t, ok)

	// Segment info and listing
	mem.put("media/m1/720p/segment000.ts", []byte("seg0"))
	mem.put("media/m1/720p/segment002.ts", []byte("seg2"))
	mem.put("media/m1/720p/not-a-segment.txt", []byte("x"))

	info, err := st.GetSegmentInfo("m1", Quality720p, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, info.Index)
	assert.Equal(t, int64(len("seg0")), info.Size)
	assert.Contains(t, info.URL, "media/m1/720p/segment000.ts")

	segments, err := st.ListSegments("m1", Quality720p)
	require.NoError(t, err)
	require.Len(t, segments, 2)
	assert.Equal(t, 0, segments[0].Index)
	assert.Equal(t, 2, segments[1].Index)
}

func TestS3MediaStorage_GetKeyframeData_Fallbacks(t *testing.T) {
	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	st := NewS3MediaStorage(newTestS3Client(srv.URL), "test-bucket", "us-east-1", db)

	// No keyframes.json, but iframe playlist exists.
	iframe := "#EXTM3U\n#EXT-X-I-FRAMES-ONLY\n"
	mem.put("media/m1/720p/iframe.m3u8", []byte(iframe))

	data, err := st.GetKeyframeData("m1", Quality720p)
	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Equal(t, iframe, string(data))

	// Neither file exists -> nil data, nil error.
	data, err = st.GetKeyframeData("m2", Quality720p)
	require.NoError(t, err)
	assert.Nil(t, data)

	// Non-notfound errors should be surfaced
	errorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "boom")
	}))
	t.Cleanup(errorSrv.Close)

	errorClient := newTestS3Client(errorSrv.URL)
	errorStorage := NewS3MediaStorage(errorClient, "test-bucket", "us-east-1", db)
	_, err = errorStorage.GetKeyframeData("m3", Quality720p)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFailedToGetKeyframeData)
}

func TestS3MediaStorage_UpdateMediaMetadata_CreateThenUpdate(t *testing.T) {
	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	st := NewS3MediaStorage(newTestS3Client(srv.URL), "test-bucket", "us-east-1", db)

	meta := &MediaMetadata{
		MediaID:            "m1",
		Status:             StatusPending,
		ProcessedAt:        time.Now(),
		AvailableQualities: []Quality{Quality720p},
	}
	require.NoError(t, st.UpdateMediaMetadata("m1", meta))

	got, err := st.GetMediaMetadata("m1")
	require.NoError(t, err)
	assert.Equal(t, StatusPending, got.Status)

	// Populate cache, then update and confirm cache invalidation.
	_, err = st.GetMediaMetadata("m1")
	require.NoError(t, err)

	meta.Status = StatusComplete
	require.NoError(t, st.UpdateMediaMetadata("m1", meta))

	got, err = st.GetMediaMetadata("m1")
	require.NoError(t, err)
	assert.Equal(t, StatusComplete, got.Status)

	// Update error branch
	db.forceUpdateErr = errors.New("boom")
	err = st.UpdateMediaMetadata("m1", meta)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUpdateMetadataInDynamoDB)

	// Create error branch (update not found => create)
	db.forceUpdateErr = nil
	db.forceCreateErr = errors.New("create boom")
	err = st.UpdateMediaMetadata("m2", meta)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCreateMetadataInDynamoDB)
}

func TestS3MediaStorage_CreateMediaStructure_AndPresign(t *testing.T) {
	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	st := NewS3MediaStorage(newTestS3Client(srv.URL), "test-bucket", "us-east-1", db)

	require.NoError(t, st.CreateMediaStructure("m1", []Quality{Quality720p, Quality1080p}))

	// Structure artifacts written
	_, ok := mem.get("media/m1/720p/index.json")
	assert.True(t, ok)
	_, ok = mem.get("media/m1/720p/segments/.processing_ready")
	assert.True(t, ok)
	_, ok = mem.get("media/m1/master_index.json")
	assert.True(t, ok)

	got, err := st.GetMediaMetadata("m1")
	require.NoError(t, err)
	assert.Equal(t, StatusPending, got.Status)
	assert.ElementsMatch(t, []Quality{Quality720p, Quality1080p}, got.AvailableQualities)

	uploadURL, err := st.GetPresignedUploadURL("m1", "video.mp4")
	require.NoError(t, err)
	assert.Contains(t, uploadURL, "media/m1/original/video.mp4")
}

func TestS3MediaStorage_CloudFrontInitialization(t *testing.T) {
	t.Cleanup(config.ResetForTests)

	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	st := &S3MediaStorage{
		client: newTestS3Client(srv.URL),
		bucket: "test-bucket",
		region: "us-east-1",
		db:     db,
		logger: zap.NewNop(),
	}

	t.Run("NotConfigured", func(t *testing.T) {
		config.ResetForTests()
		t.Setenv("CLOUDFRONT_DOMAIN", "")
		t.Setenv("CLOUDFRONT_KEY_PAIR_ID", "")
		t.Setenv("CLOUDFRONT_PRIVATE_KEY_PATH", "")

		err := st.initializeCloudFront()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCloudFrontNotConfigured)
	})

	t.Run("InvalidPath", func(t *testing.T) {
		config.ResetForTests()
		t.Setenv("CLOUDFRONT_DOMAIN", "d123.cloudfront.net")
		t.Setenv("CLOUDFRONT_KEY_PAIR_ID", "K123")
		t.Setenv("CLOUDFRONT_PRIVATE_KEY_PATH", "relative.pem")

		err := st.initializeCloudFront()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidCloudFrontPrivateKeyPath)
	})

	t.Run("FileLoadAndSign", func(t *testing.T) {
		config.ResetForTests()

		privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
		require.NotNil(t, pemBytes)

		tmp := filepath.Join(t.TempDir(), "cf.pem")
		require.NoError(t, os.WriteFile(tmp, pemBytes, 0o600))

		t.Setenv("CLOUDFRONT_DOMAIN", "d123.cloudfront.net")
		t.Setenv("CLOUDFRONT_KEY_PAIR_ID", "K123")
		t.Setenv("CLOUDFRONT_PRIVATE_KEY_PATH", tmp)

		err = st.initializeCloudFront()
		require.NoError(t, err)
		assert.True(t, st.IsCloudFrontEnabled())
		assert.Equal(t, "d123.cloudfront.net", st.GetCloudFrontDomain())

		signed := st.generateCloudFrontURL("media/m1/720p/segment000.ts")
		assert.Contains(t, signed, "d123.cloudfront.net/media/m1/720p/segment000.ts")
		assert.Contains(t, signed, "Key-Pair-Id=K123")

		// Invalid URL should fall back to S3 URL
		fallback := st.generateCloudFrontURL("bad%ZZ")
		assert.Contains(t, fallback, "s3.us-east-1.amazonaws.com")
	})

	t.Run("SecretsManagerJSONPrivateKey", func(t *testing.T) {
		config.ResetForTests()

		privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
		require.NoError(t, err)
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
		require.NotNil(t, pemBytes)

		secretServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = r.Body.Close()
			w.Header().Set("Content-Type", "application/x-amz-json-1.1")

			payload, _ := json.Marshal(map[string]any{
				"ARN":          "test",
				"Name":         "test",
				"SecretString": fmt.Sprintf("{\"privateKey\":%q}", string(pemBytes)),
				"VersionId":    "1",
			})
			_, _ = w.Write(payload)
		}))
		t.Cleanup(secretServer.Close)

		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
		t.Setenv("AWS_SESSION_TOKEN", "TOKEN")
		t.Setenv("AWS_ENDPOINT_URL_SECRETS_MANAGER", secretServer.URL)

		t.Setenv("CLOUDFRONT_DOMAIN", "d456.cloudfront.net")
		t.Setenv("CLOUDFRONT_KEY_PAIR_ID", "K456")
		t.Setenv("CLOUDFRONT_PRIVATE_KEY_PATH", "lesser/cloudfront-private-key")

		err = st.initializeCloudFront()
		require.NoError(t, err)
		assert.True(t, st.IsCloudFrontEnabled())
	})

	t.Run("PrivateKeyNotProvided", func(t *testing.T) {
		config.ResetForTests()
		t.Setenv("CLOUDFRONT_DOMAIN", "d123.cloudfront.net")
		t.Setenv("CLOUDFRONT_KEY_PAIR_ID", "K123")
		t.Setenv("CLOUDFRONT_PRIVATE_KEY_PATH", "")

		err := st.initializeCloudFront()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCloudFrontPrivateKeyNotProvided)
	})

	t.Run("InvalidPEMType", func(t *testing.T) {
		config.ResetForTests()
		tmp := filepath.Join(t.TempDir(), "cf.pem")
		require.NoError(t, os.WriteFile(tmp, []byte("-----BEGIN PRIVATE KEY-----\nABC\n-----END PRIVATE KEY-----\n"), 0o600))

		t.Setenv("CLOUDFRONT_DOMAIN", "d123.cloudfront.net")
		t.Setenv("CLOUDFRONT_KEY_PAIR_ID", "K123")
		t.Setenv("CLOUDFRONT_PRIVATE_KEY_PATH", tmp)

		err := st.initializeCloudFront()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidRSAPrivateKeyPEM)
	})

	t.Run("ParseError", func(t *testing.T) {
		config.ResetForTests()
		tmp := filepath.Join(t.TempDir(), "cf.pem")
		require.NoError(t, os.WriteFile(tmp, []byte("-----BEGIN RSA PRIVATE KEY-----\nAAA=\n-----END RSA PRIVATE KEY-----\n"), 0o600))

		t.Setenv("CLOUDFRONT_DOMAIN", "d123.cloudfront.net")
		t.Setenv("CLOUDFRONT_KEY_PAIR_ID", "K123")
		t.Setenv("CLOUDFRONT_PRIVATE_KEY_PATH", tmp)

		err := st.initializeCloudFront()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFailedToParseRSAPrivateKey)
	})
}

func TestS3MediaStorage_GenerateCloudFrontURL_FallbackWhenSignerNil(t *testing.T) {
	st := &S3MediaStorage{
		bucket: "b",
		region: "us-east-1",
	}
	got := st.generateCloudFrontURL("media/x")
	assert.Equal(t, "https://b.s3.us-east-1.amazonaws.com/media/x", got)
}

func TestS3MediaStorage_GetSegmentURL_CloudFrontEnabled(t *testing.T) {
	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	require.NotNil(t, pemBytes)

	tmp := filepath.Join(t.TempDir(), "cf.pem")
	require.NoError(t, os.WriteFile(tmp, pemBytes, 0o600))

	config.ResetForTests()
	t.Cleanup(config.ResetForTests)
	t.Setenv("CLOUDFRONT_DOMAIN", "d789.cloudfront.net")
	t.Setenv("CLOUDFRONT_KEY_PAIR_ID", "K789")
	t.Setenv("CLOUDFRONT_PRIVATE_KEY_PATH", tmp)

	st := NewS3MediaStorage(newTestS3Client(srv.URL), "test-bucket", "us-east-1", db)
	require.True(t, st.IsCloudFrontEnabled())

	url := st.getSegmentURL("media/m1/720p/segment000.ts")
	assert.Contains(t, url, "d789.cloudfront.net/media/m1/720p/segment000.ts")
}

func TestS3MediaStorage_GetSecretFromSecretsManager_Errors(t *testing.T) {
	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	st := NewS3MediaStorage(newTestS3Client(srv.URL), "test-bucket", "us-east-1", db)

	// Force endpoint to a server that returns invalid JSON.
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = fmt.Fprint(w, "not-json")
	}))
	t.Cleanup(badServer.Close)

	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
	t.Setenv("AWS_SESSION_TOKEN", "TOKEN")
	t.Setenv("AWS_ENDPOINT_URL_SECRETS_MANAGER", badServer.URL)

	_, err := st.getSecretFromSecretsManager("lesser/secret")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "failed to retrieve secret") || strings.Contains(err.Error(), "deserialization"))

	// Remove region and ensure LoadDefaultConfig fails.
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_ENDPOINT_URL_SECRETS_MANAGER", "")
	_, err = st.getSecretFromSecretsManager("lesser/secret")
	require.Error(t, err)
}

func TestS3MediaStorage_HelperConversionsAndParsing(t *testing.T) {
	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	st := NewS3MediaStorage(newTestS3Client(srv.URL), "test-bucket", "us-east-1", db)

	assert.Equal(t, "application/vnd.apple.mpegurl", st.getManifestContentType(FormatHLS))
	assert.Equal(t, "application/dash+xml", st.getManifestContentType(FormatDASH))
	assert.Equal(t, "text/plain", st.getManifestContentType("unknown"))

	assert.Equal(t, 1, st.extractSegmentIndex("media/m1/720p/segment001.ts"))
	assert.Equal(t, -1, st.extractSegmentIndex("media/m1/720p/not-a-segment.txt"))
	assert.Equal(t, -1, st.extractSegmentIndex("media/m1/720p/segmentABC.ts"))

	assert.Equal(t, `["720p", "1080p"]`, st.formatQualitiesJSON([]Quality{Quality720p, Quality1080p}))

	assert.Equal(t, []Quality{Quality720p}, convertQualities([]string{"720p"}))
	assert.Equal(t, []string{"720p"}, convertQualitiesFromStreaming([]Quality{Quality720p}))

	qs := map[Quality]QualityCodecInfo{
		Quality720p: {VideoCodec: "avc1", AudioCodec: "aac", Bandwidth: 1000, Width: 1, Height: 2},
	}
	converted := convertQualitySettingsFromStreaming(qs)
	assert.Equal(t, "avc1", converted["720p"].VideoCodec)
	assert.Equal(t, qs, convertQualitySettings(converted))
	assert.Nil(t, convertQualitySettingsFromStreaming(nil))
	assert.Nil(t, convertQualitySettings(nil))
}

func TestS3MediaStorage_GetKeyframeData_PrefersExplicitKeyframesFile(t *testing.T) {
	db := newFakeDynamormDB()
	mem := newS3Memory()
	srv := newTestS3Server(t, "test-bucket", mem)
	t.Cleanup(srv.Close)

	st := NewS3MediaStorage(newTestS3Client(srv.URL), "test-bucket", "us-east-1", db)

	mem.put("media/m1/720p/keyframes.json", []byte(`{"keyframes":[{"pts":0}]}`))
	mem.put("media/m1/720p/iframe.m3u8", []byte("#EXTM3U\n"))

	data, err := st.GetKeyframeData("m1", Quality720p)
	require.NoError(t, err)
	assert.Contains(t, string(data), "keyframes")
}
