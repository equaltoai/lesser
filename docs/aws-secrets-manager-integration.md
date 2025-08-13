# AWS Secrets Manager Integration for Recovery Federation

## Overview

Lesser now uses AWS Secrets Manager to securely store private keys for system actors used in recovery federation. This replaces the previous insecure storage method and provides enterprise-grade security for cryptographic keys.

## Features

- **Secure Storage**: Private keys are encrypted at rest using AWS Secrets Manager
- **Key Rotation**: Automated key rotation capabilities
- **Caching**: In-memory caching with TTL to reduce AWS API calls
- **Audit Logging**: All key access is logged through AWS CloudTrail
- **High Availability**: Multi-AZ replication through AWS infrastructure

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AWS_REGION` | Yes | `us-east-1` | AWS region for Secrets Manager |
| `AWS_ACCESS_KEY_ID` | Conditional | - | AWS access key (if not using IAM roles) |
| `AWS_SECRET_ACCESS_KEY` | Conditional | - | AWS secret key (if not using IAM roles) |
| `DOMAIN_NAME` | Yes | - | Your instance domain name |

### Secrets Manager Configuration

The service automatically configures itself with the following defaults:

- **Secret Prefix**: `lesser/system-actor-keys`
- **Cache TTL**: 5 minutes
- **Key Type**: RSA 2048-bit
- **Secret Format**: JSON with metadata

### IAM Permissions

The Lambda functions or service running Lesser must have the following IAM permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "secretsmanager:CreateSecret",
                "secretsmanager:GetSecretValue",
                "secretsmanager:UpdateSecret",
                "secretsmanager:DeleteSecret",
                "secretsmanager:DescribeSecret",
                "secretsmanager:ListSecrets"
            ],
            "Resource": "arn:aws:secretsmanager:*:*:secret:lesser/system-actor-keys/*"
        },
        {
            "Effect": "Allow",
            "Action": [
                "secretsmanager:ListSecrets"
            ],
            "Resource": "*"
        }
    ]
}
```

### Minimal IAM Policy (Read-Only)

For production deployments where keys are pre-created:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "secretsmanager:GetSecretValue"
            ],
            "Resource": "arn:aws:secretsmanager:*:*:secret:lesser/system-actor-keys/*"
        }
    ]
}
```

## Deployment

### 1. Create IAM Role/Policy

Create an IAM role or policy with the required permissions above.

### 2. Set Environment Variables

```bash
export AWS_REGION=us-east-1
export DOMAIN_NAME=your-instance.com
```

### 3. Deploy Application

The Secrets Manager integration will automatically:
- Initialize on first startup
- Generate system actor keys if they don't exist
- Cache keys for performance
- Handle errors gracefully

## Key Management

### Automatic Key Generation

When the service starts and no system actor keys exist:

1. A new RSA 2048-bit key pair is generated
2. Private key is stored in AWS Secrets Manager
3. Public key is stored in the database with the actor
4. Keys are tagged with service metadata

### Manual Key Rotation

To rotate system actor keys:

```go
// Access the recovery federation service
recoveryService := auth.NewRecoveryFederationService(...)

// Rotate the system actor key
err := recoveryService.RotateSystemActorKey(ctx)
if err != nil {
    log.Fatal("Failed to rotate key:", err)
}
```

### Key Rotation Best Practices

- Rotate keys every 90 days
- Monitor key usage through CloudTrail
- Test federation after rotation
- Keep old keys for a grace period during federation propagation

## Security Considerations

### Encryption

- All keys are encrypted at rest using AWS KMS
- Keys are encrypted in transit using TLS 1.2+
- Memory caching is limited to 5 minutes TTL
- Keys are cleared from memory after use

### Access Control

- Use IAM roles instead of access keys when possible
- Limit secret access to specific resources
- Enable CloudTrail logging for audit purposes
- Use resource-based policies for fine-grained control

### Monitoring

- Set up CloudWatch alarms for secret access
- Monitor for unusual access patterns
- Log all key operations at application level
- Set up notifications for key rotation events

## Error Handling

The integration handles various error scenarios:

| Error | Behavior | Fallback |
|-------|----------|----------|
| Secrets Manager unavailable | Log warning, continue without system actor | No federation signing |
| Key not found | Generate new key pair automatically | Full recovery |
| Invalid key format | Log error, attempt regeneration | New key generation |
| AWS credentials missing | Graceful degradation | System actor disabled |
| Network timeout | Retry with exponential backoff | Cache utilization |

## Performance

### Caching Strategy

- In-memory cache with 5-minute TTL
- Automatic cache invalidation on updates
- Cache cleanup runs periodically
- Cache statistics available for monitoring

### AWS API Usage

- Batch operations where possible
- Exponential backoff on failures
- Connection pooling for efficiency
- Regional endpoint usage

### Monitoring Metrics

Track these metrics for performance:

- Cache hit rate
- AWS API call frequency
- Key retrieval latency
- Error rates by type

## Cost Optimization

### Secrets Manager Pricing

- $0.40 per secret per month
- $0.05 per 10,000 API calls
- No charges for KMS encryption

### Cost Reduction Strategies

- Use caching to reduce API calls
- Implement efficient key rotation schedules
- Monitor usage with CloudWatch
- Use resource tags for cost allocation

## Troubleshooting

### Common Issues

1. **Keys not generating**
   - Check IAM permissions
   - Verify AWS region settings
   - Check AWS credentials

2. **Federation signing failures**
   - Verify key format in Secrets Manager
   - Check system actor creation in database
   - Validate domain configuration

3. **Performance issues**
   - Monitor cache hit rates
   - Check AWS API latency
   - Verify network connectivity

### Debug Commands

```bash
# Check AWS credentials
aws sts get-caller-identity

# List secrets
aws secretsmanager list-secrets --region us-east-1

# Get secret value (for debugging)
aws secretsmanager get-secret-value \
  --secret-id lesser/system-actor-keys/system-actor-yourdomain.com \
  --region us-east-1
```

## Migration from Legacy Storage

If migrating from the previous insecure storage method:

1. Identify existing system actors with stored private keys
2. Extract private keys from database
3. Store keys in Secrets Manager using the new format
4. Update actor records to remove private key from database
5. Verify federation still works after migration
6. Clean up old private key data

## Security Audit Checklist

- [ ] IAM permissions follow least privilege principle
- [ ] CloudTrail logging enabled for Secrets Manager
- [ ] Private keys never logged or exposed
- [ ] Regular key rotation schedule established
- [ ] Monitoring and alerting configured
- [ ] Access patterns reviewed regularly
- [ ] Resource tags applied for cost tracking
- [ ] Backup and recovery procedures documented

## Support and Maintenance

### Regular Tasks

- Monitor AWS costs
- Review access logs
- Update IAM policies as needed
- Test key rotation procedures
- Validate federation functionality

### Incident Response

1. Identify the scope of any key compromise
2. Rotate affected keys immediately
3. Update federation partners
4. Review access logs for unauthorized usage
5. Document lessons learned

For additional support, refer to the Lesser project documentation or open an issue in the repository.