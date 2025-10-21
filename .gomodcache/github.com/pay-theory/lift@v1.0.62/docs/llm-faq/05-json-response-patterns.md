# Lift Framework JSON Response Patterns

## Overview

This guide covers all JSON response patterns in the Lift framework, including how to send JSON responses, handle different response types, set status codes, and follow best practices for API consistency. Lift provides multiple approaches to JSON responses, each optimized for different use cases.

## Basic JSON Responses

### Simple JSON Response

The most common pattern in Lift is using `ctx.JSON()`:

```go
func GetUser(ctx *lift.Context) error {
    user := User{
        ID:   "123",
        Name: "John Doe",
    }
    
    // Returns 200 OK with JSON body
    return ctx.JSON(user)
}
```

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "123",
  "name": "John Doe"
}
```

### JSON with Custom Status Code

Use `ctx.Status()` before `ctx.JSON()` to set a custom status code:

```go
func CreateUser(ctx *lift.Context) error {
    user := User{
        ID:   "123",
        Name: "John Doe",
    }
    
    // Returns 201 Created
    ctx.Status(201)
    return ctx.JSON(user)
}
```

**Response:**
```http
HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "123",
  "name": "John Doe"
}
```

### JSON from Maps

You can return JSON from Go maps directly:

```go
func HealthCheck(ctx *lift.Context) error {
    return ctx.JSON(map[string]interface{}{
        "status":    "healthy",
        "timestamp": time.Now(),
        "version":   "1.0.0",
    })
}
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2025-10-03T19:33:09Z",
  "version": "1.0.0"
}
```

### JSON from Slices

Return arrays/lists as JSON:

```go
func ListUsers(ctx *lift.Context) error {
    users := []User{
        {ID: "1", Name: "Alice"},
        {ID: "2", Name: "Bob"},
    }
    
    return ctx.JSON(users)
}
```

**Response:**
```json
[
  {
    "id": "1",
    "name": "Alice"
  },
  {
    "id": "2",
    "name": "Bob"
  }
]
```

## Type-Safe JSON Responses

### Using SimpleHandler for Type Safety

The recommended approach for type-safe responses:

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required"`
    Email string `json:"email" validate:"required,email"`
}

type UserResponse struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

// Type-safe handler automatically serializes UserResponse to JSON
app.POST("/users", lift.SimpleHandler(func(ctx *lift.Context, req CreateUserRequest) (UserResponse, error) {
    user := UserResponse{
        ID:    uuid.New().String(),
        Name:  req.Name,
        Email: req.Email,
    }
    
    // Automatically serialized to JSON with 200 status
    return user, nil
}))
```

**Benefits of SimpleHandler:**
- Automatic JSON serialization
- Compile-time type checking
- Less boilerplate code
- Consistent response format
- Automatic error handling

## Common Response Patterns

### Pattern 1: Success Response with Data

```go
type Response struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data"`
}

func GetOrder(ctx *lift.Context) error {
    order := fetchOrder(ctx.Param("id"))
    
    return ctx.JSON(Response{
        Success: true,
        Data:    order,
    })
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "order_id": "order_123",
    "amount": 99.99
  }
}
```

### Pattern 2: Paginated Response

```go
type PaginatedResponse struct {
    Data       []interface{} `json:"data"`
    Page       int           `json:"page"`
    PerPage    int           `json:"per_page"`
    Total      int           `json:"total"`
    TotalPages int           `json:"total_pages"`
}

func ListItems(ctx *lift.Context) error {
    page := ctx.QueryInt("page", 1)
    perPage := ctx.QueryInt("per_page", 20)
    
    items, total := fetchItems(page, perPage)
    
    return ctx.JSON(PaginatedResponse{
        Data:       items,
        Page:       page,
        PerPage:    perPage,
        Total:      total,
        TotalPages: (total + perPage - 1) / perPage,
    })
}
```

**Response:**
```json
{
  "data": [...],
  "page": 1,
  "per_page": 20,
  "total": 150,
  "total_pages": 8
}
```

### Pattern 3: Response with Metadata

```go
type MetadataResponse struct {
    Data     interface{}       `json:"data"`
    Metadata map[string]string `json:"metadata"`
}

func GetUserWithMeta(ctx *lift.Context) error {
    user := fetchUser(ctx.Param("id"))
    
    return ctx.JSON(MetadataResponse{
        Data: user,
        Metadata: map[string]string{
            "request_id": ctx.RequestID(),
            "tenant_id":  ctx.TenantID(),
            "timestamp":  time.Now().Format(time.RFC3339),
        },
    })
}
```

**Response:**
```json
{
  "data": {
    "id": "user_123",
    "name": "John Doe"
  },
  "metadata": {
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "tenant_id": "tenant_abc",
    "timestamp": "2025-10-03T19:33:09Z"
  }
}
```

### Pattern 4: Empty/No Content Response

```go
func DeleteUser(ctx *lift.Context) error {
    userID := ctx.Param("id")
    deleteUser(userID)
    
    // Returns 204 No Content (no body)
    ctx.Status(204)
    return nil
}
```

**Response:**
```http
HTTP/1.1 204 No Content
```

### Pattern 5: Bulk Operation Response

```go
type BulkOperationResponse struct {
    Successful []string   `json:"successful"`
    Failed     []BulkError `json:"failed"`
    Total      int         `json:"total"`
}

type BulkError struct {
    ID    string `json:"id"`
    Error string `json:"error"`
}

func BulkCreateUsers(ctx *lift.Context) error {
    var requests []CreateUserRequest
    ctx.ParseRequest(&requests)
    
    response := BulkOperationResponse{
        Successful: []string{},
        Failed:     []BulkError{},
        Total:      len(requests),
    }
    
    for _, req := range requests {
        id, err := createUser(req)
        if err != nil {
            response.Failed = append(response.Failed, BulkError{
                ID:    req.Email,
                Error: err.Error(),
            })
        } else {
            response.Successful = append(response.Successful, id)
        }
    }
    
    // Use 207 Multi-Status for partial success
    ctx.Status(207)
    return ctx.JSON(response)
}
```

**Response:**
```json
{
  "successful": ["user_1", "user_2"],
  "failed": [
    {
      "id": "duplicate@example.com",
      "error": "email already exists"
    }
  ],
  "total": 3
}
```

## Error Response Patterns

Lift automatically formats errors as JSON. All error responses follow a consistent structure:

### Standard Error Response

```go
func GetUser(ctx *lift.Context) error {
    user, err := db.GetUser(ctx.Param("id"))
    if err != nil {
        return lift.NotFound("user not found")
    }
    return ctx.JSON(user)
}
```

**Error Response:**
```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "error": "not found",
  "message": "user not found",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### Validation Error Response

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required,min=2"`
    Email string `json:"email" validate:"required,email"`
}

func CreateUser(ctx *lift.Context) error {
    var req CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        // Automatically returns detailed validation errors
        return err
    }
    // ...
}
```

**Validation Error Response:**
```http
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/json

{
  "error": "validation error",
  "message": "validation failed",
  "details": {
    "name": "must be at least 2 characters",
    "email": "must be a valid email"
  },
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### Custom Error Response

```go
type CustomError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details string `json:"details,omitempty"`
}

func ProcessPayment(ctx *lift.Context) error {
    err := chargeCard()
    if err != nil {
        ctx.Status(402)
        return ctx.JSON(CustomError{
            Code:    "PAYMENT_FAILED",
            Message: "Unable to process payment",
            Details: "Insufficient funds",
        })
    }
    // ...
}
```

**Custom Error Response:**
```http
HTTP/1.1 402 Payment Required
Content-Type: application/json

{
  "code": "PAYMENT_FAILED",
  "message": "Unable to process payment",
  "details": "Insufficient funds"
}
```

## Response Headers

### Setting Custom Headers

```go
func DownloadFile(ctx *lift.Context) error {
    data := getFileData()
    
    // Set custom headers
    ctx.SetHeader("Content-Disposition", "attachment; filename=report.json")
    ctx.SetHeader("Cache-Control", "no-cache")
    
    return ctx.JSON(data)
}
```

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json
Content-Disposition: attachment; filename=report.json
Cache-Control: no-cache

{...}
```

### Automatic Headers

Lift automatically adds these headers to all JSON responses:

```http
Content-Type: application/json
X-Request-ID: <unique-request-id>
```

If CORS is enabled:

```http
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization
```

## Status Codes Reference

### Success Responses

```go
// 200 OK - Standard success
return ctx.JSON(data)

// 201 Created - Resource created
ctx.Status(201)
return ctx.JSON(newResource)

// 202 Accepted - Async processing started
ctx.Status(202)
return ctx.JSON(map[string]string{"status": "processing"})

// 204 No Content - Success with no body
ctx.Status(204)
return nil
```

### Client Error Responses

```go
// 400 Bad Request - Generic client error
return lift.ValidationError("invalid request")

// 401 Unauthorized - Authentication required
return lift.Unauthorized("authentication required")

// 403 Forbidden - Not authorized
return lift.AuthorizationError("insufficient permissions")

// 404 Not Found - Resource doesn't exist
return lift.NotFound("resource not found")

// 422 Unprocessable Entity - Validation failed
return lift.ValidationError("validation failed")

// 429 Too Many Requests - Rate limit exceeded
// Automatically returned by rate limiting middleware
```

### Server Error Responses

```go
// 500 Internal Server Error - Generic server error
return lift.SystemError("internal error")

// 503 Service Unavailable - Temporary unavailability
// Automatically returned by load shedding middleware
```

## JSON Serialization Control

### Custom JSON Tags

```go
type User struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    Password  string    `json:"-"`                    // Never serialize
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at,omitempty"` // Omit if zero value
}

func GetUser(ctx *lift.Context) error {
    user := User{
        ID:       "123",
        Name:     "John",
        Email:    "john@example.com",
        Password: "secret", // Won't appear in JSON
    }
    
    return ctx.JSON(user)
}
```

**Response:**
```json
{
  "id": "123",
  "name": "John",
  "email": "john@example.com",
  "created_at": "2025-10-03T19:33:09Z"
}
```

### Nested JSON Structures

```go
type OrderResponse struct {
    ID     string   `json:"id"`
    Amount float64  `json:"amount"`
    User   UserInfo `json:"user"`
    Items  []Item   `json:"items"`
}

type UserInfo struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

type Item struct {
    ProductID string  `json:"product_id"`
    Quantity  int     `json:"quantity"`
    Price     float64 `json:"price"`
}
```

**Response:**
```json
{
  "id": "order_123",
  "amount": 199.99,
  "user": {
    "id": "user_456",
    "name": "John Doe"
  },
  "items": [
    {
      "product_id": "prod_789",
      "quantity": 2,
      "price": 99.99
    }
  ]
}
```

## Best Practices for JSON Responses

### 1. Consistent Response Structure

Use a standard response wrapper for all endpoints:

```go
type APIResponse struct {
    Success   bool        `json:"success"`
    Data      interface{} `json:"data,omitempty"`
    Error     string      `json:"error,omitempty"`
    RequestID string      `json:"request_id"`
}

func wrapResponse(ctx *lift.Context, data interface{}) error {
    return ctx.JSON(APIResponse{
        Success:   true,
        Data:      data,
        RequestID: ctx.RequestID(),
    })
}

// Usage
func GetUser(ctx *lift.Context) error {
    user := fetchUser(ctx.Param("id"))
    return wrapResponse(ctx, user)
}
```

### 2. Use Appropriate Status Codes

```go
func Handler(ctx *lift.Context) error {
    // Created resource - use 201
    ctx.Status(201)
    return ctx.JSON(newResource)
    
    // Updated resource - use 200
    return ctx.JSON(updatedResource)
    
    // Deleted resource - use 204
    ctx.Status(204)
    return nil
    
    // Async processing - use 202
    ctx.Status(202)
    return ctx.JSON(map[string]string{"status": "processing"})
}
```

### 3. Include Metadata

```go
func GetUsers(ctx *lift.Context) error {
    users := fetchUsers()
    
    return ctx.JSON(map[string]interface{}{
        "data": users,
        "meta": map[string]interface{}{
            "request_id": ctx.RequestID(),
            "timestamp":  time.Now(),
            "count":      len(users),
        },
    })
}
```

### 4. Handle Empty Results

```go
func ListItems(ctx *lift.Context) error {
    items := fetchItems()
    
    // Return empty array, not null
    if items == nil {
        items = []Item{}
    }
    
    return ctx.JSON(items)
}
```

**Response (empty):**
```json
[]
```

Not:
```json
null
```

### 5. Use omitempty for Optional Fields

```go
type User struct {
    ID        string  `json:"id"`
    Name      string  `json:"name"`
    Email     string  `json:"email,omitempty"`     // Optional
    Phone     *string `json:"phone,omitempty"`     // Optional pointer
    CreatedAt time.Time `json:"created_at"`
}
```

### 6. Version Your API Responses

```go
type UserV1 struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

type UserV2 struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    CreatedAt time.Time `json:"created_at"`
}

// Different versions return different structures
app.GET("/api/v1/users/:id", GetUserV1)
app.GET("/api/v2/users/:id", GetUserV2)
```

## Summary

### Key Response Patterns
1. **Simple JSON:** `return ctx.JSON(data)`
2. **With Status:** `ctx.Status(201); return ctx.JSON(data)`
3. **Type-Safe:** Use `lift.SimpleHandler` for compile-time safety
4. **Errors:** Use built-in error types (`lift.NotFound`, `lift.ValidationError`, etc.)

### Best Practices
- Use consistent response structures
- Include request IDs in responses
- Use appropriate HTTP status codes
- Return empty arrays, not null
- Use `omitempty` for optional fields
- Version your API responses
- Never expose sensitive data
- Include helpful metadata

### Standard Status Codes
- **200:** Success (GET, PUT, PATCH)
- **201:** Created (POST)
- **204:** No Content (DELETE)
- **400:** Bad Request
- **401:** Unauthorized
- **403:** Forbidden
- **404:** Not Found
- **422:** Validation Error
- **500:** Internal Server Error

With these patterns, you can build consistent, well-structured JSON APIs with Lift that are easy to consume and maintain.



