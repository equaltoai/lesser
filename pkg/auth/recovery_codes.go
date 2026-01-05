package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"

	"go.uber.org/zap"
)

// RecoveryCodeService handles backup recovery codes
type RecoveryCodeService struct {
	repo   recoveryCodesRepository
	logger *zap.Logger
}

type recoveryCodesRepository interface {
	StoreRecoveryCode(ctx context.Context, username string, code *storage.RecoveryCodeItem) error
	GetRecoveryCodes(ctx context.Context, username string) ([]*storage.RecoveryCodeItem, error)
	MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error
	CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error)
	DeleteAllRecoveryCodes(ctx context.Context, username string) error
}

// NewRecoveryCodeService creates a new recovery code service
func NewRecoveryCodeService(repos StorageProvider, logger *zap.Logger) *RecoveryCodeService {
	return &RecoveryCodeService{
		repo:   repos.Recovery(),
		logger: logger,
	}
}

// We use RecoveryCodeItem type from storage package

// GenerateRecoveryCodes generates new recovery codes for a user
func (s *RecoveryCodeService) GenerateRecoveryCodes(ctx context.Context, username string, count int) ([]string, error) {
	if count <= 0 {
		count = 8 // Default to 8 recovery codes
	}

	// Clear any existing codes first
	if err := s.clearExistingCodes(ctx, username); err != nil {
		s.logger.Error("failed to clear existing codes", zap.String("username", username), zap.Error(err))
		return nil, errors.Join(ErrRecoveryCodeClear, err)
	}

	codes := make([]string, count)

	for i := 0; i < count; i++ {
		code, err := s.generateCode()
		if err != nil {
			return nil, errors.Join(ErrRecoveryCodeGeneration, err)
		}
		codes[i] = code

		normalizedCode := strings.ReplaceAll(code, "-", "")
		normalizedCode = strings.ReplaceAll(normalizedCode, " ", "")
		normalizedCode = strings.ToUpper(normalizedCode)

		// Store hashed version
		hashedCode, err := HashPassword(normalizedCode)
		if err != nil {
			return nil, errors.Join(ErrRecoveryCodeHashing, err)
		}

		// Store in DynamoDB
		recoveryCode := &storage.RecoveryCodeItem{
			Username:  username,
			CodeHash:  hashedCode,
			CreatedAt: time.Now(),
			Position:  i,
		}

		// Store in DynamoDB
		if err := s.repo.StoreRecoveryCode(ctx, username, recoveryCode); err != nil {
			return nil, errors.Join(ErrRecoveryCodeStorage, err)
		}
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
	codes, err := s.repo.GetRecoveryCodes(ctx, username)
	if err != nil {
		return false, errors.Join(ErrRecoveryCodeRetrieval, err)
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
			if err := s.repo.MarkRecoveryCodeUsed(ctx, username, storedCode.CodeHash); err != nil {
				return false, errors.Join(ErrRecoveryCodeMarkUsed, err)
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
	return s.repo.CountUnusedRecoveryCodes(ctx, username)
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

	return s.repo.DeleteAllRecoveryCodes(ctx, username)
}
