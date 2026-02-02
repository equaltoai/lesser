// Package migrations defines constants and status values for DynamORM database migration management.
package migrations

// Migration-related constants
const (
	MigrationStatusKey = "MIGRATION#STATUS"
)

// Migration status constants
const (
	StatusActive     = "ACTIVE"
	StatusCurrent    = "CURRENT"
	StatusApplied    = "applied"
	StatusRolledBack = "rolled_back"
	StatusPending    = "pending"
)

// Event type constants
const (
	EventInsert = "INSERT"
	EventModify = "MODIFY"
	EventRemove = "REMOVE"
)
