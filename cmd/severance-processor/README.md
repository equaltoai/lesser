# Severance Processor

The severance processor Lambda function automatically detects federation relationship severances by monitoring DynamoDB streams for:

1. **Domain Blocks**: When a domain is blocked, all relationships with users from that domain are severed
2. **Federation Issues**: When critical federation issues are detected (unreachable instances, timeouts)
3. **Instance Health**: When an instance becomes unhealthy based on federation metrics

## Event Flow

1. DynamoDB stream event triggers Lambda
2. Processor examines the event for severance indicators
3. If a severance is detected, affected relationships are counted
4. A severance record is created with:
   - Remote instance
   - Reason (domain block, instance down, etc.)
   - Affected followers/following counts
   - Auto-detection flag
   - Severity based on impact
5. Event is emitted to streaming system for real-time notifications

## Severity Levels

- **High**: > 1000 affected relationships
- **Medium**: 100-1000 affected relationships
- **Low**: < 100 affected relationships

## Integration

The processor integrates with:
- **Severance Service**: Creates and manages severance records
- **Event Bus**: Publishes SEVERANCE_DETECTED events
- **Federation Service**: Monitors federation health
- **Relationship Storage**: Counts affected relationships

## Deployment

Built and deployed as part of the CDK infrastructure stack:
- Triggered by DynamoDB stream events
- IAM permissions for DynamoDB, CloudWatch
- Environment variables from CDK config

