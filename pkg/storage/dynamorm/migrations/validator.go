package migrations

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// ValidationResult contains the results of migration validation
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidationError represents a validation error that prevents migration
type ValidationError struct {
	MigrationID string
	Type        string
	Message     string
}

// ValidationWarning represents a validation warning that doesn't prevent migration
type ValidationWarning struct {
	MigrationID string
	Type        string
	Message     string
}

// Validator validates migrations before execution
type Validator struct {
	migrator *Migrator
	logger   *zap.Logger
}

// NewValidator creates a new migration validator
func NewValidator(migrator *Migrator, logger *zap.Logger) *Validator {
	return &Validator{
		migrator: migrator,
		logger:   logger,
	}
}

// ValidateAll validates all pending migrations
func (v *Validator) ValidateAll(ctx context.Context) (*ValidationResult, error) {
	result := &ValidationResult{Valid: true}

	// Get applied migrations
	applied, err := v.migrator.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Get pending migrations
	pending := v.migrator.registry.GetPending(applied)

	// Get ordered migrations
	ordered, err := v.migrator.registry.GetInOrder(pending)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Type:    "dependency_error",
			Message: err.Error(),
		})
		return result, nil
	}

	// Validate each migration
	for _, migration := range ordered {
		v.validateMigration(ctx, migration, applied, result)

		// Simulate applying this migration for dependency validation
		applied[migration.ID()] = true
	}

	return result, nil
}

// ValidateMigration validates a specific migration
func (v *Validator) ValidateMigration(ctx context.Context, migrationID string) (*ValidationResult, error) {
	result := &ValidationResult{Valid: true}

	migration, exists := v.migrator.registry.Get(migrationID)
	if !exists {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			MigrationID: migrationID,
			Type:        "not_found",
			Message:     "Migration not found in registry",
		})
		return result, nil
	}

	// Get applied migrations
	applied, err := v.migrator.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	v.validateMigration(ctx, migration, applied, result)

	return result, nil
}

// validateMigration performs validation checks on a single migration
func (v *Validator) validateMigration(ctx context.Context, migration Migration, applied map[string]bool, result *ValidationResult) {
	id := migration.ID()

	// Check if already applied
	if applied[id] {
		result.Errors = append(result.Errors, ValidationError{
			MigrationID: id,
			Type:        "already_applied",
			Message:     "Migration has already been applied",
		})
		result.Valid = false
	}

	// Validate ID format
	if err := v.validateIDFormat(id); err != nil {
		result.Errors = append(result.Errors, ValidationError{
			MigrationID: id,
			Type:        "invalid_id",
			Message:     err.Error(),
		})
		result.Valid = false
	}

	// Validate version
	if migration.Version() <= 0 {
		result.Errors = append(result.Errors, ValidationError{
			MigrationID: id,
			Type:        "invalid_version",
			Message:     "Version must be greater than 0",
		})
		result.Valid = false
	}

	// Validate dependencies
	for _, depID := range migration.Dependencies() {
		// Check if dependency exists
		if _, exists := v.migrator.registry.Get(depID); !exists {
			result.Errors = append(result.Errors, ValidationError{
				MigrationID: id,
				Type:        "missing_dependency",
				Message:     fmt.Sprintf("Dependency %s not found in registry", depID),
			})
			result.Valid = false
			continue
		}

		// Check if dependency is applied
		if !applied[depID] {
			result.Errors = append(result.Errors, ValidationError{
				MigrationID: id,
				Type:        "unapplied_dependency",
				Message:     fmt.Sprintf("Dependency %s has not been applied", depID),
			})
			result.Valid = false
		}
	}

	// Check for duplicate versions (warning only)
	v.checkDuplicateVersions(migration, result)

	// Check migration history for conflicts
	v.checkHistoryConflicts(ctx, migration, result)
}

// validateIDFormat validates the migration ID format
func (v *Validator) validateIDFormat(id string) error {
	if err := common.ValidateRequiredParam("id", id); err != nil {
		return fmt.Errorf("migration ID cannot be empty")
	}

	if len(id) > 100 {
		return fmt.Errorf("migration ID too long (max 100 characters)")
	}

	// Check for invalid characters
	invalidChars := []string{" ", "\t", "\n", "\r", "/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range invalidChars {
		if strings.Contains(id, char) {
			return fmt.Errorf("migration ID contains invalid character: %s", char)
		}
	}

	return nil
}

// checkDuplicateVersions checks for migrations with duplicate version numbers
func (v *Validator) checkDuplicateVersions(migration Migration, result *ValidationResult) {
	version := migration.Version()

	for _, m := range v.migrator.registry.All() {
		if m.ID() != migration.ID() && m.Version() == version {
			result.Warnings = append(result.Warnings, ValidationWarning{
				MigrationID: migration.ID(),
				Type:        "duplicate_version",
				Message:     fmt.Sprintf("Migration has same version as %s", m.ID()),
			})
		}
	}
}

// checkHistoryConflicts checks for conflicts with migration history
func (v *Validator) checkHistoryConflicts(_ context.Context, migration Migration, result *ValidationResult) {
	history := &MigrationHistory{}
	history.PK = "MIGRATION#HISTORY"
	history.SK = "MIGRATION#" + migration.ID()
	err := v.migrator.db.Model(history).
		Where("PK", "=", history.PK).
		Where("SK", "=", history.SK).
		First(history)

	if err == nil {
		// Migration exists in history
		switch history.Status {
		case "failed":
			result.Warnings = append(result.Warnings, ValidationWarning{
				MigrationID: migration.ID(),
				Type:        "previous_failure",
				Message:     fmt.Sprintf("Migration previously failed: %s", history.Error),
			})
		case StatusRolledBack:
			result.Warnings = append(result.Warnings, ValidationWarning{
				MigrationID: migration.ID(),
				Type:        "previously_rolled_back",
				Message:     "Migration was previously rolled back",
			})
		}

		// Check checksum
		currentChecksum := v.migrator.calculateChecksum(migration)
		if history.Checksum != "" && history.Checksum != currentChecksum {
			result.Warnings = append(result.Warnings, ValidationWarning{
				MigrationID: migration.ID(),
				Type:        "checksum_mismatch",
				Message:     "Migration has been modified since last execution",
			})
		}
	}
}

// ValidateRollback validates that a rollback can be safely performed
func (v *Validator) ValidateRollback(ctx context.Context, opts RollbackOptions) (*ValidationResult, error) {
	result := &ValidationResult{Valid: true}

	// Get rollback plan
	toRollback, err := v.migrator.GetRollbackPlan(ctx, opts)
	if err != nil {
		return nil, err
	}

	if err := common.ValidateSliceNotEmpty("toRollback", toRollback); err != nil {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Type:    "no_migrations",
			Message: "No migrations to rollback",
		})
		return result, nil
	}

	// Get applied migrations
	history, err := v.migrator.GetMigrationHistory(ctx)
	if err != nil {
		return nil, err
	}

	appliedMap := make(map[string]bool)
	for _, h := range history {
		if h.Status == "applied" {
			appliedMap[h.ID] = true
		}
	}

	// Validate rollback is safe
	if err := v.migrator.validateRollback(ctx, toRollback, appliedMap); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Type:    "dependency_conflict",
			Message: err.Error(),
		})
	}

	// Check if migrations have Down methods implemented
	for _, h := range toRollback {
		migration, exists := v.migrator.registry.Get(h.ID)
		if !exists {
			result.Errors = append(result.Errors, ValidationError{
				MigrationID: h.ID,
				Type:        "not_found",
				Message:     "Migration not found in registry",
			})
			result.Valid = false
			continue
		}

		// Try to detect if Down method is implemented (this is a heuristic)
		if err := v.checkDownMethodImplemented(migration); err != nil {
			result.Warnings = append(result.Warnings, ValidationWarning{
				MigrationID: h.ID,
				Type:        "down_method_warning",
				Message:     err.Error(),
			})
		}
	}

	return result, nil
}

// checkDownMethodImplemented tries to detect if a migration has a proper Down implementation
func (v *Validator) checkDownMethodImplemented(migration Migration) error {
	// This is a heuristic check - we can't actually verify the implementation
	// without executing it, but we can check for common patterns

	// Check if it's a GSI migration (which we know has proper Down implementation)
	if _, ok := migration.(GSIMigration); ok {
		return nil
	}

	// For custom migrations, we can't verify without execution
	return fmt.Errorf("cannot verify Down method implementation for custom migration")
}

// Format formats validation results for display
func (r *ValidationResult) Format() string {
	var sb strings.Builder

	if r.Valid {
		sb.WriteString("✓ All validations passed\n")
	} else {
		sb.WriteString("✗ Validation failed\n")
	}

	if len(r.Errors) > 0 {
		sb.WriteString("\nErrors:\n")
		for _, err := range r.Errors {
			if err.MigrationID != "" {
				sb.WriteString(fmt.Sprintf("  - [%s] %s: %s\n", err.MigrationID, err.Type, err.Message))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s: %s\n", err.Type, err.Message))
			}
		}
	}

	if len(r.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, warn := range r.Warnings {
			if warn.MigrationID != "" {
				sb.WriteString(fmt.Sprintf("  - [%s] %s: %s\n", warn.MigrationID, warn.Type, warn.Message))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s: %s\n", warn.Type, warn.Message))
			}
		}
	}

	return sb.String()
}
