# CloudWatch Logging with SNS Notifications Example

This example demonstrates how to use Lift's CloudWatch logger with SNS error notifications.

## Architecture

The SNS configuration is now separate from the CloudWatch logger configuration, providing better separation of concerns:

- **SNSConfig**: Contains SNS client and topic ARN
- **SNSNotifier**: Handles sending error notifications to SNS
- **CloudWatchLogger**: Accepts an optional notifier for error notifications

## Configuration Options

### 1. Using Default SNS Topic
```go
logger, err := cloudwatch.NewCloudWatchLogger(loggerConfig, cwClient,
    cloudwatch.WithDefaultErrorNotifications(snsClient))
```

### 2. Using Custom SNS Topic
```go
logger, err := cloudwatch.NewCloudWatchLogger(loggerConfig, cwClient,
    cloudwatch.WithErrorNotifications(snsClient, customTopicARN))
```

### 3. Creating SNS Notifier Separately
```go
snsConfig := cloudwatch.SNSConfig{
    Client:   snsClient,
    TopicARN: "arn:aws:sns:region:account:topic",
}
notifier := cloudwatch.NewSNSNotifier(snsConfig)

logger, err := cloudwatch.NewCloudWatchLogger(loggerConfig, cwClient,
    cloudwatch.CloudWatchLoggerOptions{
        Notifier: notifier,
    })
```

### 4. CloudWatch Logger Without SNS
```go
logger, err := cloudwatch.NewCloudWatchLogger(loggerConfig, cwClient)
```

## Features

- **Automatic Error Notifications**: ERROR level logs automatically trigger SNS notifications
- **Environment Variables**: The notifier reads PARTNER, STAGE, AWS_REGION, etc. from environment
- **Case Normalization**: Partner and stage values are converted to lowercase in notifications
- **Structured Messages**: Notifications include alert configuration for Slack integration

## Running the Example

1. Set up AWS credentials:
```bash
export AWS_REGION=us-east-1
export AWS_PROFILE=your-profile
```

2. Set environment variables:
```bash
export PARTNER=yourpartner
export STAGE=dev
export AWS_ACCOUNT_ID=123456789012
```

3. Run the example:
```bash
go run main.go
```

## SNS Notification Format

Error notifications are sent to SNS in this format:
```json
{
  "alert_config": {
    "alert_type": "LiftError",
    "alert_target_type": "SLACK"
  },
  "log_time": "2024-01-15T10:30:00Z",
  "partner": "yourpartner",
  "stage": "dev",
  "aws_region": "us-east-1",
  "aws_account": "123456789012",
  "function": "my-lambda-function",
  "subsystem": "my-lambda-function",
  "severity": "ERROR",
  "message": "{\"timestamp\":\"2024-01-15T10:30:00Z\",\"level\":\"ERROR\",...}"
}
```