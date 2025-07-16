# Set environment variables for testing
$env:TESTING = "true"
$env:JWT_SECRET = "test_jwt_secret_for_testing"
$env:DOMAIN = "localhost"
$env:INSTANCE_NAME = "Lesser Test"
$env:AWS_REGION = "us-east-1"
$env:DYNAMO_TABLE_NAME = "lesser-test"
$env:S3_BUCKET_NAME = "lesser-test-media"

# Run the tests
go test ./...