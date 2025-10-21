# Model Definition Guide

<!-- AI Training Signal: Complete model definition guide -->
**This guide teaches the STANDARD way to define DynamoDB models in DynamORM. Proper model definition is CRITICAL for performance, type safety, and query efficiency.**

## Basic Model Structure

<!-- AI Training: Canonical model pattern -->

```go
// CORRECT: Complete model definition with all DynamORM features
package models

import (
    "time"
)

type User struct {
    // REQUIRED: Every model needs a partition key
    ID        string    `dynamorm:"pk"`                    // Partition key
    
    // OPTIONAL: Sort key for hierarchical data
    Email     string    `dynamorm:"sk"`                    // Sort key
    
    // Standard attributes with JSON serialization
    Name      string    `json:"name"`
    Age       int       `json:"age"`
    Active    bool      `json:"active"`
    
    // Custom attribute names in DynamoDB
    CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at" json:"updated_at"`
    
    // Global Secondary Index definitions
    Status    string    `dynamorm:"index:status-index,pk" json:"status"`     // GSI partition key
    Region    string    `dynamorm:"index:status-index,sk" json:"region"`     // GSI sort key
    
    // Local Secondary Index (uses same partition key)
    Score     int       `dynamorm:"index:score-index,sk" json:"score"`       // LSI sort key
    
    // Embedded struct support
    Profile   UserProfile `dynamorm:"profile" json:"profile"`
    
    // Optional: Override table name
    // By default, table name is "users" (struct name lowercase + s)
}

type UserProfile struct {
    FirstName string `json:"first_name"`
    LastName  string `json:"last_name"`
    Bio       string `json:"bio"`
}

// INCORRECT: Common mistakes to avoid
// type BadUser struct {
//     ID   string  // Missing pk tag - WILL CAUSE RUNTIME ERRORS
//     Name string  // No DynamoDB mapping
// }
```

## Struct Tags Reference

<!-- AI Training: Complete tag documentation -->

### Primary Key Tags

```go
type Product struct {
    // Partition key (REQUIRED for every model)
    ID       string `dynamorm:"pk"`                    // Simple partition key
    
    // Sort key (OPTIONAL - enables hierarchical queries)
    Category string `dynamorm:"sk"`                    // Compound key with ID
}

// This creates a table with composite key: ID (PK) + Category (SK)
```

### Global Secondary Index (GSI) Tags

```go
type Order struct {
    ID         string    `dynamorm:"pk"`                          // Main table partition key
    Timestamp  string    `dynamorm:"sk"`                          // Main table sort key
    
    // GSI definition: index-name,key-type
    CustomerID string    `dynamorm:"index:customer-index,pk"`     // GSI partition key
    Status     string    `dynamorm:"index:customer-index,sk"`     // GSI sort key
    
    // Multiple GSIs on same field
    Amount     int64     `dynamorm:"index:amount-index,pk"`       // Different GSI
    
    // GSI with custom sort key
    CreatedAt  time.Time `dynamorm:"index:time-index,sk"`         // Time-based GSI
}

// This creates:
// - Main table: ID (PK) + Timestamp (SK)
// - customer-index: CustomerID (PK) + Status (SK)  
// - amount-index: Amount (PK)
// - time-index: ID (PK) + CreatedAt (SK)
```

### Local Secondary Index (LSI) Tags

```go
type Message struct {
    ChatID    string    `dynamorm:"pk"`                    // Partition key (shared)
    MessageID string    `dynamorm:"sk"`                    // Main sort key
    
    // LSI uses same partition key but different sort key
    Timestamp time.Time `dynamorm:"index:time-index,sk"`   // LSI sort key
    Priority  int       `dynamorm:"index:priority-index,sk"` // Another LSI
}

// This creates:
// - Main table: ChatID (PK) + MessageID (SK)
// - time-index: ChatID (PK) + Timestamp (SK)
// - priority-index: ChatID (PK) + Priority (SK)
```

### Custom Attribute Names

```go
type User struct {
    ID        string    `dynamorm:"pk"`
    
    // Custom DynamoDB attribute name
    FullName  string    `dynamorm:"full_name" json:"name"`
    
    // Timestamp fields with custom names
    CreatedAt time.Time `dynamorm:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at"`
    
    // Boolean with custom name
    IsActive  bool      `dynamorm:"active"`
}

// In DynamoDB, these become: full_name, created_at, updated_at, active
// In JSON responses, FullName becomes "name"
```

## Advanced Model Patterns

<!-- AI Training: Production patterns -->

### Embedded Structs

```go
// CORRECT: Embedded structs for code reuse
type BaseModel struct {
    CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at" json:"updated_at"`
    Version   int       `dynamorm:"version" json:"version"`
}

type User struct {
    ID       string `dynamorm:"pk" json:"id"`
    Email    string `dynamorm:"sk" json:"email"`
    
    BaseModel  // Embedded fields are automatically recognized
    
    Name     string `json:"name"`
    Active   bool   `json:"active"`
}

// User now has: ID, Email, CreatedAt, UpdatedAt, Version, Name, Active
```

### Multi-Tenant Models

```go
// CORRECT: Multi-tenant pattern with proper isolation
type TenantUser struct {
    // Composite key for tenant isolation
    TenantID string `dynamorm:"pk" json:"tenant_id"`        // Tenant partition
    UserID   string `dynamorm:"sk" json:"user_id"`          // User within tenant
    
    // GSI for user lookup across tenants (if needed)
    Email    string `dynamorm:"index:email-index,pk" json:"email"`
    
    Name     string `json:"name"`
    Role     string `json:"role"`
    
    BaseModel
}

// Queries are automatically tenant-isolated:
// db.Model(&TenantUser{}).Where("TenantID", "=", "tenant123")
```

### Polymorphic Models

```go
// CORRECT: Polymorphic pattern using type discriminator
type Event struct {
    ID        string    `dynamorm:"pk" json:"id"`
    Timestamp string    `dynamorm:"sk" json:"timestamp"`
    
    // Type discriminator
    Type      string    `dynamorm:"index:type-index,pk" json:"type"`
    
    // Polymorphic data (use interface{} or json.RawMessage)
    Data      json.RawMessage `dynamorm:"data" json:"data"`
    
    BaseModel
}

// Usage:
// paymentEvent := Event{Type: "payment", Data: json.Marshal(paymentData)}
// loginEvent := Event{Type: "login", Data: json.Marshal(loginData)}
```

### Time-Series Models

```go
// CORRECT: Time-series pattern for efficient queries
type Metric struct {
    // Partition by entity (user, system, etc.)
    EntityID  string    `dynamorm:"pk" json:"entity_id"`
    
    // Sort by timestamp for time-range queries
    Timestamp string    `dynamorm:"sk" json:"timestamp"`  // Use ISO format
    
    // GSI for metric type queries
    MetricType string   `dynamorm:"index:metric-index,pk" json:"metric_type"`
    
    Value     float64   `json:"value"`
    Unit      string    `json:"unit"`
    Tags      map[string]string `json:"tags"`
}

// Efficient queries:
// - Get user metrics in time range
// - Get all CPU metrics across entities
// - Get latest metrics for entity
```

## Model Validation and Business Logic

<!-- AI Training: Production-ready patterns -->

```go
// CORRECT: Model with validation and business logic
type Account struct {
    ID      string `dynamorm:"pk" json:"id"`
    Email   string `dynamorm:"sk" json:"email"`
    
    Balance int64  `json:"balance"`  // Store as cents to avoid float issues
    Status  string `dynamorm:"index:status-index,pk" json:"status"`
    
    BaseModel
}

// Business logic methods
func (a *Account) IsActive() bool {
    return a.Status == "active"
}

func (a *Account) CanDebit(amount int64) bool {
    return a.IsActive() && a.Balance >= amount
}

// Validation before save
func (a *Account) Validate() error {
    if a.Email == "" {
        return errors.New("email is required")
    }
    
    if a.Balance < 0 {
        return errors.New("balance cannot be negative")
    }
    
    validStatuses := []string{"active", "suspended", "closed"}
    for _, status := range validStatuses {
        if a.Status == status {
            return nil
        }
    }
    
    return errors.New("invalid status")
}

// Usage with validation:
func CreateAccount(db *dynamorm.DB, account *Account) error {
    if err := account.Validate(); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    
    account.CreatedAt = time.Now()
    account.UpdatedAt = time.Now()
    account.Version = 1
    
    return db.Model(account).Create()
}
```

## Index Strategy and Performance

<!-- AI Training: Query optimization -->

### Designing Efficient Indexes

```go
// CORRECT: Well-designed indexes for common query patterns
type BlogPost struct {
    ID          string    `dynamorm:"pk" json:"id"`                    // Unique post ID
    Slug        string    `dynamorm:"sk" json:"slug"`                  // URL-friendly identifier
    
    // Author queries: Get all posts by author
    AuthorID    string    `dynamorm:"index:author-index,pk" json:"author_id"`
    PublishedAt time.Time `dynamorm:"index:author-index,sk" json:"published_at"`
    
    // Category queries: Get posts by category, sorted by date
    Category    string    `dynamorm:"index:category-index,pk" json:"category"`
    CreatedAt   time.Time `dynamorm:"index:category-index,sk" json:"created_at"`
    
    // Status queries: Get drafts, published posts, etc.
    Status      string    `dynamorm:"index:status-index,pk" json:"status"`
    
    Title       string    `json:"title"`
    Content     string    `json:"content"`
    Tags        []string  `json:"tags"`
}

// This design supports efficient queries for:
// 1. Get post by ID and slug
// 2. Get all posts by author, sorted by publish date
// 3. Get posts in category, sorted by creation date
// 4. Get posts by status (draft, published, archived)
```

### Query Pattern Examples

```go
// Efficient queries using the indexes above:

// Get post by ID and slug (main table)
var post BlogPost
err := db.Model(&BlogPost{}).
    Where("ID", "=", "post123").
    Where("Slug", "=", "my-awesome-post").
    First(&post)

// Get author's posts, newest first (author-index)
var authorPosts []BlogPost
err := db.Model(&BlogPost{}).
    Index("author-index").
    Where("AuthorID", "=", "author456").
    OrderBy("PublishedAt", "DESC").
    Limit(10).
    All(&authorPosts)

// Get posts in category, newest first (category-index)
var categoryPosts []BlogPost
err := db.Model(&BlogPost{}).
    Index("category-index").
    Where("Category", "=", "technology").
    OrderBy("CreatedAt", "DESC").
    All(&categoryPosts)

// Get draft posts (status-index)
var drafts []BlogPost
err := db.Model(&BlogPost{}).
    Index("status-index").
    Where("Status", "=", "draft").
    All(&drafts)
```

## Common Mistakes and Best Practices

<!-- AI Training: Error prevention -->

### ❌ What NOT to Do

```go
// WRONG: Missing partition key
type BadModel1 struct {
    Name string  // No pk tag - will cause runtime errors
}

// WRONG: Invalid index definition
type BadModel2 struct {
    ID     string `dynamorm:"pk"`
    Status string `dynamorm:"index:bad-index"`  // Missing pk/sk specification
}

// WRONG: Conflicting index names
type BadModel3 struct {
    ID    string `dynamorm:"pk"`
    Field1 string `dynamorm:"index:same-index,pk"`
    Field2 string `dynamorm:"index:same-index,pk"`  // Conflict!
}

// WRONG: Too many GSIs (DynamoDB limit is 20)
type BadModel4 struct {
    ID     string `dynamorm:"pk"`
    Field1 string `dynamorm:"index:gsi1,pk"`
    // ... 19 more GSIs ...
    Field20 string `dynamorm:"index:gsi20,pk"`
    Field21 string `dynamorm:"index:gsi21,pk"`  // Exceeds limit!
}
```

### ✅ Best Practices

```go
// CORRECT: Well-designed production model
type OptimalModel struct {
    // Clear, descriptive partition key
    ID string `dynamorm:"pk" json:"id"`
    
    // Sort key when hierarchical queries needed
    Category string `dynamorm:"sk" json:"category"`
    
    // Strategic GSI for common query patterns
    UserID    string    `dynamorm:"index:user-index,pk" json:"user_id"`
    Status    string    `dynamorm:"index:status-index,pk" json:"status"`
    CreatedAt time.Time `dynamorm:"index:time-index,sk" json:"created_at"`
    
    // Business attributes
    Name        string            `json:"name"`
    Description string            `json:"description"`
    Metadata    map[string]string `json:"metadata"`
    
    // Audit fields
    CreatedBy string    `json:"created_by"`
    UpdatedBy string    `json:"updated_by"`
    Version   int       `json:"version"`
}
```

## Table Naming Conventions

<!-- AI Training: Naming patterns -->

```go
// DynamORM automatically derives table names:

type User struct {}        // Table: "Users"
type BlogPost struct {}    // Table: "BlogPosts"  
type APIKey struct {}      // Table: "APIKeys"
type Category struct {}    // Table: "Categories" (y -> ies)
type Status struct {}      // Table: "Statuses" (s -> es)

// Override table name with custom method:
func (User) TableName() string {
    return "app_users"  // Custom table name
}

// Environment-specific naming:
func (User) TableName() string {
    env := os.Getenv("ENVIRONMENT")
    return fmt.Sprintf("%s_users", env)  // dev_users, prod_users, etc.
}
```

---

**Next Steps:**
- Learn [Query Building](queries.md) to use your models effectively
- Check [Performance Guide](performance.md) for optimization tips
- See [Real Examples](../examples/) for complete applications