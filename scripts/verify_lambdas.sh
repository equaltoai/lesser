#!/bin/bash

echo "=== Lambda Functions Verification ==="
echo

# Find all Lambda functions (directories with main.go)
echo "1. Lambda functions in cmd/ directory:"
echo "-----------------------------------"
lambda_dirs=""
for dir in cmd/*/; do
    if [ -f "${dir}main.go" ] && grep -q "func main()" "${dir}main.go" 2>/dev/null; then
        lambda_name=$(basename "$dir")
        lambda_dirs="$lambda_dirs$lambda_name\n"
        echo "✓ $lambda_name"
    fi
done
echo

# Check which are included in build-lambdas target by looking for build commands
echo "2. Lambda functions in Makefile build-lambdas target:"
echo "---------------------------------------------------"
# Look for lines that build Lambda functions in the build-lambdas target
build_lambdas_section=$(sed -n '/^build-lambdas:/,/^[^ \t]/p' Makefile)
lambdas_in_makefile=""
echo "$lambda_dirs" | while read -r lambda; do
    if [ -n "$lambda" ] && [ "$lambda" != "configure-instance" ]; then
        if echo "$build_lambdas_section" | grep -q "./cmd/$lambda"; then
            echo "✓ $lambda"
            lambdas_in_makefile="$lambdas_in_makefile$lambda\n"
        fi
    fi
done
echo

# Find Lambda functions NOT in Makefile
echo "3. Lambda functions MISSING from Makefile build-lambdas:"
echo "------------------------------------------------------"
missing_count=0
echo "$lambda_dirs" | while read -r lambda; do
    if [ -n "$lambda" ] && [ "$lambda" != "configure-instance" ]; then
        if ! sed -n '/^build-lambdas:/,/^[^ \t]/p' Makefile | grep -q "./cmd/$lambda"; then
            echo "✗ $lambda (needs to be added to build-lambdas target)"
            ((missing_count++))
        fi
    fi
done
echo

# Check which are defined in Pulumi infrastructure
echo "4. Lambda functions defined in infra/main.go:"
echo "-------------------------------------------"
if [ -f "infra/main.go" ]; then
    # Look for Lambda function definitions
    grep -E "lambda\.NewFunction|createLambdaFunction" infra/main.go | sed -E 's/.*"([^"]+)".*/\1/' | sort | uniq | while read -r lambda; do
        echo "✓ $lambda"
    done
else
    echo "infra/main.go not found!"
fi
echo

# Check build artifacts
echo "5. Built Lambda artifacts in bin/:"
echo "---------------------------------"
if [ -d "bin" ]; then
    for zip in bin/*.zip; do
        if [ -f "$zip" ]; then
            lambda_name=$(basename "$zip" .zip)
            echo "✓ $lambda_name.zip"
        fi
    done
else
    echo "bin/ directory not found - run 'make build-lambdas' first"
fi

echo
echo "=== Summary ==="
echo "To ensure all Lambda functions are deployed:"
echo "1. Add any missing functions to the Makefile build-lambdas target"
echo "2. Add Lambda function definitions to infra/main.go"
echo "3. Run 'make deploy' to build and deploy all functions" 