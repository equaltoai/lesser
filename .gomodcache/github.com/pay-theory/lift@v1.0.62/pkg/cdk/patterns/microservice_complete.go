package patterns

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapplicationautoscaling"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awselasticloadbalancingv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsservicediscovery"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	liftconstructs "github.com/pay-theory/lift/pkg/cdk/constructs"
)

// ServiceDiscoveryConfig defines service discovery configuration
type ServiceDiscoveryConfig struct {
	Namespace               *string
	ServiceName             *string
	HealthCheckPath         *string
	HealthCheckInterval     *awscdk.Duration
	HealthCheckTimeout      *awscdk.Duration
	HealthyThresholdCount   *float64
	UnhealthyThresholdCount *float64
	TTL                     *awscdk.Duration
	DNSRecordType           awsservicediscovery.DnsRecordType
}

// LoadBalancerConfig defines load balancer configuration
type LoadBalancerConfig struct {
	Enabled                 *bool
	Certificate             awselasticloadbalancingv2.IListenerCertificate
	DomainName              *string
	EnableHTTP2             *bool
	EnableSSLRedirect       *bool
	IdleTimeout             *awscdk.Duration
	HealthCheckPath         *string
	HealthCheckInterval     *awscdk.Duration
	HealthCheckTimeout      *awscdk.Duration
	HealthyThresholdCount   *float64
	UnhealthyThresholdCount *float64
	DeregistrationDelay     *awscdk.Duration
	StickinessEnabled       *bool
	TargetGroupProtocol     awselasticloadbalancingv2.ApplicationProtocol
}

// AutoScalingConfig defines auto-scaling configuration
type AutoScalingConfig struct {
	MinCapacity             *float64
	MaxCapacity             *float64
	TargetCPUUtilization    *float64
	TargetMemoryUtilization *float64
	ScaleInCooldown         *awscdk.Duration
	ScaleOutCooldown        *awscdk.Duration
	RequestsPerTarget       *float64
	EnablePredictiveScaling *bool
	EnableScheduledScaling  *bool
	ScheduledScalingActions *[]ScheduledScalingAction
}

// ScheduledScalingAction defines a scheduled scaling action
type ScheduledScalingAction struct {
	MinCapacity     *float64
	MaxCapacity     *float64
	DesiredCapacity *float64
	Timezone        *string
	Name            string
	Schedule        string
}

// HealthCheckConfig defines health check configuration
type HealthCheckConfig struct {
	Path               *string
	Port               *float64
	Protocol           *string
	Interval           *awscdk.Duration
	Timeout            *awscdk.Duration
	HealthyThreshold   *float64
	UnhealthyThreshold *float64
	GracePeriod        *awscdk.Duration
}

// NetworkConfig defines network configuration
type NetworkConfig struct {
	VPC                     awsec2.IVpc
	SubnetSelection         *awsec2.SubnetSelection
	SecurityGroups          *[]awsec2.ISecurityGroup
	AssignPublicIP          *bool
	EnableVPCLogs           *bool
	EnableContainerInsights *bool
}

// ContainerConfig defines container configuration
type ContainerConfig struct {
	Platform          awsecs.CpuArchitecture
	Secrets           *map[string]awsecs.Secret
	CodeAssetPath     *string
	CPU               *float64
	Memory            *float64
	Environment       *map[string]*string
	ImageURI          *string
	EnableXRayTracing *bool
	EnableFirelens    *bool
	Command           *[]*string
	EntryPoint        *[]*string
	WorkingDirectory  *string
	User              *string
	LogRetentionDays  awslogs.RetentionDays
}

// MicroserviceCompleteProps defines comprehensive microservice properties
type MicroserviceCompleteProps struct {
	awscdk.StackProps
	// Basic configuration
	ServiceName *string
	Environment *string
	// Network configuration
	NetworkConfig *NetworkConfig
	// Container configuration
	ContainerConfig *ContainerConfig
	// Service discovery
	ServiceDiscovery *ServiceDiscoveryConfig
	// Load balancer
	LoadBalancer *LoadBalancerConfig
	// Auto scaling
	AutoScaling *AutoScalingConfig
	// Health checks
	HealthCheck *HealthCheckConfig
	// Enable enhanced monitoring
	EnableEnhancedMonitoring *bool
	// Enable enhanced security
	EnableEnhancedSecurity *bool
	// Tags
	Tags *map[string]*string
}

// MicroserviceComplete represents a complete microservice implementation
type MicroserviceComplete struct {
	constructs.Construct
	// Infrastructure
	VPC            awsec2.IVpc
	Cluster        awsecs.ICluster
	Service        awsecs.FargateService
	TaskDefinition awsecs.FargateTaskDefinition
	// Service discovery
	Namespace        awsservicediscovery.IPrivateDnsNamespace
	ServiceDiscovery awsservicediscovery.IService
	// Load balancing
	LoadBalancer awselasticloadbalancingv2.IApplicationLoadBalancer
	TargetGroup  awselasticloadbalancingv2.IApplicationTargetGroup
	Listener     awselasticloadbalancingv2.IApplicationListener
	// Auto scaling
	ScalableTarget awsecs.ScalableTaskCount
	// Monitoring and security
	Monitoring *liftconstructs.EnhancedMonitoring
	Security   *liftconstructs.EnhancedSecurity
	// Outputs
	ServiceEndpoint          *string
	LoadBalancerDNS          *string
	ServiceDiscoveryEndpoint *string
}

// NewMicroserviceComplete creates a comprehensive microservice stack
func NewMicroserviceComplete(scope constructs.Construct, id *string, props *MicroserviceCompleteProps) *MicroserviceComplete {
	this := constructs.NewConstruct(scope, id)

	microservice := &MicroserviceComplete{
		Construct: this,
	}

	// Set defaults
	microservice.setDefaults(props)

	// Create or use VPC
	microservice.setupNetworking(props)

	// Create ECS cluster
	microservice.createCluster(props)

	// Set up service discovery
	microservice.setupServiceDiscovery(props)

	// Create task definition and container
	microservice.createTaskDefinition(props)

	// Create ECS service
	microservice.createService(props)

	// Set up load balancer if enabled
	if props.LoadBalancer != nil && props.LoadBalancer.Enabled != nil && *props.LoadBalancer.Enabled {
		microservice.setupLoadBalancer(props)
	}

	// Configure auto scaling
	microservice.setupAutoScaling(props)

	// Enable enhanced monitoring if requested
	if props.EnableEnhancedMonitoring != nil && *props.EnableEnhancedMonitoring {
		microservice.setupEnhancedMonitoring(props)
	}

	// Enable enhanced security if requested
	if props.EnableEnhancedSecurity != nil && *props.EnableEnhancedSecurity {
		microservice.setupEnhancedSecurity(props)
	}

	// Create outputs
	microservice.createOutputs(props)

	// Apply tags
	microservice.applyTags(props)

	return microservice
}

func (m *MicroserviceComplete) setDefaults(props *MicroserviceCompleteProps) {
	defaultsSetter := newMicroserviceDefaultsSetter(props)
	defaultsSetter.applyAllDefaults()
}

// microserviceDefaultsSetter applies default values to microservice configuration
type microserviceDefaultsSetter struct {
	props *MicroserviceCompleteProps
}

// newMicroserviceDefaultsSetter creates a new defaults setter
func newMicroserviceDefaultsSetter(props *MicroserviceCompleteProps) *microserviceDefaultsSetter {
	return &microserviceDefaultsSetter{
		props: props,
	}
}

// applyAllDefaults applies defaults to all configuration sections
func (mds *microserviceDefaultsSetter) applyAllDefaults() {
	mds.applyBasicDefaults()
	mds.applyNetworkDefaults()
	mds.applyContainerDefaults()
	mds.applyServiceDiscoveryDefaults()
	mds.applyAutoScalingDefaults()
	mds.applyHealthCheckDefaults()
}

// applyBasicDefaults sets basic service defaults
func (mds *microserviceDefaultsSetter) applyBasicDefaults() {
	if mds.props.ServiceName == nil {
		mds.props.ServiceName = jsii.String("microservice")
	}
	if mds.props.Environment == nil {
		mds.props.Environment = jsii.String("production")
	}
	if mds.props.EnableEnhancedMonitoring == nil {
		mds.props.EnableEnhancedMonitoring = jsii.Bool(true)
	}
	if mds.props.EnableEnhancedSecurity == nil {
		mds.props.EnableEnhancedSecurity = jsii.Bool(true)
	}
}

// applyNetworkDefaults sets network configuration defaults
func (mds *microserviceDefaultsSetter) applyNetworkDefaults() {
	if mds.props.NetworkConfig == nil {
		mds.props.NetworkConfig = &NetworkConfig{}
	}
	if mds.props.NetworkConfig.AssignPublicIP == nil {
		mds.props.NetworkConfig.AssignPublicIP = jsii.Bool(false)
	}
	if mds.props.NetworkConfig.EnableContainerInsights == nil {
		mds.props.NetworkConfig.EnableContainerInsights = jsii.Bool(true)
	}
}

// applyContainerDefaults sets container configuration defaults
func (mds *microserviceDefaultsSetter) applyContainerDefaults() {
	if mds.props.ContainerConfig == nil {
		mds.props.ContainerConfig = &ContainerConfig{}
	}

	containerSetter := newContainerDefaultsSetter(mds.props.ContainerConfig)
	containerSetter.applyDefaults()
}

// applyServiceDiscoveryDefaults sets service discovery defaults
func (mds *microserviceDefaultsSetter) applyServiceDiscoveryDefaults() {
	if mds.props.ServiceDiscovery == nil {
		mds.props.ServiceDiscovery = &ServiceDiscoveryConfig{}
	}

	discoverySetter := newServiceDiscoveryDefaultsSetter(mds.props.ServiceDiscovery, mds.props.ServiceName)
	discoverySetter.applyDefaults()
}

// applyAutoScalingDefaults sets auto scaling defaults
func (mds *microserviceDefaultsSetter) applyAutoScalingDefaults() {
	if mds.props.AutoScaling == nil {
		mds.props.AutoScaling = &AutoScalingConfig{}
	}

	scalingSetter := newAutoScalingDefaultsSetter(mds.props.AutoScaling)
	scalingSetter.applyDefaults()
}

// applyHealthCheckDefaults sets health check defaults
func (mds *microserviceDefaultsSetter) applyHealthCheckDefaults() {
	if mds.props.HealthCheck == nil {
		mds.props.HealthCheck = &HealthCheckConfig{}
	}

	healthSetter := newHealthCheckDefaultsSetter(mds.props.HealthCheck)
	healthSetter.applyDefaults()
}

// containerDefaultsSetter handles container-specific defaults
type containerDefaultsSetter struct {
	config *ContainerConfig
}

// newContainerDefaultsSetter creates a new container defaults setter
func newContainerDefaultsSetter(config *ContainerConfig) *containerDefaultsSetter {
	return &containerDefaultsSetter{config: config}
}

// applyDefaults sets container configuration defaults
func (cds *containerDefaultsSetter) applyDefaults() {
	if cds.config.Platform == "" {
		cds.config.Platform = awsecs.CpuArchitecture_ARM64()
	}
	if cds.config.CPU == nil {
		cds.config.CPU = jsii.Number(256)
	}
	if cds.config.Memory == nil {
		cds.config.Memory = jsii.Number(512)
	}
	if cds.config.LogRetentionDays == "" {
		cds.config.LogRetentionDays = awslogs.RetentionDays_ONE_WEEK
	}
	if cds.config.EnableXRayTracing == nil {
		cds.config.EnableXRayTracing = jsii.Bool(true)
	}
}

// serviceDiscoveryDefaultsSetter handles service discovery defaults
type serviceDiscoveryDefaultsSetter struct {
	config      *ServiceDiscoveryConfig
	serviceName *string
}

// newServiceDiscoveryDefaultsSetter creates a new service discovery defaults setter
func newServiceDiscoveryDefaultsSetter(config *ServiceDiscoveryConfig, serviceName *string) *serviceDiscoveryDefaultsSetter {
	return &serviceDiscoveryDefaultsSetter{
		config:      config,
		serviceName: serviceName,
	}
}

// applyDefaults sets service discovery defaults
func (sdds *serviceDiscoveryDefaultsSetter) applyDefaults() {
	if sdds.config.Namespace == nil {
		sdds.config.Namespace = jsii.String(fmt.Sprintf("%s.local", *sdds.serviceName))
	}
	if sdds.config.ServiceName == nil {
		sdds.config.ServiceName = sdds.serviceName
	}
	if sdds.config.DNSRecordType == "" {
		sdds.config.DNSRecordType = awsservicediscovery.DnsRecordType_A
	}
	if sdds.config.TTL == nil {
		ttl := awscdk.Duration_Seconds(jsii.Number(10))
		sdds.config.TTL = &ttl
	}
}

// autoScalingDefaultsSetter handles auto scaling defaults
type autoScalingDefaultsSetter struct {
	config *AutoScalingConfig
}

// newAutoScalingDefaultsSetter creates a new auto scaling defaults setter
func newAutoScalingDefaultsSetter(config *AutoScalingConfig) *autoScalingDefaultsSetter {
	return &autoScalingDefaultsSetter{config: config}
}

// applyDefaults sets auto scaling defaults
func (asds *autoScalingDefaultsSetter) applyDefaults() {
	if asds.config.MinCapacity == nil {
		asds.config.MinCapacity = jsii.Number(2)
	}
	if asds.config.MaxCapacity == nil {
		asds.config.MaxCapacity = jsii.Number(10)
	}
	if asds.config.TargetCPUUtilization == nil {
		asds.config.TargetCPUUtilization = jsii.Number(70)
	}
	if asds.config.TargetMemoryUtilization == nil {
		asds.config.TargetMemoryUtilization = jsii.Number(80)
	}
	if asds.config.ScaleInCooldown == nil {
		cooldown := awscdk.Duration_Seconds(jsii.Number(300))
		asds.config.ScaleInCooldown = &cooldown
	}
	if asds.config.ScaleOutCooldown == nil {
		cooldown := awscdk.Duration_Seconds(jsii.Number(300))
		asds.config.ScaleOutCooldown = &cooldown
	}
}

// healthCheckDefaultsSetter handles health check defaults
type healthCheckDefaultsSetter struct {
	config *HealthCheckConfig
}

// newHealthCheckDefaultsSetter creates a new health check defaults setter
func newHealthCheckDefaultsSetter(config *HealthCheckConfig) *healthCheckDefaultsSetter {
	return &healthCheckDefaultsSetter{config: config}
}

// applyDefaults sets health check defaults
func (hcds *healthCheckDefaultsSetter) applyDefaults() {
	if hcds.config.Path == nil {
		hcds.config.Path = jsii.String("/health")
	}
	if hcds.config.Port == nil {
		hcds.config.Port = jsii.Number(8080)
	}
	if hcds.config.Protocol == nil {
		hcds.config.Protocol = jsii.String("HTTP")
	}
	if hcds.config.Interval == nil {
		interval := awscdk.Duration_Seconds(jsii.Number(30))
		hcds.config.Interval = &interval
	}
	if hcds.config.Timeout == nil {
		timeout := awscdk.Duration_Seconds(jsii.Number(5))
		hcds.config.Timeout = &timeout
	}
	if hcds.config.HealthyThreshold == nil {
		hcds.config.HealthyThreshold = jsii.Number(2)
	}
	if hcds.config.UnhealthyThreshold == nil {
		hcds.config.UnhealthyThreshold = jsii.Number(3)
	}
	if hcds.config.GracePeriod == nil {
		grace := awscdk.Duration_Seconds(jsii.Number(60))
		hcds.config.GracePeriod = &grace
	}
}

func (m *MicroserviceComplete) setupNetworking(props *MicroserviceCompleteProps) {
	if props.NetworkConfig.VPC != nil {
		m.VPC = props.NetworkConfig.VPC
	} else {
		// Create a new VPC with proper configuration
		m.VPC = awsec2.NewVpc(m.Construct, jsii.String("VPC"), &awsec2.VpcProps{
			MaxAzs:      jsii.Number(3),
			NatGateways: jsii.Number(1),
			SubnetConfiguration: &[]*awsec2.SubnetConfiguration{
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
			},
			EnableDnsHostnames: jsii.Bool(true),
			EnableDnsSupport:   jsii.Bool(true),
		})
	}
}

func (m *MicroserviceComplete) createCluster(props *MicroserviceCompleteProps) {
	clusterProps := &awsecs.ClusterProps{
		Vpc:               m.VPC,
		ContainerInsights: props.NetworkConfig.EnableContainerInsights,
	}

	m.Cluster = awsecs.NewCluster(m.Construct, jsii.String("Cluster"), clusterProps)
}

func (m *MicroserviceComplete) setupServiceDiscovery(props *MicroserviceCompleteProps) {
	// Create private DNS namespace
	m.Namespace = awsservicediscovery.NewPrivateDnsNamespace(m.Construct, jsii.String("Namespace"), &awsservicediscovery.PrivateDnsNamespaceProps{
		Name:        props.ServiceDiscovery.Namespace,
		Vpc:         m.VPC,
		Description: jsii.String(fmt.Sprintf("Service discovery namespace for %s", *props.ServiceName)),
	})

	// Note: Cannot set default namespace on ICluster interface
	// This would need to be done when creating the cluster
}

func (m *MicroserviceComplete) createTaskDefinition(props *MicroserviceCompleteProps) {
	// Create task definition
	m.TaskDefinition = awsecs.NewFargateTaskDefinition(m.Construct, jsii.String("TaskDef"), &awsecs.FargateTaskDefinitionProps{
		MemoryLimitMiB: jsii.Number(*props.ContainerConfig.Memory),
		Cpu:            jsii.Number(*props.ContainerConfig.CPU),
		RuntimePlatform: &awsecs.RuntimePlatform{
			CpuArchitecture:       props.ContainerConfig.Platform,
			OperatingSystemFamily: awsecs.OperatingSystemFamily_LINUX(),
		},
	})

	// Create container image
	var containerImage awsecs.ContainerImage
	switch {
	case props.ContainerConfig.ImageURI != nil:
		containerImage = awsecs.ContainerImage_FromRegistry(props.ContainerConfig.ImageURI, &awsecs.RepositoryImageProps{})
	case props.ContainerConfig.CodeAssetPath != nil:
		containerImage = awsecs.ContainerImage_FromAsset(props.ContainerConfig.CodeAssetPath, &awsecs.AssetImageProps{
			// Platform: awsecs.Platform_LINUX_ARM64(), // Platform API may have changed
		})
	default:
		// Default to a simple HTTP server for demonstration
		containerImage = awsecs.ContainerImage_FromRegistry(jsii.String("nginx:alpine"), &awsecs.RepositoryImageProps{})
	}

	// Create container definition
	containerOptions := &awsecs.ContainerDefinitionOptions{
		Image: containerImage,
		Logging: awsecs.LogDrivers_AwsLogs(&awsecs.AwsLogDriverProps{
			StreamPrefix: jsii.String(*props.ServiceName),
			LogRetention: props.ContainerConfig.LogRetentionDays,
			LogGroup: awslogs.NewLogGroup(m.Construct, jsii.String("LogGroup"), &awslogs.LogGroupProps{
				LogGroupName:  jsii.String(fmt.Sprintf("/ecs/%s", *props.ServiceName)),
				Retention:     props.ContainerConfig.LogRetentionDays,
				RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
			}),
		}),
		Environment: props.ContainerConfig.Environment,
		Secrets:     props.ContainerConfig.Secrets,
		HealthCheck: &awsecs.HealthCheck{
			Command: &[]*string{
				jsii.String("CMD-SHELL"),
				jsii.String(fmt.Sprintf("curl -f http://localhost:%s%s || exit 1",
					fmt.Sprintf("%.0f", *props.HealthCheck.Port),
					*props.HealthCheck.Path)),
			},
			Interval:    *props.HealthCheck.Interval,
			Timeout:     *props.HealthCheck.Timeout,
			Retries:     jsii.Number(*props.HealthCheck.HealthyThreshold),
			StartPeriod: *props.HealthCheck.GracePeriod,
		},
		Essential: jsii.Bool(true),
	}

	// Add optional container configuration
	if props.ContainerConfig.Command != nil {
		containerOptions.Command = props.ContainerConfig.Command
	}
	if props.ContainerConfig.EntryPoint != nil {
		containerOptions.EntryPoint = props.ContainerConfig.EntryPoint
	}
	if props.ContainerConfig.WorkingDirectory != nil {
		containerOptions.WorkingDirectory = props.ContainerConfig.WorkingDirectory
	}
	if props.ContainerConfig.User != nil {
		containerOptions.User = props.ContainerConfig.User
	}

	container := m.TaskDefinition.AddContainer(jsii.String("Container"), containerOptions)

	// Add port mapping
	container.AddPortMappings(&awsecs.PortMapping{
		ContainerPort: jsii.Number(*props.HealthCheck.Port),
		Protocol:      awsecs.Protocol_TCP,
	})

	// Add X-Ray sidecar if enabled
	if props.ContainerConfig.EnableXRayTracing != nil && *props.ContainerConfig.EnableXRayTracing {
		m.TaskDefinition.AddContainer(jsii.String("XRayDaemon"), &awsecs.ContainerDefinitionOptions{
			Image: awsecs.ContainerImage_FromRegistry(jsii.String("amazon/aws-xray-daemon:latest"), &awsecs.RepositoryImageProps{}),
			Logging: awsecs.LogDrivers_AwsLogs(&awsecs.AwsLogDriverProps{
				StreamPrefix: jsii.String("xray"),
				LogRetention: awslogs.RetentionDays_ONE_WEEK,
			}),
			Essential:            jsii.Bool(false),
			MemoryReservationMiB: jsii.Number(256),
		}).AddPortMappings(&awsecs.PortMapping{
			ContainerPort: jsii.Number(2000),
			Protocol:      awsecs.Protocol_UDP,
		})
	}
}

func (m *MicroserviceComplete) createService(props *MicroserviceCompleteProps) {
	// Configure subnet selection
	subnetSelection := props.NetworkConfig.SubnetSelection
	if subnetSelection == nil {
		subnetSelection = &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PRIVATE_WITH_EGRESS,
		}
	}

	// Create the Fargate service
	serviceProps := &awsecs.FargateServiceProps{
		Cluster:         m.Cluster,
		TaskDefinition:  m.TaskDefinition,
		DesiredCount:    props.AutoScaling.MinCapacity,
		AssignPublicIp:  props.NetworkConfig.AssignPublicIP,
		VpcSubnets:      subnetSelection,
		SecurityGroups:  props.NetworkConfig.SecurityGroups,
		PlatformVersion: awsecs.FargatePlatformVersion_LATEST,
		CircuitBreaker: &awsecs.DeploymentCircuitBreaker{
			Rollback: jsii.Bool(true),
		},
		MaxHealthyPercent: jsii.Number(200),
		MinHealthyPercent: jsii.Number(50),
	}

	// Add service discovery configuration
	if props.ServiceDiscovery != nil {
		serviceProps.CloudMapOptions = &awsecs.CloudMapOptions{
			Name:          props.ServiceDiscovery.ServiceName,
			DnsRecordType: props.ServiceDiscovery.DNSRecordType,
			DnsTtl:        *props.ServiceDiscovery.TTL,
			Container:     m.TaskDefinition.FindContainer(jsii.String("Container")),
			ContainerPort: jsii.Number(*props.HealthCheck.Port),
		}

		// Add health check configuration if specified
		if props.ServiceDiscovery.HealthCheckPath != nil {
			serviceProps.CloudMapOptions.FailureThreshold = jsii.Number(2)
		}
	}

	m.Service = awsecs.NewFargateService(m.Construct, jsii.String("Service"), serviceProps)

	// Store service discovery endpoint
	if props.ServiceDiscovery != nil {
		m.ServiceDiscoveryEndpoint = jsii.String(fmt.Sprintf("%s.%s",
			*props.ServiceDiscovery.ServiceName,
			*props.ServiceDiscovery.Namespace))
	}
}

func (m *MicroserviceComplete) setupLoadBalancer(props *MicroserviceCompleteProps) {
	// Create Application Load Balancer
	m.LoadBalancer = awselasticloadbalancingv2.NewApplicationLoadBalancer(m.Construct, jsii.String("ALB"), &awselasticloadbalancingv2.ApplicationLoadBalancerProps{
		Vpc:            m.VPC,
		InternetFacing: jsii.Bool(true),
		Http2Enabled:   props.LoadBalancer.EnableHTTP2,
		IdleTimeout:    *props.LoadBalancer.IdleTimeout,
		VpcSubnets: &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PUBLIC,
		},
	})

	// Create target group
	m.TargetGroup = awselasticloadbalancingv2.NewApplicationTargetGroup(m.Construct, jsii.String("TargetGroup"), &awselasticloadbalancingv2.ApplicationTargetGroupProps{
		Vpc:        m.VPC,
		Port:       jsii.Number(*props.HealthCheck.Port),
		Protocol:   props.LoadBalancer.TargetGroupProtocol,
		TargetType: awselasticloadbalancingv2.TargetType_IP,
		HealthCheck: &awselasticloadbalancingv2.HealthCheck{
			Path:                    props.LoadBalancer.HealthCheckPath,
			HealthyHttpCodes:        jsii.String("200"),
			Interval:                *props.LoadBalancer.HealthCheckInterval,
			Timeout:                 *props.LoadBalancer.HealthCheckTimeout,
			HealthyThresholdCount:   jsii.Number(*props.LoadBalancer.HealthyThresholdCount),
			UnhealthyThresholdCount: jsii.Number(*props.LoadBalancer.UnhealthyThresholdCount),
		},
		DeregistrationDelay: *props.LoadBalancer.DeregistrationDelay,
		Targets: &[]awselasticloadbalancingv2.IApplicationLoadBalancerTarget{
			m.Service.LoadBalancerTarget(&awsecs.LoadBalancerTargetOptions{
				ContainerName: jsii.String("Container"),
				ContainerPort: jsii.Number(*props.HealthCheck.Port),
			}),
		},
	})

	// Note: Stickiness configuration would be done on the target group creation
	// The IApplicationTargetGroup interface doesn't expose ConfigureHealthCheck

	// Create HTTPS listener if certificate provided
	if props.LoadBalancer.Certificate != nil {
		m.Listener = m.LoadBalancer.AddListener(jsii.String("HTTPSListener"), &awselasticloadbalancingv2.BaseApplicationListenerProps{
			Port:     jsii.Number(443),
			Protocol: awselasticloadbalancingv2.ApplicationProtocol_HTTPS,
			Certificates: &[]awselasticloadbalancingv2.IListenerCertificate{
				props.LoadBalancer.Certificate,
			},
			DefaultTargetGroups: &[]awselasticloadbalancingv2.IApplicationTargetGroup{
				m.TargetGroup,
			},
		})

		// Add HTTP to HTTPS redirect if enabled
		if props.LoadBalancer.EnableSSLRedirect != nil && *props.LoadBalancer.EnableSSLRedirect {
			m.LoadBalancer.AddListener(jsii.String("HTTPListener"), &awselasticloadbalancingv2.BaseApplicationListenerProps{
				Port:     jsii.Number(80),
				Protocol: awselasticloadbalancingv2.ApplicationProtocol_HTTP,
				DefaultAction: awselasticloadbalancingv2.ListenerAction_Redirect(&awselasticloadbalancingv2.RedirectOptions{
					Protocol:  jsii.String("HTTPS"),
					Port:      jsii.String("443"),
					Permanent: jsii.Bool(true),
				}),
			})
		}
	} else {
		// Create HTTP listener
		m.Listener = m.LoadBalancer.AddListener(jsii.String("HTTPListener"), &awselasticloadbalancingv2.BaseApplicationListenerProps{
			Port:     jsii.Number(80),
			Protocol: awselasticloadbalancingv2.ApplicationProtocol_HTTP,
			DefaultTargetGroups: &[]awselasticloadbalancingv2.IApplicationTargetGroup{
				m.TargetGroup,
			},
		})
	}

	// Store load balancer DNS
	m.LoadBalancerDNS = m.LoadBalancer.LoadBalancerDnsName()
}

func (m *MicroserviceComplete) setupAutoScaling(props *MicroserviceCompleteProps) {
	// Create scalable target
	m.ScalableTarget = m.Service.AutoScaleTaskCount(&awsapplicationautoscaling.EnableScalingProps{
		MinCapacity: props.AutoScaling.MinCapacity,
		MaxCapacity: props.AutoScaling.MaxCapacity,
	})

	// CPU-based scaling
	m.ScalableTarget.ScaleOnCpuUtilization(jsii.String("CpuScaling"), &awsecs.CpuUtilizationScalingProps{
		TargetUtilizationPercent: props.AutoScaling.TargetCPUUtilization,
		ScaleInCooldown:          *props.AutoScaling.ScaleInCooldown,
		ScaleOutCooldown:         *props.AutoScaling.ScaleOutCooldown,
	})

	// Memory-based scaling
	m.ScalableTarget.ScaleOnMemoryUtilization(jsii.String("MemoryScaling"), &awsecs.MemoryUtilizationScalingProps{
		TargetUtilizationPercent: props.AutoScaling.TargetMemoryUtilization,
		ScaleInCooldown:          *props.AutoScaling.ScaleInCooldown,
		ScaleOutCooldown:         *props.AutoScaling.ScaleOutCooldown,
	})

	// Request-based scaling if load balancer is configured
	if m.TargetGroup != nil && props.AutoScaling.RequestsPerTarget != nil {
		m.ScalableTarget.ScaleOnRequestCount(jsii.String("RequestScaling"), &awsecs.RequestCountScalingProps{
			RequestsPerTarget: props.AutoScaling.RequestsPerTarget,
			// TargetGroup should be concrete type, not interface
			// This would need refactoring to work properly
			ScaleInCooldown:  *props.AutoScaling.ScaleInCooldown,
			ScaleOutCooldown: *props.AutoScaling.ScaleOutCooldown,
		})
	}

	// Scheduled scaling actions
	if props.AutoScaling.ScheduledScalingActions != nil {
		for _, action := range *props.AutoScaling.ScheduledScalingActions {
			m.ScalableTarget.ScaleOnSchedule(jsii.String(action.Name), &awsapplicationautoscaling.ScalingSchedule{
				Schedule:    awsapplicationautoscaling.Schedule_Expression(jsii.String(action.Schedule)),
				MinCapacity: action.MinCapacity,
				MaxCapacity: action.MaxCapacity,
				// TimeZone: action.Timezone, // TimeZone requires awscdk.TimeZone type
			})
		}
	}
}

func (m *MicroserviceComplete) setupEnhancedMonitoring(props *MicroserviceCompleteProps) {
	// Convert service to a monitorable resource (this would need interface implementation)
	m.Monitoring = liftconstructs.NewEnhancedMonitoring(m.Construct, jsii.String("Monitoring"), &liftconstructs.EnhancedMonitoringProps{
		// Resource: m.Service, // Would need to implement MonitorableResource interface
		Namespace:   jsii.String(fmt.Sprintf("Microservice/%s", *props.ServiceName)),
		Environment: props.Environment,
		MetricConfig: &liftconstructs.MetricConfiguration{
			DetailedMetrics:       jsii.Bool(true),
			EnableBusinessMetrics: jsii.Bool(true),
		},
		EnableRealTimeStreaming: jsii.Bool(true),
	})
}

func (m *MicroserviceComplete) setupEnhancedSecurity(props *MicroserviceCompleteProps) {
	// Create enhanced security configuration
	m.Security = liftconstructs.NewEnhancedSecurity(m.Construct, jsii.String("Security"), &liftconstructs.EnhancedSecurityProps{
		Vpc:               m.VPC,
		EnableWAF:         jsii.Bool(true),
		EnableVPCFlowLogs: jsii.Bool(true),
		Environment:       props.Environment,
		ApplicationName:   props.ServiceName,
		IngressRules: []liftconstructs.SecurityRule{
			{
				Port:        *props.HealthCheck.Port,
				Protocol:    awsec2.Protocol_TCP,
				Source:      awsec2.Peer_AnyIpv4(),
				Description: "Allow application traffic",
			},
		},
		EgressRules: []liftconstructs.SecurityRule{
			{
				Port:        443,
				Protocol:    awsec2.Protocol_TCP,
				Source:      awsec2.Peer_AnyIpv4(),
				Description: "Allow HTTPS outbound",
			},
		},
	})

	// Update service security groups if security is enabled
	if props.NetworkConfig.SecurityGroups == nil {
		props.NetworkConfig.SecurityGroups = &[]awsec2.ISecurityGroup{
			m.Security.GetSecurityGroup(),
		}
	}
}

func (m *MicroserviceComplete) createOutputs(props *MicroserviceCompleteProps) {
	// Service endpoint output
	if m.LoadBalancerDNS != nil {
		m.ServiceEndpoint = m.LoadBalancerDNS
		awscdk.NewCfnOutput(m.Construct, jsii.String("ServiceEndpoint"), &awscdk.CfnOutputProps{
			Value:       m.ServiceEndpoint,
			Description: jsii.String("Service endpoint URL"),
			ExportName:  jsii.String(fmt.Sprintf("%s-endpoint", *props.ServiceName)),
		})
	}

	// Service discovery endpoint output
	if m.ServiceDiscoveryEndpoint != nil {
		awscdk.NewCfnOutput(m.Construct, jsii.String("ServiceDiscoveryEndpoint"), &awscdk.CfnOutputProps{
			Value:       m.ServiceDiscoveryEndpoint,
			Description: jsii.String("Service discovery endpoint"),
			ExportName:  jsii.String(fmt.Sprintf("%s-discovery", *props.ServiceName)),
		})
	}

	// Cluster ARN output
	awscdk.NewCfnOutput(m.Construct, jsii.String("ClusterArn"), &awscdk.CfnOutputProps{
		Value:       m.Cluster.ClusterArn(),
		Description: jsii.String("ECS cluster ARN"),
		ExportName:  jsii.String(fmt.Sprintf("%s-cluster", *props.ServiceName)),
	})

	// Service ARN output
	awscdk.NewCfnOutput(m.Construct, jsii.String("ServiceArn"), &awscdk.CfnOutputProps{
		Value:       m.Service.ServiceArn(),
		Description: jsii.String("ECS service ARN"),
		ExportName:  jsii.String(fmt.Sprintf("%s-service", *props.ServiceName)),
	})
}

func (m *MicroserviceComplete) applyTags(props *MicroserviceCompleteProps) {
	// Apply default tags
	defaultTags := map[string]*string{
		"Environment": props.Environment,
		"ServiceName": props.ServiceName,
		"ManagedBy":   jsii.String("CDK"),
		"Project":     jsii.String("Lift"),
	}

	// Merge with custom tags
	if props.Tags != nil {
		for key, value := range *props.Tags {
			defaultTags[key] = value
		}
	}

	// Apply tags to all constructs
	for key, value := range defaultTags {
		awscdk.Tags_Of(m.Construct).Add(jsii.String(key), value, &awscdk.TagProps{})
	}
}

// GetService returns the ECS Fargate service
func (m *MicroserviceComplete) GetService() awsecs.FargateService {
	return m.Service
}

// GetCluster returns the ECS cluster
func (m *MicroserviceComplete) GetCluster() awsecs.ICluster {
	return m.Cluster
}

// GetLoadBalancer returns the application load balancer
func (m *MicroserviceComplete) GetLoadBalancer() awselasticloadbalancingv2.IApplicationLoadBalancer {
	return m.LoadBalancer
}

// GetServiceDiscoveryEndpoint returns the service discovery endpoint
func (m *MicroserviceComplete) GetServiceDiscoveryEndpoint() *string {
	return m.ServiceDiscoveryEndpoint
}

// GetServiceEndpoint returns the public service endpoint
func (m *MicroserviceComplete) GetServiceEndpoint() *string {
	return m.ServiceEndpoint
}

// GetMonitoring returns the enhanced monitoring construct
func (m *MicroserviceComplete) GetMonitoring() *liftconstructs.EnhancedMonitoring {
	return m.Monitoring
}

// GetSecurity returns the enhanced security construct
func (m *MicroserviceComplete) GetSecurity() *liftconstructs.EnhancedSecurity {
	return m.Security
}
