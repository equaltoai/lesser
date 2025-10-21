package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// LiftCertificateProps defines properties for ACM certificate with DNS validation
type LiftCertificateProps struct {
	// Domain name for the certificate (e.g., "api.example.com")
	DomainName *string

	// Subject Alternative Names (SANs) for the certificate
	SubjectAlternativeNames *[]*string

	// Hosted zone for DNS validation (required)
	HostedZone awsroute53.IHostedZone

	// Optional: Override the validation zone (if different from hosted zone)
	ValidationZone awsroute53.IHostedZone

	// Optional: Enable/disable certificate transparency logging (default: true)
	TransparencyLoggingEnabled *bool

	// Optional: Certificate name for identification
	CertificateName *string
}

// LiftCertificate provides a simplified ACM certificate with DNS validation
type LiftCertificate struct {
	constructs.Construct
	Certificate awscertificatemanager.Certificate
}

// NewLiftCertificate creates a new ACM certificate with DNS validation
func NewLiftCertificate(scope constructs.Construct, id *string, props *LiftCertificateProps) *LiftCertificate {
	this := constructs.NewConstruct(scope, id)

	liftCert := &LiftCertificate{
		Construct: this,
	}

	// Set defaults
	if props.TransparencyLoggingEnabled == nil {
		props.TransparencyLoggingEnabled = jsii.Bool(true)
	}

	// Use ValidationZone if provided, otherwise use HostedZone
	validationZone := props.HostedZone
	if props.ValidationZone != nil {
		validationZone = props.ValidationZone
	}

	// Build certificate props
	certProps := &awscertificatemanager.CertificateProps{
		DomainName:                 props.DomainName,
		Validation:                 awscertificatemanager.CertificateValidation_FromDns(validationZone),
		TransparencyLoggingEnabled: props.TransparencyLoggingEnabled,
	}

	// Add SANs if provided
	if props.SubjectAlternativeNames != nil {
		certProps.SubjectAlternativeNames = props.SubjectAlternativeNames
	}

	// Add certificate name if provided
	if props.CertificateName != nil {
		certProps.CertificateName = props.CertificateName
	}

	// Create the certificate
	liftCert.Certificate = awscertificatemanager.NewCertificate(this, jsii.String("Certificate"), certProps)

	return liftCert
}

// GetCertificate returns the underlying ACM certificate
func (c *LiftCertificate) GetCertificate() awscertificatemanager.ICertificate {
	return c.Certificate
}

// GetCertificateArn returns the certificate ARN
func (c *LiftCertificate) GetCertificateArn() *string {
	return c.Certificate.CertificateArn()
}

// AddDependency adds a dependency to the certificate (useful for NS delegation)
func (c *LiftCertificate) AddDependency(dependency constructs.IConstruct) {
	c.Certificate.Node().AddDependency(dependency)
}
