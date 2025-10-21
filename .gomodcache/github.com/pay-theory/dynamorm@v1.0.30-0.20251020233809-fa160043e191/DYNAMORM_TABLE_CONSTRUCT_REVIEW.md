# Review of DynamORMTable CDK Construct Implementation

## Overall Assessment
The implementation is a reasonable foundation but misses several key DynamORM requirements and has some architectural issues.

## Issues and Recommendations

### 1. **Table Creation Pattern is Problematic**

**Current Issue**:
```go
// Note: The actual table creation is deferred until CreateTable() is called
// by extending constructs with specific partition and sort keys.
```

**Problem**: This two-step creation pattern is confusing and doesn't align with CDK best practices.

**Recommended Fix**:
```go
type DynamORMTableProps struct {
    // Required for DynamORM
    PartitionKey *awsdynamodb.Attribute
    SortKey      *awsdynamodb.Attribute  // Optional but common in DynamORM
    
    // Rest of props...
}

func NewDynamORMTable(scope constructs.Construct, id *string, props *DynamORMTableProps) *DynamORMTable {
    // Create table immediately with proper keys
    tableProps := &awsdynamodb.TableProps{
        TableName:    props.TableName,
        PartitionKey: props.PartitionKey,
        SortKey:      props.SortKey,
        BillingMode:  props.BillingMode,
        // ... other props
    }
    
    table := awsdynamodb.NewTable(this, jsii.String("Table"), tableProps)
    // ...
}
```

### 2. **Missing Critical DynamORM Features**

**Missing Elements**:
- No support for composite keys (common in DynamORM)
- No automatic timestamp attributes (`created_at`, `updated_at`)
- No version attribute support for optimistic locking
- No encryption configuration

**Add These Methods**:
```go
// ConfigureForDynamORM sets up standard DynamORM patterns
func (t *DynamORMTable) ConfigureForDynamORM() {
    // Add standard attributes that DynamORM expects
    t.AddDynamORMAttributes()
    
    // Configure encryption
    t.Table.SetEncryption(awsdynamodb.TableEncryption_AWS_MANAGED)
}

// AddDynamORMAttributes adds commonly used DynamORM attributes
func (t *DynamORMTable) AddDynamORMAttributes() {
    // These are commonly used in DynamORM models
    // Note: Actual attributes depend on the model
}
```

### 3. **GSI Implementation Needs Enhancement**

**Current Issue**: GSIs are too simplified and don't support DynamORM patterns.

**Enhanced GSI Support**:
```go
type GSIProps struct {
    IndexName      *string
    PartitionKey   *awsdynamodb.Attribute  // Full attribute definition
    SortKey        *awsdynamodb.Attribute  // Optional
    ProjectionType awsdynamodb.ProjectionType
    // For composite keys
    CompositeFields []string  // For dynamorm composite key support
}

// AddDynamORMIndex adds an index following DynamORM naming conventions
func (t *DynamORMTable) AddDynamORMIndex(indexName string, pkAttr, skAttr *awsdynamodb.Attribute) {
    // DynamORM expects specific index naming: "gsi-{name}"
    gsiName := fmt.Sprintf("gsi-%s", indexName)
    
    t.Table.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
        IndexName:      jsii.String(gsiName),
        PartitionKey:   pkAttr,
        SortKey:        skAttr,
        ProjectionType: awsdynamodb.ProjectionType_ALL,
    })
}
```

### 4. **Environment Variable Configuration Missing**

**Add Method**:
```go
// GetEnvironmentVariables returns environment variables for Lambda functions
func (t *DynamORMTable) GetEnvironmentVariables() *map[string]*string {
    return &map[string]*string{
        "DYNAMODB_TABLE_NAME": t.Table.TableName(),
        "AWS_REGION":         jsii.String(*awscdk.Stack_Of(t).Region()),
    }
}
```

### 5. **Multi-Tenant Support Incomplete**

**Current**: Has a flag but no implementation.

**Add Implementation**:
```go
// ConfigureMultiTenant sets up multi-tenant patterns
func (t *DynamORMTable) ConfigureMultiTenant(tenantAttribute string) {
    if tenantAttribute == "" {
        tenantAttribute = "TenantID"
    }
    
    // Add GSI for tenant queries
    t.AddDynamORMIndex("tenant", 
        &awsdynamodb.Attribute{
            Name: jsii.String(tenantAttribute),
            Type: awsdynamodb.AttributeType_STRING,
        },
        nil,
    )
    
    // Add tenant isolation tag
    awscdk.Tags_Of(t.Table).Add(jsii.String("MultiTenant"), jsii.String("true"), nil)
}

// GrantTenantIsolatedAccess grants access with tenant isolation
func (t *DynamORMTable) GrantTenantIsolatedAccess(grantee awsiam.IGrantable, tenantAttribute string) {
    if tenantAttribute == "" {
        tenantAttribute = "TenantID"
    }
    
    grantee.GrantPrincipal().AddToPrincipalPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
        Actions: &[]*string{
            jsii.String("dynamodb:Query"),
            jsii.String("dynamodb:GetItem"),
            jsii.String("dynamodb:PutItem"),
            jsii.String("dynamodb:UpdateItem"),
            jsii.String("dynamodb:DeleteItem"),
        },
        Resources: &[]*string{
            t.Table.TableArn(),
            jsii.String(*t.Table.TableArn() + "/index/gsi-tenant"),
        },
        Conditions: &map[string]interface{}{
            "ForAllValues:StringEquals": map[string]interface{}{
                "dynamodb:LeadingKeys": []string{"${aws:PrincipalTag/TenantID}"},
            },
        },
    }))
}
```

### 6. **Better Integration with DynamORM Models**

**Add Model Compatibility Check**:
```go
// DynamORMModelSpec defines expected model structure
type DynamORMModelSpec struct {
    ModelName    string
    PartitionKey string
    SortKey      string
    GSIs         []GSISpec
    TTLAttribute string
    Attributes   map[string]string
}

// ValidateModelCompatibility checks if table matches DynamORM model
func (t *DynamORMTable) ValidateModelCompatibility(spec DynamORMModelSpec) error {
    // Validate partition key matches
    // Validate sort key matches
    // Validate GSIs exist
    // Return errors if mismatched
    return nil
}
```

## Improved Implementation Example

Here's how the construct should be structured:

```go
package constructs

import (
    "fmt"
    
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
    "github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
)

type DynamORMTableProps struct {
    // Required
    PartitionKey *awsdynamodb.Attribute
    SortKey      *awsdynamodb.Attribute // Optional but common
    
    // Table configuration
    TableName                 *string
    BillingMode              awsdynamodb.BillingMode
    PointInTimeRecovery      *bool
    Stream                   awsdynamodb.StreamViewType
    TimeToLiveAttribute      *string
    DeletionProtection       *bool
    RemovalPolicy            awscdk.RemovalPolicy
    
    // DynamORM specific
    EnableMultiTenant        *bool
    TenantAttribute          *string
    EnableVersioning         *bool
    EnableTimestamps         *bool
    
    // Capacity (for provisioned mode)
    ReadCapacity             *float64
    WriteCapacity            *float64
    
    // Tags
    Tags                     *map[string]*string
}

type DynamORMTable struct {
    constructs.Construct
    Table       awsdynamodb.Table
    props       *DynamORMTableProps
    modelSpec   *DynamORMModelSpec
}

func NewDynamORMTable(scope constructs.Construct, id *string, props *DynamORMTableProps) *DynamORMTable {
    this := constructs.NewConstruct(scope, id)
    
    // Apply defaults
    props = applyDefaults(props)
    
    // Create table with DynamORM-optimized settings
    tableProps := &awsdynamodb.TableProps{
        TableName:           props.TableName,
        PartitionKey:        props.PartitionKey,
        SortKey:             props.SortKey,
        BillingMode:         props.BillingMode,
        PointInTimeRecovery: props.PointInTimeRecovery,
        DeletionProtection:  props.DeletionProtection,
        RemovalPolicy:       props.RemovalPolicy,
        Encryption:          awsdynamodb.TableEncryption_AWS_MANAGED,
    }
    
    // Configure TTL if specified
    if props.TimeToLiveAttribute != nil {
        tableProps.TimeToLiveAttribute = props.TimeToLiveAttribute
    }
    
    // Configure streams if needed
    if props.Stream != "" {
        tableProps.Stream = props.Stream
    }
    
    table := awsdynamodb.NewTable(this, jsii.String("Table"), tableProps)
    
    dt := &DynamORMTable{
        Construct: this,
        Table:     table,
        props:     props,
    }
    
    // Configure DynamORM-specific features
    if *props.EnableMultiTenant {
        dt.ConfigureMultiTenant(*props.TenantAttribute)
    }
    
    // Add standard tags
    dt.addStandardTags()
    
    // Add custom tags
    if props.Tags != nil {
        dt.AddTags(props.Tags)
    }
    
    return dt
}

func applyDefaults(props *DynamORMTableProps) *DynamORMTableProps {
    if props.BillingMode == "" {
        props.BillingMode = awsdynamodb.BillingMode_PAY_PER_REQUEST
    }
    if props.PointInTimeRecovery == nil {
        props.PointInTimeRecovery = jsii.Bool(true)
    }
    if props.RemovalPolicy == "" {
        props.RemovalPolicy = awscdk.RemovalPolicy_RETAIN
    }
    if props.DeletionProtection == nil {
        props.DeletionProtection = jsii.Bool(true) // Default to true for production safety
    }
    if props.EnableMultiTenant == nil {
        props.EnableMultiTenant = jsii.Bool(false)
    }
    if props.TenantAttribute == nil {
        props.TenantAttribute = jsii.String("TenantID")
    }
    if props.EnableVersioning == nil {
        props.EnableVersioning = jsii.Bool(true)
    }
    if props.EnableTimestamps == nil {
        props.EnableTimestamps = jsii.Bool(true)
    }
    return props
}

func (t *DynamORMTable) addStandardTags() {
    stack := awscdk.Stack_Of(t)
    awscdk.Tags_Of(t.Table).Add(jsii.String("Framework"), jsii.String("DynamORM"), nil)
    awscdk.Tags_Of(t.Table).Add(jsii.String("ManagedBy"), jsii.String("CDK"), nil)
    awscdk.Tags_Of(t.Table).Add(jsii.String("Environment"), stack.StackName(), nil)
}

// Rest of implementation...
```

## Summary

The current implementation is a good foundation but needs:

1. **Immediate table creation** instead of deferred pattern
2. **DynamORM-specific features** like composite keys and timestamps
3. **Proper multi-tenant implementation** with policies and GSIs
4. **Environment variable helpers** for Lambda integration
5. **Model compatibility validation** to ensure table matches Go models
6. **Better defaults** aligned with DynamORM best practices

These changes will make the construct more useful for DynamORM users and prevent common configuration mistakes.