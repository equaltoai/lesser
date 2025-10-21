#!/bin/bash

# Integration test runner for Lift CDK constructs
# This script handles the full deployment and testing cycle

set -e

# Configuration
TEST_PREFIX="lift-cdk-integration"
TIMESTAMP=$(date +%s)
STACK_NAME="${TEST_PREFIX}-${TIMESTAMP}"
REGION="${AWS_REGION:-us-east-1}"
TEST_TIMEOUT="30m"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

cleanup() {
    log_info "Cleaning up resources..."
    
    # Delete test stacks
    aws cloudformation list-stacks \
        --stack-status-filter CREATE_COMPLETE UPDATE_COMPLETE \
        --query "StackSummaries[?starts_with(StackName, '${TEST_PREFIX}')].StackName" \
        --output text | tr '\t' '\n' | while read stack; do
        if [ ! -z "$stack" ]; then
            log_info "Deleting stack: $stack"
            aws cloudformation delete-stack --stack-name "$stack" --region "$REGION" || true
        fi
    done
    
    # Clean up test artifacts
    rm -rf cdk.out/
    rm -rf test-artifacts/
}

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check AWS CLI
    if ! command -v aws &> /dev/null; then
        log_error "AWS CLI is not installed"
        exit 1
    fi
    
    # Check CDK CLI
    if ! command -v cdk &> /dev/null; then
        log_error "AWS CDK CLI is not installed"
        log_info "Install with: npm install -g aws-cdk"
        exit 1
    fi
    
    # Check AWS credentials
    if ! aws sts get-caller-identity &> /dev/null; then
        log_error "AWS credentials are not configured"
        exit 1
    fi
    
    # Check Go
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed"
        exit 1
    fi
    
    log_info "Prerequisites check passed"
}

create_test_app() {
    log_info "Creating test CDK app..."
    
    mkdir -p test-artifacts
    cd test-artifacts
    
    # Create a test CDK app
    cat > app.go << 'EOF'
package main

import (
    "os"
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
    "github.com/aws/jsii-runtime-go"
    "github.com/pay-theory/lift/pkg/cdk/constructs"
    "github.com/pay-theory/lift/pkg/cdk/patterns"
)

func main() {
    app := awscdk.NewApp(nil)
    stackName := os.Getenv("STACK_NAME")
    if stackName == "" {
        stackName = "lift-integration-test"
    }
    
    stack := awscdk.NewStack(app, jsii.String(stackName), &awscdk.StackProps{
        StackName: jsii.String(stackName),
    })
    
    // Test constructs
    testConstructs(stack)
    
    app.Synth(nil)
}

func testConstructs(stack awscdk.Stack) {
    // Basic Lift Function
    constructs.NewLiftFunction(stack, jsii.String("BasicFunction"), &constructs.LiftFunctionProps{
        FunctionProps: awslambda.FunctionProps{
            Code:    awslambda.Code_FromAsset(jsii.String("../lambda"), nil),
            Handler: jsii.String("bootstrap"),
        },
        AppName: jsii.String("integration-test"),
    })
    
    // Rate Limited Function
    constructs.NewRateLimitedFunction(stack, jsii.String("RateLimited"), &constructs.RateLimitedFunctionProps{
        LiftFunctionProps: constructs.LiftFunctionProps{
            FunctionProps: awslambda.FunctionProps{
                Code:    awslambda.Code_FromAsset(jsii.String("../lambda"), nil),
                Handler: jsii.String("bootstrap"),
            },
            AppName: jsii.String("rate-limited-test"),
        },
        RateLimitType: jsii.String("IP"),
        RequestLimit:  jsii.Number(100),
        WindowMinutes: jsii.Number(5),
    })
    
    // Monitored Function
    constructs.NewMonitoredFunction(stack, jsii.String("Monitored"), &constructs.MonitoredFunctionProps{
        LiftFunctionProps: constructs.LiftFunctionProps{
            FunctionProps: awslambda.FunctionProps{
                Code:    awslambda.Code_FromAsset(jsii.String("../lambda"), nil),
                Handler: jsii.String("bootstrap"),
            },
            AppName: jsii.String("monitored-test"),
        },
        EnableDashboard:          jsii.Bool(true),
        EnableLogInsightsQueries: jsii.Bool(true),
    })
    
    // Basic API Pattern
    patterns.NewBasicAPI(stack, jsii.String("BasicAPI"), &patterns.BasicAPIProps{
        AppName:     jsii.String("basic-api-test"),
        CodePath:    awslambda.Code_FromAsset(jsii.String("../lambda"), nil),
        EnableCORS:  jsii.Bool(true),
        Description: jsii.String("Integration test API"),
    })
}
EOF

    # Create cdk.json
    cat > cdk.json << EOF
{
    "app": "go run app.go",
    "context": {
        "@aws-cdk/core:stackRelativeExports": true
    }
}
EOF
    
    # Create test Lambda code
    mkdir -p ../lambda
    cat > ../lambda/main.go << 'EOF'
package main

import (
    "context"
    "encoding/json"
    "github.com/aws/aws-lambda-go/lambda"
)

type Response struct {
    StatusCode int               `json:"statusCode"`
    Headers    map[string]string `json:"headers"`
    Body       string           `json:"body"`
}

func handler(ctx context.Context, event interface{}) (Response, error) {
    body := map[string]string{
        "message": "Integration test successful",
        "status":  "ok",
    }
    
    bodyJSON, _ := json.Marshal(body)
    
    return Response{
        StatusCode: 200,
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
        Body: string(bodyJSON),
    }, nil
}

func main() {
    lambda.Start(handler)
}
EOF

    # Build Lambda
    cd ../lambda
    GOOS=linux GOARCH=arm64 go build -o bootstrap main.go
    cd ../test-artifacts
    
    log_info "Test app created"
}

run_integration_tests() {
    log_info "Running integration tests..."
    
    # Set environment variables
    export STACK_NAME="$STACK_NAME"
    export AWS_REGION="$REGION"
    
    # Bootstrap CDK if needed
    log_info "Bootstrapping CDK environment..."
    cdk bootstrap aws://${AWS_ACCOUNT_ID}/${REGION} || log_warn "CDK bootstrap failed (may already exist)"
    
    # Synthesize
    log_info "Synthesizing CDK app..."
    cdk synth
    
    # Deploy
    log_info "Deploying test stack..."
    cdk deploy --require-approval never
    
    # Run Go integration tests
    log_info "Running Go integration tests..."
    cd ../..
    go test -tags=integration -timeout="$TEST_TIMEOUT" ./pkg/cdk/integration -v
    
    # Destroy stack
    log_info "Destroying test stack..."
    cd test-artifacts
    cdk destroy --force
}

run_unit_tests() {
    log_info "Running unit tests first..."
    go test ./pkg/cdk/constructs -v
    go test ./pkg/cdk/patterns -v
    go test ./pkg/cdk/stacks -v
}

# Main execution
main() {
    log_info "Starting Lift CDK integration tests"
    log_info "Region: $REGION"
    log_info "Stack name: $STACK_NAME"
    
    # Trap to ensure cleanup
    trap cleanup EXIT
    
    # Check prerequisites
    check_prerequisites
    
    # Get AWS account ID
    AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
    log_info "AWS Account: $AWS_ACCOUNT_ID"
    
    # Run unit tests first
    if [ "${SKIP_UNIT_TESTS}" != "true" ]; then
        run_unit_tests
    fi
    
    # Create and run integration tests
    if [ "${SKIP_INTEGRATION_TESTS}" != "true" ]; then
        create_test_app
        run_integration_tests
    fi
    
    log_info "All tests completed successfully!"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --region)
            REGION="$2"
            shift 2
            ;;
        --skip-unit-tests)
            SKIP_UNIT_TESTS="true"
            shift
            ;;
        --skip-integration-tests)
            SKIP_INTEGRATION_TESTS="true"
            shift
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  --region <region>          AWS region (default: us-east-1)"
            echo "  --skip-unit-tests          Skip unit tests"
            echo "  --skip-integration-tests   Skip integration tests"
            echo "  --help                     Show this help message"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Run main
main