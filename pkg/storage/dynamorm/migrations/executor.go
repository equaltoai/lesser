package migrations

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// MigrateOptions contains options for running migrations
type MigrateOptions struct {
	// DryRun if true, will only log what would be done without executing
	DryRun bool

	// Target specific migration ID to migrate up to (inclusive)
	Target string

	// Force allows running migrations even if there are validation warnings
	Force bool
}

// Migrate runs all pending migrations up to the target (or all if target is empty)
func (m *Migrator) Migrate(ctx context.Context, opts MigrateOptions) error {
	// Acquire lock unless doing a dry run
	if !opts.DryRun {
		if err := m.acquireLock(ctx); err != nil {
			return fmt.Errorf("failed to acquire migration lock: %w", err)
		}
		defer func() {
			if err := m.releaseLock(ctx); err != nil {
				m.logger.Error("Failed to release migration lock", zap.Error(err))
			}
		}()
	}

	// Get applied migrations
	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Get pending migrations
	pending := m.registry.GetPending(applied)
	if len(pending) == 0 {
		m.logger.Info("No pending migrations")
		return nil
	}

	// Filter up to target if specified
	if opts.Target != "" {
		var filtered []Migration
		for _, migration := range pending {
			filtered = append(filtered, migration)
			if migration.ID() == opts.Target {
				break
			}
		}
		pending = filtered
	}

	// Sort migrations in dependency order
	ordered, err := m.registry.GetInOrder(pending)
	if err != nil {
		return fmt.Errorf("failed to order migrations: %w", err)
	}

	m.logger.Info("Found pending migrations",
		zap.Int("count", len(ordered)),
		zap.Bool("dry_run", opts.DryRun))

	if opts.DryRun {
		// Just log what would be done
		for _, migration := range ordered {
			m.logger.Info("Would execute migration",
				zap.String("id", migration.ID()),
				zap.Int64("version", migration.Version()),
				zap.String("description", migration.Description()))
		}
		return nil
	}

	// Execute migrations
	for _, migration := range ordered {
		if err := m.executeMigration(ctx, migration); err != nil {
			return fmt.Errorf("migration %s failed: %w", migration.ID(), err)
		}

		// Mark as applied for dependency tracking
		applied[migration.ID()] = true
	}

	// Update overall status
	status, _ := m.GetMigrationStatus(ctx)
	// TotalMigrations field doesn't exist
	if len(ordered) > 0 {
		status.LastMigrationID = ordered[len(ordered)-1].ID()
		status.LastVersion = ordered[len(ordered)-1].Version()
	}
	status.UpdatedAt = time.Now()
	// LastExecutedBy field doesn't exist

	return m.UpdateMigrationStatus(ctx, status)
}

// MigrateDown rolls back migrations down to a target version
func (m *Migrator) MigrateDown(ctx context.Context, target string) error {
	// Acquire lock
	if err := m.acquireLock(ctx); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer func() {
		if err := m.releaseLock(ctx); err != nil {
			m.logger.Error("failed to release migration lock", zap.Error(err))
		}
	}()

	// Get migration history in reverse order
	history, err := m.GetMigrationHistory(ctx)
	if err != nil {
		return err
	}

	if len(history) == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	// Find migrations to rollback
	toRollback := make([]*MigrationHistory, 0, len(history))
	for _, h := range history {
		if h.Status != "applied" {
			continue
		}

		toRollback = append(toRollback, h)

		if h.ID == target {
			break
		}
	}

	if len(toRollback) == 0 {
		return fmt.Errorf("no migrations found to rollback to %s", target)
	}

	// Execute rollbacks
	for _, h := range toRollback {
		migration, found := m.registry.Get(h.ID)
		if !found {
			return fmt.Errorf("migration %s not found in registry", h.ID)
		}

		if err := m.rollbackMigration(ctx, migration); err != nil {
			return fmt.Errorf("rollback of %s failed: %w", h.ID, err)
		}
	}

	return nil
}

// rollbackMigration executes a single migration rollback
func (m *Migrator) rollbackMigration(ctx context.Context, migration Migration) error {
	m.logger.Info("Rolling back migration",
		zap.String("id", migration.ID()),
		zap.Int64("version", migration.Version()),
		zap.String("description", migration.Description()))

	// Execute rollback
	err := migration.Down(ctx, m.db)

	// Record result
	status := "rolled_back"
	if err != nil {
		status = "rollback_failed"
		m.logger.Error("Rollback failed",
			zap.String("id", migration.ID()),
			zap.Error(err))
	} else {
		m.logger.Info("Rollback completed",
			zap.String("id", migration.ID()))
	}

	// Record in history
	return m.recordMigrationHistory(ctx, migration, status, 0, err)
}

// GetPendingMigrations returns a list of migrations that haven't been applied
func (m *Migrator) GetPendingMigrations(ctx context.Context) ([]Migration, error) {
	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	return m.registry.GetPending(applied), nil
}
