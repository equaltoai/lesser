package reputation

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/storage"
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

		// Log the generated private key in PEM format for future use
		privKeyBytes, _ := x509.MarshalPKCS8PrivateKey(privateKey)
		pemBlock := &pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privKeyBytes,
		}
		pemData := pem.EncodeToMemory(pemBlock)
		logger.Info("generated new Ed25519 key pair",
			zap.String("private_key_pem", string(pemData)))
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
	storage     storage.Storage
	// Cache of known instance public keys
	keyCache map[string]ed25519.PublicKey
}

// NewVerifier creates a new reputation verifier
func NewVerifier(instanceURL string, logger *zap.Logger, storage storage.Storage) *Verifier {
	return &Verifier{
		instanceURL: instanceURL,
		logger:      logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		storage:  storage,
		keyCache: make(map[string]ed25519.PublicKey),
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

	// Get issuer's public key
	publicKey, err := v.getInstancePublicKey(pr.Issuer)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get issuer public key: %v", err)
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

	// Check if issuer is trusted
	result.IssuerTrusted = v.isInstanceTrusted(pr.Issuer)

	result.Valid = result.SignatureValid && result.NotExpired && result.IssuerTrusted

	return result, nil
}

// getInstancePublicKey fetches the public key for an instance
func (v *Verifier) getInstancePublicKey(instanceURL string) (ed25519.PublicKey, error) {
	// Check cache
	if key, ok := v.keyCache[instanceURL]; ok {
		return key, nil
	}

	// Fetch from .well-known endpoint
	resp, err := v.httpClient.Get(instanceURL + "/.well-known/reputation-keys")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch public keys: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var keys struct {
		PublicKey string `json:"publicKey"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return nil, fmt.Errorf("failed to decode keys: %w", err)
	}

	keyBytes, err := base64.StdEncoding.DecodeString(keys.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key encoding: %w", err)
	}

	publicKey := ed25519.PublicKey(keyBytes)
	v.keyCache[instanceURL] = publicKey

	return publicKey, nil
}

// isInstanceTrusted checks if an instance is trusted
func (v *Verifier) isInstanceTrusted(instanceURL string) bool {
	// Extract domain from URL
	parsedURL, err := url.Parse(instanceURL)
	if err != nil {
		v.logger.Error("failed to parse instance URL",
			zap.String("url", instanceURL),
			zap.Error(err))
		return false
	}

	domain := parsedURL.Host
	// Remove port if present
	if idx := strings.IndexByte(domain, ':'); idx != -1 {
		domain = domain[:idx]
	}

	// Check if we have storage available
	if v.storage == nil {
		v.logger.Warn("no storage configured for trust checking, allowing by default")
		return true
	}

	ctx := context.Background()

	// First, check if domain is blocked
	isBlocked, block, err := v.storage.IsDomainBlocked(ctx, domain)
	if err != nil {
		v.logger.Error("failed to check domain block",
			zap.String("domain", domain),
			zap.Error(err))
		// On error, default to not trusting
		return false
	}

	if isBlocked {
		v.logger.Info("domain is blocked",
			zap.String("domain", domain),
			zap.String("severity", block.Severity))
		return false
	}

	// Check if we have any domain allows configured (allow-list mode)
	domainAllows, _, err := v.storage.GetDomainAllows(ctx, 1, "")
	if err != nil {
		v.logger.Error("failed to check domain allows",
			zap.String("domain", domain),
			zap.Error(err))
		// On error checking allows, assume open federation
		return true
	}

	// If we have domain allows configured, we're in allow-list mode
	if len(domainAllows) > 0 {
		// In allow-list mode, check if this specific domain is allowed
		// We need to check all allows, not just the first one
		allAllows, _, err := v.storage.GetDomainAllows(ctx, 1000, "")
		if err != nil {
			v.logger.Error("failed to get all domain allows",
				zap.String("domain", domain),
				zap.Error(err))
			return false
		}

		// Check if domain is in the allow list
		for _, allow := range allAllows {
			if allow.Domain == domain {
				v.logger.Debug("domain is in allow list",
					zap.String("domain", domain))
				return true
			}
		}

		v.logger.Info("domain not in allow list",
			zap.String("domain", domain))
		return false
	}

	// If not blocked and not in allow-list mode, it's trusted
	return true
}

// canonicalizeJSON creates a canonical JSON representation
func canonicalizeJSON(v interface{}) ([]byte, error) {
	// Simple canonicalization - in production use a proper JSON-LD library
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	// Remove signature fields for canonicalization
	delete(m, "signature")
	delete(m, "Signature")
	delete(m, "issuerProof")
	delete(m, "IssuerProof")

	return json.Marshal(m)
}

// VerifyVouchSignature verifies a vouch's signature using the issuer's public key
func (v *Verifier) VerifyVouchSignature(vouch *Vouch) (bool, error) {
	// Get the issuer's public key from the instance
	publicKey, err := v.getInstancePublicKey(vouch.InstanceURL)
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
