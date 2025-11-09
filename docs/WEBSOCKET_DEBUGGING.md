# WebSocket Connection Debugging Guide

This guide helps diagnose and fix WebSocket connection failures in Lesser.

## WebSocket Endpoints

For `dev.lesser.host`:
- **GraphQL WebSocket**: `wss://graphql-ws.dev.lesser.host`
- **Streaming WebSocket**: `wss://stream.dev.lesser.host`

## Common Failure Scenarios

### 1. Authentication Failures (401/403)

**Symptoms:**
- Connection immediately closes with HTTP 401/403
- Client sees "authentication required" or "invalid token"

**For GraphQL WebSocket:**
- **Requires authentication** - token must be provided
- Token can be passed via:
  - Query parameter: `?access_token=<token>`
  - Authorization header: `Authorization: Bearer <token>`

**For Streaming WebSocket:**
- Allows anonymous connections for public streams
- Authentication required for user-specific streams (`user`, `user:notification`, `direct`, `list:*`)

**Debugging Steps:**

1. Verify token is valid:
```bash
# Test token validation
curl -H "Authorization: Bearer $TOKEN" https://dev.lesser.host/api/v1/accounts/verify_credentials
```

2. Check token format:
```bash
# Token should be passed in WebSocket URL or headers
wss://graphql-ws.dev.lesser.host?access_token=<token>
# OR
# With Authorization header: Bearer <token>
```

3. Check CloudWatch logs for authentication errors:
```bash
# Look for these log messages:
# - "websocket connect missing authentication token"
# - "websocket connect failed token validation"
# - "invalid token"
```

### 2. Connection Persistence Failures (500)

**Symptoms:**
- Connection closes immediately after connect
- CloudWatch logs show "failed to write connection" or "failed to persist websocket connection"

**Root Causes:**
- DynamoDB write failures
- Missing or incorrect table configuration
- IAM permissions issues

**Debugging Steps:**

1. Check DynamoDB table exists:
```bash
aws dynamodb describe-table --table-name lesser-dev --region us-east-1
```

2. Verify Lambda IAM permissions:
```bash
# Lambda needs permissions to:
# - dynamodb:PutItem
# - dynamodb:GetItem
# - dynamodb:UpdateItem
# - dynamodb:DeleteItem
```

3. Check CloudWatch logs:
```bash
# Look for:
# - "failed to write connection"
# - "failed to persist websocket connection"
# - DynamoDB error messages
```

4. Verify environment variables:
```bash
# Required env vars:
# - DYNAMODB_TABLE (or DYNAMO_TABLE_NAME)
# - CONNECTIONS_TABLE (optional override)
# - SUBSCRIPTIONS_TABLE (optional override)
```

### 3. Management API Endpoint Issues

**Symptoms:**
- Connection establishes but messages fail to send
- Errors when trying to send messages to client
- "failed to send websocket message via management endpoint"

**Root Causes:**
- Incorrect management API endpoint construction
- Region mismatch
- Stage/domain configuration issues

**Debugging Steps:**

1. Verify endpoint construction:
```bash
# Management endpoint should be:
# https://<api-id>.execute-api.<region>.amazonaws.com/<stage>
# OR for custom domain:
# https://<custom-domain>/<stage>
```

2. Check API Gateway configuration:
```bash
# Verify WebSocket API exists
aws apigatewayv2 get-apis --region us-east-1

# Check stage configuration
aws apigatewayv2 get-stage --api-id <api-id> --stage-name dev --region us-east-1
```

3. Verify custom domain mapping:
```bash
# Check domain name configuration
aws apigatewayv2 get-domain-name --domain-name graphql-ws.dev.lesser.host --region us-east-1
```

### 4. Domain/Certificate Issues

**Symptoms:**
- SSL/TLS handshake failures
- Certificate validation errors
- DNS resolution failures

**Debugging Steps:**

1. Test DNS resolution:
```bash
dig graphql-ws.dev.lesser.host
dig stream.dev.lesser.host
```

2. Test SSL certificate:
```bash
openssl s_client -connect graphql-ws.dev.lesser.host:443 -servername graphql-ws.dev.lesser.host
```

3. Verify certificate in ACM:
```bash
aws acm list-certificates --region us-east-1
aws acm describe-certificate --certificate-arn <arn> --region us-east-1
```

4. Check certificate validation status:
```bash
# Certificate must be in "ISSUED" status
aws acm list-certificates --region us-east-1 --certificate-statuses ISSUED
```

### 5. Connection Timeout Issues

**Symptoms:**
- Connection establishes but closes after inactivity
- No response to ping messages

**Root Causes:**
- Idle timeout configuration
- Missing keepalive/ping handling

**Debugging Steps:**

1. Check idle timeout settings:
```bash
# Default idle timeout is usually 10 minutes
# Check API Gateway stage configuration
```

2. Verify ping/pong handling:
```bash
# Client should send ping every 20-30 seconds
# Server should respond with pong
```

## Diagnostic Tools

### Test Script

Use the provided test script to diagnose connection issues:

```bash
# Test GraphQL WebSocket
python tests/system/test_streaming.py dev <access_token>

# Test Streaming WebSocket
python tests/system/test_streaming.py dev <access_token>
```

### Manual Connection Test

```bash
# Using wscat (install via npm: npm install -g wscat)
wscat -c wss://graphql-ws.dev.lesser.host?access_token=<token>

# Send connection_init
{"type":"connection_init"}

# Send ping
{"type":"ping"}
```

## CloudWatch Log Groups

Monitor these log groups for WebSocket issues:

- `/aws/lambda/lesser-dev-graphql-ws` - GraphQL WebSocket handler
- `/aws/lambda/lesser-dev-streaming` - Streaming WebSocket handler
- `/aws/apigateway/lesser-dev` - API Gateway access logs

## Common Log Patterns

### Successful Connection
```
websocket connection established
connection_id=<id>
username=<username>
```

### Authentication Failure
```
websocket connect missing authentication token
connection_id=<id>
```
OR
```
websocket connect failed token validation
connection_id=<id>
error=<error>
```

### Persistence Failure
```
failed to persist websocket connection
connection_id=<id>
username=<username>
error=<error>
```

### Message Send Failure
```
failed to send websocket message via management endpoint
connection_id=<id>
endpoint=<endpoint>
error=<error>
```

## Quick Fixes

### Issue: 401 Authentication Required

**Solution:**
1. Ensure token is passed correctly
2. Verify token is not expired
3. Check token has required scopes

### Issue: 500 Connection Persistence Failed

**Solution:**
1. Verify DynamoDB table exists and is accessible
2. Check Lambda IAM role has DynamoDB permissions
3. Verify environment variables are set correctly

### Issue: Messages Not Received

**Solution:**
1. Verify management API endpoint is correct
2. Check API Gateway stage configuration
3. Ensure custom domain is properly mapped

### Issue: SSL Certificate Errors

**Solution:**
1. Verify certificate is issued and validated in ACM
2. Check certificate matches domain name
3. Ensure Route53 records point to API Gateway

## Environment Variables Checklist

For WebSocket Lambdas:

```bash
# Required
DYNAMODB_TABLE=lesser-dev
AWS_REGION=us-east-1
JWT_SECRET_ARN=arn:aws:secretsmanager:...

# Optional
CONNECTIONS_TABLE=lesser-dev  # Override for connections table
SUBSCRIPTIONS_TABLE=lesser-dev  # Override for subscriptions table
WEBSOCKET_ENDPOINT=https://graphql-ws.dev.lesser.host
WEBSOCKET_API_URL=https://graphql-ws.dev.lesser.host
```

## Testing Checklist

- [ ] DNS resolution works for WebSocket domains
- [ ] SSL certificates are valid and issued
- [ ] API Gateway WebSocket APIs are deployed
- [ ] Custom domain mappings are configured
- [ ] Lambda functions have correct IAM permissions
- [ ] DynamoDB tables exist and are accessible
- [ ] Environment variables are set correctly
- [ ] Authentication tokens are valid
- [ ] Management API endpoints are correct

## Getting Help

If issues persist:

1. Check CloudWatch logs for detailed error messages
2. Verify infrastructure is deployed correctly (`make deploy-dev`)
3. Test with the provided test scripts
4. Review API Gateway metrics in CloudWatch
5. Check DynamoDB throttling metrics

