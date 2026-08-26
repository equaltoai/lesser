package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	mediasvc "github.com/equaltoai/lesser/pkg/services/media"
	"github.com/stretchr/testify/require"
)

// s3PresignTestServer emulates the S3 enforcement surface that matters for the
// presigned-companion PUT contract: every header listed in the URL's
// X-Amz-SignedHeaders (except host, which HTTP always sends) MUST be present
// and non-empty on the request, or S3 rejects the PUT with
// 403 SignatureDoesNotMatch. Real S3 additionally recomputes the SigV4
// signature over those headers' exact values — the presence+non-empty check is
// the observable contract of that enforcement, which is what the acceptance
// test reproduces: a naive client that omits (or empties) the signed SSE
// headers fails exactly as it does in production.
type s3PresignTestServer struct {
	server       *httptest.Server
	mu           sync.Mutex
	forbidden    int
	accepted     int
	seenRequests []string // "METHOD path?query" of every request, for diagnostics
}

func newS3PresignTestServer(t *testing.T) *s3PresignTestServer {
	t.Helper()
	ts := &s3PresignTestServer{}
	ts.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		ts.seenRequests = append(ts.seenRequests, r.Method+" "+r.URL.RequestURI())
		ts.mu.Unlock()
		signedHeaders := parseAmzSignedHeaders(r.URL.Query().Get("X-Amz-SignedHeaders"))
		for _, name := range signedHeaders {
			if name == "host" {
				continue
			}
			if r.Header.Get(name) == "" {
				ts.mu.Lock()
				ts.forbidden++
				ts.mu.Unlock()
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `<Error><Code>SignatureDoesNotMatch</Code><Message>The request signature we calculated does not match the signature you provided. Check your key and signing method.</Message></Error>`)
				return
			}
		}
		ts.mu.Lock()
		ts.accepted++
		ts.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.server.Close)
	return ts
}

// parseAmzSignedHeaders decodes the URL-encoded X-Amz-SignedHeaders value into
// its header names.
func parseAmzSignedHeaders(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ";")
}

func TestMediaS3PresignedPut_SSESignedHeadersRequireEcho(t *testing.T) {
	srv := newS3PresignTestServer(t)

	// A real SDK v2 S3 client + presigner pointed at the emulation, exactly as
	// the registry wires it (wireMediaStorageDependencies), with static test
	// credentials: the minted URL is a genuine SigV4 presigned PUT.
	client := s3.NewFromConfig(aws.Config{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("test-access-key", "test-secret-key", ""),
		BaseEndpoint: aws.String(srv.server.URL),
	}, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	store := &mediaS3ObjectStore{client: client, presigner: s3.NewPresignClient(client)}

	body := []byte("presigned-companion acceptance bytes")
	sum := sha256.Sum256(body)
	declaredSHA256 := hex.EncodeToString(sum[:])
	const kmsKeyID = "alias/theory-acceptance-encryption"

	// Mint the presigned PUT the editorial lane mints (SSE-KMS headers signed
	// into the signature): the old signed-header shape is unchanged by the fix,
	// and the URL must declare both SSE headers as signed.
	presignedURL, err := store.PresignPutObject(
		context.Background(), "lesser-media-bucket", "media/2026/08/26/acceptance.png",
		"image/png", declaredSHA256, kmsKeyID, 15*time.Minute,
	)
	require.NoError(t, err)
	require.NotEmpty(t, presignedURL)

	parsed, err := url.Parse(presignedURL)
	require.NoError(t, err)
	signedHeaders := parseAmzSignedHeaders(parsed.Query().Get("X-Amz-SignedHeaders"))
	require.Contains(t, signedHeaders, mediasvc.UploadGrantSSEEncryptionHeader,
		"the minted PUT must sign x-amz-server-side-encryption (SSE-KMS enforcement)")
	require.Contains(t, signedHeaders, mediasvc.UploadGrantSSEKMSKeyIDHeader,
		"the minted PUT must sign x-amz-server-side-encryption-aws-kms-key-id (SSE-KMS enforcement)")

	// The naive client PUTs the exact declared bytes with only Content-Type —
	// no SSE headers — exactly what production clients did. S3 rejects it with
	// 403 SignatureDoesNotMatch: reproduce the production failure.
	naiveReq, err := http.NewRequest(http.MethodPut, presignedURL, bytes.NewReader(body))
	require.NoError(t, err)
	naiveReq.Header.Set("Content-Type", "image/png")
	naiveResp, err := http.DefaultClient.Do(naiveReq)
	require.NoError(t, err)
	defer naiveResp.Body.Close()
	naiveBody, err := io.ReadAll(naiveResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, naiveResp.StatusCode,
		"a PUT omitting the signed SSE headers must fail closed like production (403 SignatureDoesNotMatch)")
	require.Contains(t, string(naiveBody), "SignatureDoesNotMatch")

	// The compliant client echoes the grant contract: the two signed headers
	// with the exact values the grant response carries (media.UploadGrantSSEAlgorithm
	// for the algorithm and the instance KMS key id for the key). The PUT lands.
	echoReq, err := http.NewRequest(http.MethodPut, presignedURL, bytes.NewReader(body))
	require.NoError(t, err)
	echoReq.Header.Set("Content-Type", "image/png")
	echoReq.Header.Set(mediasvc.UploadGrantSSEEncryptionHeader, mediasvc.UploadGrantSSEAlgorithm)
	echoReq.Header.Set(mediasvc.UploadGrantSSEKMSKeyIDHeader, kmsKeyID)
	echoResp, err := http.DefaultClient.Do(echoReq)
	require.NoError(t, err)
	defer echoResp.Body.Close()
	require.Equal(t, http.StatusOK, echoResp.StatusCode,
		"a PUT echoing the signed SSE headers with the exact grant values must succeed")

	// The naive request must not have been accepted; the echo request must have.
	require.Equal(t, 1, srv.forbidden, "exactly one naive PUT rejected (production 403 repro)")
	require.Equal(t, 1, srv.accepted, "exactly one compliant PUT accepted")
}
