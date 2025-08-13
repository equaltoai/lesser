package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"

	"go.uber.org/zap"
)

// RecoveryCodeService handles backup recovery codes
type RecoveryCodeService struct {
	repos  StorageProvider
	logger *zap.Logger
}

// NewRecoveryCodeService creates a new recovery code service
func NewRecoveryCodeService(repos StorageProvider, logger *zap.Logger) *RecoveryCodeService {
	return &RecoveryCodeService{
		repos:  repos,
		logger: logger,
	}
}

// We use RecoveryCodeItem type from storage package

// GenerateRecoveryCodes generates new recovery codes for a user
func (s *RecoveryCodeService) GenerateRecoveryCodes(ctx context.Context, username string, count int) ([]string, error) {
	if count <= 0 {
		count = 8 // Default to 8 recovery codes
	}

	codes := make([]string, count)

	for i := 0; i < count; i++ {
		code, err := s.generateCode()
		if err != nil {
			return nil, fmt.Errorf("failed to generate recovery code: %w", err)
		}
		codes[i] = code

		// Store hashed version
		hashedCode, err := HashPassword(code)
		if err != nil {
			return nil, fmt.Errorf("failed to hash recovery code: %w", err)
		}

		// Store in DynamoDB
		recoveryCode := &storage.RecoveryCodeItem{
			Username:  username,
			CodeHash:  hashedCode,
			CreatedAt: time.Now(),
			Position:  i,
		}

		// Store in DynamoDB
		if err := s.repos.Recovery().StoreRecoveryCode(ctx, username, recoveryCode); err != nil {
			return nil, fmt.Errorf("failed to store recovery code: %w", err)
		}
	}

	// Clear any existing codes first
	if err := s.clearExistingCodes(ctx, username); err != nil {
		s.logger.Error("failed to clear existing codes", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("failed to clear existing recovery codes: %w", err)
	}

	s.logger.Info("generated recovery codes",
		zap.String("username", username),
		zap.Int("count", count))

	return codes, nil
}

// ValidateRecoveryCode validates and consumes a recovery code
func (s *RecoveryCodeService) ValidateRecoveryCode(ctx context.Context, username, code string) (bool, error) {
	// Normalize code (remove hyphens and spaces)
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ToUpper(code)

	// Get all recovery codes for user
	codes, err := s.repos.Recovery().GetRecoveryCodes(ctx, username)
	if err != nil {
		return false, fmt.Errorf("failed to get recovery codes: %w", err)
	}

	// For each stored code:
	for _, storedCode := range codes {
		// 1. Check if already used
		if storedCode.UsedAt != nil {
			continue
		}

		// 2. Verify hash matches
		if err := VerifyPassword(code, storedCode.CodeHash); err == nil {
			// 3. Mark as used if valid
			if err := s.repos.Recovery().MarkRecoveryCodeUsed(ctx, username, storedCode.CodeHash); err != nil {
				return false, fmt.Errorf("failed to mark recovery code as used: %w", err)
			}

			s.logger.Info("recovery code used successfully",
				zap.String("username", username),
				zap.Int("position", storedCode.Position))

			return true, nil
		}
	}

	s.logger.Warn("invalid recovery code attempt",
		zap.String("username", username))

	return false, nil
}

// GetRecoveryCodeCount returns the number of unused recovery codes
func (s *RecoveryCodeService) GetRecoveryCodeCount(ctx context.Context, username string) (int, error) {
	return s.repos.Recovery().CountUnusedRecoveryCodes(ctx, username)
}

// ClearRecoveryCodes removes all recovery codes for a user
func (s *RecoveryCodeService) ClearRecoveryCodes(ctx context.Context, username string) error {
	return s.clearExistingCodes(ctx, username)
}

// Helper methods

func (s *RecoveryCodeService) generateCode() (string, error) {
	// Generate 10 random bytes (80 bits of entropy)
	bytes := make([]byte, 10)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Encode as base32 (without padding)
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes)

	// Format as XXXX-XXXX-XXXX-XXXX for readability
	if len(encoded) >= 16 {
		formatted := fmt.Sprintf("%s-%s-%s-%s",
			encoded[0:4], encoded[4:8], encoded[8:12], encoded[12:16])
		return formatted, nil
	}

	return encoded, nil
}

func (s *RecoveryCodeService) clearExistingCodes(ctx context.Context, username string) error {
	s.logger.Info("clearing existing recovery codes",
		zap.String("username", username))

	return s.repos.Recovery().DeleteAllRecoveryCodes(ctx, username)
}
