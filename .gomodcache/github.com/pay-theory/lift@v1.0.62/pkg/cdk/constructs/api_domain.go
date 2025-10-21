package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// LiftApiDomainProps defines properties for API Gateway custom domain
type LiftApiDomainProps struct {
	// Domain name for the API (e.g., "api.example.com")
	DomainName *string

	// Optional: API mapping key (base path)
	ApiMappingKey *string

	// ACM certificate for the domain (required)
	Certificate awscertificatemanager.ICertificate

	// HTTP API to map to the domain (required)
	HttpAPI awsapigatewayv2.IHttpApi

	// Optional: Stage to map (defaults to HttpAPI.DefaultStage() if not provided)
	Stage awsapigatewayv2.IStage

	// Optional: Hosted zone for creating DNS records
	// If provided, a CNAME record will be created pointing to the API Gateway domain
	HostedZone awsroute53.IHostedZone

	// Optional: Enable mutual TLS authentication
	MutualTlsAuthentication *awsapigatewayv2.MTLSConfig

	// Optional: TTL for the CNAME record in seconds (default: 300)
	RecordTTL *float64

	// Optional: Create CNAME record in Route53 (default: true if HostedZone is provided)
	CreateCNAME *bool

	// Optional: Security policy (default: TLS_1_2)
	SecurityPolicy awsapigatewayv2.SecurityPolicy
}

// LiftApiDomain provides simplified API Gateway custom domain with Route53 integration
type LiftApiDomain struct {
	constructs.Construct
	DomainName   awsapigatewayv2.DomainName
	ApiMapping   awsapigatewayv2.ApiMapping
	CNAMERecord  awsroute53.CnameRecord
	DomainString *string
}

// NewLiftApiDomain creates API Gateway custom domain with optional Route53 integration
func NewLiftApiDomain(scope constructs.Construct, id *string, props *LiftApiDomainProps) *LiftApiDomain {
	this := constructs.NewConstruct(scope, id)

	liftDomain := &LiftApiDomain{
		Construct:    this,
		DomainString: props.DomainName,
	}

	// Set defaults
	if props.CreateCNAME == nil {
		// Default to true if HostedZone is provided
		props.CreateCNAME = jsii.Bool(props.HostedZone != nil)
	}
	if props.RecordTTL == nil {
		props.RecordTTL = jsii.Number(300)
	}

	// Build domain name props
	domainProps := &awsapigatewayv2.DomainNameProps{
		DomainName:  props.DomainName,
		Certificate: props.Certificate,
	}

	// Add security policy if provided
	if props.SecurityPolicy != "" {
		domainProps.SecurityPolicy = props.SecurityPolicy
	}

	// Add mutual TLS if provided
	if props.MutualTlsAuthentication != nil {
		domainProps.Mtls = props.MutualTlsAuthentication
	}

	// Create the custom domain
	liftDomain.DomainName = awsapigatewayv2.NewDomainName(this, jsii.String("CustomDomain"), domainProps)

	// Determine which stage to use
	stage := props.Stage
	if stage == nil {
		// Fallback to default stage if not explicitly provided
		stage = props.HttpAPI.DefaultStage()
	}

	// Create API mapping
	mappingProps := &awsapigatewayv2.ApiMappingProps{
		Api:        props.HttpAPI,
		DomainName: liftDomain.DomainName,
		Stage:      stage,
	}

	// Add mapping key if provided
	if props.ApiMappingKey != nil {
		mappingProps.ApiMappingKey = props.ApiMappingKey
	}

	liftDomain.ApiMapping = awsapigatewayv2.NewApiMapping(this, jsii.String("ApiMapping"), mappingProps)

	// Create CNAME record if requested and HostedZone is provided
	if *props.CreateCNAME && props.HostedZone != nil {
		liftDomain.CNAMERecord = awsroute53.NewCnameRecord(this, jsii.String("CNAMERecord"), &awsroute53.CnameRecordProps{
			Zone:       props.HostedZone,
			RecordName: props.DomainName,
			DomainName: liftDomain.DomainName.RegionalDomainName(),
			Ttl:        awscdk.Duration_Seconds(props.RecordTTL),
		})
	}

	// Create CloudFormation outputs
	awscdk.NewCfnOutput(this, jsii.String("CustomDomainName"), &awscdk.CfnOutputProps{
		Value:       props.DomainName,
		Description: jsii.String("API Custom Domain Name"),
	})

	awscdk.NewCfnOutput(this, jsii.String("RegionalDomainName"), &awscdk.CfnOutputProps{
		Value:       liftDomain.DomainName.RegionalDomainName(),
		Description: jsii.String("API Gateway Regional Domain Name"),
	})

	return liftDomain
}

// GetDomainName returns the underlying API Gateway domain name
func (d *LiftApiDomain) GetDomainName() awsapigatewayv2.IDomainName {
	return d.DomainName
}

// GetRegionalDomainName returns the regional domain name for DNS records
func (d *LiftApiDomain) GetRegionalDomainName() *string {
	return d.DomainName.RegionalDomainName()
}

// GetApiMapping returns the API mapping
func (d *LiftApiDomain) GetApiMapping() awsapigatewayv2.ApiMapping {
	return d.ApiMapping
}

// GetCNAMERecord returns the Route53 CNAME record (may be nil)
func (d *LiftApiDomain) GetCNAMERecord() awsroute53.CnameRecord {
	return d.CNAMERecord
}

// AddAdditionalMapping adds another API mapping to the same domain
func (d *LiftApiDomain) AddAdditionalMapping(api awsapigatewayv2.IHttpApi, mappingKey *string) awsapigatewayv2.ApiMapping {
	mappingProps := &awsapigatewayv2.ApiMappingProps{
		Api:        api,
		DomainName: d.DomainName,
		Stage:      api.DefaultStage(),
	}

	if mappingKey != nil {
		mappingProps.ApiMappingKey = mappingKey
	}

	return awsapigatewayv2.NewApiMapping(d.Construct, jsii.String(fmt.Sprintf("AdditionalMapping-%s", *mappingKey)), mappingProps)
}
