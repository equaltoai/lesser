package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// LiftHostedZoneProps defines properties for Route53 hosted zone
type LiftHostedZoneProps struct {
	// Zone name (e.g., "example.com")
	ZoneName *string

	// Comment for the hosted zone
	Comment *string

	// If true, attempts to import existing zone instead of creating new one
	// Requires ExistingZoneId to be provided
	ImportIfExists *bool

	// Existing zone ID (for import mode)
	ExistingZoneId *string

	// Enable SSM parameter export for zone ID
	EnableSSMExport *bool

	// SSM parameter path for zone ID (only used if EnableSSMExport is true)
	// Default: /route53/zones/{ZoneName}/id
	SSMParameterPath *string

	// Enable CloudFormation output export
	EnableCfnExport *bool

	// CloudFormation export name
	CfnExportName *string

	// Tags to apply to the hosted zone
	Tags *map[string]*string
}

// LiftHostedZone provides simplified Route53 hosted zone creation/import
type LiftHostedZone struct {
	constructs.Construct
	HostedZone   awsroute53.IHostedZone
	HostedZoneId *string
	ZoneName     *string
	IsImported   bool
}

// NewLiftHostedZone creates or imports a Route53 hosted zone
func NewLiftHostedZone(scope constructs.Construct, id *string, props *LiftHostedZoneProps) *LiftHostedZone {
	this := constructs.NewConstruct(scope, id)

	liftZone := &LiftHostedZone{
		Construct: this,
		ZoneName:  props.ZoneName,
	}

	// Set defaults
	if props.ImportIfExists == nil {
		props.ImportIfExists = jsii.Bool(false)
	}
	if props.EnableSSMExport == nil {
		props.EnableSSMExport = jsii.Bool(false)
	}
	if props.EnableCfnExport == nil {
		props.EnableCfnExport = jsii.Bool(false)
	}

	// Import or create hosted zone
	if *props.ImportIfExists && props.ExistingZoneId != nil {
		// Import existing hosted zone
		liftZone.HostedZone = awsroute53.HostedZone_FromHostedZoneAttributes(this, jsii.String("HostedZone"), &awsroute53.HostedZoneAttributes{
			HostedZoneId: props.ExistingZoneId,
			ZoneName:     props.ZoneName,
		})
		liftZone.HostedZoneId = props.ExistingZoneId
		liftZone.IsImported = true
	} else {
		// Create new hosted zone
		zoneProps := &awsroute53.PublicHostedZoneProps{
			ZoneName: props.ZoneName,
		}

		if props.Comment != nil {
			zoneProps.Comment = props.Comment
		}

		zone := awsroute53.NewPublicHostedZone(this, jsii.String("HostedZone"), zoneProps)
		liftZone.HostedZone = zone
		liftZone.HostedZoneId = zone.HostedZoneId()
		liftZone.IsImported = false

		// Apply tags if provided
		if props.Tags != nil {
			for key, value := range *props.Tags {
				awscdk.Tags_Of(zone).Add(jsii.String(key), value, nil)
			}
		}
	}

	// Export to SSM if enabled
	if *props.EnableSSMExport {
		paramPath := props.SSMParameterPath
		if paramPath == nil {
			paramPath = jsii.String(fmt.Sprintf("/route53/zones/%s/id", *props.ZoneName))
		}

		awsssm.NewStringParameter(this, jsii.String("ZoneIdParameter"), &awsssm.StringParameterProps{
			ParameterName: paramPath,
			StringValue:   liftZone.HostedZoneId,
			Description:   jsii.String(fmt.Sprintf("Hosted Zone ID for %s", *props.ZoneName)),
		})
	}

	// Export to CloudFormation if enabled
	if *props.EnableCfnExport {
		exportName := props.CfnExportName
		if exportName == nil {
			exportName = jsii.String(fmt.Sprintf("HostedZoneId-%s", *props.ZoneName))
		}

		awscdk.NewCfnOutput(this, jsii.String("ZoneIdOutput"), &awscdk.CfnOutputProps{
			Value:       liftZone.HostedZoneId,
			Description: jsii.String(fmt.Sprintf("Hosted Zone ID for %s", *props.ZoneName)),
			ExportName:  exportName,
		})
	}

	return liftZone
}

// GetHostedZone returns the underlying Route53 hosted zone
func (z *LiftHostedZone) GetHostedZone() awsroute53.IHostedZone {
	return z.HostedZone
}

// GetHostedZoneId returns the hosted zone ID
func (z *LiftHostedZone) GetHostedZoneId() *string {
	return z.HostedZoneId
}

// GetZoneName returns the zone name
func (z *LiftHostedZone) GetZoneName() *string {
	return z.ZoneName
}

// GetNameServers returns the name servers for the hosted zone
// Only works for created zones (not imported)
func (z *LiftHostedZone) GetNameServers() *[]*string {
	if z.IsImported {
		return nil
	}
	return z.HostedZone.HostedZoneNameServers()
}

// AddNSRecord creates NS record delegation to another zone
func (z *LiftHostedZone) AddNSRecord(recordName *string, targetNameServers *[]*string, ttl awscdk.Duration) awsroute53.NsRecord {
	return awsroute53.NewNsRecord(z.Construct, jsii.String(fmt.Sprintf("NSRecord-%s", *recordName)), &awsroute53.NsRecordProps{
		Zone:       z.HostedZone,
		RecordName: recordName,
		Values:     targetNameServers,
		Ttl:        ttl,
	})
}

// AddCNAMERecord creates a CNAME record in the zone
func (z *LiftHostedZone) AddCNAMERecord(recordName *string, domainName *string, ttl awscdk.Duration) awsroute53.CnameRecord {
	return awsroute53.NewCnameRecord(z.Construct, jsii.String(fmt.Sprintf("CNAMERecord-%s", *recordName)), &awsroute53.CnameRecordProps{
		Zone:       z.HostedZone,
		RecordName: recordName,
		DomainName: domainName,
		Ttl:        ttl,
	})
}
