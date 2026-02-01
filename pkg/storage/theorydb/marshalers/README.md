# DynamORM Custom Marshalers

This package provides custom marshaling and unmarshaling functionality for complex data types in DynamoDB.

## Features

### Core Interfaces

- **Marshaler**: Interface for custom DynamoDB marshaling
- **Unmarshaler**: Interface for custom DynamoDB unmarshaling  
- **MarshalUnmarshaler**: Combined interface for both operations

### Built-in Custom Types

#### PreciseTime
Time values with configurable precision (second, millisecond, microsecond, nanosecond).

```go
// Create precise time with millisecond precision
pt := marshalers.NewPreciseTime(time.Now(), time.Millisecond)

// Create with current time
pt := marshalers.NewPreciseTimeNow(time.Second)
```

#### Money
Monetary values with currency information using decimal precision.

```go
// Create from decimal
money := marshalers.NewMoney(decimal.NewFromFloat(123.45), "USD")

// Create from float
money := marshalers.NewMoneyFromFloat(123.45, "USD")

// Create from string
money, err := marshalers.NewMoneyFromString("123.45", "USD")

// Arithmetic operations
sum, err := money1.Add(money2)
diff, err := money1.Sub(money2)
```

#### EncryptedString
Encrypted string values using AES-GCM encryption.

```go
// Setup encryption
encryptor, err := marshalers.NewAESEncryptor() // Uses DYNAMODB_ENCRYPTION_KEY env var
// Or with custom key
key, _ := marshalers.GenerateEncryptionKey()
encryptor, err := marshalers.NewAESEncryptorWithKey(key)

// Create encrypted string
encrypted := marshalers.NewEncryptedString("secret", encryptor)
```

#### JSONField
Store arbitrary JSON data in DynamoDB.

```go
// Store any data as JSON
data := map[string]interface{}{
    "name": "John",
    "age": 30,
}
field := marshalers.NewJSONField(data)

// Unmarshal into specific type
var user User
err := field.UnmarshalInto(&user)
```

#### StringSet
DynamoDB string set with additional functionality.

```go
// Create string set
ss := marshalers.NewStringSet("a", "b", "c")

// Operations
ss.Add("d", "e")
ss.Remove("b")
contains := ss.Contains("a")
size := ss.Size()
```

## Usage Examples

### Model Integration

```go
type User struct {
    ID          string                    `dynamodbav:"pk"`
    Balance     marshalers.Money          `dynamodbav:"balance"`
    LastLogin   marshalers.PreciseTime    `dynamodbav:"last_login"`
    APIKey      marshalers.EncryptedString `dynamodbav:"api_key"`
    Preferences marshalers.JSONField      `dynamodbav:"preferences"`
    Tags        marshalers.StringSet      `dynamodbav:"tags"`
}
```

### Environment Setup

For encrypted fields, set the encryption key:

```bash
export DYNAMODB_ENCRYPTION_KEY="<base64-encoded-32-byte-key>"
```

Generate a key:

```go
key, err := marshalers.GenerateEncryptionKey()
keyBase64 := marshalers.EncodeEncryptionKey(key)
```

## Testing

All custom marshalers include comprehensive tests covering:
- Marshal/unmarshal round trips
- Error handling
- Edge cases
- Type validation
- Encryption/decryption

Run tests:
```bash
go test ./pkg/storage/dynamorm/marshalers/ -v
```

## Security Considerations

- **EncryptedString**: Uses AES-256-GCM for authenticated encryption
- **Money**: Decimal precision prevents floating-point errors
- **JSONField**: Sanitizes JSON input/output
- All types validate input and handle edge cases gracefully