package middleware

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/appmesh"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/google/uuid"

	"github.com/pay-theory/lift/pkg/lift"
)

// ServiceMeshConfig holds configuration for service mesh integration
type ServiceMeshConfig struct {
	MeshName            string        `json:"mesh_name"`
	VirtualNode         string        `json:"virtual_node"`
	ServiceName         string        `json:"service_name"`
	Namespace           string        `json:"namespace"`
	HealthCheckPath     string        `json:"health_check_path"`
	Port                string        `json:"port"`
	Region              string        `json:"region"`
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	HealthCheckTimeout  time.Duration `json:"health_check_timeout"`
}

// ServiceMeshAdapter provides AWS App Mesh integration
// Memory optimized: 184 → 168 bytes (16 bytes saved)
type ServiceMeshAdapter struct {
	registrationError error
	appMeshClient     *appmesh.Client
	sdClient          *servicediscovery.Client
	instanceID        string
	serviceID         string
	config            ServiceMeshConfig
	loggedError       bool
}

// NewServiceMeshAdapter creates a new service mesh adapter
func NewServiceMeshAdapter(meshConfig ServiceMeshConfig) (*ServiceMeshAdapter, error) {
	// Set defaults
	if meshConfig.HealthCheckPath == "" {
		meshConfig.HealthCheckPath = "/health"
	}
	if meshConfig.HealthCheckInterval == 0 {
		meshConfig.HealthCheckInterval = 30 * time.Second
	}
	if meshConfig.HealthCheckTimeout == 0 {
		meshConfig.HealthCheckTimeout = 5 * time.Second
	}
	if meshConfig.Port == "" {
		meshConfig.Port = "8080"
	}

	// Initialize AWS clients
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(meshConfig.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &ServiceMeshAdapter{
		config:        meshConfig,
		appMeshClient: appmesh.NewFromConfig(cfg),
		sdClient:      servicediscovery.NewFromConfig(cfg),
		instanceID:    fmt.Sprintf("%s-%s", meshConfig.ServiceName, uuid.New().String()),
	}, nil
}

// RegisterService registers the service with AWS Cloud Map
func (s *ServiceMeshAdapter) RegisterService(ctx context.Context) error {
	// Get service ID from Cloud Map
	listServicesResp, err := s.sdClient.ListServices(ctx, &servicediscovery.ListServicesInput{
		Filters: []types.ServiceFilter{
			{
				Name:   types.ServiceFilterNameNamespaceId,
				Values: []string{s.config.Namespace},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	// Find our service
	for _, service := range listServicesResp.Services {
		if aws.ToString(service.Name) == s.config.ServiceName {
			s.serviceID = aws.ToString(service.Id)
			break
		}
	}

	if s.serviceID == "" {
		return fmt.Errorf("service %s not found in namespace %s", s.config.ServiceName, s.config.Namespace)
	}

	// Register instance
	privateIP := s.getPrivateIP()
	_, err = s.sdClient.RegisterInstance(ctx, &servicediscovery.RegisterInstanceInput{
		ServiceId:  aws.String(s.serviceID),
		InstanceId: aws.String(s.instanceID),
		Attributes: map[string]string{
			"AWS_INSTANCE_IPV4": privateIP,
			"AWS_INSTANCE_PORT": s.config.Port,
			"VIRTUAL_NODE":      s.config.VirtualNode,
			"AVAILABILITY_ZONE": s.getAvailabilityZone(),
		},
	})

	if err != nil {
		return fmt.Errorf("failed to register service instance: %w", err)
	}

	return nil
}

// DeregisterService removes the service instance from AWS Cloud Map
func (s *ServiceMeshAdapter) DeregisterService(ctx context.Context) error {
	if s.serviceID == "" {
		return nil // Not registered
	}

	_, err := s.sdClient.DeregisterInstance(ctx, &servicediscovery.DeregisterInstanceInput{
		ServiceId:  aws.String(s.serviceID),
		InstanceId: aws.String(s.instanceID),
	})

	if err != nil {
		return fmt.Errorf("failed to deregister service instance: %w", err)
	}

	return nil
}

// Middleware returns the service mesh middleware
func (s *ServiceMeshAdapter) Middleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Log registration error once with proper context
			if s.registrationError != nil && !s.loggedError {
				ctx.Logger.Warn("Failed to register with service mesh", map[string]any{
					"error":        s.registrationError.Error(),
					"service_name": s.config.ServiceName,
					"mesh_name":    s.config.MeshName,
				})
				s.loggedError = true
			}

			// Add service mesh headers
			ctx.Response.Header("X-Service-Name", s.config.ServiceName)
			ctx.Response.Header("X-Virtual-Node", s.config.VirtualNode)
			ctx.Response.Header("X-Mesh-Name", s.config.MeshName)

			// Extract and propagate trace headers
			traceHeaders := s.extractTraceHeaders(ctx)
			for k, v := range traceHeaders {
				ctx.Set(k, v)
			}

			// Add service mesh metadata to context
			ctx.Set("mesh_name", s.config.MeshName)
			ctx.Set("virtual_node", s.config.VirtualNode)
			ctx.Set("service_name", s.config.ServiceName)

			// Handle health check requests
			if ctx.Request.Path == s.config.HealthCheckPath {
				return s.handleHealthCheck(ctx)
			}

			return next.Handle(ctx)
		})
	}
}

// HealthCheckHandler returns a health check handler
func (s *ServiceMeshAdapter) HealthCheckHandler() lift.Handler {
	return lift.HandlerFunc(func(ctx *lift.Context) error {
		return s.handleHealthCheck(ctx)
	})
}

// handleHealthCheck processes health check requests
func (s *ServiceMeshAdapter) handleHealthCheck(ctx *lift.Context) error {
	health := s.checkHealth(ctx)

	if !health.Healthy {
		return ctx.Response.Status(503).JSON(health)
	}

	return ctx.Response.JSON(health)
}

// ServiceMeshHealthStatus represents the health check response for service mesh
type ServiceMeshHealthStatus struct {
	Dependencies map[string]bool        `json:"dependencies,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	Service      string                 `json:"service"`
	VirtualNode  string                 `json:"virtual_node"`
	Healthy      bool                   `json:"healthy"`
}

// checkHealth performs health checks
func (s *ServiceMeshAdapter) checkHealth(_ *lift.Context) ServiceMeshHealthStatus {
	status := ServiceMeshHealthStatus{
		Healthy:      true,
		Service:      s.config.ServiceName,
		VirtualNode:  s.config.VirtualNode,
		Dependencies: make(map[string]bool),
		Metadata: map[string]interface{}{
			"instance_id": s.instanceID,
			"mesh_name":   s.config.MeshName,
			"timestamp":   time.Now().UTC(),
		},
	}

	// Add any dependency checks here
	// For example, check database connectivity, downstream services, etc.

	return status
}

// extractTraceHeaders extracts distributed tracing headers
func (s *ServiceMeshAdapter) extractTraceHeaders(ctx *lift.Context) map[string]string {
	traceHeaders := make(map[string]string)

	// X-Ray tracing headers
	if traceID := ctx.Header("X-Amzn-Trace-Id"); traceID != "" {
		traceHeaders["trace_id"] = traceID
	}

	// OpenTelemetry headers
	if traceParent := ctx.Header("traceparent"); traceParent != "" {
		traceHeaders["traceparent"] = traceParent
	}
	if traceState := ctx.Header("tracestate"); traceState != "" {
		traceHeaders["tracestate"] = traceState
	}

	// Jaeger headers
	if uberTraceID := ctx.Header("uber-trace-id"); uberTraceID != "" {
		traceHeaders["uber-trace-id"] = uberTraceID
	}

	// B3 headers (Zipkin)
	if b3TraceID := ctx.Header("X-B3-TraceId"); b3TraceID != "" {
		traceHeaders["X-B3-TraceId"] = b3TraceID
	}
	if b3SpanID := ctx.Header("X-B3-SpanId"); b3SpanID != "" {
		traceHeaders["X-B3-SpanId"] = b3SpanID
	}
	if b3ParentSpanID := ctx.Header("X-B3-ParentSpanId"); b3ParentSpanID != "" {
		traceHeaders["X-B3-ParentSpanId"] = b3ParentSpanID
	}
	if b3Sampled := ctx.Header("X-B3-Sampled"); b3Sampled != "" {
		traceHeaders["X-B3-Sampled"] = b3Sampled
	}

	return traceHeaders
}

// getPrivateIP returns the private IP address of the instance
func (s *ServiceMeshAdapter) getPrivateIP() string {
	// First, try to get from EC2 metadata
	if ip := s.getEC2PrivateIP(); ip != "" {
		return ip
	}

	// Fall back to local network interface
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return "127.0.0.1"
}

// getEC2PrivateIP attempts to get the private IP from EC2 metadata
func (s *ServiceMeshAdapter) getEC2PrivateIP() string {
	// This would normally use the EC2 metadata service
	// For Lambda, we can use environment variables
	if ip := os.Getenv("AWS_LAMBDA_FUNCTION_PRIVATE_IP"); ip != "" {
		return ip
	}

	// In ECS, check task metadata
	if ip := os.Getenv("ECS_TASK_PRIVATE_IP"); ip != "" {
		return ip
	}

	return ""
}

// getAvailabilityZone returns the availability zone
func (s *ServiceMeshAdapter) getAvailabilityZone() string {
	// Try various sources
	if az := os.Getenv("AWS_AVAILABILITY_ZONE"); az != "" {
		return az
	}

	if az := os.Getenv("AWS_DEFAULT_AVAILABILITY_ZONE"); az != "" {
		return az
	}

	// Default for Lambda
	if region := os.Getenv("AWS_REGION"); region != "" {
		return region + "a" // Default to first AZ
	}

	return "us-east-1a"
}

// ServiceMesh creates a service mesh middleware with the given configuration
func ServiceMesh(config ServiceMeshConfig) (lift.Middleware, error) {
	adapter, err := NewServiceMeshAdapter(config)
	if err != nil {
		return nil, err
	}

	// Register service on startup
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := adapter.RegisterService(ctx); err != nil {
		// Log error but don't fail startup
		// Will be logged when middleware is first used with proper context
		adapter.registrationError = err
	}

	return adapter.Middleware(), nil
}

// PropagateTraceHeaders is a helper middleware that propagates trace headers to outgoing requests
func PropagateTraceHeaders() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Store trace headers in context for outgoing requests
			traceHeaders := make(map[string]string)

			// Common trace header names
			headerNames := []string{
				"X-Amzn-Trace-Id",
				"traceparent",
				"tracestate",
				"uber-trace-id",
				"X-B3-TraceId",
				"X-B3-SpanId",
				"X-B3-ParentSpanId",
				"X-B3-Sampled",
			}

			for _, name := range headerNames {
				if value := ctx.Header(name); value != "" {
					traceHeaders[name] = value
				}
			}

			ctx.Set("trace_headers", traceHeaders)

			return next.Handle(ctx)
		})
	}
}
