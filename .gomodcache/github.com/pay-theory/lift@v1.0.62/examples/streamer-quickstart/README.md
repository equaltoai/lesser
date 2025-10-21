# Streamer Quickstart - Lift v1.0.12

This is a complete working example of WebSocket support in Lift v1.0.12.

## Quick Test

1. **Verify your Lift version:**
```bash
go list -m github.com/pay-theory/lift
# Should show: github.com/pay-theory/lift v1.0.12
```

2. **If not v1.0.12, update it:**
```bash
go get github.com/pay-theory/lift@v1.0.12
go mod tidy
```

3. **Build the example:**
```bash
go build -o streamer main.go
```

4. **Deploy to AWS Lambda** (example with SAM):

Create `template.yaml`:
```yaml
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Resources:
  WebSocketApi:
    Type: AWS::ApiGatewayV2::Api
    Properties:
      Name: StreamerWebSocket
      ProtocolType: WEBSOCKET
      RouteSelectionExpression: "$request.body.action"

  ConnectRoute:
    Type: AWS::ApiGatewayV2::Route
    Properties:
      ApiId: !Ref WebSocketApi
      RouteKey: $connect
      Target: !Sub integrations/${ConnectIntegration}

  ConnectIntegration:
    Type: AWS::ApiGatewayV2::Integration
    Properties:
      ApiId: !Ref WebSocketApi
      IntegrationType: AWS_PROXY
      IntegrationUri: !Sub arn:aws:apigateway:${AWS::Region}:lambda:path/2015-03-31/functions/${StreamerFunction.Arn}/invocations

  StreamerFunction:
    Type: AWS::Serverless::Function
    Properties:
      CodeUri: .
      Handler: streamer
      Runtime: go1.x
      Policies:
        - Statement:
          - Effect: Allow
            Action:
              - execute-api:ManageConnections
            Resource: !Sub arn:aws:execute-api:${AWS::Region}:${AWS::AccountId}:${WebSocketApi}/*
```

## Testing Locally

You can test the handler logic locally:

```go
package main

import (
    "testing"
    "github.com/pay-theory/lift/pkg/lift"
)

func TestWebSocketHandler(t *testing.T) {
    app := lift.New(lift.WithWebSocketSupport())
    
    // Your handler registration here
    app.WebSocket("$connect", func(ctx *lift.Context) error {
        return ctx.Status(200).JSON(map[string]string{
            "status": "connected",
        })
    })
    
    // Test it
    // ... test code
}
```

## Common Issues

### "undefined: lift.WithWebSocketSupport"
- You're not using v1.0.12. Run: `go get github.com/pay-theory/lift@v1.0.12`

### "app.WebSocket undefined"
- Clear module cache: `go clean -modcache`
- Re-download: `go mod download`

### Build errors
- Make sure you have all dependencies:
  ```bash
  go get github.com/aws/aws-lambda-go/lambda
  go get github.com/aws/aws-lambda-go/events
  ```

## What This Example Shows

1. **Connection Management** - Handling $connect and $disconnect
2. **Authentication** - Using query parameters for auth tokens
3. **Message Routing** - Routing messages based on action field
4. **Broadcasting** - Using WebSocket context to send messages
5. **Error Handling** - Proper error responses

## Next Steps

1. Add DynamoDB connection store for real connection management
2. Implement room/channel functionality
3. Add metrics and monitoring
4. Set up CloudWatch logging

## Need Help?

Run the verification script:
```bash
go run ../../scripts/verify-websocket-v1.0.12.go
```

This will check your environment and confirm WebSocket support is working. 