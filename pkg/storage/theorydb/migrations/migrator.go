package migrations

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"
)

// Migration key constants
const (
	migrationHistoryPK     = "MIGRATION#HISTORY"
	migrationLockPK        = "MIGRATION#LOCK"
	migrationStatusApplied = "applied"
)

// Migrator handles migration execution and tracking
type Migrator struct {
	db       core.DB
	registry *Registry
	logger   *zap.Logger
	executor string // Who is running the migrations (e.g., Lambda function name)
}

// NewMigrator creates a new migrator instance
func NewMigrator(db core.DB, registry *Registry, logger *zap.Logger) *Migrator {
	executor := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	if err := common.ValidateRequiredParam("executor", executor); err != nil {
		executor = "local"
	}

	return &Migrator{
		db:       db,
		registry: registry,
		logger:   logger,
		executor: executor,
	}
}

// GetAppliedMigrations returns a map of applied migration IDs
func (m *Migrator) GetAppliedMigrations(_ context.Context) (map[string]bool, error) {
	applied := make(map[string]bool)

	// Query all migration history records
	var histories []MigrationHistory
	err := m.db.Model(&MigrationHistory{}).
		Where("PK", "=", migrationHistoryPK).
		All(&histories)

	if err != nil {
		return nil, fmt.Errorf("failed to query migration history: %w", err)
	}

	for _, history := range histories {
		// Only count successfully applied migrations
		if history.Status == migrationStatusApplied {
			applied[history.ID] = true
		}
	}

	return applied, nil
}

// GetMigrationStatus returns the current migration status
func (m *Migrator) GetMigrationStatus(_ context.Context) (*MigrationStatus, error) {
	status := &MigrationStatus{}
	status.PK = MigrationStatusKey
	status.SK = "CURRENT"

	err := m.db.Model(status).
		Where("PK", "=", status.PK).
		Where("SK", "=", status.SK).
		First(status)

	if err != nil {
		// If not found, return empty status
		return &MigrationStatus{}, nil
	}

	return status, nil
}

// UpdateMigrationStatus updates the overall migration status
func (m *Migrator) UpdateMigrationStatus(_ context.Context, status *MigrationStatus) error {
	status.PK = MigrationStatusKey
	status.SK = "CURRENT"
	status.UpdatedAt = time.Now()

	return m.db.Model(status).Create()
}

// recordMigrationHistory records a migration execution
func (m *Migrator) recordMigrationHistory(_ context.Context, migration Migration, status string, _ time.Duration, execErr error) error {
	history := &MigrationHistory{
		ID:          migration.ID(),
		Version:     migration.Version(),
		Description: migration.Description(),
		Status:      status,
		AppliedBy:   m.executor,
		AppliedAt:   time.Now(),
		Checksum:    m.calculateChecksum(migration),
	}

	if execErr != nil {
		history.Error = execErr.Error()
	}

	history.PK, history.SK = history.GetTableKeys()

	return m.db.Model(history).Create()
}

// calculateChecksum calculates a checksum for a migration
func (m *Migrator) calculateChecksum(migration Migration) string {
	h := sha256.New()
	h.Write([]byte(migration.ID()))
	_, _ = fmt.Fprintf(h, "%d", migration.Version())
	h.Write([]byte(migration.Description()))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// acquireLock attempts to acquire the migration lock
func (m *Migrator) acquireLock(_ context.Context) error {
	lock := &MigrationStatus{
		PK:       migrationLockPK,
		SK:       "ACTIVE",
		IsLocked: true,
		LockedBy: m.executor,
		LockedAt: time.Now(),
	}

	// Try to create lock (will fail if already exists)
	err := m.db.Model(lock).Create()
	if err != nil {
		// Check if lock exists
		existingLock := &MigrationStatus{}
		existingLock.PK = migrationLockPK
		existingLock.SK = "ACTIVE"

		checkErr := m.db.Model(existingLock).
			Where("PK", "=", existingLock.PK).
			Where("SK", "=", existingLock.SK).
			First(existingLock)

		if checkErr == nil && existingLock.IsLocked {
			// Check if lock is stale (older than 10 minutes)
			if existingLock.LockedAt.Before(time.Now().Add(-10 * time.Minute)) {
				// Lock is stale, try to update it
				return m.db.Model(lock).Update()
			}
			return fmt.Errorf("migration lock held by %s since %s", existingLock.LockedBy, existingLock.LockedAt)
		}
	}

	return nil
}

// releaseLock releases the migration lock
func (m *Migrator) releaseLock(_ context.Context) error {
	lock := &MigrationStatus{}
	lock.PK = migrationLockPK
	lock.SK = "ACTIVE"

	return m.db.Model(lock).Delete()
}

// Run executes all pending migrations
func (m *Migrator) Run(ctx context.Context) error {
	// Acquire lock
	if err := m.acquireLock(ctx); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer func() {
		if releaseErr := m.releaseLock(ctx); releaseErr != nil {
			m.logger.Error("failed to release migration lock", zap.Error(releaseErr))
		}
	}()

	// Get applied migrations
	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Get pending migrations
	pending := m.registry.GetPending(applied)
	if err := common.ValidateSliceNotEmpty("pending", pending); err != nil {
		m.logger.Info("No pending migrations")
		return nil
	}

	m.logger.Info("Found pending migrations", zap.Int("count", len(pending)))

	// Execute migrations
	for _, migration := range pending {
		if err := m.executeMigration(ctx, migration); err != nil {
			return fmt.Errorf("migration %s failed: %w", migration.ID(), err)
		}
	}

	// Update overall status
	status, _ := m.GetMigrationStatus(ctx)
	// TotalMigrations field doesn't exist
	status.LastMigrationID = pending[len(pending)-1].ID()
	status.LastVersion = pending[len(pending)-1].Version()
	status.UpdatedAt = time.Now()
	// LastExecutedBy field doesn't exist

	return m.UpdateMigrationStatus(ctx, status)
}

// executeMigration executes a single migration
func (m *Migrator) executeMigration(ctx context.Context, migration Migration) error {
	m.logger.Info("Executing migration",
		zap.String("id", migration.ID()),
		zap.Int64("version", migration.Version()),
		zap.String("description", migration.Description()))

	// Record start
	startTime := time.Now()
	if err := m.recordMigrationHistory(ctx, migration, "in_progress", 0, nil); err != nil {
		m.logger.Error("Failed to record migration start", zap.Error(err))
	}

	// Execute migration
	err := migration.Up(ctx, m.db)
	duration := time.Since(startTime)

	// Record result
	status := migrationStatusApplied
	if err != nil {
		status = "failed"
		m.logger.Error("Migration failed",
			zap.String("id", migration.ID()),
			zap.Error(err),
			zap.Duration("duration", duration))
	} else {
		m.logger.Info("Migration completed",
			zap.String("id", migration.ID()),
			zap.Duration("duration", duration))
	}

	if recordErr := m.recordMigrationHistory(ctx, migration, status, duration, err); recordErr != nil {
		m.logger.Error("Failed to record migration result", zap.Error(recordErr))
	}

	return err
}

// Rollback rolls back the last migration
func (m *Migrator) Rollback(ctx context.Context) error {
	// Acquire lock
	if err := m.acquireLock(ctx); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer func() {
		if releaseErr := m.releaseLock(ctx); releaseErr != nil {
			m.logger.Error("failed to release migration lock", zap.Error(releaseErr))
		}
	}()

	// Get migration history
	var histories []MigrationHistory
	err := m.db.Model(&MigrationHistory{}).
		Where("PK", "=", migrationHistoryPK).
		OrderBy("Version", "DESC").
		Limit(1).
		All(&histories)

	if err != nil || len(histories) == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	lastMigration := histories[0]
	if lastMigration.Status != migrationStatusApplied {
		return fmt.Errorf("last migration %s is in status %s, cannot rollback", lastMigration.ID, lastMigration.Status)
	}

	// Find the migration in registry
	migration, found := m.registry.Get(lastMigration.ID)
	if !found {
		return fmt.Errorf("migration %s not found in registry", lastMigration.ID)
	}

	m.logger.Info("Rolling back migration",
		zap.String("id", migration.ID()),
		zap.Int64("version", migration.Version()))

	// Execute rollback
	startTime := time.Now()
	err = migration.Down(ctx, m.db)
	duration := time.Since(startTime)

	// Record rollback
	status := "rolled_back"
	if err != nil {
		status = "rollback_failed"
		m.logger.Error("Rollback failed",
			zap.String("id", migration.ID()),
			zap.Error(err))
	} else {
		m.logger.Info("Rollback completed",
			zap.String("id", migration.ID()),
			zap.Duration("duration", duration))
	}

	return m.recordMigrationHistory(ctx, migration, status, duration, err)
}

// GetMigrationHistory returns the migration history
func (m *Migrator) GetMigrationHistory(_ context.Context) ([]*MigrationHistory, error) {
	var histories []MigrationHistory
	err := m.db.Model(&MigrationHistory{}).
		Where("PK", "=", migrationHistoryPK).
		OrderBy("Version", "DESC").
		All(&histories)

	if err != nil {
		return nil, fmt.Errorf("failed to query migration history: %w", err)
	}

	// Convert to pointer slice
	result := make([]*MigrationHistory, len(histories))
	for i := range histories {
		result[i] = &histories[i]
	}

	return result, nil
}
