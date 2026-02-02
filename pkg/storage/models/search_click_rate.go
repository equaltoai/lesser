package models

import (
	"fmt"
	"time"
)

// SearchClickRate represents click-through rate tracking for search results
type SearchClickRate struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// Fields
	Query       string    `theorydb:"attr:query" json:"query"`
	ActorID     string    `theorydb:"attr:actorID" json:"actor_id"`
	ClickCount  int       `theorydb:"attr:clickCount" json:"click_count"`
	LastClicked time.Time `theorydb:"attr:lastClicked" json:"last_clicked"`
}

// TableName returns the DynamoDB table backing SearchClickRate.
func (SearchClickRate) TableName() string {
	return MainTableName
}

// UpdateKeys updates the partition and sort keys based on the model's attributes
func (c *SearchClickRate) UpdateKeys() {
	c.PK = fmt.Sprintf("CTR#%s", c.Query)
	c.SK = fmt.Sprintf(KeyPatternActor, c.ActorID)
}
