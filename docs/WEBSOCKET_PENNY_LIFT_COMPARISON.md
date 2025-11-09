# Key Finding: Query Parameter Extraction Difference

After reviewing penny-lift's WebSocket implementation, I found that they use **Lift framework's `ctx.Query()` method** to extract query parameters, while lesser's graphql-ws uses **raw API Gateway event fields** (`event.QueryStringParameters`).

## Penny-Lift Approach (Working)

```go
// From penny-lift/cmd/lambda/websocket/main.go:276-282
tokenValue := ctx.Query("PTToken")
if tokenValue == "" {
    tokenValue = ctx.Query("token")
}
if tokenValue == "" {
    tokenValue = ctx.Query("accessToken")
}
```

They're using Lift framework's WebSocket context which abstracts away the API Gateway event structure.

## Lesser Approach (Current)

```go
// From lesser/cmd/graphql-ws/main.go:569-577
for key, value := range event.QueryStringParameters {
    if strings.EqualFold(key, "access_token") || strings.EqualFold(key, "token") {
        if token := strings.TrimSpace(value); token != "" {
            return token
        }
    }
}
```

We're directly accessing `event.QueryStringParameters` from the raw API Gateway event.

## Potential Issue

API Gateway WebSocket `$connect` events might pass query parameters differently than HTTP requests. Lift framework's abstraction layer may handle this correctly, while direct access might miss parameters.

## Next Steps

1. **Deploy the enhanced logging** we added to see what's actually in the event
2. **Check the logs** to verify if `QueryStringParameters` is populated
3. **If empty**, check if Lift framework handles WebSocket query parameters differently
4. **Consider** using Lift framework for WebSocket handling like penny-lift does, OR
5. **Check** if API Gateway requires query parameters to be passed via headers instead

The logging we added will show us exactly what API Gateway is sending, which will help diagnose the issue.

