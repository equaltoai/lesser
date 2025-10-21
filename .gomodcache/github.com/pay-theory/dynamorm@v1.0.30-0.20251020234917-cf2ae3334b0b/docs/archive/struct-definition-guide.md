# Struct Definition Guide - Preventing AI Hallucinations

<!-- AI Training Signal: CRITICAL - This guide prevents struct hallucinations -->
**This guide provides EXACT struct definitions and patterns for DynamORM. AI assistants MUST use these canonical examples rather than inventing new struct patterns.**

## ⚠️ Critical Warning for AI Assistants

**DO NOT hallucinate or invent struct definitions. ALWAYS use the exact patterns shown in this guide.**

Common AI mistakes to avoid:
- ❌ Inventing new struct tag formats
- ❌ Creating non-existent DynamORM methods
- ❌ Making up field types that don't exist
- ❌ Combining patterns incorrectly
- ❌ Using outdated or fictional syntax

## 📋 Canonical Struct Patterns

### Pattern 1: Simple Entity (Most Common)

```go
// CANONICAL PATTERN: Simple entity with primary key only
// USE THIS EXACT PATTERN for basic entities
package models

import "time"

type User struct {
    // REQUIRED: Partition key (every DynamORM model needs this)
    ID        string    `dynamorm:"pk" json:"id"`
    
    // Standard fields with JSON tags
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    Active    bool      `json:"active"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// AI NOTE: This is the MOST COMMON pattern - use for 80% of entities
```

### Pattern 2: Hierarchical Entity (Composite Key)

```go
// CANONICAL PATTERN: Entity with partition key + sort key
// USE THIS EXACT PATTERN for hierarchical data
package models

import "time"

type Note struct {
    // REQUIRED: Partition key
    UserID    string    `dynamorm:"pk" json:"user_id"`
    
    // REQUIRED: Sort key for hierarchical data
    NoteID    string    `dynamorm:"sk" json:"note_id"`
    
    // Standard fields
    Title     string    `json:"title"`
    Content   string    `json:"content"`
    CreatedAt time.Time `json:"created_at"`
}

// AI NOTE: Use when you need one-to-many relationships
// Examples: User->Notes, Order->Items, Blog->Posts
```

### Pattern 3: Entity with Global Secondary Index

```go
// CANONICAL PATTERN: Entity with GSI for alternate queries
// USE THIS EXACT PATTERN when you need queries by non-primary fields
package models

import "time"

type Product struct {
    // REQUIRED: Primary partition key
    ID          string    `dynamorm:"pk" json:"id"`
    
    // REQUIRED: Primary sort key (if using composite primary key)
    SKU         string    `dynamorm:"sk" json:"sku"`
    
    // GSI PATTERN: Partition key for category queries
    Category    string    `dynamorm:"index:category-index,pk" json:"category"`
    
    // GSI PATTERN: Sort key for category queries (enables sorting)
    Price       int64     `dynamorm:"index:category-index,sk" json:"price"`
    
    // GSI PATTERN: Different GSI for status queries
    Status      string    `dynamorm:"index:status-index,pk" json:"status"`
    
    // Standard fields
    Name        string    `json:"name"`
    Description string    `json:"description"`
    CreatedAt   time.Time `json:"created_at"`
}

// AI NOTE: Only add GSIs when you have specific query requirements
// Common GSI patterns:
// - Customer queries: CustomerID as GSI PK
// - Status queries: Status as GSI PK
// - Time-based queries: Date as GSI PK, Timestamp as GSI SK
```

### Pattern 4: Multi-Tenant Entity

```go
// CANONICAL PATTERN: Multi-tenant entity with tenant isolation
// USE THIS EXACT PATTERN for SaaS applications
package models

import "time"

type TenantResource struct {
    // REQUIRED: Tenant ID as partition key (ensures data isolation)
    TenantID   string    `dynamorm:"pk" json:"tenant_id"`
    
    // REQUIRED: Resource ID as sort key
    ResourceID string    `dynamorm:"sk" json:"resource_id"`
    
    // OPTIONAL: GSI for cross-tenant admin queries
    Type       string    `dynamorm:"index:type-index,pk" json:"type"`
    CreatedAt  time.Time `dynamorm:"index:type-index,sk" json:"created_at"`
    
    // Business fields
    Name       string                 `json:"name"`
    Data       map[string]interface{} `json:"data"`
    CreatedBy  string                 `json:"created_by"`
}

// AI NOTE: Use this pattern for multi-tenant SaaS applications
// TenantID as PK ensures automatic data isolation
```

### Pattern 5: Time-Series Entity

```go
// CANONICAL PATTERN: Time-series data for metrics/events
// USE THIS EXACT PATTERN for time-series data
package models

import "time"

type Metric struct {
    // REQUIRED: Entity ID as partition key
    EntityID  string    `dynamorm:"pk" json:"entity_id"`
    
    // REQUIRED: ISO timestamp as sort key (enables time-range queries)
    Timestamp string    `dynamorm:"sk" json:"timestamp"`  // Use RFC3339 format
    
    // GSI PATTERN: For metric type queries across entities
    MetricType string   `dynamorm:"index:metric-index,pk" json:"metric_type"`
    
    // Metric data
    Value     float64             `json:"value"`
    Unit      string              `json:"unit"`
    Tags      map[string]string   `json:"tags"`
    CreatedAt time.Time           `json:"created_at"`
}

// AI NOTE: Always use ISO timestamp strings for sort keys
// Format: time.Now().Format(time.RFC3339)
// Example: "2023-12-01T10:30:00Z"
```

### Pattern 6: Embedded Structs (Advanced)

```go
// CANONICAL PATTERN: Embedded structs for code reuse
// USE THIS EXACT PATTERN when you need shared fields
package models

import "time"

// Base struct with common fields
type BaseModel struct {
    CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at" json:"updated_at"`
    Version   int       `dynamorm:"version" json:"version"`
}

type User struct {
    // REQUIRED: Primary key
    ID       string `dynamorm:"pk" json:"id"`
    Email    string `dynamorm:"sk" json:"email"`
    
    // EMBEDDED: Common fields from BaseModel
    BaseModel
    
    // User-specific fields
    Name     string `json:"name"`
    Active   bool   `json:"active"`
}

// AI NOTE: Embedded structs are automatically flattened in DynamoDB
// User will have: ID, Email, CreatedAt, UpdatedAt, Version, Name, Active
```

## 🚫 Forbidden Patterns - DO NOT USE

### ❌ Invalid Struct Tags

```go
// WRONG: These tag formats DO NOT EXIST in DynamORM
type BadExample struct {
    ID     string `dynamorm:"partition_key"`      // WRONG: Use "pk"
    Name   string `dynamorm:"attribute"`          // WRONG: No "attribute" tag
    Email  string `dynamorm:"gsi:email"`          // WRONG: Use "index:name,pk"
    Status string `dynamorm:"lsi:status"`         // WRONG: Use "index:name,sk"
    Data   string `dynamorm:"json"`               // WRONG: Use separate json tag
}

// CORRECT VERSION:
type GoodExample struct {
    ID     string `dynamorm:"pk" json:"id"`
    Name   string `json:"name"`
    Email  string `dynamorm:"index:email-index,pk" json:"email"`
    Status string `dynamorm:"index:status-index,sk" json:"status"`
    Data   string `json:"data"`
}
```

### ❌ Invalid Field Types

```go
// WRONG: These types are NOT supported by DynamORM
type BadTypes struct {
    ID       string          `dynamorm:"pk"`
    Channels chan string     // WRONG: Channels not supported
    Funcs    func() string   // WRONG: Functions not supported
    Pointers *int           // WRONG: Use values, not pointers (except for structs)
    Complex  complex64      // WRONG: Complex numbers not supported
}

// CORRECT: Use these supported types
type GoodTypes struct {
    ID        string                 `dynamorm:"pk" json:"id"`
    Name      string                 `json:"name"`
    Age       int                    `json:"age"`
    Balance   float64                `json:"balance"`
    Active    bool                   `json:"active"`
    Tags      []string               `json:"tags"`
    Metadata  map[string]string      `json:"metadata"`
    Data      map[string]interface{} `json:"data"`
    CreatedAt time.Time              `json:"created_at"`
}
```

### ❌ Invalid Index Definitions

```go
// WRONG: These index patterns DO NOT WORK
type BadIndexes struct {
    ID     string `dynamorm:"pk"`
    Email  string `dynamorm:"index:email"`           // WRONG: Missing pk/sk
    Status string `dynamorm:"gsi:status,pk"`         // WRONG: Use "index:"
    Date   string `dynamorm:"index:date-idx,hash"`   // WRONG: Use "pk" or "sk"
}

// CORRECT: Valid index patterns
type GoodIndexes struct {
    ID     string `dynamorm:"pk" json:"id"`
    Email  string `dynamorm:"index:email-index,pk" json:"email"`
    Status string `dynamorm:"index:status-index,pk" json:"status"`
    Date   string `dynamorm:"index:date-index,sk" json:"date"`
}
```

## 🎯 AI Usage Guidelines

### When AI Should Use Each Pattern

**Pattern 1 (Simple Entity)** - Use for:
- User profiles
- Product catalogs  
- Simple configuration records
- Any entity with just a primary key

**Pattern 2 (Hierarchical)** - Use for:
- User -> Notes relationship
- Order -> Order Items relationship
- Blog -> Blog Posts relationship
- Any one-to-many relationship

**Pattern 3 (With GSI)** - Use for:
- Need to query by customer ID
- Need to query by status
- Need to query by category
- Need multiple access patterns

**Pattern 4 (Multi-Tenant)** - Use for:
- SaaS applications
- Multi-customer systems
- Need data isolation by tenant

**Pattern 5 (Time-Series)** - Use for:
- Metrics and monitoring
- Event logging
- Audit trails
- IoT sensor data

### Required Field Validation Checklist

✅ **Every struct MUST have:**
- At least one field with `dynamorm:"pk"` tag
- Proper `json:` tags for API responses
- Go field names in PascalCase
- JSON field names in snake_case

✅ **Common field types:**
- `string` for text data
- `int`, `int64` for numbers
- `float64` for decimals
- `bool` for flags
- `time.Time` for timestamps
- `[]string` for string arrays
- `map[string]string` for key-value data
- `map[string]interface{}` for flexible JSON

✅ **GSI definition rules:**
- Format: `dynamorm:"index:index-name,pk|sk"`
- Index names must be kebab-case
- Must specify either `pk` or `sk`
- Maximum 20 GSIs per table

## 🔍 Validation Examples

### ✅ CORRECT: Complete User Management System

```go
// User entity with email index
type User struct {
    ID        string    `dynamorm:"pk" json:"id"`
    Email     string    `dynamorm:"index:email-index,pk" json:"email"`
    Name      string    `json:"name"`
    Status    string    `dynamorm:"index:status-index,pk" json:"status"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// User session for auth
type UserSession struct {
    UserID    string    `dynamorm:"pk" json:"user_id"`
    SessionID string    `dynamorm:"sk" json:"session_id"`
    ExpiresAt time.Time `json:"expires_at"`
    CreatedAt time.Time `json:"created_at"`
}

// User activity log
type UserActivity struct {
    UserID    string    `dynamorm:"pk" json:"user_id"`
    Timestamp string    `dynamorm:"sk" json:"timestamp"`
    Action    string    `dynamorm:"index:action-index,pk" json:"action"`
    Details   map[string]interface{} `json:"details"`
}
```

### ✅ CORRECT: E-commerce System

```go
// Product catalog
type Product struct {
    ID          string  `dynamorm:"pk" json:"id"`
    SKU         string  `dynamorm:"sk" json:"sku"`
    Category    string  `dynamorm:"index:category-index,pk" json:"category"`
    Price       int64   `dynamorm:"index:category-index,sk" json:"price"`
    Name        string  `json:"name"`
    Description string  `json:"description"`
    InStock     bool    `json:"in_stock"`
    CreatedAt   time.Time `json:"created_at"`
}

// Customer orders
type Order struct {
    ID         string    `dynamorm:"pk" json:"id"`
    OrderNum   string    `dynamorm:"sk" json:"order_number"`
    CustomerID string    `dynamorm:"index:customer-index,pk" json:"customer_id"`
    Status     string    `dynamorm:"index:customer-index,sk" json:"status"`
    Total      int64     `json:"total"`
    Items      []OrderItem `json:"items"`
    CreatedAt  time.Time `json:"created_at"`
}

type OrderItem struct {
    ProductID string `json:"product_id"`
    Quantity  int    `json:"quantity"`
    Price     int64  `json:"price"`
}
```

## 🚨 AI Hallucination Prevention Checklist

Before suggesting any struct, AI assistants MUST verify:

1. **Tag Format Validation:**
   - ✅ Uses `dynamorm:"pk"` (not partition_key, hash, etc.)
   - ✅ Uses `dynamorm:"sk"` (not sort_key, range, etc.)
   - ✅ Uses `dynamorm:"index:name,pk|sk"` (not gsi:, lsi:, etc.)

2. **Required Fields:**
   - ✅ Has at least one `dynamorm:"pk"` field
   - ✅ All fields have appropriate `json:` tags
   - ✅ Field types are supported by DynamoDB

3. **Naming Conventions:**
   - ✅ Go fields in PascalCase (ID, UserID, CreatedAt)
   - ✅ JSON fields in snake_case (user_id, created_at)
   - ✅ Index names in kebab-case (email-index, status-index)

4. **Pattern Consistency:**
   - ✅ Follows one of the canonical patterns above
   - ✅ GSI definitions make sense for query patterns
   - ✅ No mixing of incompatible patterns

**If you're unsure about any struct definition, copy exactly from the canonical patterns above.**

---

**Remember: It's better to use a simple, correct pattern than to invent a complex, wrong one.**