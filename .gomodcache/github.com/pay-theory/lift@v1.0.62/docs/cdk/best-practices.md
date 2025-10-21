# Lift CDK Best Practices Guide

This guide covers best practices for using AWS CDK with Lift applications to ensure reliable, maintainable, and cost-effective deployments.

## Table of Contents
- [Project Structure](#project-structure)
- [Stack Design](#stack-design)
- [Environment Management](#environment-management)
- [Security Best Practices](#security-best-practices)
- [Performance Optimization](#performance-optimization)
- [Cost Optimization](#cost-optimization)
- [Testing](#testing)
- [CI/CD Integration](#cicd-integration)
- [Monitoring and Observability](#monitoring-and-observability)
- [Common Patterns](#common-patterns)

## Project Structure

### Recommended Directory Layout
```
my-lift-app/
├── cmd/
│   └── main.go              # Lambda handler entry point
├── pkg/
│   └── handlers/            # Business logic
├── cdk/
│   ├── main.go             # CDK app entry point
│   ├── cdk.json            # CDK configuration
│   └── stacks/             # Custom stack definitions
├── dist/                   # Build output (gitignored)
├── Makefile               # Build and deploy commands
└── README.md
```

### DO: Separate Infrastructure from Application Code
```go
// cdk/main.go - Keep CDK code separate
import "github.com/pay-theory/lift/pkg/cdk/patterns"

func main() {
    app := awscdk.NewApp(nil)
    patterns.NewLiftApp(app, jsii.String("MyApp"), &props)
    app.Synth(nil)
}
```

### DON'T: Mix CDK and Application Logic
```go
// main.go - Avoid mixing concerns
func handler() {
    // Application logic
    stack := awscdk.NewStack(...) // ❌ Don't create stacks in handlers
}
```

## Stack Design

### DO: Use Lift's Pre-built Constructs
```go
// ✅ Leverage tested, optimized constructs
app := patterns.NewLiftApp(stack, jsii.String("MyApp"), &patterns.LiftAppProps{
    AppName:           jsii.String("my-app"),
    CodeAssetPath:     jsii.String("../dist"),
    EnableDatabase:    jsii.Bool(true),
    EnableRateLimiting: jsii.Bool(true),
})
```

### DON'T: Recreate Standard Infrastructure
```go
// ❌ Avoid reimplementing what Lift provides
lambda := awslambda.NewFunction(...)
api := awsapigateway.NewRestApi(...)
table := awsdynamodb.NewTable(...)
// Missing optimizations, configurations, and integrations
```

### DO: One Stack Per Environment
```go
// ✅ Separate stacks for different environments
func main() {
    app := awscdk.NewApp(nil)
    
    // Development stack
    NewAppStack(app, "MyApp-Dev", &AppStackProps{
        Environment: "development",
        MemorySize:  512,
    })
    
    // Production stack
    NewAppStack(app, "MyApp-Prod", &AppStackProps{
        Environment: "production",
        MemorySize:  2048,
        EnableAlarms: true,
    })
}
```

### DO: Use Stack Outputs for Cross-Stack References
```go
// ✅ Export important values
awscdk.NewCfnOutput(stack, jsii.String("ApiEndpoint"), &awscdk.CfnOutputProps{
    Value:       api.ApiEndpoint(),
    ExportName:  jsii.String("MyApp-ApiEndpoint"),
    Description: jsii.String("API Gateway endpoint URL"),
})
```

## Environment Management

### DO: Use Environment Variables for Configuration
```go
// ✅ Make stacks configurable
appName := os.Getenv("APP_NAME")
if appName == "" {
    appName = "my-default-app"
}

props := &patterns.LiftAppProps{
    AppName: jsii.String(appName),
    // ... other config from environment
}
```

### DO: Specify AWS Account and Region
```go
// ✅ Be explicit about deployment targets
func env() *awscdk.Environment {
    account := os.Getenv("CDK_DEFAULT_ACCOUNT")
    region := os.Getenv("CDK_DEFAULT_REGION")
    
    if account == "" || region == "" {
        return nil // Use CLI defaults
    }
    
    return &awscdk.Environment{
        Account: jsii.String(account),
        Region:  jsii.String(region),
    }
}
```

### DON'T: Hardcode Sensitive Values
```go
// ❌ Never hardcode secrets
props := &LiftAppProps{
    DatabasePassword: jsii.String("mypassword123"), // ❌ Security risk
}

// ✅ Use Secrets Manager or Parameter Store
secret := awssecretsmanager.NewSecret(stack, jsii.String("DBPassword"), nil)
props.DatabasePassword = secret.SecretValue()
```

## Security Best Practices

### DO: Enable Encryption Everywhere
```go
// ✅ DynamoDB encryption
table := constructs.NewLiftTable(stack, jsii.String("Table"), &constructs.LiftTableProps{
    EnablePointInTimeRecovery: jsii.Bool(true),
    // Encryption at rest is enabled by default
})

// ✅ S3 encryption
bucket := awss3.NewBucket(stack, jsii.String("Bucket"), &awss3.BucketProps{
    Encryption: awss3.BucketEncryption_S3_MANAGED,
    BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
})
```

### DO: Use Least Privilege IAM
```go
// ✅ Grant only necessary permissions
table.GrantReadWriteData(function)  // Specific permissions

// ❌ Avoid overly broad permissions
function.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
    Actions:   &[]*string{jsii.String("dynamodb:*")}, // Too broad
    Resources: &[]*string{jsii.String("*")},          // Too broad
}))
```

### DO: Enable Tracing and Logging
```go
// ✅ Enable observability features
props := &constructs.LiftFunctionProps{
    EnableTracing: jsii.Bool(true),
    FunctionProps: awslambda.FunctionProps{
        LogRetention: awslogs.RetentionDays_ONE_WEEK,
        TracingConfig: awslambda.Tracing_ACTIVE,
    },
}
```

## Performance Optimization

### DO: Use ARM64 Architecture
```go
// ✅ ARM64 for better price/performance
// Lift constructs default to ARM64
function := constructs.NewLiftFunction(stack, id, &props)
```

### DO: Right-size Lambda Memory
```go
// ✅ Start with reasonable defaults, then optimize based on metrics
props := &patterns.LiftAppProps{
    MemorySize: jsii.Number(512),  // Development
    // MemorySize: jsii.Number(1024), // Production after profiling
}
```

### DO: Enable Auto-scaling for DynamoDB
```go
// ✅ Handle traffic spikes gracefully
table := constructs.NewLiftTable(stack, id, &constructs.LiftTableProps{
    EnableAutoScaling: jsii.Bool(true),
    ReadCapacity:      jsii.Number(5),   // Minimum
    WriteCapacity:     jsii.Number(5),   // Minimum
})
```

## Cost Optimization

### DO: Use On-Demand Billing for Variable Workloads
```go
// ✅ Default to on-demand for unpredictable traffic
table := constructs.NewLiftTable(stack, id, &constructs.LiftTableProps{
    // Defaults to PAY_PER_REQUEST billing mode
})
```

### DO: Set Appropriate Log Retention
```go
// ✅ Don't keep logs forever
awslogs.NewLogGroup(stack, jsii.String("Logs"), &awslogs.LogGroupProps{
    Retention: awslogs.RetentionDays_ONE_WEEK,  // Development
    // Retention: awslogs.RetentionDays_ONE_MONTH, // Production
})
```

### DO: Use Tags for Cost Tracking
```go
// ✅ Tag all resources
awscdk.Tags_Of(stack).Add(jsii.String("Project"), jsii.String("MyApp"), nil)
awscdk.Tags_Of(stack).Add(jsii.String("Environment"), jsii.String("Production"), nil)
awscdk.Tags_Of(stack).Add(jsii.String("Owner"), jsii.String("TeamName"), nil)
```

## Testing

### DO: Write CDK Unit Tests
```go
// ✅ Test your infrastructure
func TestProductionStack(t *testing.T) {
    tester := test.NewLiftStackTester(t)
    
    NewProductionStack(tester.Stack(), "TestStack", &props)
    
    tester.Synthesize()
    tester.AssertLiftFunction(map[string]interface{}{
        "MemorySize": 2048,
        "Timeout":    300,
    })
    tester.AssertCompleteInfrastructure("my-app", true, true)
}
```

### DO: Use CDK Assertions
```go
// ✅ Verify critical configurations
template := assertions.Template_FromStack(stack, nil)
template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
    "Runtime": "provided.al2023",
    "Architectures": []string{"arm64"},
})
```

### DO: Test Different Configurations
```go
// ✅ Test edge cases
testCases := []struct {
    name  string
    props LiftAppProps
}{
    {"minimal", LiftAppProps{AppName: jsii.String("test")}},
    {"with-database", LiftAppProps{EnableDatabase: jsii.Bool(true)}},
    {"multi-tenant", LiftAppProps{EnableMultiTenant: jsii.Bool(true)}},
}
```

## CI/CD Integration

### DO: Automate CDK Deployments
```yaml
# ✅ GitHub Actions example
- name: Deploy to AWS
  run: |
    npm install -g aws-cdk
    make build
    cd cdk && cdk deploy --require-approval never
```

### DO: Use CDK Diff in Pull Requests
```yaml
# ✅ Show infrastructure changes in PRs
- name: CDK Diff
  run: |
    cd cdk && cdk diff > diff.txt
    cat diff.txt >> $GITHUB_STEP_SUMMARY
```

### DO: Separate Build and Deploy Stages
```makefile
# ✅ Clear separation of concerns
build:
    GOOS=linux GOARCH=arm64 go build -o dist/bootstrap cmd/main.go

synth: build
    cd cdk && cdk synth

deploy: synth
    cd cdk && cdk deploy --require-approval never
```

## Monitoring and Observability

### DO: Create CloudWatch Dashboards
```go
// ✅ Monitor key metrics
dashboard := awscloudwatch.NewDashboard(stack, jsii.String("Dashboard"), &awscloudwatch.DashboardProps{
    DashboardName: jsii.String("my-app-dashboard"),
})

dashboard.AddWidgets(
    awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
        Title: jsii.String("Lambda Invocations"),
        Left: &[]awscloudwatch.IMetric{
            function.MetricInvocations(),
            function.MetricErrors(),
        },
    }),
)
```

### DO: Set Up Alarms
```go
// ✅ Alert on errors
alarm := awscloudwatch.NewAlarm(stack, jsii.String("ErrorAlarm"), &awscloudwatch.AlarmProps{
    Metric:            function.MetricErrors(),
    Threshold:         jsii.Number(10),
    EvaluationPeriods: jsii.Number(2),
})
```

## Common Patterns

### Pattern: Multi-Environment Deployment
```go
type EnvironmentConfig struct {
    Name        string
    MemorySize  float64
    AutoScaling bool
    Alarms      bool
}

configs := map[string]EnvironmentConfig{
    "dev":  {Name: "development", MemorySize: 512, AutoScaling: false},
    "prod": {Name: "production", MemorySize: 2048, AutoScaling: true, Alarms: true},
}

env := os.Getenv("DEPLOY_ENV")
config := configs[env]
```

### Pattern: Feature Flags
```go
props := &patterns.LiftAppProps{
    AppName:            jsii.String("my-app"),
    EnableDatabase:     jsii.Bool(os.Getenv("ENABLE_DATABASE") == "true"),
    EnableRateLimiting: jsii.Bool(os.Getenv("ENABLE_RATE_LIMIT") == "true"),
}
```

### Pattern: Custom Domain with Certificate
```go
hostedZone := awsroute53.HostedZone_FromLookup(stack, jsii.String("Zone"), &awsroute53.HostedZoneProviderProps{
    DomainName: jsii.String("example.com"),
})

certificate := awscertificatemanager.NewCertificate(stack, jsii.String("Cert"), &awscertificatemanager.CertificateProps{
    DomainName: jsii.String("api.example.com"),
    Validation: awscertificatemanager.CertificateValidation_FromDns(hostedZone),
})

props := &patterns.LiftAppProps{
    DomainName:     jsii.String("api.example.com"),
    CertificateArn: certificate.CertificateArn(),
}
```

## Summary

### Key Takeaways
1. **Use Lift's constructs** - They're optimized and tested
2. **Separate concerns** - Infrastructure code != application code
3. **Environment-specific stacks** - Different configs for dev/prod
4. **Security first** - Encryption, least privilege, monitoring
5. **Cost aware** - Right-size resources, set retention policies
6. **Test everything** - Unit test your infrastructure
7. **Automate deployment** - CI/CD integration is crucial

### Anti-patterns to Avoid
- ❌ Hardcoding sensitive values
- ❌ Using default VPC for production
- ❌ Ignoring cost optimization
- ❌ Skipping infrastructure tests
- ❌ Manual deployments
- ❌ Over-provisioning resources
- ❌ Missing monitoring/alarms

### Next Steps
1. Start with `lift cdk-init` command
2. Use pre-built stacks from `pkg/cdk/stacks`
3. Customize based on your needs
4. Add tests for your infrastructure
5. Set up CI/CD pipeline
6. Monitor and optimize based on metrics