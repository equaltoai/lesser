package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	// via the hoisted x-amz-checksum-sha256 query parameter), so the server-side
	// recomputation must use the same sentinel or every request would mismatch.
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
// exactly like production.
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

// TestMediaS3PresignedPut_SignsOnlyHost pins the fixed presigned-companion PUT
// contract (issue #1509): the minted URL signs only the host header, carries no
// SSE query parameters, and still binds the declared sha256 as the hoisted
// X-Amz-Checksum-Sha256 query parameter. A client holding nothing but the URL
// completes the PUT with Content-Type plus the declared bytes — no SSE headers
// to echo — and the signature contract accepts it. The SSE-KMS headers that the
// old presign signed are undiscoverable by clients (the KMS key ARN is never
// disclosed), which made every client PUT fail 403 SignatureDoesNotMatch; this
// pin asserts the regression cannot silently return.
func TestMediaS3PresignedPut_SignsOnlyHost(t *testing.T) {
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

	// Mint the presigned PUT the editorial lane mints: no SSE parameters.
	presignedURL, err := store.PresignPutObject(
		context.Background(), "lesser-media-bucket", "media/2026/08/29/acceptance.png",
		"image/png", declaredSHA256, 15*time.Minute,
	)
	require.NoError(t, err)
	require.NotEmpty(t, presignedURL)

	parsed, err := url.Parse(presignedURL)
	require.NoError(t, err)
	query := parsed.Query()

	// Regression pin: the URL signs exactly the host header. Any other signed
	// header would force clients to echo an undiscoverable value.
	require.Equal(t, "host", query.Get("X-Amz-SignedHeaders"),
		"the minted PUT must sign exactly the host header (X-Amz-SignedHeaders=host)")
	// The URL must carry no SSE query parameters: neither the algorithm nor the
	// KMS key id headers may re-enter the signed surface in any casing.
	for key := range query {
		require.NotContains(t, strings.ToLower(key), "server-side-encryption",
			"the minted PUT must carry no SSE query parameters (key %q)", key)
	}

	// The integrity binding survives: the declared sha256 is hoisted into the
	// URL as the base64 x-amz-checksum-sha256 query parameter, so S3 validates
	// the body on receipt (and finalize recomputes the digest over the stored
	// bytes as defense-in-depth).
	expectedChecksum := base64.StdEncoding.EncodeToString(sum[:])
	require.Equal(t, expectedChecksum, query.Get("X-Amz-Checksum-Sha256"),
		"the declared sha256 must remain bound into the URL as X-Amz-Checksum-Sha256")

	// A client holding nothing but the URL completes the PUT: Content-Type plus
	// the exact declared bytes, no SSE headers. The signature contract accepts
	// it — this is the flow that was impossible before the fix.
	plainReq, err := http.NewRequest(http.MethodPut, presignedURL, bytes.NewReader(body))
	require.NoError(t, err)
	plainReq.Header.Set("Content-Type", "image/png")
	plainResp, err := http.DefaultClient.Do(plainReq)
	require.NoError(t, err)
	defer plainResp.Body.Close()
	plainBody, err := io.ReadAll(plainResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, plainResp.StatusCode,
		"a PUT with only Content-Type and the declared bytes must succeed against the signature contract")
	require.NotContains(t, string(plainBody), "SignatureDoesNotMatch")

	// Client-added unsigned headers (Cache-Control, x-amz-meta-*) must not
	// invalidate the signature: the URL signs only host, so extra headers are
	// outside the canonical signed surface.
	extraReq, err := http.NewRequest(http.MethodPut, presignedURL, bytes.NewReader(body))
	require.NoError(t, err)
	extraReq.Header.Set("Content-Type", "image/png")
	extraReq.Header.Set("Cache-Control", "max-age=0")
	extraReq.Header.Set("X-Amz-Meta-Client", "acceptance")
	extraResp, err := http.DefaultClient.Do(extraReq)
	require.NoError(t, err)
	defer extraResp.Body.Close()
	require.Equal(t, http.StatusOK, extraResp.StatusCode,
		"client-added unsigned headers must not invalidate the host-only signature")

	// Every PUT landed; nothing was rejected.
	require.Equal(t, 0, srv.forbidden, "no PUT may be rejected: the URL is self-sufficient")
	require.Equal(t, 2, srv.accepted, "both compliant PUTs accepted")
}
