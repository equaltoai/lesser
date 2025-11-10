package models

import (
	"fmt"
	"time"
)

// SearchClickRate represents click-through rate tracking for search results
type SearchClickRate struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// Fields
	Query       string    `dynamorm:"attr:query" json:"query"`
	ActorID     string    `dynamorm:"attr:actorID" json:"actor_id"`
	ClickCount  int       `dynamorm:"attr:clickCount" json:"click_count"`
	LastClicked time.Time `dynamorm:"attr:lastClicked" json:"last_clicked"`
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
