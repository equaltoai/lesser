# Lift Error Handling Guide

This guide documents the standardized error handling patterns implemented for the Lesser project using the Lift framework.

## Overview

The error handling system provides:
- Domain-specific error types that map to appropriate HTTP status codes
- Automatic error mapping for storage and AWS service errors
- Consistent error response formatting
- Context-aware error logging
- Type-safe error handling with Lift's built-in features

## Key Principles

1. **Let Lift Handle Formatting**: Always return errors - Lift automatically formats them as JSON
2. **Log Internally, Sanitize Externally**: Log detailed errors for debugging, return safe messages to users
3. **Use Structured Errors**: Use `lift.NewLiftError()` or the provided convenience functions
4. **No Manual JSON Formatting**: Never manually format error responses

## Error Types

### Domain-Specific Errors

```go
// Not Found (404)
NotFoundError(resource string)

// Validation Error (422)
ValidationError(message string)
ValidationErrorWithField(field, message string)

// Unauthorized (401)
UnauthorizedError(message string)

// Forbidden (403)
ForbiddenError(action, resource string)

// Conflict (409)
ConflictError(resource, message string)

// Rate Limited (429)
RateLimitError(message string)

// Service Unavailable (503)
ServiceUnavailableError(service string)

// Timeout (504)
TimeoutError(operation string)

// Internal Error (500)
InternalError(message string)
```

### Error Response Format

Lift automatically formats all errors as JSON:

```json
{
  "code": "VALIDATION_ERROR",
  "message": "Invalid input",
  "details": {
    "field": "email",
    "reason": "invalid format"
  }
}
```

## Usage Patterns

### Basic Error Handling

```go
func CreateUserHandler(ctx *lift.Context) error {
    var req CreateUserRequest
    
    // Automatic validation error on parse failure
    if err := ctx.ParseRequest(&req); err != nil {
        return err  // Lift returns 400 with details
    }
    
    // Business logic validation
    if !isValid(req.Email) {
        return ValidationErrorWithField("email", "Invalid email format")
    }
    
    // Database operation
    user, err := db.CreateUser(req)
    if err != nil {
        return WrapDatabaseError(ctx, err, "CreateUser", "user")
    }
    
    return Created(ctx, user.ID, user)
}
```

### Authentication Errors

```go
func ProtectedHandler(ctx *lift.Context) error {
    userID := ctx.UserID()
    if userID == "" {
        return UnauthorizedError("Authentication required")
    }
    
    // Check permissions
    if !hasPermission(userID, "admin") {
        return ForbiddenError("access", "admin resources")
    }
    
    return OK(ctx, data)
}
```

### Database Error Handling

```go
func GetUserHandler(ctx *lift.Context) error {
    userID := ctx.Param("id")
    
    user, err := store.GetUser(ctx, userID)
    if err != nil {
        if err == storage.ErrNotFound {
            return ResourceNotFound(ctx, "user", userID)
        }
        // Logs error details, returns safe message
        return WrapDatabaseError(ctx, err, "GetUser", "user")
    }
    
    return OK(ctx, user)
}
```

### External Service Errors

```go
func PaymentHandler(ctx *lift.Context) error {
    err := paymentService.Charge(amount)
    if err != nil {
        // Logs actual error, returns generic message
        return WrapExternalServiceError(ctx, err, "payment", "charge")
    }
    
    return OK(ctx, result)
}
```

### Validation with Multiple Errors

```go
func RegisterHandler(ctx *lift.Context) error {
    var req RegisterRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return err
    }
    
    // Collect validation errors
    errors := make(map[string]string)
    
    if len(req.Username) < 3 {
        errors["username"] = "Must be at least 3 characters"
    }
    
    if !isValidEmail(req.Email) {
        errors["email"] = "Invalid email format"
    }
    
    if len(errors) > 0 {
        return ValidationFailed(ctx, errors)
    }
    
    return OK(ctx, result)
}
```

## Error Context Functions

### LogAndReturnError

Logs an error with context and returns an appropriate user-facing error:

```go
err := someOperation()
if err != nil {
    return LogAndReturnError(ctx, err, "Operation failed", map[string]any{
        "operation": "user_creation",
        "user_email": email,
    })
}
```

### WrapDatabaseError

Wraps database errors with context and maps to appropriate HTTP errors:

```go
user, err := db.GetUser(id)
if err != nil {
    return WrapDatabaseError(ctx, err, "GetUser", "user")
}
```

### WrapExternalServiceError

Wraps external service errors to avoid leaking implementation details:

```go
result, err := stripeClient.CreateCharge(params)
if err != nil {
    return WrapExternalServiceError(ctx, err, "stripe", "create_charge")
}
```

## Response Helpers

### Success Responses

```go
// 200 OK
OK(ctx, data)

// 201 Created
Created(ctx, id, resource)

// 204 No Content
NoContent(ctx)

// 202 Accepted
Accepted(ctx, data)
```

### Collection Responses

```go
// Simple list
List(ctx, items, count)

// Paginated response
Paginated(ctx, items, nextCursor, prevCursor, hasMore)

// Paginated with total
PaginatedWithTotal(ctx, items, nextCursor, prevCursor, hasMore, total)
```

### Special Content Types

```go
// ActivityPub (application/activity+json)
ActivityPubResponse(ctx, data)

// WebFinger (application/jrd+json)
WebFingerResponse(ctx, data)

// NodeInfo
NodeInfoResponse(ctx, data)
```

## Error Mapping

### Storage Errors

The system automatically maps storage package errors:
- `storage.ErrNotFound` → 404 Not Found
- `storage.ErrAlreadyExists` → 409 Conflict
- `storage.ErrInvalidInput` → 422 Validation Error
- `storage.ErrUnauthorized` → 401 Unauthorized

### AWS Service Errors

AWS SDK errors are automatically mapped to appropriate HTTP status codes:
- Access denied → 403 Forbidden
- Resource not found → 404 Not Found
- Throttling → 429 Rate Limited
- Service unavailable → 503 Service Unavailable

### DynamoDB Errors

DynamoDB-specific errors are handled:
- `ConditionalCheckFailedException` → 409 Conflict
- `ResourceNotFoundException` → 404 Not Found
- `ProvisionedThroughputExceededException` → 429 Rate Limited

## Best Practices

1. **Always Return Errors**: Let Lift handle the formatting
   ```go
   // Good
   return ValidationError("Invalid input")
   
   // Bad
   ctx.Status(400)
   return ctx.JSON(map[string]string{"error": "Invalid input"})
   ```

2. **Use Specific Error Types**: Choose the most appropriate error type
   ```go
   // Good
   return NotFoundError("user")
   
   // Bad
   return InternalError("Not found")
   ```

3. **Add Context with Details**: Use `WithDetail()` for debugging
   ```go
   return ValidationError("Invalid format").
       WithDetail("field", "email").
       WithDetail("format", "RFC5322")
   ```

4. **Log Before Returning**: Use the context functions for automatic logging
   ```go
   // Good
   return WrapDatabaseError(ctx, err, "GetUser", "user")
   
   // Bad
   return InternalError("Database error")
   ```

5. **Handle All Error Cases**: Always check for specific errors first
   ```go
   if err != nil {
       if err == storage.ErrNotFound {
           return NotFoundError("user")
       }
       return WrapDatabaseError(ctx, err, "GetUser", "user")
   }
   ```

## Migration from API Gateway Responses

When migrating from API Gateway response patterns to Lift:

### Before (API Gateway)
```go
if err != nil {
    return common.BadRequest(err), nil
}

return common.JSONResponse(http.StatusOK, data), nil
```

### After (Lift)
```go
if err != nil {
    return ValidationError(err.Error())
}

return OK(ctx, data)
```

## Testing Error Handling

When testing handlers that use the new error handling:

```go
func TestCreateUser_ValidationError(t *testing.T) {
    handler := ExampleCreateUserHandler(mockStore)
    
    ctx := testing.NewTestContext(testing.WithRequest(testing.Request{
        Method: "POST",
        Body:   []byte(`{"name": "a"}`), // Too short
    }))
    
    err := handler.Handle(ctx)
    
    // Check that it returns a LiftError
    liftErr, ok := err.(*lift.LiftError)
    assert.True(t, ok)
    assert.Equal(t, "VALIDATION_ERROR", liftErr.Code)
    assert.Equal(t, 422, liftErr.StatusCode)
}
```

## Summary

The standardized error handling system provides:
- Consistent error responses across all endpoints
- Automatic error mapping for common scenarios
- Safe error messages for users while logging details
- Integration with Lift's built-in error handling
- Type-safe error creation and handling

By following these patterns, you ensure that your API returns consistent, informative error responses while maintaining security and providing good debugging information.