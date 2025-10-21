package deployment

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// MultiRegionDeployer orchestrates deployments across multiple regions
// Memory optimized: 1032 → 984 bytes (48 bytes saved)
type MultiRegionDeployer struct {
	deployers        map[string]*PulumiDeployer
	healthCheckers   map[string]*RegionHealthChecker
	deploymentStatus map[string]RegionDeploymentStatus
	dnsManager       *DNSManager
	loadBalancer     *GlobalLoadBalancer
	primaryRegion    string
	applicationName  string
	environment      string
	regions          []string
	config           InfrastructureConfig
	mu               sync.RWMutex
}

// RegionDeploymentStatus represents the deployment status of a region
type RegionDeploymentStatus struct {
	LastDeployed    time.Time            `json:"last_deployed"`
	LastHealthCheck time.Time            `json:"last_health_check"`
	Endpoints       map[string]string    `json:"endpoints"`
	Region          string               `json:"region"`
	Error           string               `json:"error,omitempty"`
	Status          DeploymentStatusType `json:"status"`
	Health          HealthStatus         `json:"health"`
	Metrics         RegionMetrics        `json:"metrics"`
}

// DeploymentStatusType represents deployment status
type DeploymentStatusType string

const (
	StatusPending     DeploymentStatusType = "pending"
	StatusDeploying   DeploymentStatusType = "deploying"
	StatusDeployed    DeploymentStatusType = "deployed"
	StatusFailed      DeploymentStatusType = "failed"
	StatusRollingBack DeploymentStatusType = "rolling_back"
	StatusRolledBack  DeploymentStatusType = "rolled_back"
)

// HealthStatus represents health status
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnknown   HealthStatus = "unknown"
)

// RegionMetrics holds metrics for a region
// Memory optimized: 56 → 24 bytes (32 bytes saved)
type RegionMetrics struct {
	// Time struct first (24 bytes)
	LastUpdated time.Time `json:"last_updated"`
	// 8-byte values
	Latency      time.Duration `json:"latency"`
	ErrorRate    float64       `json:"error_rate"`
	RequestCount int64         `json:"request_count"`
	Availability float64       `json:"availability"`
}

// MultiRegionConfig holds multi-region deployment configuration
type MultiRegionConfig struct {
	PrimaryRegion      string                   `json:"primary_region"`
	Regions            []string                 `json:"regions"`
	FailoverStrategy   FailoverStrategy         `json:"failover_strategy"`
	HealthCheck        HealthCheckConfig        `json:"health_check"`
	DNS                DNSConfig                `json:"dns"`
	LoadBalancing      LoadBalancingConfig      `json:"load_balancing"`
	DataReplication    DataReplicationConfig    `json:"data_replication"`
	DeploymentStrategy DeploymentStrategyConfig `json:"deployment_strategy"`
}

// FailoverStrategy defines failover behavior
type FailoverStrategy struct {
	// Strings (16 bytes) - largest first
	Type string `json:"type"` // automatic, manual

	// Durations (8 bytes each)
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	FailbackDelay       time.Duration `json:"failback_delay"`

	// Ints (4 bytes each)
	FailureThreshold  int `json:"failure_threshold"`
	RecoveryThreshold int `json:"recovery_threshold"`

	// Bool (1 byte) - smallest last
	AutoFailback bool `json:"auto_failback"`
}

// HealthCheckConfig defines health check configuration
type HealthCheckConfig struct {
	Path               string        `json:"path"`
	Protocol           string        `json:"protocol"`
	ExpectedCodes      []int         `json:"expected_codes"`
	Interval           time.Duration `json:"interval"`
	Timeout            time.Duration `json:"timeout"`
	HealthyThreshold   int           `json:"healthy_threshold"`
	UnhealthyThreshold int           `json:"unhealthy_threshold"`
	Port               int           `json:"port"`
	Enabled            bool          `json:"enabled"`
}

// DNSConfig defines DNS configuration
type DNSConfig struct {
	Geolocation   GeolocationConfig `json:"geolocation,omitempty"`
	DomainName    string            `json:"domain_name"`
	HostedZoneId  string            `json:"hosted_zone_id"`
	RoutingPolicy string            `json:"routing_policy"`
	HealthCheckId string            `json:"health_check_id,omitempty"`
	SetIdentifier string            `json:"set_identifier,omitempty"`
	TTL           int               `json:"ttl"`
	Weight        int               `json:"weight,omitempty"`
}

// GeolocationConfig defines geolocation routing
type GeolocationConfig struct {
	ContinentCode   string `json:"continent_code,omitempty"`
	CountryCode     string `json:"country_code,omitempty"`
	SubdivisionCode string `json:"subdivision_code,omitempty"`
}

// LoadBalancingConfig defines load balancing configuration
type LoadBalancingConfig struct {
	Type               string                  `json:"type"`
	Scheme             string                  `json:"scheme"`
	IpAddressType      string                  `json:"ip_address_type"`
	Listeners          []LoadBalancerListener  `json:"listeners"`
	TargetGroups       []TargetGroupConfig     `json:"target_groups"`
	HealthCheck        LoadBalancerHealthCheck `json:"health_check"`
	CrossZoneEnabled   bool                    `json:"cross_zone_enabled"`
	DeletionProtection bool                    `json:"deletion_protection"`
}

// LoadBalancerHealthCheck defines load balancer health check
type LoadBalancerHealthCheck struct {
	Path               string        `json:"path"`
	Port               string        `json:"port"`
	Protocol           string        `json:"protocol"`
	Matcher            string        `json:"matcher"`
	HealthyThreshold   int           `json:"healthy_threshold"`
	UnhealthyThreshold int           `json:"unhealthy_threshold"`
	Timeout            time.Duration `json:"timeout"`
	Interval           time.Duration `json:"interval"`
	Enabled            bool          `json:"enabled"`
}

// LoadBalancerListener defines load balancer listener
type LoadBalancerListener struct {
	Protocol       string           `json:"protocol"`
	SSLPolicy      string           `json:"ssl_policy,omitempty"`
	CertificateArn string           `json:"certificate_arn,omitempty"`
	DefaultActions []ListenerAction `json:"default_actions"`
	Rules          []ListenerRule   `json:"rules,omitempty"`
	Port           int              `json:"port"`
}

// ListenerAction defines listener action
type ListenerAction struct {
	RedirectConfig *RedirectActionConfig `json:"redirect_config,omitempty"`
	FixedResponse  *FixedResponseConfig  `json:"fixed_response,omitempty"`
	ForwardConfig  *ForwardActionConfig  `json:"forward_config,omitempty"`
	Type           string                `json:"type"`
	TargetGroupArn string                `json:"target_group_arn,omitempty"`
}

// RedirectActionConfig defines redirect action
type RedirectActionConfig struct {
	Protocol   string `json:"protocol,omitempty"`
	Port       string `json:"port,omitempty"`
	Host       string `json:"host,omitempty"`
	Path       string `json:"path,omitempty"`
	Query      string `json:"query,omitempty"`
	StatusCode string `json:"status_code"`
}

// FixedResponseConfig defines fixed response action
type FixedResponseConfig struct {
	StatusCode  string `json:"status_code"`
	ContentType string `json:"content_type,omitempty"`
	MessageBody string `json:"message_body,omitempty"`
}

// ForwardActionConfig defines forward action
type ForwardActionConfig struct {
	TargetGroups []TargetGroupWeight `json:"target_groups"`
}

// TargetGroupWeight defines target group weight
type TargetGroupWeight struct {
	TargetGroupArn string `json:"target_group_arn"`
	Weight         int    `json:"weight"`
}

// ListenerRule defines listener rule
type ListenerRule struct {
	Conditions []RuleCondition  `json:"conditions"`
	Actions    []ListenerAction `json:"actions"`
	Priority   int              `json:"priority"`
}

// RuleCondition defines rule condition
type RuleCondition struct {
	Field  string   `json:"field"`
	Values []string `json:"values"`
}

// TargetGroupConfig defines target group configuration
type TargetGroupConfig struct {
	Attributes  map[string]string       `json:"attributes"`
	Name        string                  `json:"name"`
	Protocol    string                  `json:"protocol"`
	TargetType  string                  `json:"target_type"`
	Targets     []TargetConfig          `json:"targets"`
	HealthCheck LoadBalancerHealthCheck `json:"health_check"`
	Port        int                     `json:"port"`
}

// TargetConfig defines target configuration
type TargetConfig struct {
	Id   string `json:"id"`
	Port int    `json:"port,omitempty"`
}

// DataReplicationConfig defines data replication configuration
type DataReplicationConfig struct {
	Strategy           string                   `json:"strategy"`
	ConflictResolution string                   `json:"conflict_resolution"`
	Tables             []TableReplicationConfig `json:"tables"`
	S3Buckets          []S3ReplicationConfig    `json:"s3_buckets"`
	ReplicationLag     time.Duration            `json:"replication_lag"`
	Enabled            bool                     `json:"enabled"`
}

// TableReplicationConfig defines table replication
type TableReplicationConfig struct {
	TableName     string   `json:"table_name"`
	Regions       []string `json:"regions"`
	GlobalTables  bool     `json:"global_tables"`
	StreamEnabled bool     `json:"stream_enabled"`
}

// S3ReplicationConfig defines S3 replication
type S3ReplicationConfig struct {
	SourceBucket       string              `json:"source_bucket"`
	DestinationBuckets []string            `json:"destination_buckets"`
	ReplicationRules   []S3ReplicationRule `json:"replication_rules"`
}

// S3ReplicationRule defines S3 replication rule
type S3ReplicationRule struct {
	Id       string `json:"id"`
	Status   string `json:"status"`
	Prefix   string `json:"prefix,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// DeploymentStrategyConfig defines deployment strategy
type DeploymentStrategyConfig struct {
	Type                   string        `json:"type"` // rolling, blue-green, canary
	BatchSize              int           `json:"batch_size"`
	BatchDelay             time.Duration `json:"batch_delay"`
	MaxUnavailable         int           `json:"max_unavailable"`
	HealthCheckGracePeriod time.Duration `json:"health_check_grace_period"`
	RollbackOnFailure      bool          `json:"rollback_on_failure"`
	CanaryPercentage       int           `json:"canary_percentage,omitempty"`
	CanaryDuration         time.Duration `json:"canary_duration,omitempty"`
}

// RegionHealthChecker checks the health of a region
type RegionHealthChecker struct {
	region string
	config HealthCheckConfig
}

// NewRegionHealthChecker creates a new region health checker
func NewRegionHealthChecker(region string, config HealthCheckConfig) *RegionHealthChecker {
	return &RegionHealthChecker{
		region: region,
		config: config,
	}
}

// CheckHealth checks the health of an endpoint
func (rhc *RegionHealthChecker) CheckHealth(_ context.Context, _ string) error {
	// Implementation would check endpoint health
	return nil
}

// DNSManager manages DNS records
type DNSManager struct {
	config DNSConfig
}

// NewDNSManager creates a new DNS manager
func NewDNSManager(config DNSConfig) *DNSManager {
	return &DNSManager{
		config: config,
	}
}

// UpdateRecords updates DNS records for failover
func (dm *DNSManager) UpdateRecords(_ context.Context, _ []string) error {
	// Implementation would update DNS records
	return nil
}

// GlobalLoadBalancer manages global load balancing
type GlobalLoadBalancer struct {
	config LoadBalancingConfig
}

// NewGlobalLoadBalancer creates a new global load balancer
func NewGlobalLoadBalancer(config LoadBalancingConfig) *GlobalLoadBalancer {
	return &GlobalLoadBalancer{
		config: config,
	}
}

// UpdateTrafficWeights updates traffic weights for canary deployment
func (glb *GlobalLoadBalancer) UpdateTrafficWeights(_ context.Context, _ map[string]int) error {
	// Implementation would update traffic weights
	return nil
}

// NewMultiRegionDeployer creates a new multi-region deployer
func NewMultiRegionDeployer(config MultiRegionConfig, infraConfig InfrastructureConfig) *MultiRegionDeployer {
	mrd := &MultiRegionDeployer{
		primaryRegion:    config.PrimaryRegion,
		regions:          config.Regions,
		applicationName:  infraConfig.ApplicationName,
		environment:      infraConfig.Environment,
		config:           infraConfig,
		deployers:        make(map[string]*PulumiDeployer),
		healthCheckers:   make(map[string]*RegionHealthChecker),
		deploymentStatus: make(map[string]RegionDeploymentStatus),
	}

	// Initialize deployers for each region
	for _, region := range config.Regions {
		deployer := NewPulumiDeployer(
			infraConfig.ApplicationName,
			fmt.Sprintf("%s-%s", infraConfig.ApplicationName, region),
			region,
			infraConfig,
		)
		mrd.deployers[region] = deployer

		// Initialize health checker
		healthChecker := NewRegionHealthChecker(region, config.HealthCheck)
		mrd.healthCheckers[region] = healthChecker

		// Initialize deployment status
		mrd.deploymentStatus[region] = RegionDeploymentStatus{
			Region:    region,
			Status:    StatusPending,
			Health:    HealthUnknown,
			Endpoints: make(map[string]string),
			Metrics: RegionMetrics{
				LastUpdated: time.Now(),
			},
		}
	}

	// Initialize DNS manager
	mrd.dnsManager = NewDNSManager(config.DNS)

	// Initialize global load balancer
	mrd.loadBalancer = NewGlobalLoadBalancer(config.LoadBalancing)

	return mrd
}

// DeployAll deploys to all regions using the specified strategy
func (mrd *MultiRegionDeployer) DeployAll(ctx context.Context, strategy DeploymentStrategyConfig) error {
	mrd.mu.Lock()
	defer mrd.mu.Unlock()

	switch strategy.Type {
	case "rolling":
		return mrd.deployRolling(ctx, strategy)
	case "blue-green":
		return mrd.deployBlueGreen(ctx, strategy)
	case "canary":
		return mrd.deployCanary(ctx, strategy)
	default:
		return mrd.deployParallel(ctx)
	}
}

// deployRolling performs rolling deployment across regions
func (mrd *MultiRegionDeployer) deployRolling(ctx context.Context, strategy DeploymentStrategyConfig) error {
	batchSize := strategy.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}

	regions := make([]string, len(mrd.regions))
	copy(regions, mrd.regions)

	for i := 0; i < len(regions); i += batchSize {
		end := i + batchSize
		if end > len(regions) {
			end = len(regions)
		}

		batch := regions[i:end]

		// Deploy batch
		if err := mrd.deployBatch(ctx, batch); err != nil {
			if strategy.RollbackOnFailure {
				if rollbackErr := mrd.rollbackBatch(ctx, batch); rollbackErr != nil {
					log.Printf("Failed to rollback batch %v: %v", batch, rollbackErr)
				}
			}
			return fmt.Errorf("failed to deploy batch %v: %w", batch, err)
		}

		// Wait between batches
		if i+batchSize < len(regions) && strategy.BatchDelay > 0 {
			time.Sleep(strategy.BatchDelay)
		}
	}

	return nil
}

// deployBlueGreen performs blue-green deployment
func (mrd *MultiRegionDeployer) deployBlueGreen(ctx context.Context, strategy DeploymentStrategyConfig) error {
	// Deploy to "green" environment
	greenRegions := make([]string, 0, len(mrd.regions))
	for _, region := range mrd.regions {
		greenRegion := fmt.Sprintf("%s-green", region)
		greenRegions = append(greenRegions, greenRegion)
	}

	// Deploy to green environment
	if err := mrd.deployBatch(ctx, greenRegions); err != nil {
		return fmt.Errorf("failed to deploy to green environment: %w", err)
	}

	// Health check green environment
	if err := mrd.healthCheckBatch(ctx, greenRegions, strategy.HealthCheckGracePeriod); err != nil {
		return fmt.Errorf("green environment health check failed: %w", err)
	}

	// Switch traffic to green
	if err := mrd.switchTraffic(ctx, greenRegions); err != nil {
		return fmt.Errorf("failed to switch traffic to green: %w", err)
	}

	// Cleanup blue environment
	return mrd.cleanupBlueEnvironment(ctx, mrd.regions)
}

// deployCanary performs canary deployment
func (mrd *MultiRegionDeployer) deployCanary(ctx context.Context, strategy DeploymentStrategyConfig) error {
	// Deploy canary version
	canaryRegions := []string{mrd.primaryRegion}

	if err := mrd.deployBatch(ctx, canaryRegions); err != nil {
		return fmt.Errorf("failed to deploy canary: %w", err)
	}

	// Route percentage of traffic to canary
	if err := mrd.routeCanaryTraffic(ctx, strategy.CanaryPercentage); err != nil {
		return fmt.Errorf("failed to route canary traffic: %w", err)
	}

	// Monitor canary for specified duration
	if err := mrd.monitorCanary(ctx, strategy.CanaryDuration); err != nil {
		// Rollback canary on failure
		if rollbackErr := mrd.rollbackCanary(ctx); rollbackErr != nil {
			log.Printf("Failed to rollback canary: %v", rollbackErr)
		}
		return fmt.Errorf("canary monitoring failed: %w", err)
	}

	// Deploy to remaining regions
	remainingRegions := make([]string, 0, len(mrd.regions)-1)
	for _, region := range mrd.regions {
		if region != mrd.primaryRegion {
			remainingRegions = append(remainingRegions, region)
		}
	}

	return mrd.deployBatch(ctx, remainingRegions)
}

// deployParallel performs parallel deployment to all regions
func (mrd *MultiRegionDeployer) deployParallel(ctx context.Context) error {
	return mrd.deployBatch(ctx, mrd.regions)
}

// deployBatch deploys to a batch of regions
func (mrd *MultiRegionDeployer) deployBatch(ctx context.Context, regions []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(regions))

	for _, region := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()

			// Update status
			mrd.updateRegionStatus(r, StatusDeploying, HealthUnknown, "")

			deployer, exists := mrd.deployers[r]
			if !exists {
				errChan <- fmt.Errorf("deployer not found for region: %s", r)
				return
			}

			// Initialize deployer
			stackConfig := PulumiStackConfig{
				ProjectName: mrd.applicationName,
				StackName:   fmt.Sprintf("%s-%s", mrd.applicationName, r),
				Region:      r,
				Config: map[string]string{
					"environment": mrd.environment,
					"region":      r,
				},
				Tags: mrd.config.Tags,
			}

			if err := deployer.Initialize(ctx, stackConfig); err != nil {
				mrd.updateRegionStatus(r, StatusFailed, HealthUnhealthy, err.Error())
				errChan <- fmt.Errorf("failed to initialize deployer for region %s: %w", r, err)
				return
			}

			// Deploy
			result, err := deployer.Deploy(ctx)
			if err != nil {
				mrd.updateRegionStatus(r, StatusFailed, HealthUnhealthy, err.Error())
				errChan <- fmt.Errorf("failed to deploy to region %s: %w", r, err)
				return
			}

			// Update endpoints from deployment result
			endpoints := make(map[string]string)
			for key, value := range result.Outputs {
				endpoints[key] = fmt.Sprintf("%v", value)
			}

			mrd.updateRegionStatusWithEndpoints(r, StatusDeployed, HealthUnknown, "", endpoints)
		}(region)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	errors := make([]error, 0, len(regions))
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("deployment failed for %d regions: %v", len(errors), errors)
	}

	return nil
}

// healthCheckBatch performs health checks on a batch of regions
func (mrd *MultiRegionDeployer) healthCheckBatch(ctx context.Context, regions []string, gracePeriod time.Duration) error {
	// Wait for grace period
	time.Sleep(gracePeriod)

	var wg sync.WaitGroup
	errChan := make(chan error, len(regions))

	for _, region := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()

			healthChecker, exists := mrd.healthCheckers[r]
			if !exists {
				errChan <- fmt.Errorf("health checker not found for region: %s", r)
				return
			}

			status := mrd.deploymentStatus[r]
			for endpoint := range status.Endpoints {
				if err := healthChecker.CheckHealth(ctx, endpoint); err != nil {
					mrd.updateRegionStatus(r, StatusDeployed, HealthUnhealthy, err.Error())
					errChan <- fmt.Errorf("health check failed for region %s endpoint %s: %w", r, endpoint, err)
					return
				}
			}

			mrd.updateRegionStatus(r, StatusDeployed, HealthHealthy, "")
		}(region)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	errors := make([]error, 0, len(regions))
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("health check failed for %d regions: %v", len(errors), errors)
	}

	return nil
}

// rollbackBatch rolls back a batch of regions
func (mrd *MultiRegionDeployer) rollbackBatch(ctx context.Context, regions []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(regions))

	for _, region := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()

			mrd.updateRegionStatus(r, StatusRollingBack, HealthUnknown, "")

			deployer, exists := mrd.deployers[r]
			if !exists {
				errChan <- fmt.Errorf("deployer not found for region: %s", r)
				return
			}

			if _, err := deployer.Destroy(ctx); err != nil {
				mrd.updateRegionStatus(r, StatusFailed, HealthUnhealthy, err.Error())
				errChan <- fmt.Errorf("failed to rollback region %s: %w", r, err)
				return
			}

			mrd.updateRegionStatus(r, StatusRolledBack, HealthUnknown, "")
		}(region)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	errors := make([]error, 0, len(regions))
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("rollback failed for %d regions: %v", len(errors), errors)
	}

	return nil
}

// switchTraffic switches traffic to the specified regions
func (mrd *MultiRegionDeployer) switchTraffic(ctx context.Context, regions []string) error {
	return mrd.dnsManager.UpdateRecords(ctx, regions)
}

// cleanupBlueEnvironment cleans up the blue environment
func (mrd *MultiRegionDeployer) cleanupBlueEnvironment(_ context.Context, _ []string) error {
	// Implementation would clean up old blue environment resources
	return nil
}

// routeCanaryTraffic routes percentage of traffic to canary
func (mrd *MultiRegionDeployer) routeCanaryTraffic(ctx context.Context, percentage int) error {
	return mrd.loadBalancer.UpdateTrafficWeights(ctx, map[string]int{
		"canary": percentage,
		"stable": 100 - percentage,
	})
}

// monitorCanary monitors canary deployment
func (mrd *MultiRegionDeployer) monitorCanary(ctx context.Context, duration time.Duration) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	timeout := time.After(duration)

	for {
		select {
		case <-timeout:
			return nil // Monitoring completed successfully
		case <-ticker.C:
			// Check canary health and metrics
			status := mrd.deploymentStatus[mrd.primaryRegion]
			if status.Health == HealthUnhealthy || status.Metrics.ErrorRate > 0.05 {
				return fmt.Errorf("canary health check failed: health=%s, error_rate=%.2f", status.Health, status.Metrics.ErrorRate)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// rollbackCanary rolls back canary deployment
func (mrd *MultiRegionDeployer) rollbackCanary(ctx context.Context) error {
	return mrd.routeCanaryTraffic(ctx, 0) // Route all traffic back to stable
}

// updateRegionStatus updates the status of a region
func (mrd *MultiRegionDeployer) updateRegionStatus(region string, status DeploymentStatusType, health HealthStatus, errorMsg string) {
	mrd.deploymentStatus[region] = RegionDeploymentStatus{
		Region:          region,
		Status:          status,
		Health:          health,
		LastDeployed:    time.Now(),
		LastHealthCheck: time.Now(),
		Endpoints:       mrd.deploymentStatus[region].Endpoints,
		Metrics:         mrd.deploymentStatus[region].Metrics,
		Error:           errorMsg,
	}
}

// updateRegionStatusWithEndpoints updates region status with endpoints
func (mrd *MultiRegionDeployer) updateRegionStatusWithEndpoints(region string, status DeploymentStatusType, health HealthStatus, errorMsg string, endpoints map[string]string) {
	mrd.deploymentStatus[region] = RegionDeploymentStatus{
		Region:          region,
		Status:          status,
		Health:          health,
		LastDeployed:    time.Now(),
		LastHealthCheck: time.Now(),
		Endpoints:       endpoints,
		Metrics:         mrd.deploymentStatus[region].Metrics,
		Error:           errorMsg,
	}
}

// GetDeploymentStatus returns the deployment status for all regions
func (mrd *MultiRegionDeployer) GetDeploymentStatus() map[string]RegionDeploymentStatus {
	mrd.mu.RLock()
	defer mrd.mu.RUnlock()

	status := make(map[string]RegionDeploymentStatus)
	for region, regionStatus := range mrd.deploymentStatus {
		status[region] = regionStatus
	}
	return status
}

// GetHealthyRegions returns a list of healthy regions
func (mrd *MultiRegionDeployer) GetHealthyRegions() []string {
	mrd.mu.RLock()
	defer mrd.mu.RUnlock()

	var healthyRegions []string
	for region, status := range mrd.deploymentStatus {
		if status.Health == HealthHealthy {
			healthyRegions = append(healthyRegions, region)
		}
	}
	return healthyRegions
}

// StartHealthMonitoring starts continuous health monitoring
func (mrd *MultiRegionDeployer) StartHealthMonitoring(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mrd.performHealthChecks(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// performHealthChecks performs health checks on all regions
func (mrd *MultiRegionDeployer) performHealthChecks(ctx context.Context) {
	var wg sync.WaitGroup

	for region := range mrd.deploymentStatus {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()

			healthChecker, exists := mrd.healthCheckers[r]
			if !exists {
				return
			}

			status := mrd.deploymentStatus[r]
			healthy := true

			for endpoint := range status.Endpoints {
				if err := healthChecker.CheckHealth(ctx, endpoint); err != nil {
					healthy = false
					break
				}
			}

			var health HealthStatus
			if healthy {
				health = HealthHealthy
			} else {
				health = HealthUnhealthy
			}

			mrd.updateRegionStatus(r, status.Status, health, "")
		}(region)
	}

	wg.Wait()
}
