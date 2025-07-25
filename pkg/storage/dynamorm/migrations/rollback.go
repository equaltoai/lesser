package migrations

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
)

// RollbackOptions contains options for rolling back migrations
type RollbackOptions struct {
	// DryRun if true, will only log what would be done without executing
	DryRun bool
	
	// Target specific migration ID to rollback to (exclusive)
	Target string
	
	// Steps number of migrations to rollback (ignored if Target is set)
	Steps int
	
	// Force allows rollback even if there are dependency conflicts
	Force bool
}

// Rollback reverses applied migrations
func (m *Migrator) RollbackWithOptions(ctx context.Context, opts RollbackOptions) error {
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
	
	// Get migration history
	history, err := m.GetMigrationHistory(ctx)
	if err != nil {
		return err
	}
	
	// Filter to only successfully applied migrations
	var appliedMigrations []*MigrationHistory
	appliedMap := make(map[string]bool)
	
	for _, h := range history {
		if h.Status == "applied" {
			appliedMigrations = append(appliedMigrations, h)
			appliedMap[h.ID] = true
		}
	}
	
	if len(appliedMigrations) == 0 {
		m.logger.Info("No migrations to rollback")
		return nil
	}
	
	// Sort by version descending (newest first)
	sort.Slice(appliedMigrations, func(i, j int) bool {
		return appliedMigrations[i].Version > appliedMigrations[j].Version
	})
	
	// Determine which migrations to rollback
	var toRollback []*MigrationHistory
	
	if opts.Target != "" {
		// Rollback all migrations after the target
		found := false
		for _, h := range appliedMigrations {
			if h.ID == opts.Target {
				found = true
				break
			}
			toRollback = append(toRollback, h)
		}
		if !found {
			return fmt.Errorf("target migration %s not found in applied migrations", opts.Target)
		}
	} else if opts.Steps > 0 {
		// Rollback specific number of migrations
		count := opts.Steps
		if count > len(appliedMigrations) {
			count = len(appliedMigrations)
		}
		toRollback = appliedMigrations[:count]
	} else {
		// Default to rolling back the last migration
		toRollback = appliedMigrations[:1]
	}
	
	if len(toRollback) == 0 {
		m.logger.Info("No migrations selected for rollback")
		return nil
	}
	
	m.logger.Info("Preparing to rollback migrations",
		zap.Int("count", len(toRollback)),
		zap.Bool("dry_run", opts.DryRun))
	
	// Validate rollback is safe
	if !opts.Force {
		if err := m.validateRollback(ctx, toRollback, appliedMap); err != nil {
			return err
		}
	}
	
	// Execute rollbacks
	for _, historyItem := range toRollback {
		migration, exists := m.registry.Get(historyItem.ID)
		if !exists {
			return fmt.Errorf("migration %s not found in registry", historyItem.ID)
		}
		
		if err := m.executeRollback(ctx, migration, opts); err != nil {
			return fmt.Errorf("rollback of %s failed: %w", migration.ID(), err)
		}
		
		// Remove from applied map for dependency checking
		delete(appliedMap, historyItem.ID)
	}
	
	return nil
}

// validateRollback checks if the rollback can be safely performed
func (m *Migrator) validateRollback(ctx context.Context, toRollback []*MigrationHistory, appliedMap map[string]bool) error {
	// Create a map of migrations to be rolled back
	rollbackMap := make(map[string]bool)
	for _, h := range toRollback {
		rollbackMap[h.ID] = true
	}
	
	// Check for dependency conflicts
	for id := range appliedMap {
		if rollbackMap[id] {
			continue // This migration is being rolled back
		}
		
		migration, exists := m.registry.Get(id)
		if !exists {
			m.logger.Warn("Applied migration not found in registry",
				zap.String("id", id))
			continue
		}
		
		// Check if this migration depends on any migrations being rolled back
		for _, dep := range migration.Dependencies() {
			if rollbackMap[dep] {
				return fmt.Errorf("cannot rollback %s because %s depends on it", dep, id)
			}
		}
	}
	
	return nil
}

// executeRollback runs the Down method of a migration
func (m *Migrator) executeRollback(ctx context.Context, migration Migration, opts RollbackOptions) error {
	m.logger.Info("Rolling back migration",
		zap.String("id", migration.ID()),
		zap.Int64("version", migration.Version()),
		zap.String("description", migration.Description()),
		zap.Bool("dry_run", opts.DryRun))
	
	if opts.DryRun {
		m.logger.Info("Would rollback migration (dry run)",
			zap.String("id", migration.ID()))
		return nil
	}
	
	// Record rollback start
	startTime := time.Now()
	if err := m.recordMigrationHistory(ctx, migration, "rolling_back", 0, nil); err != nil {
		return err
	}
	
	// Execute the rollback
	if err := migration.Down(ctx, m.db); err != nil {
		// Record failure
		if recordErr := m.recordMigrationHistory(ctx, migration, "rollback_failed", time.Since(startTime), err); recordErr != nil {
			m.logger.Error("Failed to record rollback failure", 
				zap.Error(recordErr),
				zap.String("migration", migration.ID()))
		}
		return err
	}
	
	// Record success
	if err := m.recordMigrationHistory(ctx, migration, "rolled_back", time.Since(startTime), nil); err != nil {
		return fmt.Errorf("rollback succeeded but failed to record: %w", err)
	}
	
	m.logger.Info("Migration rolled back successfully",
		zap.String("id", migration.ID()))
	
	return nil
}

// RollbackLast rolls back the most recent migration
func (m *Migrator) RollbackLast(ctx context.Context) error {
	return m.RollbackWithOptions(ctx, RollbackOptions{Steps: 1})
}

// RollbackTo rolls back all migrations after the target (target remains applied)
func (m *Migrator) RollbackTo(ctx context.Context, targetID string) error {
	return m.RollbackWithOptions(ctx, RollbackOptions{Target: targetID})
}

// RollbackSteps rolls back the specified number of migrations
func (m *Migrator) RollbackSteps(ctx context.Context, steps int) error {
	return m.RollbackWithOptions(ctx, RollbackOptions{Steps: steps})
}

// GetRollbackPlan returns the migrations that would be rolled back
func (m *Migrator) GetRollbackPlan(ctx context.Context, opts RollbackOptions) ([]*MigrationHistory, error) {
	// Get migration history
	history, err := m.GetMigrationHistory(ctx)
	if err != nil {
		return nil, err
	}
	
	// Filter to only successfully applied migrations
	var appliedMigrations []*MigrationHistory
	
	for _, h := range history {
		if h.Status == "applied" {
			appliedMigrations = append(appliedMigrations, h)
		}
	}
	
	if len(appliedMigrations) == 0 {
		return nil, nil
	}
	
	// Sort by version descending (newest first)
	sort.Slice(appliedMigrations, func(i, j int) bool {
		return appliedMigrations[i].Version > appliedMigrations[j].Version
	})
	
	// Determine which migrations would be rolled back
	var toRollback []*MigrationHistory
	
	if opts.Target != "" {
		// Rollback all migrations after the target
		for _, h := range appliedMigrations {
			if h.ID == opts.Target {
				break
			}
			toRollback = append(toRollback, h)
		}
	} else if opts.Steps > 0 {
		// Rollback specific number of migrations
		count := opts.Steps
		if count > len(appliedMigrations) {
			count = len(appliedMigrations)
		}
		toRollback = appliedMigrations[:count]
	} else {
		// Default to rolling back the last migration
		if len(appliedMigrations) > 0 {
			toRollback = appliedMigrations[:1]
		}
	}
	
	return toRollback, nil
}