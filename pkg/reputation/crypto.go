package reputation

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/jsonld"
	"github.com/equaltoai/lesser/pkg/ssrf"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// Signer handles cryptographic signing of reputation documents
type Signer struct {
	privateKey  ed25519.PrivateKey
	publicKey   ed25519.PublicKey
	instanceURL string
	logger      *zap.Logger
}

// NewSigner creates a new reputation signer
func NewSigner(privateKeyPEM string, instanceURL string, logger *zap.Logger) (*Signer, error) {
	var privateKey ed25519.PrivateKey
	var publicKey ed25519.PublicKey

	if privateKeyPEM != "" {
		// Parse PEM-encoded private key
		block, _ := pem.Decode([]byte(privateKeyPEM))
		if block == nil {
			return nil, fmt.Errorf("failed to parse PEM block containing private key")
		}

		// Check for different PEM types
		switch block.Type {
		case "PRIVATE KEY":
			// PKCS#8 format
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse PKCS#8 private key: %w", err)
			}

			ed25519Key, ok := key.(ed25519.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("private key is not an Ed25519 key")
			}
			privateKey = ed25519Key
			publicKey = ed25519Key.Public().(ed25519.PublicKey)

		case "ED25519 PRIVATE KEY":
			// Raw Ed25519 format (32 bytes for private key)
			if len(block.Bytes) != ed25519.PrivateKeySize {
				return nil, fmt.Errorf("invalid Ed25519 private key size: expected %d, got %d",
					ed25519.PrivateKeySize, len(block.Bytes))
			}
			privateKey = ed25519.PrivateKey(block.Bytes)
			publicKey = privateKey.Public().(ed25519.PublicKey)

		default:
			return nil, fmt.Errorf("unsupported PEM block type: %s", block.Type)
		}

		logger.Info("loaded Ed25519 key from PEM",
			zap.String("instance", instanceURL))
	} else {
		// Generate a new key pair
		logger.Warn("no private key provided, generating new Ed25519 key pair")
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate key pair: %w", err)
		}
		publicKey = pub
		privateKey = priv

		publicKeyFingerprint := sha256.Sum256(publicKey)
		logger.Info("generated new Ed25519 key pair",
			zap.String("instance", instanceURL),
			zap.String("public_key_fingerprint", base64.RawStdEncoding.EncodeToString(publicKeyFingerprint[:])))
	}

	return &Signer{
		privateKey:  privateKey,
		publicKey:   publicKey,
		instanceURL: instanceURL,
		logger:      logger,
	}, nil
}

// SignReputation signs a reputation document
func (s *Signer) SignReputation(rep *Reputation) error {
	// Set public key
	rep.PublicKey = base64.StdEncoding.EncodeToString(s.publicKey)

	// Clear any existing signature
	rep.Signature = ""

	// Canonicalize the reputation data
	canonical, err := canonicalizeJSON(rep)
	if err != nil {
		return fmt.Errorf("failed to canonicalize: %w", err)
	}

	// Create signature
	hash := sha256.Sum256(canonical)
	signature := ed25519.Sign(s.privateKey, hash[:])

	// Set signature
	rep.Signature = base64.StdEncoding.EncodeToString(signature)

	s.logger.Debug("Signed reputation",
		zap.String("actor", rep.ActorID),
		zap.String("signature", rep.Signature[:20]+"..."))

	return nil
}

// SignVouch signs a vouch document
func (s *Signer) SignVouch(vouch *Vouch) error {
	// Clear any existing signature
	vouch.Signature = ""

	// Canonicalize the vouch data
	canonical, err := canonicalizeJSON(vouch)
	if err != nil {
		return fmt.Errorf("failed to canonicalize: %w", err)
	}

	// Create signature
	hash := sha256.Sum256(canonical)
	signature := ed25519.Sign(s.privateKey, hash[:])

	// Set signature
	vouch.Signature = base64.StdEncoding.EncodeToString(signature)

	return nil
}

// SignPortableReputation signs a complete portable reputation document
func (s *Signer) SignPortableReputation(pr *PortableReputation) error {
	// Set issuer information
	pr.Issuer = s.instanceURL
	pr.IssuedAt = time.Now()
	pr.ExpiresAt = time.Now().Add(30 * 24 * time.Hour) // 30 days validity

	// Sign the reputation component
	if pr.Reputation != nil {
		if err := s.SignReputation(pr.Reputation); err != nil {
			return fmt.Errorf("failed to sign reputation: %w", err)
		}
	}

	// Sign each vouch
	for i := range pr.Vouches {
		if err := s.SignVouch(&pr.Vouches[i]); err != nil {
			return fmt.Errorf("failed to sign vouch %d: %w", i, err)
		}
	}

	// Create issuer proof (sign the whole document)
	pr.IssuerProof = ""
	canonical, err := canonicalizeJSON(pr)
	if err != nil {
		return fmt.Errorf("failed to canonicalize portable reputation: %w", err)
	}

	hash := sha256.Sum256(canonical)
	signature := ed25519.Sign(s.privateKey, hash[:])
	pr.IssuerProof = base64.StdEncoding.EncodeToString(signature)

	return nil
}

// GetPublicKeyBase64 returns the base64-encoded public key
func (s *Signer) GetPublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(s.publicKey)
}

// Verifier handles verification of reputation documents
type Verifier struct {
	instanceURL string
	logger      *zap.Logger
	httpClient  *http.Client
	domainTrust domainTrustRepository
	// Cache of known instance public keys
	keyCache map[string]ed25519.PublicKey
}

const verifierMaxRedirects = 10

func verifierCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= verifierMaxRedirects {
		return fmt.Errorf("too many redirects: %d", len(via))
	}
	if err := ssrf.ValidateURL(req.URL); err != nil {
		return fmt.Errorf("redirect URL blocked: %w", err)
	}
	return nil
}

func newSSRFProtectedHTTPClient(logger *zap.Logger) *http.Client {
	if logger == nil {
		logger = zap.NewNop()
	}

	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	transport := base.Clone()
	transport.Proxy = nil

	dialer := &net.Dialer{}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid dial address: %w", err)
		}

		if ip := net.ParseIP(host); ip != nil {
			if ssrf.IsBlockedIP(ip) {
				logger.Warn("blocked dial to private IP", zap.String("ip", ip.String()))
				return nil, fmt.Errorf("blocked dial to private IP: %s", ip.String())
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}

		if ssrf.IsBlockedHostname(host) {
			logger.Warn("blocked dial to internal hostname", zap.String("host", host))
			return nil, fmt.Errorf("blocked dial to internal hostname: %s", host)
		}

		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("DNS resolution failed: %w", err)
		}
		for _, ip := range ips {
			if ssrf.IsBlockedIP(ip) {
				logger.Warn("blocked dial to private IP",
					zap.String("host", host),
					zap.String("ip", ip.String()))
				return nil, fmt.Errorf("blocked dial to private IP: %s", ip.String())
			}
		}

		var lastErr error
		for _, ip := range ips {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}

		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("DNS resolution returned no IPs for %s", host)
	}

	return &http.Client{
		Timeout:       10 * time.Second,
		Transport:     transport,
		CheckRedirect: verifierCheckRedirect,
	}
}

type domainTrustRepository interface {
	IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error)
	GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error)
}

// NewVerifier creates a new reputation verifier
func NewVerifier(instanceURL string, logger *zap.Logger, domainTrust domainTrustRepository) *Verifier {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Verifier{
		instanceURL: instanceURL,
		logger:      logger,
		httpClient:  newSSRFProtectedHTTPClient(logger),
		domainTrust: domainTrust,
		keyCache:    make(map[string]ed25519.PublicKey),
	}
}

// VerifyReputation verifies a reputation document's signature
func (v *Verifier) VerifyReputation(rep *Reputation) (bool, error) {
	// Get the public key
	publicKeyBytes, err := base64.StdEncoding.DecodeString(rep.PublicKey)
	if err != nil {
		return false, fmt.Errorf("invalid public key encoding: %w", err)
	}

	publicKey := ed25519.PublicKey(publicKeyBytes)

	// Get and clear signature
	signatureB64 := rep.Signature
	rep.Signature = ""
	defer func() { rep.Signature = signatureB64 }()

	// Decode signature
	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return false, fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Canonicalize and verify
	canonical, err := canonicalizeJSON(rep)
	if err != nil {
		return false, fmt.Errorf("failed to canonicalize: %w", err)
	}

	hash := sha256.Sum256(canonical)
	valid := ed25519.Verify(publicKey, hash[:], signature)

	v.logger.Debug("Verified reputation signature",
		zap.String("actor", rep.ActorID),
		zap.Bool("valid", valid))

	return valid, nil
}

// VerifyPortableReputation verifies a complete portable reputation document
func (v *Verifier) VerifyPortableReputation(pr *PortableReputation) (*VerificationResult, error) {
	result := &VerificationResult{
		ActorID:   pr.Actor,
		Issuer:    pr.Issuer,
		IssuedAt:  pr.IssuedAt,
		ExpiresAt: pr.ExpiresAt,
	}

	// Check expiration
	result.NotExpired = time.Now().Before(pr.ExpiresAt)
	if !result.NotExpired {
		result.Error = "reputation document has expired"
		return result, nil
	}

	// Check if issuer is trusted (and safe to fetch from)
	trustedIssuerURL, ok := v.trustedInstanceURLForKeyFetch(pr.Issuer)
	result.IssuerTrusted = ok
	if !ok {
		result.Error = "issuer is not trusted"
		return result, nil
	}

	// Verify issuer proof
	issuerProof := pr.IssuerProof
	pr.IssuerProof = ""
	defer func() { pr.IssuerProof = issuerProof }()

	proofBytes, err := base64.StdEncoding.DecodeString(issuerProof)
	if err != nil {
		result.Error = "invalid issuer proof encoding"
		return result, nil
	}

	// Get issuer's public key
	publicKey, err := v.getInstancePublicKey(context.Background(), trustedIssuerURL)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get issuer public key: %v", err)
		return result, nil
	}

	canonical, err := canonicalizeJSON(pr)
	if err != nil {
		result.Error = fmt.Sprintf("failed to canonicalize: %v", err)
		return result, nil
	}

	hash := sha256.Sum256(canonical)
	result.SignatureValid = ed25519.Verify(publicKey, hash[:], proofBytes)

	// Verify individual reputation signature
	if pr.Reputation != nil {
		repValid, err := v.VerifyReputation(pr.Reputation)
		if err != nil || !repValid {
			result.SignatureValid = false
		}
	}

	result.Valid = result.SignatureValid && result.NotExpired && result.IssuerTrusted

	return result, nil
}

// getInstancePublicKey fetches the public key for an instance
func (v *Verifier) getInstancePublicKey(ctx context.Context, instanceURL string) (ed25519.PublicKey, error) {
	normalized, err := ssrf.ValidateURLString(strings.TrimSpace(instanceURL))
	if err != nil {
		return nil, fmt.Errorf("invalid instance URL: %w", err)
	}

	cacheKey := (&url.URL{
		Scheme: strings.ToLower(strings.TrimSpace(normalized.Scheme)),
		Host:   strings.ToLower(strings.TrimSpace(normalized.Host)),
	}).String()

	// Check cache
	if key, ok := v.keyCache[cacheKey]; ok {
		return key, nil
	}

	wellKnown := &url.URL{
		Scheme: strings.ToLower(strings.TrimSpace(normalized.Scheme)),
		Host:   strings.ToLower(strings.TrimSpace(normalized.Host)),
		Path:   "/.well-known/reputation-keys",
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Fetch from .well-known endpoint
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch public keys: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var keys struct {
		PublicKey string `json:"public_key"`
	}
	if err := common.ParseHTTPResponse(resp.Body, &keys); err != nil {
		return nil, fmt.Errorf("failed to decode keys: %w", err)
	}

	keyBytes, err := base64.StdEncoding.DecodeString(keys.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key encoding: %w", err)
	}

	publicKey := ed25519.PublicKey(keyBytes)
	v.keyCache[cacheKey] = publicKey

	return publicKey, nil
}

func (v *Verifier) trustedInstanceURLForKeyFetch(instanceURL string) (string, bool) {
	parsedURL, err := ssrf.ValidateURLString(strings.TrimSpace(instanceURL))
	if err != nil {
		v.logger.Error("failed to parse instance URL",
			zap.String("url", instanceURL),
			zap.Error(err))
		return "", false
	}

	domain := strings.ToLower(strings.TrimSpace(parsedURL.Hostname()))
	if domain == "" {
		return "", false
	}

	if v.domainTrust == nil {
		v.logger.Warn("no domain trust repository configured, rejecting by default",
			zap.String("domain", domain))
		return "", false
	}

	hostForAllow := domain
	port := strings.TrimSpace(parsedURL.Port())
	scheme := strings.ToLower(strings.TrimSpace(parsedURL.Scheme))
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		if strings.Contains(domain, ":") {
			hostForAllow = "[" + domain + "]:" + port
		} else {
			hostForAllow = domain + ":" + port
		}
	}

	ctx := context.Background()

	// First, check if domain is blocked
	isBlocked, block, err := v.domainTrust.IsDomainBlocked(ctx, domain)
	if err != nil {
		v.logger.Error("failed to check domain block",
			zap.String("domain", domain),
			zap.Error(err))
		// On error, default to not trusting
		return "", false
	}

	if isBlocked {
		if block != nil {
			v.logger.Info("domain is blocked",
				zap.String("domain", domain),
				zap.String("severity", block.Severity))
		} else {
			v.logger.Info("domain is blocked",
				zap.String("domain", domain))
		}
		return "", false
	}

	// For reputation key fetches, require explicit allow list entries.
	domainAllows, _, err := v.domainTrust.GetDomainAllows(ctx, 1000, "")
	if err != nil {
		v.logger.Error("failed to check domain allows",
			zap.String("domain", domain),
			zap.Error(err))
		return "", false
	}

	if len(domainAllows) == 0 {
		v.logger.Info("no domain allows configured; rejecting instance for key fetch",
			zap.String("domain", domain))
		return "", false
	}

	for _, allow := range domainAllows {
		if allow == nil {
			continue
		}
		allowed := strings.ToLower(strings.TrimSpace(allow.Domain))
		if allowed == hostForAllow {
			v.logger.Debug("domain is in allow list",
				zap.String("domain", domain))
			return (&url.URL{Scheme: "https", Host: allowed}).String(), true
		}
	}

	v.logger.Info("domain not in allow list",
		zap.String("domain", domain))
	return "", false
}

// canonicalizeJSON creates a canonical JSON representation using proper JSON-LD canonicalization
// This follows URDNA2015 algorithm for deterministic canonicalization suitable for cryptographic signatures
func canonicalizeJSON(v any) ([]byte, error) {
	// Use the new JSON-LD canonicalization with signature field removal
	canonical, err := jsonld.CanonicalizeStructToJSON(v, true)
	if err != nil {
		return nil, fmt.Errorf("JSON-LD canonicalization failed: %w", err)
	}

	return canonical, nil
}

// VerifyVouchSignature verifies a vouch's signature using the issuer's public key
func (v *Verifier) VerifyVouchSignature(vouch *Vouch) (bool, error) {
	trustedInstanceURL, ok := v.trustedInstanceURLForKeyFetch(vouch.InstanceURL)
	if !ok {
		return false, fmt.Errorf("issuer is not trusted")
	}

	// Get the issuer's public key from the instance
	publicKey, err := v.getInstancePublicKey(context.Background(), trustedInstanceURL)
	if err != nil {
		// If we can't get the public key from the instance, check if we have it embedded in the vouch
		// This would need to be added to the Vouch struct if not already there
		return false, fmt.Errorf("failed to get issuer public key: %w", err)
	}

	// Get and clear signature
	signatureB64 := vouch.Signature
	vouch.Signature = ""
	defer func() { vouch.Signature = signatureB64 }()

	// Decode signature
	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return false, fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Canonicalize and verify
	canonical, err := canonicalizeJSON(vouch)
	if err != nil {
		return false, fmt.Errorf("failed to canonicalize: %w", err)
	}

	hash := sha256.Sum256(canonical)
	valid := ed25519.Verify(publicKey, hash[:], signature)

	v.logger.Debug("Verified vouch signature",
		zap.String("vouch_id", vouch.ID),
		zap.String("from", vouch.From),
		zap.String("to", vouch.To),
		zap.Bool("valid", valid))

	return valid, nil
}
