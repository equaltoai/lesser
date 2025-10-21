package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// LiftKMSKeyProps defines properties for creating a KMS key with Lift
//
//nolint:govet // Field order mirrors documentation sections for readability.
type LiftKMSKeyProps struct {
	// Key configuration
	Description *string

	// Alias configuration
	AliasName *string

	// Replica configuration
	PrimaryKeyArn    *string     // ARN of the primary key for replicas
	AdministratorArn *string     // Optional admin principal ARN
	CustomKeyPolicy  interface{} // Optional custom key policy
	SSMParameterPath *string     // Parameter Store path to store key ARN
	Tags             *map[string]*string
	EnabledRegions   *[]*string

	// Boolean flags
	MultiRegion        *bool
	IsReplicaKey       *bool
	EnableKeyRotation  *bool
	EnableSSMParameter *bool

	// Additional permissions
	GrantEncryptDecrypt []awsiam.IGrantable
	GrantGenerateMac    []awsiam.IGrantable

	// Non-pointer configuration
	KeySpec       awskms.KeySpec
	KeyUsage      awskms.KeyUsage
	PendingWindow awscdk.Duration
	RemovalPolicy awscdk.RemovalPolicy
}

// LiftKMSKey represents a KMS key with multi-region support
type LiftKMSKey struct {
	constructs.Construct

	// The KMS key (either direct key or replica)
	Key awskms.IKey

	// Alias for the key
	Alias awskms.Alias

	// SSM Parameter (if enabled)
	SSMParameter awsssm.StringParameter

	// Key ARN
	KeyArn *string

	// Key ID
	KeyId *string
}

// NewLiftKMSKey creates a new KMS key with Lift-optimized defaults
func NewLiftKMSKey(scope constructs.Construct, id *string, props *LiftKMSKeyProps) *LiftKMSKey {
	builder := newLiftKMSKeyBuilder(scope, id, props)
	return builder.build()
}

// liftKMSKeyBuilder builds KMS keys with multi-region support
type liftKMSKeyBuilder struct {
	scope constructs.Construct
	id    *string
	props *LiftKMSKeyProps
}

// newLiftKMSKeyBuilder creates a new KMS key builder
func newLiftKMSKeyBuilder(scope constructs.Construct, id *string, props *LiftKMSKeyProps) *liftKMSKeyBuilder {
	return &liftKMSKeyBuilder{
		scope: scope,
		id:    id,
		props: props,
	}
}

// build constructs the complete LiftKMSKey
func (b *liftKMSKeyBuilder) build() *LiftKMSKey {
	b.setDefaults()

	liftKey := &LiftKMSKey{}
	constructs.NewConstruct_Override(liftKey, b.scope, b.id)

	// Create key based on type (primary or replica)
	if b.isReplicaKey() {
		b.createReplicaKey(liftKey)
	} else {
		b.createPrimaryKey(liftKey)
	}

	// Create alias
	b.createAlias(liftKey)

	// Configure permissions
	b.configurePermissions(liftKey)

	// Store in SSM if enabled
	b.storeInSSM(liftKey)

	// Apply tags
	b.applyTags(liftKey)

	return liftKey
}

// setDefaults sets default values for optional properties
func (b *liftKMSKeyBuilder) setDefaults() {
	if b.props.MultiRegion == nil {
		b.props.MultiRegion = jsii.Bool(false)
	}
	if b.props.IsReplicaKey == nil {
		b.props.IsReplicaKey = jsii.Bool(false)
	}
	if b.props.EnableKeyRotation == nil {
		// Only enable rotation for SYMMETRIC keys
		if b.props.KeySpec == awskms.KeySpec_SYMMETRIC_DEFAULT {
			b.props.EnableKeyRotation = jsii.Bool(true)
		} else {
			b.props.EnableKeyRotation = jsii.Bool(false)
		}
	}
	if b.props.PendingWindow == nil {
		b.props.PendingWindow = awscdk.Duration_Days(jsii.Number(30))
	}
	if b.props.RemovalPolicy == "" {
		b.props.RemovalPolicy = awscdk.RemovalPolicy_RETAIN
	}
	if b.props.EnableSSMParameter == nil {
		b.props.EnableSSMParameter = jsii.Bool(false)
	}

	// Set default key spec if not provided
	if b.props.KeySpec == "" {
		b.props.KeySpec = awskms.KeySpec_SYMMETRIC_DEFAULT
	}

	// Set default key usage based on key spec
	if b.props.KeyUsage == "" {
		if b.props.KeySpec == awskms.KeySpec_HMAC_256 {
			b.props.KeyUsage = awskms.KeyUsage_GENERATE_VERIFY_MAC
		} else {
			b.props.KeyUsage = awskms.KeyUsage_ENCRYPT_DECRYPT
		}
	}
}

// isReplicaKey determines if this is a replica key
func (b *liftKMSKeyBuilder) isReplicaKey() bool {
	return b.props.IsReplicaKey != nil && *b.props.IsReplicaKey && b.props.PrimaryKeyArn != nil
}

// createPrimaryKey creates a primary KMS key
func (b *liftKMSKeyBuilder) createPrimaryKey(liftKey *LiftKMSKey) {
	keyProps := &awskms.KeyProps{
		Description:       b.props.Description,
		KeySpec:           b.props.KeySpec,
		KeyUsage:          b.props.KeyUsage,
		EnableKeyRotation: b.props.EnableKeyRotation,
		RemovalPolicy:     b.props.RemovalPolicy,
		PendingWindow:     b.props.PendingWindow,
	}

	// Set multi-region if enabled
	if b.props.MultiRegion != nil && *b.props.MultiRegion {
		keyProps.MultiRegion = jsii.Bool(true)
	}

	// Set custom key policy if provided
	if b.props.CustomKeyPolicy != nil {
		if policy, ok := b.props.CustomKeyPolicy.(awsiam.PolicyDocument); ok {
			keyProps.Policy = policy
		} else {
			panic("CustomKeyPolicy must implement awsiam.PolicyDocument")
		}
	}

	key := awskms.NewKey(liftKey, jsii.String("Key"), keyProps)
	liftKey.Key = key
	liftKey.KeyArn = key.KeyArn()
	liftKey.KeyId = key.KeyId()
}

// createReplicaKey creates a replica KMS key
func (b *liftKMSKeyBuilder) createReplicaKey(liftKey *LiftKMSKey) {
	// Create key policy for replica
	keyPolicy := b.createReplicaKeyPolicy()

	replicaKey := awskms.NewCfnReplicaKey(liftKey, jsii.String("ReplicaKey"), &awskms.CfnReplicaKeyProps{
		PrimaryKeyArn: b.props.PrimaryKeyArn,
		KeyPolicy:     keyPolicy,
		Description:   b.props.Description,
		Enabled:       jsii.Bool(true),
	})

	// Import the replica key as an IKey
	liftKey.Key = awskms.Key_FromKeyArn(liftKey, jsii.String("ImportedReplicaKey"), replicaKey.AttrArn())
	liftKey.KeyArn = replicaKey.AttrArn()
	liftKey.KeyId = replicaKey.AttrKeyId()
}

// createReplicaKeyPolicy creates a key policy for replica keys
func (b *liftKMSKeyBuilder) createReplicaKeyPolicy() interface{} {
	// If custom policy provided, use it
	if b.props.CustomKeyPolicy != nil {
		return b.props.CustomKeyPolicy
	}

	// Default replica key policy
	stack := awscdk.Stack_Of(b.scope)
	accountId := *stack.Account()
	region := *stack.Region()

	return map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Sid":    "Enable IAM User Permissions",
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"AWS": fmt.Sprintf("arn:aws:iam::%s:root", accountId),
				},
				"Action":   "kms:*",
				"Resource": "*",
			},
			{
				"Sid":    "Allow use of the key",
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"AWS": fmt.Sprintf("arn:aws:iam::%s:root", accountId),
				},
				"Action": []string{
					"kms:Encrypt",
					"kms:Decrypt",
					"kms:ReEncrypt*",
					"kms:GenerateDataKey*",
					"kms:CreateGrant",
					"kms:DescribeKey",
					"kms:GenerateMac",
					"kms:VerifyMac",
				},
				"Resource": "*",
			},
			{
				"Sid":    "Allow CloudWatch Logs",
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"Service": fmt.Sprintf("logs.%s.amazonaws.com", region),
				},
				"Action": []string{
					"kms:Encrypt",
					"kms:Decrypt",
					"kms:ReEncrypt*",
					"kms:GenerateDataKey*",
					"kms:CreateGrant",
					"kms:DescribeKey",
				},
				"Resource": "*",
			},
		},
	}
}

// createAlias creates an alias for the key
func (b *liftKMSKeyBuilder) createAlias(liftKey *LiftKMSKey) {
	if b.props.AliasName == nil {
		return
	}

	aliasProps := &awskms.AliasProps{
		AliasName: b.props.AliasName,
		TargetKey: liftKey.Key,
	}

	liftKey.Alias = awskms.NewAlias(liftKey, jsii.String("Alias"), aliasProps)
}

// configurePermissions grants permissions to specified principals
func (b *liftKMSKeyBuilder) configurePermissions(liftKey *LiftKMSKey) {
	// Grant encrypt/decrypt permissions
	if b.props.GrantEncryptDecrypt != nil {
		for _, grantee := range b.props.GrantEncryptDecrypt {
			liftKey.Key.GrantEncryptDecrypt(grantee)
		}
	}

	// Grant GenerateMac/VerifyMac permissions for HMAC keys
	if b.props.GrantGenerateMac != nil {
		for _, grantee := range b.props.GrantGenerateMac {
			liftKey.Key.Grant(grantee, jsii.String("kms:GenerateMac"), jsii.String("kms:VerifyMac"))
		}
	}
}

// storeInSSM stores the key ARN in SSM Parameter Store
func (b *liftKMSKeyBuilder) storeInSSM(liftKey *LiftKMSKey) {
	if !*b.props.EnableSSMParameter || b.props.SSMParameterPath == nil {
		return
	}

	liftKey.SSMParameter = awsssm.NewStringParameter(liftKey, jsii.String("SSMParameter"), &awsssm.StringParameterProps{
		ParameterName: b.props.SSMParameterPath,
		StringValue:   liftKey.KeyArn,
		Description:   jsii.String(fmt.Sprintf("KMS Key ARN for %s", *b.props.Description)),
		Tier:          awsssm.ParameterTier_STANDARD,
	})
}

// applyTags applies tags to the key
func (b *liftKMSKeyBuilder) applyTags(liftKey *LiftKMSKey) {
	// Add standard Lift tags
	awscdk.Tags_Of(liftKey.Key).Add(jsii.String("Framework"), jsii.String("Lift"), nil)
	awscdk.Tags_Of(liftKey.Key).Add(jsii.String("Component"), jsii.String("KMS"), nil)

	// Add custom tags
	if b.props.Tags != nil {
		for key, value := range *b.props.Tags {
			awscdk.Tags_Of(liftKey.Key).Add(jsii.String(key), value, nil)
		}
	}
}

// GetKeyArn returns the key ARN
func (k *LiftKMSKey) GetKeyArn() *string {
	return k.KeyArn
}

// GetKeyId returns the key ID
func (k *LiftKMSKey) GetKeyId() *string {
	return k.KeyId
}

// GetKey returns the underlying IKey
func (k *LiftKMSKey) GetKey() awskms.IKey {
	return k.Key
}

// GrantEncryptDecrypt grants encrypt/decrypt permissions
func (k *LiftKMSKey) GrantEncryptDecrypt(grantee awsiam.IGrantable) awsiam.Grant {
	return k.Key.GrantEncryptDecrypt(grantee)
}

// GrantGenerateMac grants GenerateMac/VerifyMac permissions (for HMAC keys)
func (k *LiftKMSKey) GrantGenerateMac(grantee awsiam.IGrantable) awsiam.Grant {
	return k.Key.Grant(grantee, jsii.String("kms:GenerateMac"), jsii.String("kms:VerifyMac"))
}

// GrantDecrypt grants decrypt permissions only
func (k *LiftKMSKey) GrantDecrypt(grantee awsiam.IGrantable) awsiam.Grant {
	return k.Key.GrantDecrypt(grantee)
}

// GrantEncrypt grants encrypt permissions only
func (k *LiftKMSKey) GrantEncrypt(grantee awsiam.IGrantable) awsiam.Grant {
	return k.Key.GrantEncrypt(grantee)
}

// AddToResourcePolicy adds a statement to the key's resource policy
func (k *LiftKMSKey) AddToResourcePolicy(statement awsiam.PolicyStatement) {
	if key, ok := k.Key.(awskms.Key); ok {
		key.AddToResourcePolicy(statement, nil)
	}
}

// GetResourceName returns the resource name for monitoring
func (k *LiftKMSKey) GetResourceName() *string {
	return k.KeyId
}
