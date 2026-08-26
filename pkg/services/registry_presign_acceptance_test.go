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
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	mediasvc "github.com/equaltoai/lesser/pkg/services/media"
	"github.com/stretchr/testify/require"
)

// Static credentials the acceptance client signs with and the emulation
// verifies against. They must stay in sync: the server recomputes the SigV4
// signature over every received PUT using exactly these values, so a wrong
// value here breaks every request.
const (
	s3PresignTestAccessKey = "test-access-key"
	s3PresignTestSecretKey = "test-secret-key"
	// s3PresignUnsignedPayload is the SigV4 payload-hash sentinel the SDK's S3
	// presigner binds into presigned PUT URLs (UNSIGNED-PAYLOAD): the signed URL
	// does not commit to a body hash (S3 validates the actual bytes separately
	// via x-amz-checksum-sha256), so the server-side recomputation must use the
	// same sentinel or every request would mismatch.
	s3PresignUnsignedPayload = "UNSIGNED-PAYLOAD"
)

// s3PresignTestServer emulates the S3 enforcement surface that matters for the
// presigned-companion PUT contract. Real S3 validates the PUT by recomputing
// the SigV4 signature from the static credentials over the received request's
// canonical form — every header listed in the URL's X-Amz-SignedHeaders
// (except host, which HTTP always sends) must be present AND carry the exact
// value that was signed, or S3 recomputes a different signature and rejects the
// PUT with 403 SignatureDoesNotMatch. This emulator does the same: it rejects a
// PUT that omits a signed header, then recomputes the signature server-side
// with the SDK's v4 signer (reconstructing the request exactly as the signer
// saw it — only the signed headers, with the client's echoed values) and
// compares it to the URL's X-Amz-Signature. Presence alone is not enough: a
// client that echoes the right header names with wrong values is rejected
// exactly like production, which is what the acceptance test below proves.
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
		if ts.requestSatisfiesSignedURL(r) {
			ts.mu.Lock()
			ts.accepted++
			ts.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		ts.mu.Lock()
		ts.forbidden++
		ts.mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<Error><Code>SignatureDoesNotMatch</Code><Message>The request signature we calculated does not match the signature you provided. Check your key and signing method.</Message></Error>`)
	}))
	t.Cleanup(ts.server.Close)
	return ts
}

// requestSatisfiesSignedURL reports whether the PUT satisfies the SigV4
// presigned-URL signature bound into its own URL: every header the URL declares
// signed (X-Amz-SignedHeaders) must be present, and the server-side
// recomputation of the signature over the received headers must equal the URL's
// X-Amz-Signature. This mirrors S3's enforcement and catches value drift — a
// client echoing the right header names with wrong values fails here exactly as
// it would against real S3.
func (ts *s3PresignTestServer) requestSatisfiesSignedURL(r *http.Request) bool {
	query := r.URL.Query()
	expectedSignature := query.Get("X-Amz-Signature")
	algorithm := query.Get("X-Amz-Algorithm")
	amzDate := query.Get("X-Amz-Date")
	credential := query.Get("X-Amz-Credential")
	signedHeaderNames := parseAmzSignedHeaders(query.Get("X-Amz-SignedHeaders"))
	if expectedSignature == "" || algorithm != "AWS4-HMAC-SHA256" || amzDate == "" || credential == "" || len(signedHeaderNames) == 0 {
		return false
	}
	// A signed header that the request omits cannot match what was signed; S3
	// rejects it as SignatureDoesNotMatch.
	for _, name := range signedHeaderNames {
		if name != "host" && r.Header.Get(name) == "" {
			return false
		}
	}

	// Reconstruct the request exactly as the signer saw it: only the signed
	// headers, carrying the values the client actually sent, against the URL
	// query minus X-Amz-Signature. Recomputing the presign over that request
	// reproduces the URL's X-Amz-Signature if and only if every signed header
	// echoes the exact signed value.
	reconstructedURL := *r.URL
	q := reconstructedURL.Query()
	q.Del("X-Amz-Signature")
	reconstructedURL.RawQuery = q.Encode()
	reconstructed, err := http.NewRequest(r.Method, reconstructedURL.String(), nil)
	if err != nil {
		return false
	}
	reconstructed.Host = r.Host
	for _, name := range signedHeaderNames {
		if name == "host" {
			continue
		}
		reconstructed.Header.Set(name, r.Header.Get(name))
	}
	signingTime, err := time.Parse("20060102T150405Z", amzDate)
	if err != nil {
		return false
	}
	// X-Amz-Credential: <access-key>/<date>/<region>/<service>/aws4_request.
	credentialParts := strings.Split(credential, "/")
	if len(credentialParts) != 5 || credentialParts[4] != "aws4_request" {
		return false
	}
	region, service := credentialParts[2], credentialParts[3]
	computedURL, _, err := v4.NewSigner().PresignHTTP(
		context.Background(),
		aws.Credentials{AccessKeyID: s3PresignTestAccessKey, SecretAccessKey: s3PresignTestSecretKey},
		reconstructed, s3PresignUnsignedPayload, service, region, signingTime,
	)
	if err != nil {
		return false
	}
	computed, err := url.Parse(computedURL)
	if err != nil {
		return false
	}
	return computed.Query().Get("X-Amz-Signature") == expectedSignature
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
		Credentials:  credentials.NewStaticCredentialsProvider(s3PresignTestAccessKey, s3PresignTestSecretKey, ""),
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

	// The value-drift control: the client echoes the right header NAMES but
	// wrong VALUES (a wrong algorithm and an attacker-chosen key id). Presence
	// is not enough — S3 recomputes the signature over the exact values and
	// rejects with 403 SignatureDoesNotMatch. This is the regression the
	// strengthened emulation exists to catch (algorithm constant change, wrong
	// key surfaced).
	wrongValueReq, err := http.NewRequest(http.MethodPut, presignedURL, bytes.NewReader(body))
	require.NoError(t, err)
	wrongValueReq.Header.Set("Content-Type", "image/png")
	wrongValueReq.Header.Set(mediasvc.UploadGrantSSEEncryptionHeader, "AES256")
	wrongValueReq.Header.Set(mediasvc.UploadGrantSSEKMSKeyIDHeader, "alias/attacker-chosen-key")
	wrongValueResp, err := http.DefaultClient.Do(wrongValueReq)
	require.NoError(t, err)
	defer wrongValueResp.Body.Close()
	wrongValueBody, err := io.ReadAll(wrongValueResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, wrongValueResp.StatusCode,
		"a PUT echoing the signed SSE header names with wrong values must fail like production (403 SignatureDoesNotMatch)")
	require.Contains(t, string(wrongValueBody), "SignatureDoesNotMatch")

	// The naive and wrong-value requests must not have been accepted; the echo
	// request must have been.
	require.Equal(t, 2, srv.forbidden, "exactly two PUTs rejected (naive omission + wrong-value echo)")
	require.Equal(t, 1, srv.accepted, "exactly one compliant PUT accepted")
}
