# Zap Logger with SNS Notifications Example

This example demonstrates how to use the Zap logger with SNS notifications for error alerts in a Lambda function.

## Features

- **Zap Logger**: Writes structured logs to stdout/stderr (captured by Lambda)
- **SNS Notifications**: Sends alerts to SNS when errors are logged
- **Lambda Runtime Logs**: All logs appear together with Lambda's INIT_START, START, END, REPORT entries

## Key Benefits

Unlike the CloudWatch logger which writes directly to CloudWatch Logs API:
- Zap logger writes to stdout/stderr
- Lambda captures these logs alongside its runtime logs
- All logs appear in the same CloudWatch log stream
- No missing Lambda runtime logs

## Usage

```go
// Create Zap logger with default SNS notifications
logger, err := zap.NewZapLogger(loggerConfig, 
    zap.WithDefaultErrorNotifications(snsClient))

// Or with custom SNS topic
logger, err := zap.NewZapLogger(loggerConfig, 
    zap.WithErrorNotifications(snsClient, customTopicARN))
```

## Environment Variables

Required for default SNS notifications:
- `PARTNER`: Your partner identifier
- `STAGE`: Deployment stage (dev, staging, prod)
- `AWS_REGION`: AWS region
- `AWS_ACCOUNT_ID`: AWS account ID

## Testing

Deploy and test the error notification:
```bash
curl https://your-api-gateway-url/test?error=true
```

This will:
1. Log an error to CloudWatch Logs (via stdout)
2. Send an SNS notification with the error details