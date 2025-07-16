# DynamORM Integration

This package implements the DynamoDB access layer using the DynamORM library. It provides a type-safe, efficient way to interact with DynamoDB tables.

## Structure

- `base.go`: Contains base models and repositories with common functionality
- `client.go`: Handles DynamoDB client initialization and optimization for Lambda functions
- `errors.go`: Maps DynamORM errors to storage errors
- `adapter.go`: Provides backward compatibility with the existing storage interface
- `model.go`: Sample model implementation (template for specific models)

## Usage

### Client Initialization

For Lambda functions, initialize the client in the `init()` function:

```go
var db core.DB

func init() {
    var err error
    db, err = dynamorm.GetClient(context.Background())
    if err != nil {
        panic(err)
    }
    
    // Pre-register models to reduce cold start time
    err = dynamorm.InitializeModels(&User{}, &Actor{}, &Status{})
    if err != nil {
        panic(err)
    }
}
```

### Model Definition

Define models with DynamORM struct tags:

```go
type User struct {
    // Primary keys
    PK        string    `dynamorm:"pk" json:"pk"`           // user#{id}
    SK        string    `dynamorm:"sk" json:"sk"`           // user#{id}
    
    // Business ID
    ID        string    `json:"id"`                         // User ID
    
    // GSI attributes
    Email     string    `dynamorm:"index:email-index,pk" json:"email"`
    
    // Standard attributes
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
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
    user := &User{}
    
    err := r.GetDB().Model(user).
        Where("PK", "=", fmt.Sprintf("user#%s", id)).
        Where("SK", "=", fmt.Sprintf("user#%s", id)).
        First(user)
        
    if err != nil {
        return nil, MapErrorWithContext(err, "failed to get user")
    }
    
    return user, nil
}
```

### Adapter Usage

Use the adapter to maintain backward compatibility:

```go
// Create the original storage implementation
originalStorage := dynamodb.New(...)

// Create the DynamORM client
db, err := dynamorm.GetClient(context.Background())
if err != nil {
    panic(err)
}

// Create the adapter
adapter := dynamorm.NewStorageAdapter(db, tableName, originalStorage)

// Use the adapter as a storage.Storage implementation
user, err := adapter.GetUser(ctx, "username")
```

## Migration Strategy

1. Implement models and repositories for core entities
2. Use the adapter to maintain backward compatibility
3. Gradually migrate methods from the original storage implementation to DynamORM
4. Once all methods are migrated, remove the adapter and use DynamORM directly