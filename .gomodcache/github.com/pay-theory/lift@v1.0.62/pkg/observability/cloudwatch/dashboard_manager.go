package cloudwatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// CloudWatchDashboardClient defines the interface for CloudWatch dashboard operations
type CloudWatchDashboardClient interface {
	PutDashboard(ctx context.Context, params *cloudwatch.PutDashboardInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutDashboardOutput, error)
	GetDashboard(ctx context.Context, params *cloudwatch.GetDashboardInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetDashboardOutput, error)
	ListDashboards(ctx context.Context, params *cloudwatch.ListDashboardsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.ListDashboardsOutput, error)
	DeleteDashboards(ctx context.Context, params *cloudwatch.DeleteDashboardsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DeleteDashboardsOutput, error)
}

// DashboardManager manages CloudWatch dashboards with templates and automation
type DashboardManager struct {
	client             CloudWatchDashboardClient
	templates          map[string]*DashboardTemplate
	deployedDashboards map[string]*DeployedDashboard
	config             DashboardManagerConfig
	mu                 sync.RWMutex
}

// DashboardManagerConfig configures the dashboard manager
type DashboardManagerConfig struct {
	DefaultTags    map[string]string `json:"default_tags"`
	Namespace      string            `json:"namespace"`
	Environment    string            `json:"environment"`
	Region         string            `json:"region"`
	UpdateInterval time.Duration     `json:"update_interval"`
	AutoUpdate     bool              `json:"auto_update"`
	VersionControl bool              `json:"version_control"`
	BackupEnabled  bool              `json:"backup_enabled"`
}

// DashboardTemplate defines a dashboard template
type DashboardTemplate struct {
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	Variables   map[string]any   `json:"variables"`
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Category    string           `json:"category"`
	Version     string           `json:"version"`
	Widgets     []WidgetTemplate `json:"widgets"`
	Layout      DashboardLayout  `json:"layout"`
}

// WidgetTemplate defines a widget template
type WidgetTemplate struct {
	Properties map[string]any     `json:"properties"`
	Type       string             `json:"type"`
	Title      string             `json:"title"`
	Metrics    []MetricDefinition `json:"metrics"`
	Position   WidgetPosition     `json:"position"`
	Size       WidgetSize         `json:"size"`
}

// MetricDefinition defines a metric for dashboard widgets
type MetricDefinition struct {
	Dimensions map[string]string `json:"dimensions"`
	Namespace  string            `json:"namespace"`
	MetricName string            `json:"metric_name"`
	Statistic  string            `json:"statistic"`
	Label      string            `json:"label,omitempty"`
	Period     int32             `json:"period"`
}

// WidgetPosition defines widget position
type WidgetPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// WidgetSize defines widget size
type WidgetSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// DashboardLayout defines dashboard layout
type DashboardLayout struct {
	GridSize   GridSize `json:"grid_size"`
	AutoLayout bool     `json:"auto_layout"`
}

// GridSize defines grid size
type GridSize struct {
	Columns int `json:"columns"`
	Rows    int `json:"rows"`
}

// DeployedDashboard represents a deployed dashboard
type DeployedDashboard struct {
	Name        string          `json:"name"`
	TemplateID  string          `json:"template_id"`
	Version     string          `json:"version"`
	Variables   map[string]any  `json:"variables"`
	DeployedAt  time.Time       `json:"deployed_at"`
	LastUpdated time.Time       `json:"last_updated"`
	Status      DashboardStatus `json:"status"`
	ARN         string          `json:"arn,omitempty"`
}

// DashboardStatus represents dashboard status
type DashboardStatus string

const (
	DashboardStatusActive  DashboardStatus = "active"
	DashboardStatusFailed  DashboardStatus = "failed"
	DashboardStatusPending DashboardStatus = "pending"
	DashboardStatusDeleted DashboardStatus = "deleted"
)

// NewDashboardManager creates a new dashboard manager
func NewDashboardManager(client CloudWatchDashboardClient, config DashboardManagerConfig) *DashboardManager {
	return &DashboardManager{
		client:             client,
		config:             config,
		templates:          make(map[string]*DashboardTemplate),
		deployedDashboards: make(map[string]*DeployedDashboard),
	}
}

// RegisterTemplate registers a dashboard template
func (dm *DashboardManager) RegisterTemplate(template *DashboardTemplate) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if template.ID == "" {
		return fmt.Errorf("template ID cannot be empty")
	}

	template.UpdatedAt = time.Now()
	if template.CreatedAt.IsZero() {
		template.CreatedAt = time.Now()
	}

	dm.templates[template.ID] = template
	return nil
}

// CreateDashboard creates a dashboard from a template
func (dm *DashboardManager) CreateDashboard(ctx context.Context, templateID, dashboardName string, variables map[string]any) error {
	dm.mu.RLock()
	template, exists := dm.templates[templateID]
	dm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("template %s not found", templateID)
	}

	// Build dashboard JSON
	dashboardBody, err := dm.buildDashboardBody(template, variables)
	if err != nil {
		return fmt.Errorf("failed to build dashboard body: %w", err)
	}

	// Deploy dashboard
	input := &cloudwatch.PutDashboardInput{
		DashboardName: aws.String(dashboardName),
		DashboardBody: aws.String(dashboardBody),
	}

	_, err = dm.client.PutDashboard(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create dashboard: %w", err)
	}

	// Track deployed dashboard
	deployed := &DeployedDashboard{
		Name:        dashboardName,
		TemplateID:  templateID,
		Version:     template.Version,
		Variables:   variables,
		DeployedAt:  time.Now(),
		LastUpdated: time.Now(),
		Status:      DashboardStatusActive,
	}

	dm.mu.Lock()
	dm.deployedDashboards[dashboardName] = deployed
	dm.mu.Unlock()

	return nil
}

// UpdateDashboard updates an existing dashboard
func (dm *DashboardManager) UpdateDashboard(ctx context.Context, dashboardName string, variables map[string]any) error {
	dm.mu.RLock()
	deployed, exists := dm.deployedDashboards[dashboardName]
	if !exists {
		dm.mu.RUnlock()
		return fmt.Errorf("deployed dashboard %s not found", dashboardName)
	}

	template, templateExists := dm.templates[deployed.TemplateID]
	dm.mu.RUnlock()

	if !templateExists {
		return fmt.Errorf("template %s for dashboard %s not found", deployed.TemplateID, dashboardName)
	}

	// Merge variables
	mergedVariables := make(map[string]any)
	for k, v := range deployed.Variables {
		mergedVariables[k] = v
	}
	for k, v := range variables {
		mergedVariables[k] = v
	}

	// Build updated dashboard JSON
	dashboardBody, err := dm.buildDashboardBody(template, mergedVariables)
	if err != nil {
		return fmt.Errorf("failed to build dashboard body: %w", err)
	}

	// Update dashboard
	input := &cloudwatch.PutDashboardInput{
		DashboardName: aws.String(dashboardName),
		DashboardBody: aws.String(dashboardBody),
	}

	_, err = dm.client.PutDashboard(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update dashboard: %w", err)
	}

	// Update tracking
	dm.mu.Lock()
	deployed.Variables = mergedVariables
	deployed.LastUpdated = time.Now()
	deployed.Version = template.Version
	dm.mu.Unlock()

	return nil
}

// DeleteDashboard deletes a dashboard
func (dm *DashboardManager) DeleteDashboard(ctx context.Context, dashboardName string) error {
	input := &cloudwatch.DeleteDashboardsInput{
		DashboardNames: []string{dashboardName},
	}

	_, err := dm.client.DeleteDashboards(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete dashboard: %w", err)
	}

	// Update tracking
	dm.mu.Lock()
	if deployed, exists := dm.deployedDashboards[dashboardName]; exists {
		deployed.Status = DashboardStatusDeleted
		deployed.LastUpdated = time.Now()
	}
	dm.mu.Unlock()

	return nil
}

// ListDashboards lists all dashboards
func (dm *DashboardManager) ListDashboards(ctx context.Context) ([]types.DashboardEntry, error) {
	input := &cloudwatch.ListDashboardsInput{}

	var allDashboards []types.DashboardEntry

	for {
		output, err := dm.client.ListDashboards(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list dashboards: %w", err)
		}

		allDashboards = append(allDashboards, output.DashboardEntries...)

		if output.NextToken == nil {
			break
		}
		input.NextToken = output.NextToken
	}

	return allDashboards, nil
}

// GetDashboard retrieves dashboard details
func (dm *DashboardManager) GetDashboard(ctx context.Context, dashboardName string) (*cloudwatch.GetDashboardOutput, error) {
	input := &cloudwatch.GetDashboardInput{
		DashboardName: aws.String(dashboardName),
	}

	output, err := dm.client.GetDashboard(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboard: %w", err)
	}

	return output, nil
}

// buildDashboardBody builds the dashboard JSON body from template and variables
func (dm *DashboardManager) buildDashboardBody(template *DashboardTemplate, variables map[string]any) (string, error) {
	// Create dashboard structure
	dashboard := map[string]any{
		"widgets": []map[string]any{},
	}

	// Add default variables
	allVariables := make(map[string]any)

	// Start with default config values
	allVariables["namespace"] = dm.config.Namespace
	allVariables["environment"] = dm.config.Environment
	allVariables["region"] = dm.config.Region

	// Add template variables
	for k, v := range template.Variables {
		allVariables[k] = v
	}

	// Override with provided variables
	for k, v := range variables {
		allVariables[k] = v
	}

	// Build widgets
	for _, widgetTemplate := range template.Widgets {
		widget := map[string]any{
			"type":       widgetTemplate.Type,
			"x":          widgetTemplate.Position.X,
			"y":          widgetTemplate.Position.Y,
			"width":      widgetTemplate.Size.Width,
			"height":     widgetTemplate.Size.Height,
			"properties": dm.buildWidgetProperties(widgetTemplate, allVariables),
		}

		widgets, ok := dashboard["widgets"].([]map[string]any)
		if !ok {
			widgets = []map[string]any{}
		}
		dashboard["widgets"] = append(widgets, widget)
	}

	// Convert to JSON
	dashboardJSON, err := json.Marshal(dashboard)
	if err != nil {
		return "", fmt.Errorf("failed to marshal dashboard JSON: %w", err)
	}

	return string(dashboardJSON), nil
}

// buildWidgetProperties builds widget properties with variable substitution
func (dm *DashboardManager) buildWidgetProperties(widgetTemplate WidgetTemplate, variables map[string]any) map[string]any {
	properties := make(map[string]any)

	// Copy base properties
	for k, v := range widgetTemplate.Properties {
		properties[k] = dm.substituteVariables(v, variables)
	}

	// Add title
	properties["title"] = dm.substituteVariables(widgetTemplate.Title, variables)

	// Build metrics if present
	if len(widgetTemplate.Metrics) > 0 {
		metrics := [][]any{}
		for _, metric := range widgetTemplate.Metrics {
			metricArray := []any{
				dm.substituteVariables(metric.Namespace, variables),
				dm.substituteVariables(metric.MetricName, variables),
			}

			// Add dimensions
			for name, value := range metric.Dimensions {
				metricArray = append(metricArray,
					dm.substituteVariables(name, variables),
					dm.substituteVariables(value, variables),
				)
			}

			// Add options if needed
			if metric.Statistic != "" || metric.Period != 0 || metric.Label != "" {
				options := make(map[string]any)
				if metric.Statistic != "" {
					options["stat"] = metric.Statistic
				}
				if metric.Period != 0 {
					options["period"] = metric.Period
				}
				if metric.Label != "" {
					options["label"] = dm.substituteVariables(metric.Label, variables)
				}
				metricArray = append(metricArray, options)
			}

			metrics = append(metrics, metricArray)
		}
		properties["metrics"] = metrics
	}

	// Set region
	properties["region"] = dm.config.Region

	return properties
}

// substituteVariables performs variable substitution in strings
func (dm *DashboardManager) substituteVariables(value any, variables map[string]any) any {
	if str, ok := value.(string); ok {
		// Simple variable substitution - replace ${var} with variable value
		for varName, varValue := range variables {
			placeholder := fmt.Sprintf("${%s}", varName)
			if strValue, ok := varValue.(string); ok {
				str = strings.ReplaceAll(str, placeholder, strValue)
			} else {
				str = strings.ReplaceAll(str, placeholder, fmt.Sprintf("%v", varValue))
			}
		}
		return str
	}
	return value
}

// GetDeployedDashboards returns all deployed dashboards
func (dm *DashboardManager) GetDeployedDashboards() map[string]*DeployedDashboard {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]*DeployedDashboard)
	for k, v := range dm.deployedDashboards {
		result[k] = v
	}
	return result
}

// GetTemplate returns a specific template
func (dm *DashboardManager) GetTemplate(templateID string) (*DashboardTemplate, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	template, exists := dm.templates[templateID]
	return template, exists
}

// GetTemplates returns all registered templates
func (dm *DashboardManager) GetTemplates() map[string]*DashboardTemplate {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]*DashboardTemplate)
	for k, v := range dm.templates {
		result[k] = v
	}
	return result
}

// SyncDashboards synchronizes deployed dashboards with CloudWatch
func (dm *DashboardManager) SyncDashboards(ctx context.Context) error {
	// Get all dashboards from CloudWatch
	cloudWatchDashboards, err := dm.ListDashboards(ctx)
	if err != nil {
		return fmt.Errorf("failed to list CloudWatch dashboards: %w", err)
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Mark all as potentially deleted
	for _, deployed := range dm.deployedDashboards {
		deployed.Status = DashboardStatusDeleted
	}

	// Check which ones actually exist
	for _, cwDashboard := range cloudWatchDashboards {
		if deployed, exists := dm.deployedDashboards[*cwDashboard.DashboardName]; exists {
			deployed.Status = DashboardStatusActive
			deployed.LastUpdated = *cwDashboard.LastModified
		}
	}

	return nil
}

// StartAutoUpdate starts automatic dashboard updates
func (dm *DashboardManager) StartAutoUpdate(ctx context.Context) {
	if !dm.config.AutoUpdate {
		return
	}

	go func() {
		ticker := time.NewTicker(dm.config.UpdateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := dm.SyncDashboards(ctx); err != nil {
					// Log error but continue running
					log.Printf("Failed to sync dashboards: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
