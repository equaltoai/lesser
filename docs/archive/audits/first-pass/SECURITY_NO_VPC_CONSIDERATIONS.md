# Security Considerations: No VPC Architecture

## Overview

Lesser operates as a pure serverless implementation without VPC connectivity. This means all AWS Lambda functions run in the AWS-managed network environment without customer-controlled network security groups or private subnets.

## Security Implications

### What We Cannot Do (VPC-Based Controls)
- ❌ Network Security Groups for egress filtering
- ❌ VPC Endpoints for private AWS service access  
- ❌ VPC Flow Logs for network monitoring
- ❌ Private subnets for isolation
- ❌ Network ACLs for traffic control

### What We Must Do (Application-Layer Controls)
- ✅ Application-level SSRF protection
- ✅ Request validation in Lambda code
- ✅ Secure HTTP client with URL filtering
- ✅ API Gateway request validation
- ✅ Lambda function permission boundaries

## SSRF Protection Without VPC

Since we cannot use network-level controls, our secure HTTP client implementation is **critical**:

```go
// pkg/httpclient/client.go
// This replaces what VPC Security Groups would normally do

var blockedNetworks = []string{
    "10.0.0.0/8",      // Private Class A
    "172.16.0.0/12",   // Private Class B  
    "192.168.0.0/16",  // Private Class C
    "127.0.0.0/8",     // Loopback
    "169.254.0.0/16",  // Link-local
    // AWS Metadata endpoints
    "169.254.169.254", 
    "fd00:ec2::254",
}
```

## Compensating Controls

### 1. Lambda Execution Role Restrictions
```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:Query"
    ],
    "Resource": "arn:aws:dynamodb:*:*:table/lesser-*"
  }]
}
```

### 2. API Gateway Request Validation
```yaml
requestValidators:
  all:
    validateRequestBody: true
    validateRequestParameters: true

models:
  ActivityPubObject:
    type: object
    required: [type, id]
    properties:
      type:
        type: string
        enum: [Create, Update, Delete, Follow, Like]
```

### 3. CloudWatch Monitoring
- Log all blocked SSRF attempts
- Alert on suspicious request patterns
- Track authentication failures
- Monitor for enumeration attempts

### 4. AWS WAF Integration
Configure WAF rules for additional protection:
- SQL injection prevention
- XSS filtering
- Rate limiting
- Geographic restrictions

## Security Benefits of No-VPC

### 1. Reduced Attack Surface
- No EC2 instances to patch
- No network misconfigurations
- No VPC peering risks
- Automatic OS patching by AWS

### 2. Simplified Security Model
- Focus on application security
- No network complexity
- Easier compliance auditing
- Clear security boundaries

### 3. Cost Optimization
- No NAT Gateway costs
- No VPC endpoint charges
- No data transfer fees within VPC
- True pay-per-use model

## Best Practices for Serverless Security

### 1. Least Privilege IAM
Each Lambda function should have minimal permissions:
```javascript
// Bad - Too permissive
"Action": "s3:*"

// Good - Specific actions on specific resources
"Action": ["s3:GetObject", "s3:PutObject"],
"Resource": "arn:aws:s3:::lesser-media/*"
```

### 2. Environment Variable Security
- Never store secrets in code
- Use AWS Secrets Manager
- Rotate credentials regularly
- Audit access logs

### 3. Input Validation
With no network filtering, input validation is critical:
```go
// Validate everything at the edge
func validateRequest(req Request) error {
    if len(req.Body) > MaxBodySize {
        return ErrBodyTooLarge
    }
    if !isValidContentType(req.ContentType) {
        return ErrInvalidContentType
    }
    // ... more validation
}
```

## Monitoring & Alerting

### CloudWatch Alarms
- Lambda errors > threshold
- Concurrent executions approaching limit
- Duration approaching timeout
- Memory usage patterns

### Security Events to Track
```go
// Log security-relevant events
logger.Info("SSRF blocked", 
    zap.String("url", attemptedURL),
    zap.String("function", functionName),
    zap.String("user", userID))
```

## Conclusion

Operating without VPC requires discipline and comprehensive application-level security controls. Lesser's architecture demonstrates that serverless applications can be secure without traditional network boundaries by:

1. Implementing robust application-level controls
2. Leveraging AWS-managed security features
3. Following serverless security best practices
4. Maintaining comprehensive monitoring

The key is recognizing that in serverless, **the application IS the perimeter**. 