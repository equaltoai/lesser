# Lesser Production Runbook

This runbook provides operational procedures for maintaining and troubleshooting Lesser in production environments.

## Table of Contents

- [Emergency Procedures](#emergency-procedures)
- [Monitoring & Alerting](#monitoring--alerting)
- [Common Issues](#common-issues)
- [Maintenance Procedures](#maintenance-procedures)
- [Performance Tuning](#performance-tuning)
- [Security Incident Response](#security-incident-response)
- [Backup & Recovery](#backup--recovery)
- [Escalation Procedures](#escalation-procedures)

## Emergency Procedures

### Service Outage Response

#### Severity 1 (Complete Outage)

**Immediate Actions (0-15 minutes):**

1. **Assess Impact**
   ```bash
   # Check overall service health
   curl -s https://your-domain.com/health | jq '.'
   
   # Check API Gateway status
   aws apigatewayv2 get-api --api-id YOUR_API_ID --query 'ApiStatus'
   
   # Check Lambda function status
   aws lambda list-functions --query 'Functions[?starts_with(FunctionName, `lesser-`)].State'
   ```

2. **Enable Maintenance Mode**
   ```bash
   # Temporarily redirect to maintenance page
   aws route53 change-resource-record-sets --hosted-zone-id Z1234567890 --change-batch '{
     "Changes": [{
       "Action": "UPSERT",
       "ResourceRecordSet": {
         "Name": "your-domain.com",
         "Type": "A",
         "TTL": 60,
         "ResourceRecords": [{"Value": "MAINTENANCE_PAGE_IP"}]
       }
     }]
   }'
   ```

3. **Investigate Root Cause**
   ```bash
   # Check recent deployments
   aws lambda list-versions-by-function --function-name lesser-api --max-items 5
   
   # Review error logs
   aws logs filter-log-events --log-group-name /aws/lambda/lesser-api --start-time $(date -d '1 hour ago' +%s)000 --filter-pattern 'ERROR'
   
   # Check DynamoDB throttling
   aws cloudwatch get-metric-statistics --namespace AWS/DynamoDB --metric-name ThrottledRequests --dimensions Name=TableName,Value=lesser-production-main --start-time $(date -d '1 hour ago' -u +%Y-%m-%dT%H:%M:%S) --end-time $(date -u +%Y-%m-%dT%H:%M:%S) --period 300 --statistics Sum
   ```

#### Severity 2 (Partial Outage)

**Response Actions:**

1. **Identify Affected Components**
   ```bash
   # Check specific Lambda functions
   for func in lesser-api lesser-federation lesser-processor; do
     aws lambda get-function --function-name $func --query 'Configuration.State'
   done
   
   # Check federation endpoints
   curl -H "Accept: application/activity+json" https://your-domain.com/.well-known/nodeinfo
   ```

2. **Implement Workarounds**
   ```bash
   # Scale up Lambda concurrency if needed
   aws lambda put-provisioned-concurrency-config --function-name lesser-api --provisioned-concurrency-config ProvisionedConcurrencyUnits=10
   
   # Increase DynamoDB capacity temporarily
   aws dynamodb update-table --table-name lesser-production-main --provisioned-throughput ReadCapacityUnits=20,WriteCapacityUnits=10
   ```

### Rollback Procedures

#### Lambda Function Rollback

```bash
# List previous versions
aws lambda list-versions-by-function --function-name lesser-api

# Get current alias
aws lambda get-alias --function-name lesser-api --name LIVE

# Rollback to previous version
PREVIOUS_VERSION=$(aws lambda list-versions-by-function --function-name lesser-api --query 'Versions[-2].Version' --output text)
aws lambda update-alias --function-name lesser-api --name LIVE --function-version $PREVIOUS_VERSION

# Verify rollback
aws lambda get-alias --function-name lesser-api --name LIVE
curl https://your-domain.com/health
```

#### Infrastructure Rollback

```bash
# Rollback Pulumi stack
cd infra
pulumi rollback

# If rollback fails, restore from state backup
pulumi stack import --file backup-$(date -d '1 day ago' +%Y%m%d).json

# Force redeploy if needed
pulumi refresh
pulumi up --yes
```

## Monitoring & Alerting

### Key Metrics to Monitor

#### Application Metrics

| Metric | Threshold | Action |
|--------|-----------|---------|
| API Response Time P95 | > 2000ms | Investigate performance |
| Error Rate | > 5% | Check logs and scaling |
| Lambda Cold Starts | > 10/min | Increase memory/concurrency |
| DynamoDB Throttles | > 0 | Scale capacity |
| Federation Success Rate | < 95% | Check network/DNS |

#### Infrastructure Metrics

| Metric | Threshold | Action |
|--------|-----------|---------|
| Lambda Errors | > 10/hour | Check function logs |
| DynamoDB Consumed Capacity | > 80% | Scale up capacity |
| S3 4xx Errors | > 1% | Check permissions |
| API Gateway 5xx Errors | > 1% | Check Lambda health |

### Monitoring Commands

```bash
# Real-time metrics dashboard
aws cloudwatch get-dashboard --dashboard-name Lesser-Production

# Current error rate
aws cloudwatch get-metric-statistics --namespace AWS/Lambda --metric-name Errors --dimensions Name=FunctionName,Value=lesser-api --start-time $(date -d '1 hour ago' -u +%Y-%m-%dT%H:%M:%S) --end-time $(date -u +%Y-%m-%dT%H:%M:%S) --period 300 --statistics Sum

# DynamoDB health check
aws dynamodb describe-table --table-name lesser-production-main --query 'Table.TableStatus'

# Recent log errors
aws logs filter-log-events --log-group-name /aws/lambda/lesser-api --start-time $(date -d '10 minutes ago' +%s)000 --filter-pattern 'ERROR'
```

### Alert Response Procedures

#### High Error Rate Alert

1. **Immediate Assessment**
   ```bash
   # Check error types
   aws logs insights start-query --log-group-name /aws/lambda/lesser-api --start-time $(date -d '1 hour ago' +%s) --end-time $(date +%s) --query-string 'fields @timestamp, @message | filter @message like /ERROR/ | stats count() by @message'
   
   # Check recent changes
   git log --oneline --since="24 hours ago"
   ```

2. **Mitigation Steps**
   - If deployment-related: Perform rollback
   - If load-related: Scale up resources
   - If external dependency: Check third-party services

#### DynamoDB Throttling Alert

1. **Immediate Response**
   ```bash
   # Increase capacity temporarily
   aws dynamodb update-table --table-name lesser-production-main --provisioned-throughput ReadCapacityUnits=50,WriteCapacityUnits=25
   
   # Check hot partitions
   aws cloudwatch get-metric-statistics --namespace AWS/DynamoDB --metric-name ConsumedReadCapacityUnits --dimensions Name=TableName,Value=lesser-production-main --start-time $(date -d '1 hour ago' -u +%Y-%m-%dT%H:%M:%S) --end-time $(date -u +%Y-%m-%dT%H:%M:%S) --period 300 --statistics Maximum
   ```

2. **Long-term Resolution**
   - Analyze access patterns
   - Optimize data model
   - Enable auto-scaling

#### Federation Failures Alert

1. **Diagnosis**
   ```bash
   # Test federation endpoints
   curl -v -H "Accept: application/activity+json" https://your-domain.com/.well-known/nodeinfo
   curl -v https://your-domain.com/.well-known/webfinger?resource=acct:admin@your-domain.com
   
   # Check DNS resolution
   dig your-domain.com
   ```

2. **Common Fixes**
   - Verify SSL certificate validity
   - Check DNS records
   - Validate ActivityPub responses

## Common Issues

### Issue: Lambda Function Timeouts

**Symptoms:** HTTP 504 errors, incomplete operations

**Diagnosis:**
```bash
# Check timeout settings
aws lambda get-function-configuration --function-name lesser-api --query 'Timeout'

# Look for timeout patterns in logs
aws logs filter-log-events --log-group-name /aws/lambda/lesser-api --filter-pattern 'Task timed out'
```

**Resolution:**
```bash
# Increase timeout (max 15 minutes)
aws lambda update-function-configuration --function-name lesser-api --timeout 300

# Or optimize function performance
# Review code for inefficient operations
# Increase memory allocation for CPU-bound tasks
aws lambda update-function-configuration --function-name lesser-api --memory-size 1024
```

### Issue: DynamoDB Hot Partition

**Symptoms:** Throttling errors, uneven performance

**Diagnosis:**
```bash
# Check consumed capacity by partition
aws dynamodb query --table-name lesser-production-main --key-condition-expression 'PK = :pk' --expression-attribute-values '{":pk":{"S":"USER#popular_user"}}' --return-consumed-capacity TOTAL
```

**Resolution:**
```bash
# Redistribute data with better partition keys
# Enable DynamoDB auto-scaling
aws application-autoscaling register-scalable-target --service-namespace dynamodb --resource-id table/lesser-production-main --scalable-dimension dynamodb:table:ReadCapacityUnits --min-capacity 5 --max-capacity 100

# Consider using Global Secondary Indexes for alternate access patterns
aws dynamodb update-table --table-name lesser-production-main --global-secondary-index-updates '[{"Create":{"IndexName":"GSI1","KeySchema":[{"AttributeName":"gsi1PK","KeyType":"HASH"},{"AttributeName":"gsi1SK","KeyType":"RANGE"}],"Projection":{"ProjectionType":"ALL"},"ProvisionedThroughput":{"ReadCapacityUnits":5,"WriteCapacityUnits":5}}}]'
```

### Issue: High Lambda Cold Start Times

**Symptoms:** Intermittent slow responses, timeouts on first request

**Diagnosis:**
```bash
# Check initialization duration
aws logs filter-log-events --log-group-name /aws/lambda/lesser-api --filter-pattern 'REPORT' | grep -o 'Init Duration: [0-9.]*'
```

**Resolution:**
```bash
# Enable provisioned concurrency
aws lambda put-provisioned-concurrency-config --function-name lesser-api --provisioned-concurrency-config ProvisionedConcurrencyUnits=5

# Optimize package size
cd cmd/api
go build -ldflags="-s -w" -o bootstrap main.go

# Use Lambda warming (if applicable)
aws events put-rule --name lesser-warmer --schedule-expression 'rate(5 minutes)'
```

### Issue: Federation Discovery Problems

**Symptoms:** Other instances can't find your users, WebFinger failures

**Diagnosis:**
```bash
# Test WebFinger endpoint
curl 'https://your-domain.com/.well-known/webfinger?resource=acct:username@your-domain.com'

# Check DNS records
dig TXT _acct.username.your-domain.com

# Test from external service
curl -H 'User-Agent: Mastodon/4.1.0' 'https://your-domain.com/.well-known/webfinger?resource=acct:username@your-domain.com'
```

**Resolution:**
```bash
# Verify CORS headers for federation
curl -H 'Origin: https://other-instance.com' -I https://your-domain.com/.well-known/webfinger

# Check ActivityPub content-type
curl -H 'Accept: application/activity+json' https://your-domain.com/users/username

# Validate JSON-LD context
curl -H 'Accept: application/ld+json; profile="https://www.w3.org/ns/activitystreams"' https://your-domain.com/users/username
```

## Maintenance Procedures

### Weekly Maintenance

#### Log Analysis
```bash
# Review error patterns
aws logs insights start-query --log-group-name /aws/lambda/lesser-api --start-time $(date -d '7 days ago' +%s) --end-time $(date +%s) --query-string 'fields @timestamp, @message | filter @message like /ERROR/ | stats count() by bin(5m)'

# Check performance trends
aws cloudwatch get-metric-statistics --namespace AWS/Lambda --metric-name Duration --dimensions Name=FunctionName,Value=lesser-api --start-time $(date -d '7 days ago' -u +%Y-%m-%dT%H:%M:%S) --end-time $(date -u +%Y-%m-%dT%H:%M:%S) --period 86400 --statistics Average
```

#### Cost Analysis
```bash
# Weekly cost report
aws ce get-cost-and-usage --time-period Start=$(date -d '7 days ago' +%Y-%m-%d),End=$(date +%Y-%m-%d) --granularity DAILY --metrics BlendedCost --group-by Type=DIMENSION,Key=SERVICE

# Identify cost anomalies
aws ce get-cost-and-usage --time-period Start=$(date -d '7 days ago' +%Y-%m-%d),End=$(date +%Y-%m-%d) --granularity DAILY --metrics BlendedCost --group-by Type=DIMENSION,Key=LINKED_ACCOUNT
```

### Monthly Maintenance

#### Security Updates
```bash
# Update Go dependencies
go get -u all
go mod tidy
go mod verify

# Scan for vulnerabilities
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...

# Update Lambda runtimes
aws lambda update-function-configuration --function-name lesser-api --runtime go1.21
```

#### Performance Optimization
```bash
# Analyze Lambda memory usage
aws logs insights start-query --log-group-name /aws/lambda/lesser-api --start-time $(date -d '30 days ago' +%s) --end-time $(date +%s) --query-string 'fields @timestamp, @type, @memorySize, @maxMemoryUsed | filter @type = "REPORT" | stats avg(@maxMemoryUsed), max(@maxMemoryUsed) by bin(1d)'

# Review DynamoDB capacity utilization
aws cloudwatch get-metric-statistics --namespace AWS/DynamoDB --metric-name ConsumedReadCapacityUnits --dimensions Name=TableName,Value=lesser-production-main --start-time $(date -d '30 days ago' -u +%Y-%m-%dT%H:%M:%S) --end-time $(date -u +%Y-%m-%dT%H:%M:%S) --period 86400 --statistics Average,Maximum
```

### Quarterly Maintenance

#### Disaster Recovery Testing
```bash
# Create test backup
aws dynamodb create-backup --table-name lesser-production-main --backup-name quarterly-dr-test-$(date +%Y%m%d)

# Test restore procedure in staging environment
aws dynamodb restore-table-from-backup --target-table-name lesser-staging-main --backup-arn arn:aws:dynamodb:us-east-1:123456789012:table/lesser-production-main/backup/01234567890123-12345678

# Verify data integrity
go run cmd/verify-backup/main.go --source-table lesser-production-main --target-table lesser-staging-main
```

#### Capacity Planning Review
```bash
# Analyze growth trends
aws cloudwatch get-metric-statistics --namespace AWS/Lambda --metric-name Invocations --dimensions Name=FunctionName,Value=lesser-api --start-time $(date -d '90 days ago' -u +%Y-%m-%dT%H:%M:%S) --end-time $(date -u +%Y-%m-%dT%H:%M:%S) --period 86400 --statistics Sum

# Project future capacity needs
# Review user growth, storage usage, and compute requirements
```

## Performance Tuning

### Lambda Optimization

#### Memory and CPU Tuning
```bash
# Test different memory configurations
for memory in 512 1024 2048 3008; do
  aws lambda update-function-configuration --function-name lesser-api --memory-size $memory
  # Run load tests and measure performance
  k6 run --duration 5m tests/k6/realistic-load.js
done

# Select optimal configuration based on cost/performance
```

#### Concurrency Management
```bash
# Set reserved concurrency to prevent throttling other functions
aws lambda put-reserved-concurrency-config --function-name lesser-api --reserved-concurrency-units 100

# Configure provisioned concurrency for critical functions
aws lambda put-provisioned-concurrency-config --function-name lesser-api --provisioned-concurrency-config ProvisionedConcurrencyUnits=10
```

### Database Optimization

#### DynamoDB Performance Tuning
```bash
# Enable auto-scaling
aws application-autoscaling register-scalable-target --service-namespace dynamodb --resource-id table/lesser-production-main --scalable-dimension dynamodb:table:ReadCapacityUnits --min-capacity 5 --max-capacity 200

aws application-autoscaling put-scaling-policy --policy-name LesserTableReadCapacityUtilizationScalingPolicy --service-namespace dynamodb --resource-id table/lesser-production-main --scalable-dimension dynamodb:table:ReadCapacityUnits --policy-type TargetTrackingScaling --target-tracking-scaling-policy-configuration TargetValue=70.0,PredefinedMetricSpecification='{PredefinedMetricType=DynamoDBReadCapacityUtilization}'
```

#### Query Optimization
```bash
# Analyze slow queries
aws logs insights start-query --log-group-name /aws/lambda/lesser-api --start-time $(date -d '24 hours ago' +%s) --end-time $(date +%s) --query-string 'fields @timestamp, @message | filter @message like /DynamoDB/ and @duration > 1000 | sort @timestamp desc'

# Monitor GSI usage
aws cloudwatch get-metric-statistics --namespace AWS/DynamoDB --metric-name ConsumedReadCapacityUnits --dimensions Name=TableName,Value=lesser-production-main,Name=GlobalSecondaryIndexName,Value=GSI1 --start-time $(date -d '24 hours ago' -u +%Y-%m-%dT%H:%M:%S) --end-time $(date -u +%Y-%m-%dT%H:%M:%S) --period 3600 --statistics Sum
```

## Security Incident Response

### Security Alert Response

#### Suspicious Activity Detection
```bash
# Check for unusual API patterns
aws logs insights start-query --log-group-name /aws/lambda/lesser-api --start-time $(date -d '24 hours ago' +%s) --end-time $(date +%s) --query-string 'fields @timestamp, @requestId, sourceIP, userAgent | filter sourceIP like /SUSPICIOUS_IP/ | sort @timestamp desc'

# Monitor failed authentication attempts
aws logs insights start-query --log-group-name /aws/lambda/lesser-auth --start-time $(date -d '24 hours ago' +%s) --end-time $(date +%s) --query-string 'fields @timestamp, @message | filter @message like /authentication failed/ | stats count() by sourceIP'
```

#### Incident Containment
```bash
# Block suspicious IP addresses
aws wafv2 update-ip-set --scope CLOUDFRONT --id IP_SET_ID --addresses SUSPICIOUS_IP1,SUSPICIOUS_IP2

# Temporarily increase rate limiting
aws apigatewayv2 update-stage --api-id YOUR_API_ID --stage-name prod --throttle RateLimit=10,BurstLimit=20

# Force user re-authentication by rotating JWT secrets
aws secretsmanager update-secret --secret-id lesser-jwt-secret --secret-string "$(openssl rand -base64 32)"
```

#### Evidence Collection
```bash
# Export relevant logs
aws logs create-export-task --log-group-name /aws/lambda/lesser-api --from $(date -d '24 hours ago' +%s)000 --to $(date +%s)000 --destination security-logs-bucket --destination-prefix incident-$(date +%Y%m%d)

# Capture current system state
aws lambda list-functions > incident-lambda-state.json
aws dynamodb describe-table --table-name lesser-production-main > incident-db-state.json
```

### Recovery Procedures

#### Post-Incident Recovery
```bash
# Reset security configurations
aws wafv2 update-ip-set --scope CLOUDFRONT --id IP_SET_ID --addresses ""

# Restore normal rate limits
aws apigatewayv2 update-stage --api-id YOUR_API_ID --stage-name prod --throttle RateLimit=100,BurstLimit=200

# Update security patches
go get -u all
make build-lambdas
pulumi up --yes
```

## Backup & Recovery

### Automated Backups

#### DynamoDB Backup Management
```bash
# Enable continuous backups
aws dynamodb put-backup-policy --table-name lesser-production-main --backup-policy BackupEnabled=true

# Create on-demand backup
aws dynamodb create-backup --table-name lesser-production-main --backup-name scheduled-backup-$(date +%Y%m%d-%H%M%S)

# List available backups
aws dynamodb list-backups --table-name lesser-production-main --time-range-lower-bound $(date -d '7 days ago' +%s) --time-range-upper-bound $(date +%s)
```

#### S3 Media Backup
```bash
# Enable versioning on media bucket
aws s3api put-bucket-versioning --bucket lesser-production-media --versioning-configuration Status=Enabled

# Configure cross-region replication
aws s3api put-bucket-replication --bucket lesser-production-media --replication-configuration file://replication-config.json

# Create point-in-time snapshot
aws s3 sync s3://lesser-production-media s3://lesser-backup-media/$(date +%Y%m%d)
```

### Recovery Procedures

#### Full System Recovery
```bash
# 1. Restore DynamoDB table
BACKUP_ARN="arn:aws:dynamodb:us-east-1:123456789012:table/lesser-production-main/backup/01234567890123-12345678"
aws dynamodb restore-table-from-backup --target-table-name lesser-production-main-restored --backup-arn $BACKUP_ARN

# 2. Restore media files
aws s3 sync s3://lesser-backup-media/20241225 s3://lesser-production-media-restored

# 3. Update Pulumi configuration to use restored resources
pulumi config set lesser:dynamoTable lesser-production-main-restored
pulumi config set lesser:s3Bucket lesser-production-media-restored

# 4. Redeploy infrastructure
pulumi up --yes

# 5. Update DNS to point to recovered instance
aws route53 change-resource-record-sets --hosted-zone-id Z1234567890 --change-batch file://recovery-dns-change.json
```

#### Partial Recovery (Single Function)
```bash
# Restore single Lambda function from previous version
aws lambda update-alias --function-name lesser-api --name LIVE --function-version $GOOD_VERSION

# Or redeploy from source
cd cmd/api
make build
aws lambda update-function-code --function-name lesser-api --zip-file fileb://deployment-package.zip
```

## Escalation Procedures

### Escalation Matrix

| Issue Type | Severity | Initial Response Time | Escalation Level |
|------------|----------|----------------------|------------------|
| Complete Outage | Critical | Immediate | On-call Engineer → Lead → CTO |
| Performance Degradation | High | 15 minutes | On-call Engineer → Lead |
| Security Incident | Critical | Immediate | On-call Engineer → Security Team → Legal |
| Data Loss | Critical | Immediate | On-call Engineer → Lead → DPO |

### Contact Information

#### Internal Team
- **On-call Engineer**: [Slack/Phone]
- **Lead Engineer**: [Contact Info]
- **DevOps Lead**: [Contact Info]
- **Security Team**: [Contact Info]

#### External Vendors
- **AWS Support**: [Case Portal / Phone]
- **DNS Provider**: [Support Contact]
- **Monitoring Service**: [Support Contact]

### Escalation Scripts

#### Incident Notification
```bash
# Send alert to team
curl -X POST -H 'Content-type: application/json' --data '{"text":"🚨 Lesser Production Issue: [DESCRIPTION]\nSeverity: [LEVEL]\nTime: $(date)\nIncident Lead: [NAME]"}' YOUR_SLACK_WEBHOOK_URL

# Create support case (if needed)
aws support create-case --subject "Lesser Production Issue" --service-code "lambda" --severity-code "high" --category-code "performance" --communication-body "Description of issue and current status"
```

#### Status Page Updates
```bash
# Update status page (if applicable)
curl -X POST "https://api.statuspage.io/v1/pages/PAGE_ID/incidents" \
  -H "Authorization: OAuth TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"incident":{"name":"Lesser API Issues","status":"investigating","impact_override":"major","component_ids":["COMPONENT_ID"]}}'
```

---

## Quick Reference

### Emergency Commands
```bash
# Check overall health
curl -s https://your-domain.com/health/detailed | jq '.'

# View live logs
aws logs tail /aws/lambda/lesser-api --follow

# Scale up immediately
aws lambda put-provisioned-concurrency-config --function-name lesser-api --provisioned-concurrency-config ProvisionedConcurrencyUnits=20

# Emergency rollback
aws lambda update-alias --function-name lesser-api --name LIVE --function-version $PREVIOUS_VERSION

# Enable maintenance mode
# [Update DNS to maintenance page IP]
```

### Important URLs
- **Health Check**: `https://your-domain.com/health`
- **Detailed Health**: `https://your-domain.com/health/detailed`
- **Metrics Dashboard**: [CloudWatch Dashboard URL]
- **Cost Dashboard**: [AWS Cost Explorer URL]

This runbook should be kept current with any infrastructure or procedural changes. Review and update quarterly or after significant incidents.
