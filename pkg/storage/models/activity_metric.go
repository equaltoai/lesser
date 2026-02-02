package models

import (
	"fmt"
	"time"
)

// ActivityMetric represents a lightweight activity metric record written to the main table.
// These records power coarse-grained analytics such as push delivery counts.
type ActivityMetric struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"PK"`
	SK string `theorydb:"sk,attr:SK" json:"SK"`

	ActivityType string    `theorydb:"attr:activityType" json:"ActivityType"`
	ActorID      string    `theorydb:"attr:actorID" json:"ActorID"`
	Timestamp    string    `theorydb:"attr:timestamp" json:"Timestamp"`
	CreatedAt    time.Time `theorydb:"attr:createdAt" json:"CreatedAt"`
	Type         string    `theorydb:"attr:type" json:"Type"`
}

// TableName ensures activity metrics live in the shared single table.
func (ActivityMetric) TableName() string {
	return MainTableName
}

// NewActivityMetric constructs an activity metric record for the provided actor and activity type.
func NewActivityMetric(activityType, actorID string, timestamp time.Time) *ActivityMetric {
	timestampNano := timestamp.Format(time.RFC3339Nano)

	return &ActivityMetric{
		PK:           fmt.Sprintf("activity_metric#%s", actorID),
		SK:           fmt.Sprintf("%s#%s", activityType, timestampNano),
		ActivityType: activityType,
		ActorID:      actorID,
		Timestamp:    timestamp.Format(time.RFC3339),
		CreatedAt:    timestamp,
		Type:         "activity_metric",
	}
}
