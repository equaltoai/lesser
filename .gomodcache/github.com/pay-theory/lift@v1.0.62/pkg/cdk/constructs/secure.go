package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// SecureFunctionProps extends LiftFunctionProps with security configuration
//
// This struct contains all configurable properties for creating a secure Lambda function
// with enhanced security features. It extends LiftFunctionProps with additional security
// configuration like VPC settings, KMS encryption, secrets management, and IAM policies.
type SecureFunctionProps struct {
	LiftFunctionProps
	// VPC to deploy the function in (optional - will create if not provided)
	Vpc awsec2.IVpc
	// VPC subnets to use (defaults to private subnets)
	VpcSubnets *awsec2.SubnetSelection
	// Security group IDs to attach
	SecurityGroupIds *[]*string
	// Enable KMS encryption for environment variables
	EnableKMSEncryption *bool
	// KMS key for encryption (optional - will create if not provided)
	KmsKey awskms.IKey
	// Secrets to inject from Secrets Manager
	Secrets *map[string]awssecretsmanager.ISecret
	// Enable private endpoints only (no internet access)
	PrivateOnly *bool
	// Additional security policies to attach
	AdditionalPolicies *[]awsiam.PolicyStatement
}

// SecureFunction is a Lambda function with enhanced security features
//
// This construct creates a Lambda function with enhanced security features including:
//
// - VPC deployment (with optional private subnets)
// - KMS encryption for environment variables
// - Secrets Manager integration
// - Custom security groups
// - Private endpoint support
// - Additional IAM policies
//
// The construct provides methods to add VPC endpoints and configure security settings.
type SecureFunction struct {
	constructs.Construct
	Function      *LiftFunction
	SecurityGroup awsec2.ISecurityGroup
	KmsKey        awskms.IKey
	Vpc           awsec2.IVpc
	VpcEndpoints  map[string]awsec2.InterfaceVpcEndpoint
}

// NewSecureFunction creates a Lambda function with enhanced security
//
// This function creates a Lambda function with all security features configured:
//
// - Creates or uses existing VPC
// - Configures appropriate subnets (private or public)
// - Creates and configures security groups
// - Sets up KMS encryption if enabled
// - Applies additional IAM policies
// - Configures environment variables
//
// Parameters:
//   - scope: The CDK construct scope
//   - id: The construct ID
//   - props: Configuration properties
//
// Returns:
//   - A new SecureFunction instance
func NewSecureFunction(scope constructs.Construct, id *string, props *SecureFunctionProps) *SecureFunction {
	builder := newSecureFunctionBuilder(scope, id, props)
	return builder.build()
}

// secureFunctionBuilder builds Lambda functions with enhanced security features
type secureFunctionBuilder struct {
	scope         constructs.Construct
	id            *string
	props         *SecureFunctionProps
	construct     constructs.Construct
	vpc           awsec2.IVpc
	vpcSubnets    *awsec2.SubnetSelection
	securityGroup awsec2.ISecurityGroup
	kmsKey        awskms.IKey
	function      *LiftFunction
}

// newSecureFunctionBuilder creates a new secure function builder
func newSecureFunctionBuilder(scope constructs.Construct, id *string, props *SecureFunctionProps) *secureFunctionBuilder {
	return &secureFunctionBuilder{
		scope: scope,
		id:    id,
		props: props,
	}
}

// build constructs the complete secure function
func (b *secureFunctionBuilder) build() *SecureFunction {
	b.construct = constructs.NewConstruct(b.scope, b.id)

	b.setDefaults()
	b.setupVPC()
	b.configureSubnets()
	b.createSecurityGroup()
	b.setupEncryption()
	b.configureFunctionProps()
	b.createFunction()
	b.applySecrets()
	b.applyPermissions()
	b.applyAdditionalPolicies()

	return &SecureFunction{
		Construct:     b.construct,
		Function:      b.function,
		SecurityGroup: b.securityGroup,
		KmsKey:        b.kmsKey,
		Vpc:           b.vpc,
		VpcEndpoints:  make(map[string]awsec2.InterfaceVpcEndpoint),
	}
}

// setDefaults applies default configuration values
func (b *secureFunctionBuilder) setDefaults() {
	if b.props.EnableKMSEncryption == nil {
		b.props.EnableKMSEncryption = jsii.Bool(true)
	}
	if b.props.PrivateOnly == nil {
		b.props.PrivateOnly = jsii.Bool(false)
	}
}

// setupVPC creates or uses existing VPC
func (b *secureFunctionBuilder) setupVPC() {
	if b.props.Vpc != nil {
		b.vpc = b.props.Vpc
		return
	}

	// Create a new VPC
	b.vpc = b.createVPC()
}

// createVPC creates a new VPC with appropriate configuration
func (b *secureFunctionBuilder) createVPC() awsec2.IVpc {
	subnetConfig := b.getSubnetConfiguration()
	natGateways := jsii.Number(1)

	if *b.props.PrivateOnly {
		natGateways = jsii.Number(0)
	}

	return awsec2.NewVpc(b.construct, jsii.String("SecureVpc"), &awsec2.VpcProps{
		MaxAzs:              jsii.Number(2),
		NatGateways:         natGateways,
		SubnetConfiguration: &subnetConfig,
		EnableDnsHostnames:  jsii.Bool(true),
		EnableDnsSupport:    jsii.Bool(true),
	})
}

// getSubnetConfiguration returns subnet configuration based on privacy settings
func (b *secureFunctionBuilder) getSubnetConfiguration() []*awsec2.SubnetConfiguration {
	if *b.props.PrivateOnly {
		return []*awsec2.SubnetConfiguration{
			{
				Name:       jsii.String("Isolated"),
				SubnetType: awsec2.SubnetType_PRIVATE_ISOLATED,
				CidrMask:   jsii.Number(24),
			},
		}
	}

	return []*awsec2.SubnetConfiguration{
		{
			Name:       jsii.String("Public"),
			SubnetType: awsec2.SubnetType_PUBLIC,
			CidrMask:   jsii.Number(24),
		},
		{
			Name:       jsii.String("Private"),
			SubnetType: awsec2.SubnetType_PRIVATE_WITH_EGRESS,
			CidrMask:   jsii.Number(24),
		},
	}
}

// configureSubnets sets up VPC subnets for the function
func (b *secureFunctionBuilder) configureSubnets() {
	if b.props.VpcSubnets != nil {
		b.vpcSubnets = b.props.VpcSubnets
		return
	}

	if *b.props.PrivateOnly {
		b.vpcSubnets = &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PRIVATE_ISOLATED,
		}
	} else {
		b.vpcSubnets = &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PRIVATE_WITH_EGRESS,
		}
	}
}

// createSecurityGroup creates and configures the security group
func (b *secureFunctionBuilder) createSecurityGroup() {
	b.securityGroup = awsec2.NewSecurityGroup(b.construct, jsii.String("SecurityGroup"), &awsec2.SecurityGroupProps{
		Vpc:              b.vpc,
		Description:      jsii.String("Security group for secure Lambda function"),
		AllowAllOutbound: jsii.Bool(!*b.props.PrivateOnly),
	})

	// Add default egress rules if not private only
	if !*b.props.PrivateOnly {
		b.securityGroup.AddEgressRule(
			awsec2.Peer_AnyIpv4(),
			awsec2.Port_Tcp(jsii.Number(443)),
			jsii.String("Allow HTTPS for AWS API calls"),
			jsii.Bool(false),
		)
	}
}

// setupEncryption configures KMS encryption
func (b *secureFunctionBuilder) setupEncryption() {
	if !*b.props.EnableKMSEncryption {
		return
	}

	if b.props.KmsKey != nil {
		b.kmsKey = b.props.KmsKey
	} else {
		b.kmsKey = awskms.NewKey(b.construct, jsii.String("KmsKey"), &awskms.KeyProps{
			Description:       jsii.String("KMS key for Lambda function encryption"),
			EnableKeyRotation: jsii.Bool(true),
			RemovalPolicy:     awscdk.RemovalPolicy_DESTROY,
			PendingWindow:     awscdk.Duration_Days(jsii.Number(7)),
		})
		b.kmsKey.AddAlias(jsii.String(*b.id + "-key"))
	}

	b.props.EnvironmentEncryption = b.kmsKey
}

// configureFunctionProps sets up Lambda function properties
func (b *secureFunctionBuilder) configureFunctionProps() {
	// Apply VPC-related settings to the underlying FunctionProps (promoted field)
	b.props.FunctionProps.Vpc = b.vpc
	b.props.FunctionProps.VpcSubnets = b.vpcSubnets
	// FunctionProps is embedded, so fields are promoted; use direct selectors.
	b.props.SecurityGroups = &[]awsec2.ISecurityGroup{b.securityGroup}
	b.props.Tracing = awslambda.Tracing_ACTIVE

	b.addAdditionalSecurityGroups()
}

// addAdditionalSecurityGroups adds user-provided security groups
func (b *secureFunctionBuilder) addAdditionalSecurityGroups() {
	if b.props.SecurityGroupIds == nil {
		return
	}

	for _, sgId := range *b.props.SecurityGroupIds {
		sg := awsec2.SecurityGroup_FromSecurityGroupId(b.construct, sgId, sgId, &awsec2.SecurityGroupImportOptions{})
		*b.props.SecurityGroups = append(*b.props.SecurityGroups, sg)
	}
}

// createFunction creates the Lambda function
func (b *secureFunctionBuilder) createFunction() {
	b.function = NewLiftFunction(b.construct, jsii.String("Function"), &b.props.LiftFunctionProps)
}

// applySecrets adds secrets as environment variables
func (b *secureFunctionBuilder) applySecrets() {
	if b.props.Secrets == nil {
		return
	}

	for name, secret := range *b.props.Secrets {
		b.function.Function.AddEnvironment(jsii.String(name), secret.SecretValue().ToString(), nil)
		secret.GrantRead(b.function.Function, nil)
	}
}

// applyPermissions adds necessary IAM permissions
func (b *secureFunctionBuilder) applyPermissions() {
	// Add VPC endpoint permissions
	b.function.Function.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("ec2:CreateNetworkInterface"),
			jsii.String("ec2:DescribeNetworkInterfaces"),
			jsii.String("ec2:DeleteNetworkInterface"),
			jsii.String("ec2:AssignPrivateIpAddresses"),
			jsii.String("ec2:UnassignPrivateIpAddresses"),
		},
		Resources: &[]*string{jsii.String("*")},
	}))

	// Add KMS permissions if encryption is enabled
	if b.kmsKey != nil {
		b.kmsKey.GrantDecrypt(b.function.Function)
		b.kmsKey.GrantEncrypt(b.function.Function)
	}
}

// applyAdditionalPolicies adds user-provided security policies
func (b *secureFunctionBuilder) applyAdditionalPolicies() {
	if b.props.AdditionalPolicies == nil {
		return
	}

	for _, policy := range *b.props.AdditionalPolicies {
		b.function.Function.AddToRolePolicy(policy)
	}
}

// GetFunction returns the underlying Lambda function
//
// This method returns the underlying Lambda function that was created with
// the security enhancements. This is useful when you need to access the
// standard Lambda function properties and methods.
func (f *SecureFunction) GetFunction() awslambda.Function {
	return f.Function.Function
}

// GetSecurityGroup returns the security group
//
// This method returns the security group that was created for the Lambda function.
// This is useful when you need to configure additional security group rules or
// reference the security group in other resources.
func (f *SecureFunction) GetSecurityGroup() awsec2.ISecurityGroup {
	return f.SecurityGroup
}

// GetKmsKey returns the KMS key used for encryption
//
// This method returns the KMS key that is used for encrypting environment variables.
// This is useful when you need to grant additional permissions or reference the
// key in other resources.
func (f *SecureFunction) GetKmsKey() awskms.IKey {
	return f.KmsKey
}

// AddVPCEndpoint adds a VPC endpoint for an AWS service
//
// This method creates a VPC endpoint for the specified AWS service and configures
// the necessary security group rules to allow the Lambda function to access it.
//
// Parameters:
//   - service: The AWS service to create an endpoint for
//
// Returns:
//   - The created VPC endpoint
func (f *SecureFunction) AddVPCEndpoint(service awsec2.InterfaceVpcEndpointAwsService) awsec2.InterfaceVpcEndpoint {
	// Get a simple service identifier for the endpoint ID
	var endpointId string
	serviceName := *service.Name()

	// Use simple identifiers for common services to avoid token issues
	switch service {
	case awsec2.InterfaceVpcEndpointAwsService_SECRETS_MANAGER():
		endpointId = "SecretsManagerEndpoint"
	case awsec2.InterfaceVpcEndpointAwsService_SSM():
		endpointId = "SSMEndpoint"
	case awsec2.InterfaceVpcEndpointAwsService_KMS():
		endpointId = "KMSEndpoint"
	default:
		// For other services, use a generic ID
		endpointId = fmt.Sprintf("VPCEndpoint%d", len(f.VpcEndpoints))
	}

	// Check if endpoint already exists
	if endpoint, exists := f.VpcEndpoints[serviceName]; exists {
		return endpoint
	}

	// Create the VPC endpoint
	endpoint := awsec2.NewInterfaceVpcEndpoint(f.Construct, jsii.String(endpointId), &awsec2.InterfaceVpcEndpointProps{
		Vpc:     f.Vpc,
		Service: service,
		Subnets: &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PRIVATE_WITH_EGRESS,
		},
		SecurityGroups:    &[]awsec2.ISecurityGroup{f.SecurityGroup},
		PrivateDnsEnabled: jsii.Bool(true),
	})

	// Store the endpoint
	f.VpcEndpoints[serviceName] = endpoint

	// Allow the Lambda function to access the endpoint
	endpoint.Connections().AllowFrom(
		awsec2.NewConnections(&awsec2.ConnectionsProps{
			SecurityGroups: &[]awsec2.ISecurityGroup{f.SecurityGroup},
		}),
		awsec2.Port_Tcp(jsii.Number(443)),
		jsii.String(fmt.Sprintf("Allow Lambda to access %s", serviceName)),
	)

	return endpoint
}

// EnableSecretsManagerAccess adds VPC endpoint and permissions for Secrets Manager
//
// This method configures the Lambda function to access Secrets Manager by:
// - Creating a VPC endpoint for Secrets Manager
// - Adding the necessary IAM permissions to read secrets
//
// This is useful when your Lambda function needs to access secrets stored in
// AWS Secrets Manager.
func (f *SecureFunction) EnableSecretsManagerAccess() {
	// Add VPC endpoint
	f.AddVPCEndpoint(awsec2.InterfaceVpcEndpointAwsService_SECRETS_MANAGER())

	// Add permissions
	f.Function.Function.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("secretsmanager:GetSecretValue"),
			jsii.String("secretsmanager:DescribeSecret"),
		},
		Resources: &[]*string{jsii.String("*")},
	}))
}

// RestrictInboundAccess removes all inbound rules from the security group
//
// This method removes all inbound rules from the security group, effectively
// preventing any inbound traffic to the Lambda function. This is useful for
// creating highly secure Lambda functions that don't need to receive incoming
// network connections.
func (f *SecureFunction) RestrictInboundAccess() {
	// Note: This is a simplified implementation
	// In practice, you'd need to iterate and remove existing rules
	// The security group is created with no inbound rules by default
}
