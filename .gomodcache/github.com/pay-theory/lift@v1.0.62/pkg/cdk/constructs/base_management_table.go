// Package constructs provides AWS CDK constructs for Lift applications.
//
// This package contains high-level CDK constructs that implement Lift's best practices
// for AWS infrastructure. The constructs include optimized configurations for API
// Gateway, Lambda functions, DynamoDB tables, and other AWS services.
//
// # Base Management Table
//
// The base_management_table.go file provides common functionality for creating and
// managing DynamoDB tables used for various management purposes in Lift applications.
// It includes helper functions for creating tables with standard settings and
// granting appropriate permissions.
package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// BaseManagementTableProps defines common properties for management tables.
//
// This struct contains properties that are common to all management tables,
// including table name, TTL attribute, and default table name.
type BaseManagementTableProps struct {
	// Table name
	TableName *string
	// TTL attribute name for automatic cleanup
	TimeToLiveAttribute *string
	// Default table name if not provided
	DefaultTableName string
}

// createManagementTable creates a standard management table with common settings.
//
// This function creates a DynamoDB table with standard settings for management purposes,
// including TTL, point-in-time recovery, and streams.
//
// Parameters:
//   - scope: The CDK construct scope
//   - id: The construct ID
//   - props: Properties for the management table
//
// Returns:
//   - A LiftTable instance with standard management settings
func createManagementTable(scope constructs.Construct, id *string, props *BaseManagementTableProps) *LiftTable {
	// Set defaults
	if props == nil {
		props = &BaseManagementTableProps{}
	}

	// Set table specific defaults
	if props.TableName == nil {
		props.TableName = jsii.String(props.DefaultTableName)
	}

	// Enable TTL for cleanup
	if props.TimeToLiveAttribute == nil {
		props.TimeToLiveAttribute = jsii.String("ttl")
	}

	// Create the table with standard settings
	return NewLiftTable(scope, id, &LiftTableProps{
		TableName:                 props.TableName,
		PartitionKeyName:          jsii.String("PK"),
		SortKeyName:               jsii.String("SK"),
		TimeToLiveAttribute:       props.TimeToLiveAttribute,
		EnablePointInTimeRecovery: jsii.Bool(true),
		EnableStreams:             jsii.Bool(true),
	})
}

// grantManagementPermissions grants read/write permissions for management operations.
//
// This function grants read and write permissions on the specified table to the
// given grantee, which is typically a Lambda function or other AWS service.
//
// Parameters:
//   - table: The LiftTable to grant permissions on
//   - grantee: The IAM principal to grant permissions to
func grantManagementPermissions(table *LiftTable, grantee awsiam.IGrantable) {
	table.Table.GrantReadWriteData(grantee)
}

// ManagementTableConfig defines configuration for creating management tables.
//
// This struct contains configuration options for creating different types of
// management tables, including default table names and permission methods.
type ManagementTableConfig struct {
	DefaultTableName string
	PermissionMethod string // e.g., "GrantConnectionManagement", "GrantEventManagement"
}

// createTypedManagementTable creates a management table with type-specific configuration.
//
// This function creates a management table with configuration specific to the type
// of management being performed (e.g., connection management, event routing).
// It extracts common properties from the type-specific props and applies them.
//
// Parameters:
//   - scope: The CDK construct scope
//   - id: The construct ID
//   - props: Type-specific properties for the table
//   - config: Configuration for the management table
//
// Returns:
//   - A LiftTable instance with type-specific configuration
func createTypedManagementTable(scope constructs.Construct, id *string, props interface{}, config ManagementTableConfig) *LiftTable {
	baseProps := &BaseManagementTableProps{
		DefaultTableName: config.DefaultTableName,
	}

	// Extract common properties using type assertion
	switch p := props.(type) {
	case *ConnectionTableProps:
		if p != nil {
			baseProps.TableName = p.TableName
			baseProps.TimeToLiveAttribute = p.TimeToLiveAttribute
		}
	case *EventRoutingTableProps:
		if p != nil {
			baseProps.TableName = p.TableName
			baseProps.TimeToLiveAttribute = p.TimeToLiveAttribute
		}
	}

	return createManagementTable(scope, id, baseProps)
}
