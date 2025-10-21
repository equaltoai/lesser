# DynamORM Embedded Structs Support - Overview for Mockery Team

## Executive Summary

We've successfully implemented full support for embedded structs in DynamORM, resolving the runtime panic issue you encountered when trying to use common base models across your entities. This enhancement enables you to use Go's struct embedding feature to share common fields across all your models, significantly reducing code duplication and improving maintainability.

## Problem Solved

Previously, when you tried to use embedded structs like this:

```go
type BaseModel struct {
    PK        string    `dynamorm:"pk"`
    SK        string    `dynamorm:"sk"`
    UpdatedAt time.Time `dynamorm:"updated_at"`
}

type Customer struct {
    BaseModel  // ❌ This would cause: "panic: reflect: Field index out of range"
    Name string
}
```

DynamORM would panic at runtime because it couldn't properly handle fields from embedded structs. This forced you to duplicate common fields across every model, leading to maintenance headaches and potential inconsistencies.

## Solution Implemented

The enhancement adds intelligent parsing of embedded structs, allowing DynamORM to:

1. **Recursively discover fields** in embedded structs
2. **Maintain proper field paths** for nested access
3. **Preserve all struct tags** and metadata
4. **Handle multiple levels** of embedding

## How It Works for Mockery

### 1. Define Your Base Model Once

```go
// internal/models/base.go
type BaseModel struct {
    // Primary composite keys
    PK string `dynamorm:"pk"`
    SK string `dynamorm:"sk"`
    
    // Global Secondary Indexes
    GSI1PK string `dynamorm:"index:gsi1,pk"`
    GSI1SK string `dynamorm:"index:gsi1,sk"`
    GSI2PK string `dynamorm:"index:gsi2,pk"`
    GSI2SK string `dynamorm:"index:gsi2,sk"`
    
    // Common metadata fields
    Type      string    `dynamorm:"attr:type"`
    AccountID string    `dynamorm:"attr:account_id"`
    UpdatedAt time.Time `dynamorm:"updated_at"`
    Version   int       `dynamorm:"version"`
    
    // Soft delete support
    Deleted   bool      `json:"deleted,omitempty" dynamorm:"attr:deleted"`
    DeletedAt time.Time `json:"-" dynamorm:"attr:deleted_at"`
}
```

### 2. Use It in All Your Models

```go
// internal/models/customer.go
type Customer struct {
    BaseModel  // ✅ Now works perfectly!
    
    // Stripe-compatible fields
    ID      string `json:"id" dynamorm:"attr:id"`
    Object  string `json:"object" dynamorm:"attr:object"`
    Created int64  `json:"created" dynamorm:"attr:created"`  // Unix timestamp
    Email   string `json:"email" dynamorm:"attr:email"`
    Name    string `json:"name" dynamorm:"attr:name"`
}

// internal/models/payment.go
type Payment struct {
    BaseModel  // ✅ Same base fields, no duplication
    
    ID       string `json:"id" dynamorm:"attr:id"`
    Amount   int64  `json:"amount" dynamorm:"attr:amount"`
    Currency string `json:"currency" dynamorm:"attr:currency"`
    Status   string `json:"status" dynamorm:"attr:status"`
}

// internal/models/subscription.go
type Subscription struct {
    BaseModel  // ✅ Consistent structure across all entities
    
    ID         string `json:"id" dynamorm:"attr:id"`
    CustomerID string `json:"customer_id" dynamorm:"attr:customer_id"`
    PlanID     string `json:"plan_id" dynamorm:"attr:plan_id"`
    Status     string `json:"status" dynamorm:"attr:status"`
}
```

### 3. Everything Just Works

```go
// Creating a customer
customer := &Customer{
    BaseModel: BaseModel{
        PK:        "CUSTOMER#cus_123",
        SK:        "METADATA",
        GSI1PK:    "ACCOUNT#acc_456",
        GSI1SK:    "CUSTOMER#cus_123",
        GSI2PK:    "EMAIL#john@example.com",
        GSI2SK:    "CUSTOMER#cus_123",
        Type:      "customer",
        AccountID: "acc_456",
    },
    ID:      "cus_123",
    Object:  "customer",
    Created: time.Now().Unix(),
    Email:   "john@example.com",
    Name:    "John Doe",
}

// All operations work seamlessly
err := db.Model(customer).Create()  // ✅ No panic!

// Queries work with embedded fields
var customers []Customer
err = db.Model(&Customer{}).
    Index("gsi1").
    Where("GSI1PK", "=", "ACCOUNT#acc_456").
    All(&customers)  // ✅ Works perfectly

// Updates handle embedded fields
customer.Email = "newemail@example.com"
customer.GSI2PK = "EMAIL#newemail@example.com"
err = db.Model(customer).Update("Email", "GSI2PK")  // ✅ Updates both fields
```

## Benefits for Mockery

### 1. **Massive Code Reduction**
- Define common fields once instead of in 20+ models
- Reduce your model definitions by ~50-70%
- Eliminate copy-paste errors

### 2. **Consistency Guaranteed**
- All entities have the same key structure
- GSI fields are always named consistently
- Metadata fields (UpdatedAt, Version, etc.) behave identically

### 3. **Easier Refactoring**
- Change key patterns in one place
- Add new GSIs to all models at once
- Rename fields globally with a single edit

### 4. **Perfect for Single-Table Design**
Your single-table pattern with composite keys and multiple GSIs is exactly what this feature was designed for:

```go
// All entities share the same key patterns
Customer: PK="CUSTOMER#id", SK="METADATA"
Payment:  PK="PAYMENT#id",  SK="METADATA"
Subscription: PK="SUBSCRIPTION#id", SK="METADATA"

// GSI1 for account access patterns
GSI1PK="ACCOUNT#id", GSI1SK="CUSTOMER#id"
GSI1PK="ACCOUNT#id", GSI1SK="PAYMENT#id"

// GSI2 for email lookups, time-based queries, etc.
GSI2PK="EMAIL#email", GSI2SK="CUSTOMER#id"
GSI2PK="DATE#2024-01-15", GSI2SK="PAYMENT#id"
```

### 5. **Backward Compatible**
- No changes needed to existing code
- Gradually migrate models as needed
- Mix embedded and non-embedded models freely

## Migration Path

### Option 1: Gradual Migration (Recommended)
1. Keep existing models as-is (they still work)
2. Create BaseModel for new features
3. Migrate existing models during regular maintenance

### Option 2: Full Migration
1. Create BaseModel with all common fields
2. Update each model to embed BaseModel
3. Remove duplicated field definitions
4. Test thoroughly (all operations remain the same)

## Technical Details

### What Changed Under the Hood
- DynamORM now recursively parses embedded structs
- Field access uses proper index paths (e.g., [0,3] for field 3 in embedded struct at position 0)
- All operations (Create, Read, Update, Delete, Query) handle embedded fields transparently

### Performance Impact
- **Minimal**: Parsing happens once during model registration
- **No runtime overhead**: Field paths are cached
- **Same DynamoDB operations**: No additional API calls

## Common Patterns for Mockery

### 1. Time-Series Data with Soft Deletes
```go
type TimeSeriesBase struct {
    BaseModel
    
    // Time-based GSI for range queries
    TimestampSK string `dynamorm:"index:gsi3,sk"`
}

type Event struct {
    TimeSeriesBase
    EventType string
    Payload   map[string]interface{}
}
```

### 2. Multi-Tenant Isolation
```go
// GSI1 always partitioned by account
customer.GSI1PK = "ACCOUNT#" + accountID
payment.GSI1PK = "ACCOUNT#" + accountID

// Query all entities for an account
db.Model(&Customer{}).Index("gsi1").Where("GSI1PK", "=", "ACCOUNT#123").All(&customers)
db.Model(&Payment{}).Index("gsi1").Where("GSI1PK", "=", "ACCOUNT#123").All(&payments)
```

### 3. Hierarchical Data
```go
// Parent-child relationships using sort keys
parent.SK = "METADATA"
child.SK = "CHILD#" + childID
grandchild.SK = "CHILD#" + childID + "#GRANDCHILD#" + grandchildID
```

## Testing Your Integration

We've included comprehensive tests that demonstrate:
- ✅ Basic CRUD with embedded structs
- ✅ GSI queries with inherited index fields
- ✅ Updates that maintain consistency
- ✅ Soft delete patterns
- ✅ Multiple entity types in single table

Run the embedded struct tests:
```bash
go test ./tests/integration -run TestEmbeddedStructSupport -v
```

## Next Steps

1. **Update your BaseModel** to include all common fields
2. **Test with one model** (e.g., Customer) to verify everything works
3. **Gradually migrate** other models as you work on them
4. **Enjoy cleaner code** and easier maintenance!

## Support

If you encounter any issues or have questions:
- The feature is fully tested and production-ready
- All existing DynamORM features work with embedded structs
- Performance characteristics remain the same

This enhancement makes DynamORM work the way Go developers expect, following the principle of least surprise. Your single-table design pattern with shared base models is now fully supported!