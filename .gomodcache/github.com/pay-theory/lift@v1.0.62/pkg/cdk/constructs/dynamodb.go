package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// LiftTableProps extends DynamoDB table properties with Lift-specific configuration
//
// This struct contains all configurable properties for creating a Lift-optimized
// DynamoDB table. The properties include basic table configuration, advanced
// features like point-in-time recovery, streams, auto-scaling, and TTL settings.
type LiftTableProps struct {
	TableName           *string
	PartitionKeyName    *string
	SortKeyName         *string
	TimeToLiveAttribute *string

	// Billing configuration
	ReadCapacity  *float64
	WriteCapacity *float64

	// Feature flags
	EnablePointInTimeRecovery *bool
	EnableStreams             *bool
	EnableAutoScaling         *bool
	DeletionProtection        *bool

	// Auto-scaling configuration
	MinReadCapacity   *float64
	MaxReadCapacity   *float64
	MinWriteCapacity  *float64
	MaxWriteCapacity  *float64
	TargetUtilization *float64

	// Global Secondary Indexes
	GlobalSecondaryIndexes *[]*awsdynamodb.GlobalSecondaryIndexProps

	// GSI Auto-scaling configuration
	GSIMinReadCapacity  *float64
	GSIMaxReadCapacity  *float64
	GSIMinWriteCapacity *float64
	GSIMaxWriteCapacity *float64

	// Replication and tagging
	ReplicationRegions *[]*string
	Tags               *map[string]*string

	// Non-pointer configuration values
	StreamViewType awsdynamodb.StreamViewType
	RemovalPolicy  awscdk.RemovalPolicy
	Encryption     awsdynamodb.TableEncryption
}

// LiftTable is a DynamoDB table construct optimized for Lift applications
//
// This construct creates a DynamoDB table with Lift-optimized defaults including:
// - Point-in-time recovery (if enabled)
// - DynamoDB streams (if enabled)
// - Auto-scaling (if enabled)
// - TTL (if configured)
//
// The table is configured with sensible defaults for production workloads.
type LiftTable struct {
	constructs.Construct
	Table awsdynamodb.Table
	GSIs  map[string]*awsdynamodb.GlobalSecondaryIndexProps
}

// NewLiftTable creates a new DynamoDB table with Lift-optimized defaults
//
// This function creates a new DynamoDB table with all Lift-optimized features including:
// - Appropriate billing mode (provisioned or pay-per-request)
// - Point-in-time recovery (if enabled)
// - DynamoDB streams (if enabled)
// - Auto-scaling (if enabled)
// - TTL (if configured)
//
// Parameters:
//   - scope: The CDK construct scope
//   - id: The construct ID
//   - props: Configuration properties
//
// Returns:
//   - A new LiftTable instance
func NewLiftTable(scope constructs.Construct, id *string, props *LiftTableProps) *LiftTable {
	builder := newLiftTableBuilder(scope, id, props)
	return builder.build()
}

// liftTableBuilder builds DynamoDB tables optimized for DynamORM
type liftTableBuilder struct {
	scope       constructs.Construct
	id          *string
	props       *LiftTableProps
	billingMode awsdynamodb.BillingMode
}

// newLiftTableBuilder creates a new lift table builder
func newLiftTableBuilder(scope constructs.Construct, id *string, props *LiftTableProps) *liftTableBuilder {
	return &liftTableBuilder{
		scope: scope,
		id:    id,
		props: props,
	}
}

// build constructs the complete LiftTable
func (b *liftTableBuilder) build() *LiftTable {
	this := constructs.NewConstruct(b.scope, b.id)

	b.determineBillingMode()
	b.setDefaults()
	tableProps := b.createTableProps()
	table := b.createTable(this, tableProps)

	liftTable := &LiftTable{
		Construct: this,
		Table:     table,
		GSIs:      make(map[string]*awsdynamodb.GlobalSecondaryIndexProps),
	}

	// Add GSIs after table creation
	b.addGSIs(liftTable)

	// Configure auto-scaling (includes GSI auto-scaling)
	b.configureAutoScaling(table)
	b.configureTags(table)

	return liftTable
}

// addGSIs adds Global Secondary Indexes to the table
func (b *liftTableBuilder) addGSIs(liftTable *LiftTable) {
	if b.props.GlobalSecondaryIndexes != nil {
		for _, gsi := range *b.props.GlobalSecondaryIndexes {
			liftTable.Table.AddGlobalSecondaryIndex(gsi)
			if gsi.IndexName != nil {
				liftTable.GSIs[*gsi.IndexName] = gsi
			}
		}
	}
}

// setDefaults sets default values for optional properties
func (b *liftTableBuilder) setDefaults() {
	if b.props.RemovalPolicy == "" {
		b.props.RemovalPolicy = awscdk.RemovalPolicy_RETAIN
	}
	if b.props.EnableAutoScaling == nil {
		b.props.EnableAutoScaling = jsii.Bool(true)
	}
	if b.props.EnablePointInTimeRecovery == nil {
		b.props.EnablePointInTimeRecovery = jsii.Bool(true)
	}
	if b.props.DeletionProtection == nil {
		b.props.DeletionProtection = jsii.Bool(false)
	}
	if b.props.TargetUtilization == nil {
		b.props.TargetUtilization = jsii.Number(70)
	}

	// Auto-scaling defaults for table
	if b.props.MinReadCapacity == nil {
		b.props.MinReadCapacity = jsii.Number(5)
	}
	if b.props.MaxReadCapacity == nil {
		b.props.MaxReadCapacity = jsii.Number(40000)
	}
	if b.props.MinWriteCapacity == nil {
		b.props.MinWriteCapacity = jsii.Number(5)
	}
	if b.props.MaxWriteCapacity == nil {
		b.props.MaxWriteCapacity = jsii.Number(40000)
	}

	// Auto-scaling defaults for GSIs
	if b.props.GSIMinReadCapacity == nil {
		b.props.GSIMinReadCapacity = jsii.Number(5)
	}
	if b.props.GSIMaxReadCapacity == nil {
		b.props.GSIMaxReadCapacity = jsii.Number(10000)
	}
	if b.props.GSIMinWriteCapacity == nil {
		b.props.GSIMinWriteCapacity = jsii.Number(5)
	}
	if b.props.GSIMaxWriteCapacity == nil {
		b.props.GSIMaxWriteCapacity = jsii.Number(10000)
	}
}

// determineBillingMode determines the appropriate billing mode
func (b *liftTableBuilder) determineBillingMode() {
	if b.props.ReadCapacity != nil || b.props.WriteCapacity != nil {
		b.billingMode = awsdynamodb.BillingMode_PROVISIONED
	} else {
		b.billingMode = awsdynamodb.BillingMode_PAY_PER_REQUEST
	}
}

// createTableProps creates the base table properties
func (b *liftTableBuilder) createTableProps() *awsdynamodb.TableProps {
	tableProps := &awsdynamodb.TableProps{
		TableName:          b.props.TableName,
		PartitionKey:       b.createPartitionKey(),
		BillingMode:        b.billingMode,
		RemovalPolicy:      b.props.RemovalPolicy,
		DeletionProtection: b.props.DeletionProtection,
	}

	b.configureSortKey(tableProps)
	b.configureCapacity(tableProps)
	b.configureEncryption(tableProps)
	b.configureReplication(tableProps)

	return tableProps
}

// configureEncryption sets up table encryption
func (b *liftTableBuilder) configureEncryption(tableProps *awsdynamodb.TableProps) {
	if b.props.Encryption != "" {
		tableProps.Encryption = b.props.Encryption
	} else {
		tableProps.Encryption = awsdynamodb.TableEncryption_AWS_MANAGED
	}
}

// configureReplication sets up Global Tables replication
func (b *liftTableBuilder) configureReplication(tableProps *awsdynamodb.TableProps) {
	if b.props.ReplicationRegions != nil && len(*b.props.ReplicationRegions) > 0 {
		tableProps.ReplicationRegions = b.props.ReplicationRegions
	}
}

// createPartitionKey creates the partition key attribute
func (b *liftTableBuilder) createPartitionKey() *awsdynamodb.Attribute {
	if b.props.PartitionKeyName == nil {
		panic("PartitionKeyName is required in LiftTableProps to match DynamORM model field name")
	}

	return &awsdynamodb.Attribute{
		Name: b.props.PartitionKeyName,
		Type: awsdynamodb.AttributeType_STRING,
	}
}

// configureSortKey configures the sort key if provided
func (b *liftTableBuilder) configureSortKey(tableProps *awsdynamodb.TableProps) {
	if b.props.SortKeyName != nil {
		tableProps.SortKey = &awsdynamodb.Attribute{
			Name: b.props.SortKeyName,
			Type: awsdynamodb.AttributeType_STRING,
		}
	}
}

// configureCapacity configures capacity for provisioned billing
func (b *liftTableBuilder) configureCapacity(tableProps *awsdynamodb.TableProps) {
	if b.billingMode != awsdynamodb.BillingMode_PROVISIONED {
		return
	}

	if b.props.ReadCapacity != nil {
		tableProps.ReadCapacity = b.props.ReadCapacity
	} else {
		tableProps.ReadCapacity = jsii.Number(5)
	}

	if b.props.WriteCapacity != nil {
		tableProps.WriteCapacity = b.props.WriteCapacity
	} else {
		tableProps.WriteCapacity = jsii.Number(5)
	}
}

// createTable creates the DynamoDB table with advanced features
func (b *liftTableBuilder) createTable(construct constructs.Construct, tableProps *awsdynamodb.TableProps) awsdynamodb.Table {
	b.configureAdvancedFeatures(tableProps)
	return awsdynamodb.NewTable(construct, jsii.String("Table"), tableProps)
}

// configureAdvancedFeatures configures advanced DynamoDB features
func (b *liftTableBuilder) configureAdvancedFeatures(tableProps *awsdynamodb.TableProps) {
	if b.props.EnablePointInTimeRecovery != nil && *b.props.EnablePointInTimeRecovery {
		tableProps.PointInTimeRecoverySpecification = &awsdynamodb.PointInTimeRecoverySpecification{
			PointInTimeRecoveryEnabled: jsii.Bool(true),
		}
	}

	if b.props.EnableStreams != nil && *b.props.EnableStreams {
		if b.props.StreamViewType != "" {
			tableProps.Stream = b.props.StreamViewType
		} else {
			tableProps.Stream = awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES
		}
	}

	if b.props.TimeToLiveAttribute != nil {
		tableProps.TimeToLiveAttribute = b.props.TimeToLiveAttribute
	}
}

// configureAutoScaling configures auto-scaling for provisioned billing
func (b *liftTableBuilder) configureAutoScaling(table awsdynamodb.Table) {
	if b.billingMode != awsdynamodb.BillingMode_PROVISIONED {
		return
	}

	if b.props.EnableAutoScaling == nil || !*b.props.EnableAutoScaling {
		return
	}

	// Configure table-level auto-scaling
	readScaling := table.AutoScaleReadCapacity(&awsdynamodb.EnableScalingProps{
		MinCapacity: b.props.MinReadCapacity,
		MaxCapacity: b.props.MaxReadCapacity,
	})
	readScaling.ScaleOnUtilization(&awsdynamodb.UtilizationScalingProps{
		TargetUtilizationPercent: b.props.TargetUtilization,
	})

	writeScaling := table.AutoScaleWriteCapacity(&awsdynamodb.EnableScalingProps{
		MinCapacity: b.props.MinWriteCapacity,
		MaxCapacity: b.props.MaxWriteCapacity,
	})
	writeScaling.ScaleOnUtilization(&awsdynamodb.UtilizationScalingProps{
		TargetUtilizationPercent: b.props.TargetUtilization,
	})

	// Configure GSI auto-scaling
	if b.props.GlobalSecondaryIndexes != nil {
		for _, gsi := range *b.props.GlobalSecondaryIndexes {
			if gsi.IndexName != nil {
				b.configureGSIAutoScaling(table, *gsi.IndexName)
			}
		}
	}
}

// configureGSIAutoScaling configures auto-scaling for a specific GSI
func (b *liftTableBuilder) configureGSIAutoScaling(table awsdynamodb.Table, indexName string) {
	gsiReadScaling := table.AutoScaleGlobalSecondaryIndexReadCapacity(jsii.String(indexName), &awsdynamodb.EnableScalingProps{
		MinCapacity: b.props.GSIMinReadCapacity,
		MaxCapacity: b.props.GSIMaxReadCapacity,
	})
	gsiReadScaling.ScaleOnUtilization(&awsdynamodb.UtilizationScalingProps{
		TargetUtilizationPercent: b.props.TargetUtilization,
	})

	gsiWriteScaling := table.AutoScaleGlobalSecondaryIndexWriteCapacity(jsii.String(indexName), &awsdynamodb.EnableScalingProps{
		MinCapacity: b.props.GSIMinWriteCapacity,
		MaxCapacity: b.props.GSIMaxWriteCapacity,
	})
	gsiWriteScaling.ScaleOnUtilization(&awsdynamodb.UtilizationScalingProps{
		TargetUtilizationPercent: b.props.TargetUtilization,
	})
}

// configureTags applies tags to the table
func (b *liftTableBuilder) configureTags(table awsdynamodb.Table) {
	// Add standard Lift tags
	awscdk.Tags_Of(table).Add(jsii.String("Framework"), jsii.String("Lift"), nil)
	awscdk.Tags_Of(table).Add(jsii.String("Component"), jsii.String("Database"), nil)

	// Add custom tags
	if b.props.Tags != nil {
		for key, value := range *b.props.Tags {
			awscdk.Tags_Of(table).Add(jsii.String(key), value, nil)
		}
	}
}

// GrantReadWrite grants read/write permissions to a Lambda function
//
// This method grants the specified Lambda function read and write permissions
// to the DynamoDB table. This is typically used to allow Lambda functions to
// perform CRUD operations on the table.
//
// Parameters:
//   - fn: The Lambda function to grant permissions to
func (t *LiftTable) GrantReadWrite(fn awslambda.IFunction) {
	t.Table.GrantReadWriteData(fn)
}

// GrantReadWriteData grants read/write permissions to any IAM grantee
func (t *LiftTable) GrantReadWriteData(grantee awsiam.IGrantable) awsiam.Grant {
	return t.Table.GrantReadWriteData(grantee)
}

// GrantReadData grants read-only permissions to any IAM grantee
func (t *LiftTable) GrantReadData(grantee awsiam.IGrantable) awsiam.Grant {
	return t.Table.GrantReadData(grantee)
}

// GrantWriteData grants write-only permissions to any IAM grantee
func (t *LiftTable) GrantWriteData(grantee awsiam.IGrantable) awsiam.Grant {
	return t.Table.GrantWriteData(grantee)
}

// GrantStreamRead grants permissions to read from the DynamoDB stream
func (t *LiftTable) GrantStreamRead(grantee awsiam.IGrantable) awsiam.Grant {
	return t.Table.GrantStreamRead(grantee)
}

// AddGlobalSecondaryIndex adds a GSI after table creation (note: requires table update)
func (t *LiftTable) AddGlobalSecondaryIndex(props *awsdynamodb.GlobalSecondaryIndexProps) {
	if props.IndexName != nil {
		t.GSIs[*props.IndexName] = props
	}
	// Note: This stores the GSI but doesn't update the physical table
	// For runtime GSI additions, use AWS SDK or CloudFormation updates
}

// GetTableName returns the table name
//
// This method returns the name of the DynamoDB table. This is useful for
// configuration and when setting up environment variables for applications
// that need to access the table.
//
// Returns:
//   - The table name
func (t *LiftTable) GetTableName() *string {
	return t.Table.TableName()
}

// GetTableArn returns the table ARN
//
// This method returns the ARN (Amazon Resource Name) of the DynamoDB table.
// This is useful for cross-service integrations and IAM permissions.
//
// Returns:
//   - The table ARN
func (t *LiftTable) GetTableArn() *string {
	return t.Table.TableArn()
}

// GetResourceName returns the resource name for monitoring
//
// This method returns the resource name for monitoring purposes. It implements
// the MonitorableResource interface.
//
// Returns:
//   - The resource name (table name)
func (t *LiftTable) GetResourceName() *string {
	return t.Table.TableName()
}

// GetStreamArn returns the DynamoDB stream ARN if streams are enabled
//
// This method returns the ARN of the DynamoDB stream if streams are enabled
// on the table. This is useful for setting up event-driven architectures.
//
// Returns:
//   - The stream ARN, or nil if streams are not enabled
func (t *LiftTable) GetStreamArn() *string {
	return t.Table.TableStreamArn()
}

// GetEnvironmentVariables returns environment variables for DynamORM integration
func (t *LiftTable) GetEnvironmentVariables() map[string]*string {
	return map[string]*string{
		"DYNAMODB_TABLE_NAME": t.Table.TableName(),
		"DYNAMORM_REGION":     jsii.String("${AWS::Region}"),
	}
}
