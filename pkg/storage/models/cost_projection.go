package models

import (
	"fmt"
	"time"
)

// CostProjection represents projected costs for federation
type CostProjection struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"` // COST#PROJECTION
	SK string `dynamorm:"sk" json:"-"` // {period}#{timestamp}

	// Attributes from interface
	Period          string       `json:"period"`
	CurrentCost     float64      `json:"current_cost"`
	ProjectedCost   float64      `json:"projected_cost"`
	Variance        float64      `json:"variance"`
	TopDrivers      []CostDriver `json:"top_drivers"`
	Recommendations []string     `json:"recommendations"`

	// Additional metadata
	Timestamp   time.Time `json:"timestamp"`
	CalculatedAt time.Time `json:"calculated_at"`
	TTL         int64     `json:"ttl,omitempty" dynamorm:"ttl"` // 90 days retention
}

// UpdateKeys updates the partition and sort keys
func (c *CostProjection) UpdateKeys() {
	c.PK = "COST#PROJECTION"
	c.SK = fmt.Sprintf("%s#%s", c.Period, c.Timestamp.Format(time.RFC3339))
	
	// Set TTL to 90 days from calculation
	c.TTL = c.CalculatedAt.AddDate(0, 3, 0).Unix()
}

// NewCostProjection creates a new cost projection
func NewCostProjection(period string) *CostProjection {
	now := time.Now().UTC()
	projection := &CostProjection{
		Period:          period,
		Timestamp:       now,
		CalculatedAt:    now,
		TopDrivers:      []CostDriver{},
		Recommendations: []string{},
	}
	projection.UpdateKeys()
	return projection
}

// GetCostProjectionKey returns the key for retrieving a specific projection
func GetCostProjectionKey(period string, timestamp time.Time) (pk, sk string) {
	return "COST#PROJECTION", fmt.Sprintf("%s#%s", period, timestamp.Format(time.RFC3339))
}

// GetLatestProjectionKeys returns keys for querying the latest projection for a period
func GetLatestProjectionKeys(period string) (pk, skPrefix string) {
	return "COST#PROJECTION", fmt.Sprintf("%s#", period)
}

// GetProjectionRangeKeys returns keys for querying projections in a time range
func GetProjectionRangeKeys(period string, startTime, endTime time.Time) (pk, skStart, skEnd string) {
	pk = "COST#PROJECTION"
	skStart = fmt.Sprintf("%s#%s", period, startTime.Format(time.RFC3339))
	skEnd = fmt.Sprintf("%s#%s", period, endTime.Format(time.RFC3339))
	return
}

// CalculateVariance calculates the variance between current and projected costs
func (c *CostProjection) CalculateVariance() {
	if c.CurrentCost > 0 {
		c.Variance = ((c.ProjectedCost - c.CurrentCost) / c.CurrentCost) * 100
	} else {
		c.Variance = 0
	}
}

// AddDriver adds a cost driver and sorts by cost
func (c *CostProjection) AddDriver(driver CostDriver) {
	c.TopDrivers = append(c.TopDrivers, driver)
	
	// Sort by cost descending and keep top 10
	for i := len(c.TopDrivers) - 1; i > 0; i-- {
		if c.TopDrivers[i].Cost > c.TopDrivers[i-1].Cost {
			c.TopDrivers[i], c.TopDrivers[i-1] = c.TopDrivers[i-1], c.TopDrivers[i]
		} else {
			break
		}
	}
	
	if len(c.TopDrivers) > 10 {
		c.TopDrivers = c.TopDrivers[:10]
	}
}

// GenerateRecommendations creates recommendations based on the projection
func (c *CostProjection) GenerateRecommendations() {
	c.Recommendations = []string{}
	
	// High variance recommendations
	if c.Variance > 20 {
		c.Recommendations = append(c.Recommendations, 
			fmt.Sprintf("Cost projected to increase by %.1f%% - review top cost drivers", c.Variance))
	} else if c.Variance < -20 {
		c.Recommendations = append(c.Recommendations,
			fmt.Sprintf("Cost projected to decrease by %.1f%% - maintain current optimizations", -c.Variance))
	}
	
	// Driver-specific recommendations
	for _, driver := range c.TopDrivers {
		if driver.PercentOfTotal > 30 {
			c.Recommendations = append(c.Recommendations,
				fmt.Sprintf("'%s' accounts for %.1f%% of costs - consider optimization", driver.Type, driver.PercentOfTotal))
		}
		
		if driver.Trend == "increasing" && driver.Cost > 100 {
			if driver.Domain != "" {
				c.Recommendations = append(c.Recommendations,
					fmt.Sprintf("Rising costs from %s - review federation activity", driver.Domain))
			} else {
				c.Recommendations = append(c.Recommendations,
					fmt.Sprintf("'%s' costs increasing - monitor closely", driver.Type))
			}
		}
	}
	
	// General recommendations
	if c.ProjectedCost > 1000 {
		c.Recommendations = append(c.Recommendations,
			"Consider implementing cost controls or budget alerts")
	}
}

// IsOverBudget checks if projected cost exceeds a threshold
func (c *CostProjection) IsOverBudget(budgetLimit float64) bool {
	return c.ProjectedCost > budgetLimit
}