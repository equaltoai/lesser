# Advice for Lift Team: Leveraging DynamORM in CDK Constructs

## Key Recommendations

### 1. Use a DynamORMTable CDK Construct

Create a base `DynamORMTable` construct that encapsulates DynamORM-specific requirements:

```typescript
export interface DynamORMTableProps {
  modelName: string;
  partitionKey: { name: string; type: AttributeType };
  sortKey?: { name: string; type: AttributeType };
  globalSecondaryIndexes?: DynamORMGSI[];
  ttlAttribute?: string;
  streamViewType?: StreamViewType;
  multiTenant?: boolean;
  autoScaling?: AutoScalingConfig;
}

export class DynamORMTable extends Construct {
  public readonly table: Table;
  
  constructor(scope: Construct, id: string, props: DynamORMTableProps) {
    super(scope, id);
    
    this.table = new Table(this, 'Table', {
      partitionKey: props.partitionKey,
      sortKey: props.sortKey,
      billingMode: BillingMode.PAY_PER_REQUEST, // Default for Lambda
      pointInTimeRecovery: true, // Always enable
      encryption: TableEncryption.AWS_MANAGED,
      stream: props.streamViewType || StreamViewType.NEW_AND_OLD_IMAGES,
      timeToLiveAttribute: props.ttlAttribute,
      
      // DynamORM expects exact attribute names
      tableName: `${props.modelName}-${Stack.of(this).account}-${Stack.of(this).region}`,
    });
    
    // Add GSIs with DynamORM naming convention
    props.globalSecondaryIndexes?.forEach(gsi => {
      this.table.addGlobalSecondaryIndex({
        indexName: gsi.indexName, // Must match dynamorm:"index:name,pk" tag
        partitionKey: gsi.partitionKey,
        sortKey: gsi.sortKey,
        projectionType: gsi.projectionType || ProjectionType.ALL,
      });
    });
    
    // Add standard tags
    Tags.of(this.table).add('DynamORM', 'true');
    Tags.of(this.table).add('ModelName', props.modelName);
  }
  
  // Helper to grant DynamORM-specific permissions
  grantDynamORMAccess(grantee: IGrantable): void {
    this.table.grantReadWriteData(grantee);
    
    // DynamORM needs additional permissions for advanced features
    grantee.grantPrincipal.addToPrincipalPolicy(new PolicyStatement({
      actions: [
        'dynamodb:ConditionCheckItem',
        'dynamodb:DescribeTable',
        'dynamodb:DescribeTimeToLive',
      ],
      resources: [this.table.tableArn],
    }));
  }
}
```

### 2. Pattern-Specific Constructs

Build on top of `DynamORMTable` for specific patterns:

```typescript
export class RateLimitTable extends DynamORMTable {
  constructor(scope: Construct, id: string, props: RateLimitTableProps) {
    super(scope, id, {
      modelName: 'RateLimit',
      partitionKey: { name: 'Identifier', type: AttributeType.STRING },
      sortKey: { name: 'WindowTime', type: AttributeType.STRING },
      ttlAttribute: 'ExpiresAt',
      globalSecondaryIndexes: [
        {
          indexName: 'gsi-ip',
          partitionKey: { name: 'IPAddress', type: AttributeType.STRING },
        },
        {
          indexName: 'gsi-user',
          partitionKey: { name: 'UserID', type: AttributeType.STRING },
        },
        {
          indexName: 'gsi-tenant',
          partitionKey: { name: 'TenantID', type: AttributeType.STRING },
        },
      ],
      ...props,
    });
  }
}

export class IdempotencyTable extends DynamORMTable {
  constructor(scope: Construct, id: string, props: IdempotencyTableProps) {
    super(scope, id, {
      modelName: 'Idempotency',
      partitionKey: { name: 'IdempotencyKey', type: AttributeType.STRING },
      sortKey: { name: 'SK', type: AttributeType.STRING },
      ttlAttribute: 'ExpiresAt',
      globalSecondaryIndexes: [
        {
          indexName: 'gsi-function',
          partitionKey: { name: 'FunctionName', type: AttributeType.STRING },
        },
        {
          indexName: 'gsi-status',
          partitionKey: { name: 'Status', type: AttributeType.STRING },
        },
      ],
      ...props,
    });
  }
}
```

### 3. Lambda Function Integration

Create a base construct that automatically configures Lambda for DynamORM:

```typescript
export interface DynamORMFunctionProps extends FunctionProps {
  dynamormTables: DynamORMTable[];
  enableLambdaOptimizations?: boolean;
  multiTenant?: boolean;
}

export class DynamORMFunction extends Function {
  constructor(scope: Construct, id: string, props: DynamORMFunctionProps) {
    super(scope, id, {
      ...props,
      environment: {
        ...props.environment,
        // DynamORM configuration
        DYNAMORM_LAMBDA_OPTIMIZATIONS: props.enableLambdaOptimizations !== false ? 'true' : 'false',
        DYNAMORM_MULTI_TENANT: props.multiTenant ? 'true' : 'false',
        
        // Table names
        ...props.dynamormTables.reduce((env, table, index) => ({
          ...env,
          [`${table.node.id.toUpperCase()}_TABLE_NAME`]: table.table.tableName,
        }), {}),
      },
    });
    
    // Grant permissions
    props.dynamormTables.forEach(table => {
      table.grantDynamORMAccess(this);
    });
  }
}
```

### 4. Complete Pattern Implementation

Here's how the patterns should work together:

```typescript
export class RateLimitedFunction extends Construct {
  public readonly function: DynamORMFunction;
  public readonly rateLimitTable: RateLimitTable;
  
  constructor(scope: Construct, id: string, props: RateLimitedFunctionProps) {
    super(scope, id);
    
    // Create DynamORM-compatible table
    this.rateLimitTable = new RateLimitTable(this, 'RateLimitTable', {
      multiTenant: props.multiTenant,
    });
    
    // Create function with DynamORM integration
    this.function = new DynamORMFunction(this, 'Function', {
      ...props.functionProps,
      dynamormTables: [this.rateLimitTable],
      enableLambdaOptimizations: true,
      multiTenant: props.multiTenant,
      environment: {
        ...props.functionProps.environment,
        RATE_LIMIT_WINDOW_SIZE: props.windowSize || '60',
        RATE_LIMIT_MAX_REQUESTS: props.maxRequests?.toString() || '100',
      },
    });
  }
}
```

### 5. Model Registration Pattern

Provide a standardized way to bundle models with constructs:

```typescript
// In your construct package
export const RateLimitModel = `
package models

import (
    "os"
    "time"
)

type RateLimitRecord struct {
    Identifier string    \`dynamorm:"pk" json:"identifier"\`
    WindowTime string    \`dynamorm:"sk" json:"window_time"\`
    IPAddress  string    \`dynamorm:"index:gsi-ip,pk" json:"ip_address,omitempty"\`
    UserID     string    \`dynamorm:"index:gsi-user,pk" json:"user_id,omitempty"\`
    TenantID   string    \`dynamorm:"index:gsi-tenant,pk" json:"tenant_id,omitempty"\`
    Count      int       \`json:"count"\`
    ExpiresAt  time.Time \`dynamorm:"ttl" json:"expires_at"\`
    CreatedAt  time.Time \`dynamorm:"created_at" json:"created_at"\`
    UpdatedAt  time.Time \`dynamorm:"updated_at" json:"updated_at"\`
}

func (r *RateLimitRecord) TableName() string {
    return os.Getenv("RATELIMITTABLE_TABLE_NAME")
}
`;

// Users can then copy this model to their Lambda code
```

### 6. Multi-Tenant Considerations

For multi-tenant scenarios, enhance the base construct:

```typescript
export class MultiTenantDynamORMTable extends DynamORMTable {
  constructor(scope: Construct, id: string, props: DynamORMTableProps) {
    super(scope, id, props);
    
    // Add tenant isolation policy
    const tenantIsolationPolicy = new PolicyStatement({
      effect: Effect.ALLOW,
      actions: ['dynamodb:Query', 'dynamodb:GetItem'],
      resources: [
        this.table.tableArn,
        `${this.table.tableArn}/index/gsi-tenant`,
      ],
      conditions: {
        'ForAllValues:StringEquals': {
          'dynamodb:LeadingKeys': ['${aws:PrincipalTag/TenantID}'],
        },
      },
    });
    
    // This policy can be attached to tenant-scoped roles
  }
  
  grantTenantAccess(grantee: IGrantable, tenantId?: string): void {
    if (tenantId) {
      // Grant access to specific tenant
      grantee.grantPrincipal.addToPrincipalPolicy(new PolicyStatement({
        actions: ['dynamodb:*'],
        resources: [this.table.tableArn],
        conditions: {
          'ForAllValues:StringEquals': {
            'dynamodb:LeadingKeys': [tenantId],
          },
        },
      }));
    } else {
      // Grant access based on principal tags
      this.grantDynamORMAccess(grantee);
    }
  }
}
```

### 7. Best Practices for Lift Constructs

1. **Always Use DynamORMTable Base**: Don't create raw DynamoDB tables for DynamORM models
2. **Environment Variable Naming**: Use consistent `{CONSTRUCT_ID}_TABLE_NAME` pattern
3. **Model Bundling**: Include model definitions as exported constants or in documentation
4. **Type Safety**: Provide TypeScript interfaces that match Go structs
5. **Migration Support**: Include CloudFormation custom resources for table migrations if needed

### 8. Example: Complete EventDrivenAPI with DynamORM

```typescript
export class EventDrivenAPI extends Construct {
  private readonly eventTable: DynamORMTable;
  private readonly idempotencyTable: IdempotencyTable;
  
  constructor(scope: Construct, id: string, props: EventDrivenAPIProps) {
    super(scope, id);
    
    // Create DynamORM tables
    this.eventTable = new DynamORMTable(this, 'EventTable', {
      modelName: 'Event',
      partitionKey: { name: 'EventID', type: AttributeType.STRING },
      sortKey: { name: 'Timestamp', type: AttributeType.STRING },
      streamViewType: StreamViewType.NEW_AND_OLD_IMAGES,
      globalSecondaryIndexes: [
        {
          indexName: 'gsi-type',
          partitionKey: { name: 'EventType', type: AttributeType.STRING },
          sortKey: { name: 'Timestamp', type: AttributeType.STRING },
        },
      ],
    });
    
    this.idempotencyTable = new IdempotencyTable(this, 'IdempotencyTable');
    
    // Create Lambda with DynamORM integration
    const handler = new DynamORMFunction(this, 'Handler', {
      runtime: Runtime.PROVIDED_AL2,
      handler: 'bootstrap',
      code: Code.fromAsset('path/to/handler'),
      dynamormTables: [this.eventTable, this.idempotencyTable],
      environment: {
        EVENT_BUS_NAME: props.eventBusName,
      },
    });
    
    // Configure API Gateway
    const api = new RestApi(this, 'API');
    api.root.addMethod('POST', new LambdaIntegration(handler));
  }
}
```

## Summary

The key to successful DynamORM integration in Lift constructs is:

1. **Abstraction**: Hide DynamORM complexity behind well-designed constructs
2. **Convention**: Use consistent patterns for table creation and environment variables
3. **Type Safety**: Ensure TypeScript constructs match Go model expectations
4. **Flexibility**: Support both simple and advanced use cases (multi-tenant, streams, etc.)
5. **Documentation**: Bundle model definitions and usage examples with constructs

This approach ensures that Lift users get the benefits of DynamORM (performance, type safety, Lambda optimizations) without needing to understand all the implementation details.