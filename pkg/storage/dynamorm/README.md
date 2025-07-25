# DynamORM Integration

This package implements the DynamoDB access layer using the DynamORM library. It provides a type-safe, efficient way to interact with DynamoDB tables.

## Structure

- `base_model.go`: Contains standard model structs with common fields and key generation utilities
- `client.go`: Handles DynamoDB client initialization and optimization for Lambda functions
- `lambda_init.go`: Provides helpers for Lambda function initialization
- `errors.go`: Maps DynamORM errors to storage errors with detailed context
- `adapter.go`: Provides backward compatibility with the existing storage interface
- `repository_adapter.go`: Implements generic repository adapters for common patterns
- `model.go`: Sample model implementation (template for specific models)

## Usage

### Lambda Initialization

For Lambda functions, initialize the client in the `init()` function using the Lambda-optimized client:

```go
var db core.DB

func init() {
    var err error
    // Initialize with models to pre-register
    db, err = dynamorm.LambdaInit(&User{}, &Actor{}, &Status{})
    if err != nil {
        panic(err)
    }
}
```

### Model Definition

Define models by embedding the StandardModel struct:

```go
type User struct {
    // Embed StandardModel for PK, SK, CreatedAt, UpdatedAt
    dynamorm.StandardModel
    
    // Business ID
    ID        string    `json:"id"`                         // User ID
    
    // GSI attributes
    Email     string    `dynamorm:"index:email-index,pk" json:"email"`
    
    // Business attributes
    Name      string    `json:"name"`
    Username  string    `json:"username"`
    Active    bool      `json:"active"`
}

// SetKeys implements the KeySetter interface
func (u *User) SetKeys() {
    if u.PK == "" || u.SK == "" {
        u.PK, u.SK = dynamorm.GenerateSimpleKeys("user", u.ID)
    }
}
```

For multi-tenant models, embed the TenantModel:

```go
type TenantResource struct {
    // Embed TenantModel for PK, SK, CreatedAt, UpdatedAt, TenantID
    dynamorm.TenantModel
    
    // Business ID
    ID          string `json:"id"`
    
    // Business attributes
    Name        string `json:"name"`
    Description string `json:"description"`
}

// SetKeys implements the KeySetter interface
func (r *TenantResource) SetKeys() {
    if r.PK == "" || r.SK == "" {
        r.PK, r.SK = dynamorm.GenerateTenantKeys(r.TenantID, "resource", r.ID)
    }
}
```

For models with TTL, embed the TTLModel:

```go
type Session struct {
    // Embed TTLModel for PK, SK, CreatedAt, UpdatedAt, TTL
    dynamorm.TTLModel
    
    // Business ID
    UserID     string `json:"user_id"`
    SessionID  string `json:"session_id"`
    
    // Business attributes
    IP         string `json:"ip"`
    UserAgent  string `json:"user_agent"`
}

// SetKeys implements the KeySetter interface
func (s *Session) SetKeys() {
    if s.PK == "" || s.SK == "" {
        s.PK, s.SK = dynamorm.GenerateHierarchicalKeys("user", s.UserID, "session", s.SessionID)
    }
    
    // Set TTL to expire in 24 hours
    s.SetTTL(24 * time.Hour)
}
```

### Repository Implementation

Implement repositories for each model:

```go
type UserRepository struct {
    BaseRepository
}

func NewUserRepository(db core.DB, tableName string) *UserRepository {
    return &UserRepository{
        BaseRepository: *NewBaseRepository(db, tableName),
    }
}

func (r *UserRepository) GetUser(ctx context.Context, id string) (*User, error) {
    user := &User{ID: id}
    user.SetKeys()
    
    err := r.GetDB().Model(user).
        Where("PK", "=", user.PK).
        Where("SK", "=", user.SK).
        First(user)
        
    if err != nil {
        return nil, MapRepositoryError(err, "GetUser", "User", id)
    }
    
    return user, nil
}
```

### Generic Repository Usage

For simple CRUD operations, use the GenericRepository:

```go
// Create a generic repository for users
userRepo := dynamorm.NewGenericRepository(db, tableName, "user")

// Create a user
user := &User{ID: "123", Name: "John Doe"}
user.SetKeys()
err := userRepo.Create(ctx, user)

// Get a user
user = &User{}
err = userRepo.Get(ctx, "123", user)

// Update a user
user.Name = "Jane Doe"
err = userRepo.Update(ctx, user)

// Delete a user
err = userRepo.Delete(ctx, "123", &User{})

// List users
var users []*User
err = userRepo.List(ctx, map[string]any{
    "Active": true,
}, &users)
```

### Adapter Usage

Use the adapter to maintain backward compatibility:

```go
// Create the original storage implementation
originalStorage := dynamodb.New(...)

// Create the DynamORM client
db, err := dynamorm.GetLambdaClient(context.Background())
if err != nil {
    panic(err)
}

// Create the adapter
adapter := dynamorm.NewStorageAdapter(db, tableName, originalStorage)

// Use the adapter as a storage.Storage implementation
user, err := adapter.GetUser(ctx, "username")
```

### Error Handling

Use the error mapping functions to handle errors:

```go
user, err := userRepo.GetUser(ctx, id)
if err != nil {
    if dynamorm.IsNotFound(err) {
        // Handle not found error
        return nil, fmt.Errorf("user not found: %s", id)
    }
    
    // Handle other errors
    return nil, fmt.Errorf("failed to get user: %w", err)
}
```

## Migration Strategy

1. Implement models and repositories for core entities
2. Use the adapter to maintain backward compatibility
3. Gradually migrate methods from the original storage implementation to DynamORM
4. Once all methods are migrated, remove the adapter and use DynamORM directly

## Best Practices

1. **Use the StandardModel**: Embed StandardModel in all your models to get standard fields and hooks
2. **Implement KeySetter**: Implement the KeySetter interface to automatically set keys
3. **Use Lambda-optimized client**: Use LambdaInit in Lambda functions to optimize performance
4. **Use error mapping**: Use MapRepositoryError to provide detailed error context
5. **Use generic repositories**: Use GenericRepository for simple CRUD operations
6. **Use adapters**: Use adapters to maintain backward compatibility during migration