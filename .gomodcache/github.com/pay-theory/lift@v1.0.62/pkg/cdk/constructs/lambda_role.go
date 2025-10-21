package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// LiftLambdaRoleProps defines properties for Lambda execution roles
//
//nolint:govet // Field order groups related permissions for ease of use.
type LiftLambdaRoleProps struct {
	// Basic configuration
	RoleName    *string
	Description *string

	// Service principal (defaults to lambda.amazonaws.com)
	ServicePrincipal *string

	// Managed policies
	ManagedPolicyArns []string

	// Enable common AWS managed policies
	EnableBasicExecution     *bool // AWSLambdaBasicExecutionRole
	EnableVPCExecution       *bool // AWSLambdaVPCAccessExecutionRole
	EnableCloudWatchInsights *bool // CloudWatchLambdaInsightsExecutionRolePolicy
	EnableXRayDaemonWrite    *bool // AWSXRayDaemonWriteAccess

	// DynamoDB access
	DynamoDBTables       []awsdynamodb.ITable
	DynamoDBTableArns    []string
	DynamoDBStreamAccess *bool // Grant stream read access
	DynamoDBFullAccess   *bool // Grant full access vs read/write

	// KMS access
	KMSKeys              []awskms.IKey
	KMSKeyArns           []string
	EnableMultiRegionKMS *bool    // Grant access to multi-region keys (mrk-*)
	KMSActions           []string // Custom KMS actions (defaults to Encrypt, Decrypt, GenerateDataKey)

	// Secrets Manager access
	SecretsManagerArns  []string
	EnableSecretsAccess *bool // Grant access to all secrets (not recommended for production)

	// SSM Parameter Store access
	SSMParameterPaths []string
	EnableSSMAccess   *bool // Grant access to all parameters

	// Payment Cryptography (AWS Payment Cryptography Service)
	EnablePaymentCrypto  *bool
	PaymentCryptoActions []string // Defaults to DecryptData, EncryptData, GetAlias

	// SQS access
	SQSQueueArns           []string
	EnableSQSSendMessage   *bool
	EnableSQSReceiveDelete *bool

	// S3 access
	S3BucketArns  []string
	EnableS3Read  *bool
	EnableS3Write *bool

	// Custom inline policies
	InlinePolicies map[string]awsiam.PolicyDocument

	// Additional policy statements
	AdditionalPolicyStatements []awsiam.PolicyStatement

	// Tags
	Tags map[string]string
}

// LiftLambdaRole is a Lambda execution role construct with common permissions
type LiftLambdaRole struct {
	constructs.Construct
	Role awsiam.Role
}

// NewLiftLambdaRole creates a new Lambda execution role with common permissions
func NewLiftLambdaRole(scope constructs.Construct, id *string, props *LiftLambdaRoleProps) *LiftLambdaRole {
	builder := newLiftLambdaRoleBuilder(scope, id, props)
	return builder.build()
}

// liftLambdaRoleBuilder builds Lambda execution roles with best practices
type liftLambdaRoleBuilder struct {
	scope     constructs.Construct
	id        *string
	props     *LiftLambdaRoleProps
	construct constructs.Construct
}

// newLiftLambdaRoleBuilder creates a new role builder
func newLiftLambdaRoleBuilder(scope constructs.Construct, id *string, props *LiftLambdaRoleProps) *liftLambdaRoleBuilder {
	return &liftLambdaRoleBuilder{
		scope: scope,
		id:    id,
		props: props,
	}
}

// build constructs the complete Lambda role
func (b *liftLambdaRoleBuilder) build() *LiftLambdaRole {
	b.construct = constructs.NewConstruct(b.scope, b.id)
	b.setDefaults()

	role := b.createRole()
	b.attachManagedPolicies(role)
	b.attachInlinePolicies(role)
	b.grantDynamoDBAccess(role)
	b.grantKMSAccess(role)
	b.grantSecretsAccess(role)
	b.grantSSMAccess(role)
	b.grantPaymentCryptoAccess(role)
	b.grantSQSAccess(role)
	b.grantS3Access(role)
	b.addCustomPolicies(role)
	b.applyTags(role)

	return &LiftLambdaRole{
		Construct: b.construct,
		Role:      role,
	}
}

// setDefaults sets default values for optional properties
func (b *liftLambdaRoleBuilder) setDefaults() {
	if b.props.ServicePrincipal == nil {
		b.props.ServicePrincipal = jsii.String("lambda.amazonaws.com")
	}
	if b.props.EnableBasicExecution == nil {
		b.props.EnableBasicExecution = jsii.Bool(true)
	}
	if b.props.Description == nil {
		b.props.Description = jsii.String("Lambda execution role created by Lift")
	}

	// Default KMS actions
	if len(b.props.KMSActions) == 0 && (len(b.props.KMSKeys) > 0 || len(b.props.KMSKeyArns) > 0) {
		b.props.KMSActions = []string{
			"kms:Decrypt",
			"kms:Encrypt",
			"kms:GenerateDataKey",
			"kms:DescribeKey",
		}
	}

	// Default Payment Cryptography actions
	if len(b.props.PaymentCryptoActions) == 0 && b.props.EnablePaymentCrypto != nil && *b.props.EnablePaymentCrypto {
		b.props.PaymentCryptoActions = []string{
			"payment-cryptography:DecryptData",
			"payment-cryptography:EncryptData",
			"payment-cryptography:GetAlias",
		}
	}
}

// createRole creates the base IAM role
func (b *liftLambdaRoleBuilder) createRole() awsiam.Role {
	roleProps := &awsiam.RoleProps{
		AssumedBy:   awsiam.NewServicePrincipal(b.props.ServicePrincipal, nil),
		Description: b.props.Description,
	}

	if b.props.RoleName != nil {
		roleProps.RoleName = b.props.RoleName
	}

	return awsiam.NewRole(b.construct, jsii.String("Role"), roleProps)
}

// attachManagedPolicies attaches AWS managed and custom managed policies
func (b *liftLambdaRoleBuilder) attachManagedPolicies(role awsiam.Role) {
	// Basic Lambda execution
	if b.props.EnableBasicExecution != nil && *b.props.EnableBasicExecution {
		role.AddManagedPolicy(
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/AWSLambdaBasicExecutionRole")),
		)
	}

	// VPC execution
	if b.props.EnableVPCExecution != nil && *b.props.EnableVPCExecution {
		role.AddManagedPolicy(
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/AWSLambdaVPCAccessExecutionRole")),
		)
	}

	// CloudWatch Insights
	if b.props.EnableCloudWatchInsights != nil && *b.props.EnableCloudWatchInsights {
		role.AddManagedPolicy(
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("CloudWatchLambdaInsightsExecutionRolePolicy")),
		)
	}

	// X-Ray
	if b.props.EnableXRayDaemonWrite != nil && *b.props.EnableXRayDaemonWrite {
		role.AddManagedPolicy(
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("AWSXRayDaemonWriteAccess")),
		)
	}

	// Custom managed policies by ARN
	for _, policyArn := range b.props.ManagedPolicyArns {
		role.AddManagedPolicy(
			awsiam.ManagedPolicy_FromManagedPolicyArn(b.construct, jsii.String("Policy-"+policyArn), jsii.String(policyArn)),
		)
	}
}

// attachInlinePolicies attaches custom inline policies
func (b *liftLambdaRoleBuilder) attachInlinePolicies(role awsiam.Role) {
	if len(b.props.InlinePolicies) == 0 {
		return
	}

	for name, policy := range b.props.InlinePolicies {
		role.AttachInlinePolicy(awsiam.NewPolicy(b.construct, jsii.String("InlinePolicy-"+name), &awsiam.PolicyProps{
			PolicyName: jsii.String(name),
			Document:   policy,
		}))
	}
}

// grantDynamoDBAccess grants DynamoDB table access
func (b *liftLambdaRoleBuilder) grantDynamoDBAccess(role awsiam.Role) {
	if len(b.props.DynamoDBTables) == 0 && len(b.props.DynamoDBTableArns) == 0 {
		return
	}

	resources := []*string{}

	// Add tables and their indexes
	for _, table := range b.props.DynamoDBTables {
		resources = append(resources, table.TableArn())
		resources = append(resources, jsii.String(*table.TableArn()+"/index/*"))

		// Add stream access if enabled
		if b.props.DynamoDBStreamAccess != nil && *b.props.DynamoDBStreamAccess {
			if table.TableStreamArn() != nil {
				resources = append(resources, jsii.String(*table.TableArn()+"/stream/*"))
			}
		}
	}

	// Add table ARNs directly
	for _, arn := range b.props.DynamoDBTableArns {
		resources = append(resources, jsii.String(arn))
		resources = append(resources, jsii.String(arn+"/index/*"))

		if b.props.DynamoDBStreamAccess != nil && *b.props.DynamoDBStreamAccess {
			resources = append(resources, jsii.String(arn+"/stream/*"))
		}
	}

	// Determine actions
	actions := &[]*string{
		jsii.String("dynamodb:DescribeTable"),
		jsii.String("dynamodb:Query"),
		jsii.String("dynamodb:Scan"),
		jsii.String("dynamodb:GetItem"),
		jsii.String("dynamodb:PutItem"),
		jsii.String("dynamodb:UpdateItem"),
		jsii.String("dynamodb:DeleteItem"),
		jsii.String("dynamodb:BatchGetItem"),
		jsii.String("dynamodb:BatchWriteItem"),
	}

	if b.props.DynamoDBFullAccess != nil && *b.props.DynamoDBFullAccess {
		*actions = append(*actions,
			jsii.String("dynamodb:CreateTable"),
			jsii.String("dynamodb:DeleteTable"),
			jsii.String("dynamodb:UpdateTable"),
		)
	}

	if b.props.DynamoDBStreamAccess != nil && *b.props.DynamoDBStreamAccess {
		*actions = append(*actions,
			jsii.String("dynamodb:DescribeStream"),
			jsii.String("dynamodb:GetRecords"),
			jsii.String("dynamodb:GetShardIterator"),
			jsii.String("dynamodb:ListStreams"),
		)
	}

	role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect:    awsiam.Effect_ALLOW,
		Actions:   actions,
		Resources: &resources,
	}))
}

// grantKMSAccess grants KMS key access
func (b *liftLambdaRoleBuilder) grantKMSAccess(role awsiam.Role) {
	if len(b.props.KMSKeys) == 0 && len(b.props.KMSKeyArns) == 0 && (b.props.EnableMultiRegionKMS == nil || !*b.props.EnableMultiRegionKMS) {
		return
	}

	resources := []*string{}

	// Add KMS keys
	for _, key := range b.props.KMSKeys {
		resources = append(resources, key.KeyArn())
	}

	// Add KMS ARNs directly
	for _, arn := range b.props.KMSKeyArns {
		resources = append(resources, jsii.String(arn))
	}

	// Add multi-region KMS support (mrk-* keys)
	if b.props.EnableMultiRegionKMS != nil && *b.props.EnableMultiRegionKMS {
		stack := awscdk.Stack_Of(b.construct)
		resources = append(resources, jsii.String(fmt.Sprintf("arn:aws:kms:*:%s:key/mrk-*", *stack.Account())))
	}

	// Convert action strings to jsii pointers
	actions := make([]*string, len(b.props.KMSActions))
	for i, action := range b.props.KMSActions {
		actions[i] = jsii.String(action)
	}

	// Add GenerateMac and VerifyMac for HMAC keys if multi-region is enabled
	if b.props.EnableMultiRegionKMS != nil && *b.props.EnableMultiRegionKMS {
		actions = append(actions,
			jsii.String("kms:GenerateMac"),
			jsii.String("kms:VerifyMac"),
		)
	}

	role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect:    awsiam.Effect_ALLOW,
		Actions:   &actions,
		Resources: &resources,
	}))
}

// grantSecretsAccess grants Secrets Manager access
func (b *liftLambdaRoleBuilder) grantSecretsAccess(role awsiam.Role) {
	if len(b.props.SecretsManagerArns) == 0 && (b.props.EnableSecretsAccess == nil || !*b.props.EnableSecretsAccess) {
		return
	}

	resources := []*string{}

	if b.props.EnableSecretsAccess != nil && *b.props.EnableSecretsAccess {
		resources = append(resources, jsii.String("*"))
	} else {
		for _, arn := range b.props.SecretsManagerArns {
			resources = append(resources, jsii.String(arn))
		}
	}

	role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: &[]*string{
			jsii.String("secretsmanager:GetSecretValue"),
			jsii.String("secretsmanager:DescribeSecret"),
		},
		Resources: &resources,
	}))
}

// grantSSMAccess grants SSM Parameter Store access
func (b *liftLambdaRoleBuilder) grantSSMAccess(role awsiam.Role) {
	if len(b.props.SSMParameterPaths) == 0 && (b.props.EnableSSMAccess == nil || !*b.props.EnableSSMAccess) {
		return
	}

	resources := []*string{}

	if b.props.EnableSSMAccess != nil && *b.props.EnableSSMAccess {
		resources = append(resources, jsii.String("*"))
	} else {
		stack := awscdk.Stack_Of(b.construct)
		for _, path := range b.props.SSMParameterPaths {
			resources = append(resources, jsii.String(fmt.Sprintf("arn:aws:ssm:%s:%s:parameter%s", *stack.Region(), *stack.Account(), path)))
		}
	}

	role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: &[]*string{
			jsii.String("ssm:GetParameter"),
			jsii.String("ssm:GetParameters"),
			jsii.String("ssm:GetParametersByPath"),
		},
		Resources: &resources,
	}))
}

// grantPaymentCryptoAccess grants AWS Payment Cryptography access
func (b *liftLambdaRoleBuilder) grantPaymentCryptoAccess(role awsiam.Role) {
	if b.props.EnablePaymentCrypto == nil || !*b.props.EnablePaymentCrypto {
		return
	}

	actions := make([]*string, len(b.props.PaymentCryptoActions))
	for i, action := range b.props.PaymentCryptoActions {
		actions[i] = jsii.String(action)
	}

	role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect:    awsiam.Effect_ALLOW,
		Actions:   &actions,
		Resources: &[]*string{jsii.String("*")}, // Payment Cryptography requires wildcard
	}))
}

// grantSQSAccess grants SQS queue access
func (b *liftLambdaRoleBuilder) grantSQSAccess(role awsiam.Role) {
	if len(b.props.SQSQueueArns) == 0 && (b.props.EnableSQSSendMessage == nil || !*b.props.EnableSQSSendMessage) && (b.props.EnableSQSReceiveDelete == nil || !*b.props.EnableSQSReceiveDelete) {
		return
	}

	resources := make([]*string, len(b.props.SQSQueueArns))
	for i, arn := range b.props.SQSQueueArns {
		resources[i] = jsii.String(arn)
	}

	actions := []*string{}

	if b.props.EnableSQSSendMessage != nil && *b.props.EnableSQSSendMessage {
		actions = append(actions, jsii.String("sqs:SendMessage"))
	}

	if b.props.EnableSQSReceiveDelete != nil && *b.props.EnableSQSReceiveDelete {
		actions = append(actions,
			jsii.String("sqs:ReceiveMessage"),
			jsii.String("sqs:DeleteMessage"),
			jsii.String("sqs:GetQueueAttributes"),
		)
	}

	if len(actions) > 0 {
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect:    awsiam.Effect_ALLOW,
			Actions:   &actions,
			Resources: &resources,
		}))
	}
}

// grantS3Access grants S3 bucket access
func (b *liftLambdaRoleBuilder) grantS3Access(role awsiam.Role) {
	if len(b.props.S3BucketArns) == 0 {
		return
	}

	resources := []*string{}
	for _, arn := range b.props.S3BucketArns {
		resources = append(resources, jsii.String(arn))
		resources = append(resources, jsii.String(arn+"/*"))
	}

	actions := []*string{}

	if b.props.EnableS3Read != nil && *b.props.EnableS3Read {
		actions = append(actions,
			jsii.String("s3:GetObject"),
			jsii.String("s3:ListBucket"),
		)
	}

	if b.props.EnableS3Write != nil && *b.props.EnableS3Write {
		actions = append(actions,
			jsii.String("s3:PutObject"),
			jsii.String("s3:DeleteObject"),
		)
	}

	if len(actions) > 0 {
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect:    awsiam.Effect_ALLOW,
			Actions:   &actions,
			Resources: &resources,
		}))
	}
}

// addCustomPolicies adds additional custom policy statements
func (b *liftLambdaRoleBuilder) addCustomPolicies(role awsiam.Role) {
	for _, statement := range b.props.AdditionalPolicyStatements {
		role.AddToPolicy(statement)
	}
}

// applyTags applies tags to the role
func (b *liftLambdaRoleBuilder) applyTags(role awsiam.Role) {
	// Add standard Lift tags
	awscdk.Tags_Of(role).Add(jsii.String("Framework"), jsii.String("Lift"), nil)
	awscdk.Tags_Of(role).Add(jsii.String("Component"), jsii.String("LambdaRole"), nil)

	// Add custom tags
	for key, value := range b.props.Tags {
		awscdk.Tags_Of(role).Add(jsii.String(key), jsii.String(value), nil)
	}
}

// GetRole returns the underlying IAM role
func (l *LiftLambdaRole) GetRole() awsiam.IRole {
	return l.Role
}

// GetRoleArn returns the role ARN
func (l *LiftLambdaRole) GetRoleArn() *string {
	return l.Role.RoleArn()
}

// GetRoleName returns the role name
func (l *LiftLambdaRole) GetRoleName() *string {
	return l.Role.RoleName()
}

// GrantPassRole grants permission to pass this role to a service
func (l *LiftLambdaRole) GrantPassRole(grantee awsiam.IGrantable) awsiam.Grant {
	return awsiam.Grant_AddToPrincipal(&awsiam.GrantOnPrincipalOptions{
		Grantee:      grantee,
		Actions:      &[]*string{jsii.String("iam:PassRole")},
		ResourceArns: &[]*string{l.Role.RoleArn()},
	})
}

// AddToPolicy adds a policy statement to the role
func (l *LiftLambdaRole) AddToPolicy(statement awsiam.PolicyStatement) {
	l.Role.AddToPolicy(statement)
}

// AddManagedPolicy adds a managed policy to the role
func (l *LiftLambdaRole) AddManagedPolicy(policy awsiam.IManagedPolicy) {
	l.Role.AddManagedPolicy(policy)
}

// GrantDynamoDBAccess grants access to additional DynamoDB tables
func (l *LiftLambdaRole) GrantDynamoDBAccess(tables ...awsdynamodb.ITable) {
	for _, table := range tables {
		table.GrantReadWriteData(l.Role)
	}
}

// GrantKMSAccess grants access to additional KMS keys
func (l *LiftLambdaRole) GrantKMSAccess(keys ...awskms.IKey) {
	for _, key := range keys {
		key.GrantEncryptDecrypt(l.Role)
	}
}

// AsLambdaExecutionRole returns this role for use in Lambda function props
func (l *LiftLambdaRole) AsLambdaExecutionRole() awsiam.IRole {
	return l.Role
}
