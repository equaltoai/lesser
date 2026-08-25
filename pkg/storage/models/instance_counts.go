package models

import (
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Instance-count rollup models.
//
// These are additive SK patterns on the main table that back the O(1) instance
// stats reads (see docs/architecture/dynamodb-scan-inventory.md). None of them
// populate a GSI: every read path is a point read by PK/SK, and the write paths
// are keyed updates (atomic Add / conditional create), so no table or index
// change is required to deploy them.
//
// Approximation note: the active-month window is the SUM of per-UTC-day
// distinct-actor counts. An actor active on multiple days inside the window is
// counted once per day, so the sum can exceed the true window-distinct count.
// This is documented as acceptable for the public instance stats surface.

// activeDayRetentionDays bounds rollup storage via TTL. It must exceed the
// widest window any reader requests (pkg/services/accounts reads 180 days for
// the half-year figure) plus margin.
const activeDayRetentionDays = 200

// ActivityDayCounter is the per-UTC-day distinct-actor counter maintained by
// the activity write path (repositories.RecordActivityActorDay).
type ActivityDayCounter struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"` // ACTIVITY_DAY#YYYY-MM-DD
	SK string `theorydb:"sk,attr:SK"` // COUNTER

	Date      string    `theorydb:"attr:date" json:"date"`   // YYYY-MM-DD (UTC)
	Value     int64     `theorydb:"attr:value" json:"value"` // distinct actors active that day
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL       int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing ActivityDayCounter.
func (ActivityDayCounter) TableName() string { return MainTableName }

// UpdateKeys derives PK/SK and TTL for ActivityDayCounter.
func (a *ActivityDayCounter) UpdateKeys() error {
	a.PK = "ACTIVITY_DAY#" + a.Date
	a.SK = "COUNTER"
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = time.Now()
	}
	a.TTL = a.UpdatedAt.Add(activeDayRetentionDays * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key.
func (a *ActivityDayCounter) GetPK() string { return a.PK }

// GetSK returns the sort key.
func (a *ActivityDayCounter) GetSK() string { return a.SK }

// ActivityActorDay is an idempotency marker that ensures an actor is counted
// at most once per UTC day in the active-month rollup. The write path creates
// it conditionally; only a newly-created marker bumps the day counter.
type ActivityActorDay struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"` // ACTIVITY_ACTOR#<actor ID>
	SK string `theorydb:"sk,attr:SK"` // DAY#YYYY-MM-DD

	ActorID   string    `theorydb:"attr:actorId" json:"actor_id"`
	Day       string    `theorydb:"attr:day" json:"day"` // YYYY-MM-DD (UTC)
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL       int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing ActivityActorDay.
func (ActivityActorDay) TableName() string { return MainTableName }

// UpdateKeys derives PK/SK and TTL for ActivityActorDay.
func (a *ActivityActorDay) UpdateKeys() error {
	a.PK = "ACTIVITY_ACTOR#" + a.ActorID
	a.SK = "DAY#" + a.Day
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = time.Now()
	}
	a.TTL = a.UpdatedAt.Add(activeDayRetentionDays * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key.
func (a *ActivityActorDay) GetPK() string { return a.PK }

// GetSK returns the sort key.
func (a *ActivityActorDay) GetSK() string { return a.SK }

// DomainCounter tracks how many actor records exist per domain (host of the
// actor ID) and doubles as the dedupe marker for the TOTAL_DOMAINS instance
// counter: the item is created on the first actor of a domain and removed when
// the last actor of that domain is deleted.
type DomainCounter struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"` // DOMAIN#<host>
	SK string `theorydb:"sk,attr:SK"` // COUNTER

	Domain    string    `theorydb:"attr:domain" json:"domain"`
	Value     int64     `theorydb:"attr:value" json:"value"` // actor count for the domain
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing DomainCounter.
func (DomainCounter) TableName() string { return MainTableName }

// UpdateKeys derives PK/SK for DomainCounter.
func (d *DomainCounter) UpdateKeys() error {
	d.PK = "DOMAIN#" + d.Domain
	d.SK = "COUNTER"
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = time.Now()
	}
	return nil
}

// GetPK returns the partition key.
func (d *DomainCounter) GetPK() string { return d.PK }

// GetSK returns the sort key.
func (d *DomainCounter) GetSK() string { return d.SK }

// ActivityDayKey returns the PK of the day counter for a UTC date string
// (common.DateFormat).
func ActivityDayKey(day string) string { return "ACTIVITY_DAY#" + day }

// DayCounterSK is the SK of the per-day counter item.
const DayCounterSK = "COUNTER"

// InstanceMetricsPK is the partition key of the shared instance-metrics item
// family that holds the TOTAL_* counters.
const InstanceMetricsPK = "INSTANCE#METRICS"

// TotalUsersMetricSK is the SK of the maintained TOTAL_USERS counter.
const TotalUsersMetricSK = "TOTAL_USERS"

// TotalDomainsMetricSK is the SK of the maintained TOTAL_DOMAINS counter.
const TotalDomainsMetricSK = "TOTAL_DOMAINS"

// TotalStatusesMetricSK is the SK of the maintained TOTAL_STATUSES counter.
const TotalStatusesMetricSK = "TOTAL_STATUSES"

// ActiveMonthSeedMetricSK is the SK of the one-time active-month seed marker.
const ActiveMonthSeedMetricSK = "SEED#ACTIVE_MONTH"

// DayFormat is the UTC date layout used for all day-bucketed rollup keys.
func DayFormat(t time.Time) string { return t.UTC().Format(common.DateFormat) }
