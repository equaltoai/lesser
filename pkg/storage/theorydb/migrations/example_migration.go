package migrations

import (
	"context"
	"fmt"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// ExampleAddUserLookupIndex demonstrates how to create custom migrations
type ExampleAddUserLookupIndex struct {
	BaseMigration
}

// NewExampleAddUserLookupIndex creates an example migration
func NewExampleAddUserLookupIndex() *ExampleAddUserLookupIndex {
	return &ExampleAddUserLookupIndex{
		BaseMigration: NewBaseMigration(
			"20240115_add_user_lookup_index",
			20240115120000,
			"Add GSI for user lookup queries",
		),
	}
}

// Up applies the migration
func (m *ExampleAddUserLookupIndex) Up(_ context.Context, _ core.DB) error {
	// Example: Add a new GSI for user lookups
	// In a real migration, you would use the GSIHelper

	fmt.Println("Adding user lookup index...")

	// Example of data migration
	// results, err := db.Query(ctx).
	//     PK("USER#").
	//     BeginsWith("USER#").
	//     Execute()
	//
	// if err != nil {
	//     return fmt.Errorf("failed to query users: %w", err)
	// }
	//
	// for _, item := range results {
	//     // Update items as needed
	// }

	return nil
}

// Down reverses the migration
func (m *ExampleAddUserLookupIndex) Down(_ context.Context, _ core.DB) error {
	// Example: Remove the GSI

	fmt.Println("Removing user lookup index...")

	// Reverse any data changes made in Up

	return nil
}

// NewExampleGSIMigration creates an example GSI migration
func NewExampleGSIMigration() Migration {
	return NewGSIMigration(
		"20240116_add_activity_timestamp_index",
		20240116120000,
		"Add GSI for activity timestamp queries",
		"lesser-production", // table name
		GSIDefinition{
			Name:           "GSI7",
			HashKey:        "gsi7PK",
			HashKeyType:    "S",
			RangeKey:       "gsi7SK",
			RangeKeyType:   "S",
			ProjectionType: "ALL",
		},
	)
}

// ExampleDependentMigration demonstrates a migration with dependencies
type ExampleDependentMigration struct {
	BaseMigration
}

// NewExampleDependentMigration creates a new example dependent migration
func NewExampleDependentMigration() *ExampleDependentMigration {
	return &ExampleDependentMigration{
		BaseMigration: NewBaseMigration(
			"20240117_add_user_preferences",
			20240117120000,
			"Add user preferences structure",
			"20240115_add_user_lookup_index", // depends on the lookup index migration
		),
	}
}

// Up applies the migration
func (m *ExampleDependentMigration) Up(_ context.Context, _ core.DB) error {
	// This migration depends on the email index being present
	fmt.Println("Adding user preferences...")
	return nil
}

// Down reverses the migration
func (m *ExampleDependentMigration) Down(_ context.Context, _ core.DB) error {
	fmt.Println("Removing user preferences...")
	return nil
}

// Example of registering migrations (typically done in an init function)
func init() {
	// Register migrations with the global registry
	// MustRegister(NewExampleAddUserLookupIndex())
	// MustRegister(NewExampleGSIMigration())
	// MustRegister(NewExampleDependentMigration())
}
