package models

import (
	"fmt"
	"time"
)

// Driver represents a major cost contributor
type Driver struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK" json:"-"` // COST#DRIVER
	SK string `theorydb:"sk,attr:SK" json:"-"` // {category}#{resource}

	// Attributes from interface
	Type           string  `theorydb:"attr:type" json:"type"`
	Domain         string  `theorydb:"attr:domain" json:"domain,omitempty"`
	Cost           float64 `theorydb:"attr:cost" json:"cost"`
	PercentOfTotal float64 `theorydb:"attr:percentOfTotal" json:"percent_of_total"`
	Trend          string  `theorydb:"attr:trend" json:"trend"` // increasing/stable/decreasing

	// Additional attributes
	Category      string           `theorydb:"attr:category" json:"category"` // storage/compute/network/api
	Resource      string           `theorydb:"attr:resource" json:"resource"` // specific resource identifier
	Period        string           `theorydb:"attr:period" json:"period"`     // daily/weekly/monthly
	MeasuredAt    time.Time        `theorydb:"attr:measuredAt" json:"measured_at"`
	PreviousCost  float64          `theorydb:"attr:previousCost" json:"previous_cost"`             // For trend calculation
	VolumeMetrics map[string]int64 `theorydb:"attr:volumeMetrics" json:"volume_metrics,omitempty"` // requests, bytes, etc
	TTL           int64            `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`                  // 90 days retention
}

// UpdateKeys updates the partition and sort keys
func (c *Driver) UpdateKeys() {
	c.PK = CostDriverPK
	c.SK = fmt.Sprintf("%s#%s", c.Category, c.Resource)

	// Set TTL to 90 days from measurement
	c.TTL = c.MeasuredAt.AddDate(0, 3, 0).Unix()
}

// TableName returns the DynamoDB table backing Driver.
func (Driver) TableName() string {
	return MainTableName
}

// NewDriver creates a new cost driver
func NewDriver(category, resource string) *Driver {
	driver := &Driver{
		Category:      category,
		Resource:      resource,
		MeasuredAt:    time.Now().UTC(),
		VolumeMetrics: make(map[string]int64),
		Trend:         "stable",
	}
	driver.UpdateKeys()
	return driver
}

// GetCostDriverKey returns the key for retrieving a specific driver
func GetCostDriverKey(category, resource string) (pk, sk string) {
	return "COST#DRIVER", fmt.Sprintf("%s#%s", category, resource)
}

// GetCostDriversByCategoryKeys returns keys for querying all drivers in a category
func GetCostDriversByCategoryKeys(category string) (pk, skPrefix string) {
	return "COST#DRIVER", fmt.Sprintf("%s#", category)
}

// CalculateTrend determines the trend based on previous cost
func (c *Driver) CalculateTrend() {
	if c.PreviousCost == 0 {
		c.Trend = TrendStable
		return
	}

	changePercent := ((c.Cost - c.PreviousCost) / c.PreviousCost) * 100

	if changePercent > 10 {
		c.Trend = TrendIncreasing
	} else if changePercent < -10 {
		c.Trend = TrendDecreasing
	} else {
		c.Trend = TrendStable
	}
}

// SetVolumeMetric sets a volume metric value
func (c *Driver) SetVolumeMetric(metric string, value int64) {
	if c.VolumeMetrics == nil {
		c.VolumeMetrics = make(map[string]int64)
	}
	c.VolumeMetrics[metric] = value
}

// GetCostPerUnit calculates cost per unit for a given metric
func (c *Driver) GetCostPerUnit(metric string) float64 {
	if value, ok := c.VolumeMetrics[metric]; ok && value > 0 {
		return c.Cost / float64(value)
	}
	return 0
}

// DetermineCostType sets the Type field based on category and resource
func (c *Driver) DetermineCostType() {
	switch c.Category {
	case ResourceStorage:
		c.Type = fmt.Sprintf("Storage - %s", c.Resource)
	case ResourceCompute:
		c.Type = fmt.Sprintf("Compute - %s", c.Resource)
	case "network":
		if c.Domain != "" {
			c.Type = fmt.Sprintf("Network - Federation with %s", c.Domain)
		} else {
			c.Type = fmt.Sprintf("Network - %s", c.Resource)
		}
	case "api":
		c.Type = fmt.Sprintf("API Calls - %s", c.Resource)
	default:
		c.Type = fmt.Sprintf("%s - %s", c.Category, c.Resource)
	}
}

// IsSignificant checks if this driver is a significant cost contributor
func (c *Driver) IsSignificant() bool {
	return c.PercentOfTotal > 5 || c.Cost > 50
}

// FormatCostSummary returns a human-readable cost summary
func (c *Driver) FormatCostSummary() string {
	summary := fmt.Sprintf("%s: $%.2f (%.1f%%)", c.Type, c.Cost, c.PercentOfTotal)

	switch c.Trend {
	case TrendIncreasing:
		summary += " ↑"
	case TrendDecreasing:
		summary += " ↓"
	}

	return summary
}
