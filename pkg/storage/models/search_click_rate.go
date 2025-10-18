package models

import (
	"fmt"
	"time"
)

// SearchClickRate represents click-through rate tracking for search results
type SearchClickRate struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Fields
	Query       string    `json:"query"`
	ActorID     string    `json:"actor_id"`
	ClickCount  int       `json:"click_count"`
	LastClicked time.Time `json:"last_clicked"`
}

// UpdateKeys updates the partition and sort keys based on the model's attributes
func (c *SearchClickRate) UpdateKeys() {
	c.PK = fmt.Sprintf("CTR#%s", c.Query)
	c.SK = fmt.Sprintf(KeyPatternActor, c.ActorID)
}
