# Cost-Aware Federation Implementation Guide for Lesser

## 🎯 Overview

Cost-Aware Federation is a unique Lesser feature that brings transparency and sustainability to the Fediverse by tracking and managing the real costs of federation activities.

## 💰 Why Cost-Aware Federation?

### Current Problems in Fediverse
1. **Hidden Costs**: Instance operators bear unknown costs
2. **Unsustainable Growth**: Popular instances can't afford growth
3. **No Cost Attribution**: Can't identify expensive federation partners
4. **Resource Abuse**: No way to limit costly operations

### Lesser's Solution
- Track every federation operation's cost
- Set federation budgets
- Transparent cost sharing
- Automatic throttling of expensive operations

## 🏗️ Architecture

### Core Components

```go
// pkg/federation/cost/types.go
type FederationCost struct {
    InstanceDomain   string
    Period          time.Time // Daily aggregation
    InboundCosts    CostBreakdown
    OutboundCosts   CostBreakdown
    TotalCost       float64
    BudgetRemaining float64
}

type CostBreakdown struct {
    DynamoReads     int64
    DynamoWrites    int64
    S3Gets          int64
    S3Puts          int64
    DataTransferGB  float64
    LambdaInvocations int64
    MediaProcessing float64
    EstimatedCost   float64
}

type FederationBudget struct {
    InstanceDomain  string
    DailyLimit     float64
    MonthlyLimit   float64
    CurrentDaily   float64
    CurrentMonthly float64
    Actions        []BudgetAction
}

type BudgetAction struct {
    Threshold   float64 // Percentage of budget
    Action      ActionType
    Parameters  map[string]interface{}
}

type ActionType string

const (
    ActionLog        ActionType = "log"
    ActionThrottle   ActionType = "throttle"
    ActionMediaOnly  ActionType = "media_only"
    ActionTextOnly   ActionType = "text_only"
    ActionSuspend    ActionType = "suspend"
    ActionNotify     ActionType = "notify"
)
```

## 📊 Cost Tracking Implementation

### Per-Operation Tracking

```go
// pkg/federation/cost/tracker.go
type FederationCostTracker struct {
    storage     Storage
    calculator  CostCalculator
    aggregator  chan CostEvent
}

type CostEvent struct {
    Timestamp      time.Time
    InstanceDomain string
    Direction      string // "inbound" or "outbound"
    ActivityType   string
    ResourceUsage  ResourceUsage
}

type ResourceUsage struct {
    DynamoReads    int
    DynamoWrites   int
    S3Operations   int
    DataBytes      int64
    MediaProcessed bool
    CacheHit       bool
}

func (t *FederationCostTracker) TrackInboundActivity(ctx context.Context, activity *activitypub.Activity, instance string) {
    usage := t.calculateResourceUsage(activity)
    
    event := CostEvent{
        Timestamp:      time.Now(),
        InstanceDomain: instance,
        Direction:      "inbound",
        ActivityType:   activity.Type,
        ResourceUsage:  usage,
    }
    
    t.aggregator <- event
}

func (t *FederationCostTracker) calculateResourceUsage(activity *activitypub.Activity) ResourceUsage {
    usage := ResourceUsage{}
    
    switch activity.Type {
    case "Create":
        usage.DynamoWrites = 1 // Store activity
        if note, ok := activity.Object.(*activitypub.Note); ok {
            if len(note.Attachment) > 0 {
                usage.S3Operations = len(note.Attachment)
                usage.MediaProcessed = true
            }
            usage.DataBytes = int64(len(note.Content))
        }
        
    case "Follow":
        usage.DynamoWrites = 2 // Relationship + notification
        usage.DynamoReads = 1  // Check existing
        
    case "Like", "Announce":
        usage.DynamoWrites = 1
        usage.DynamoReads = 1
        
    case "Update":
        usage.DynamoWrites = 1
        usage.DynamoReads = 1
        
    case "Delete":
        usage.DynamoWrites = 1 // Tombstone
        usage.DynamoReads = 1
    }
    
    return usage
}
```

### Cost Calculation

```go
// pkg/federation/cost/calculator.go
type CostCalculator struct {
    rates CostRates
}

type CostRates struct {
    DynamoReadUnit   float64 // $0.00013 per RCU
    DynamoWriteUnit  float64 // $0.00065 per WCU
    S3Get           float64 // $0.0004 per 1000
    S3Put           float64 // $0.005 per 1000
    DataTransferGB  float64 // $0.09 per GB
    LambdaInvoke    float64 // $0.0000002 per request
    LambdaGBSecond  float64 // $0.0000166667 per GB-second
    MediaConvert    MediaConvertRates
}

func (c *CostCalculator) Calculate(usage ResourceUsage) float64 {
    cost := 0.0
    
    // DynamoDB costs
    cost += float64(usage.DynamoReads) * c.rates.DynamoReadUnit
    cost += float64(usage.DynamoWrites) * c.rates.DynamoWriteUnit
    
    // S3 costs
    cost += float64(usage.S3Operations) * c.rates.S3Get / 1000
    
    // Data transfer
    gbTransferred := float64(usage.DataBytes) / 1024 / 1024 / 1024
    cost += gbTransferred * c.rates.DataTransferGB
    
    // Media processing
    if usage.MediaProcessed {
        cost += c.estimateMediaCost(usage)
    }
    
    return cost
}
```

## 📈 Cost Aggregation

### Real-time Aggregation

```go
// pkg/federation/cost/aggregator.go
func (a *Aggregator) Run(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Minute)
    buffer := make(map[string]*FederationCost)
    
    for {
        select {
        case event := <-a.events:
            key := fmt.Sprintf("%s:%s", event.InstanceDomain, event.Timestamp.Format("2006-01-02"))
            
            if buffer[key] == nil {
                buffer[key] = &FederationCost{
                    InstanceDomain: event.InstanceDomain,
                    Period:        event.Timestamp.Truncate(24 * time.Hour),
                }
            }
            
            cost := a.calculator.Calculate(event.ResourceUsage)
            
            if event.Direction == "inbound" {
                a.updateCostBreakdown(&buffer[key].InboundCosts, event.ResourceUsage, cost)
            } else {
                a.updateCostBreakdown(&buffer[key].OutboundCosts, event.ResourceUsage, cost)
            }
            
            buffer[key].TotalCost = buffer[key].InboundCosts.EstimatedCost + 
                                   buffer[key].OutboundCosts.EstimatedCost
            
        case <-ticker.C:
            // Persist aggregated costs
            a.persistCosts(ctx, buffer)
            
            // Check budgets
            a.checkBudgets(ctx, buffer)
            
            // Clear old entries
            a.cleanupBuffer(buffer)
            
        case <-ctx.Done():
            return
        }
    }
}
```

## 🛡️ Budget Enforcement

### Budget Policy Engine

```go
// pkg/federation/cost/budget.go
type BudgetEnforcer struct {
    storage  Storage
    notifier Notifier
    limiter  RateLimiter
}

func (e *BudgetEnforcer) CheckAndEnforce(ctx context.Context, instance string, currentCost float64) error {
    budget, err := e.storage.GetFederationBudget(ctx, instance)
    if err != nil {
        return err
    }
    
    percentUsed := (currentCost / budget.DailyLimit) * 100
    
    for _, action := range budget.Actions {
        if percentUsed >= action.Threshold {
            if err := e.executeAction(ctx, instance, action); err != nil {
                return err
            }
        }
    }
    
    return nil
}

func (e *BudgetEnforcer) executeAction(ctx context.Context, instance string, action BudgetAction) error {
    switch action.Action {
    case ActionThrottle:
        rate := action.Parameters["rate"].(float64)
        e.limiter.SetRate(instance, rate)
        
    case ActionMediaOnly:
        e.storage.SetFederationMode(ctx, instance, "media_only")
        
    case ActionTextOnly:
        e.storage.SetFederationMode(ctx, instance, "text_only")
        
    case ActionSuspend:
        e.storage.SuspendFederation(ctx, instance)
        e.notifier.NotifyAdmin(ctx, fmt.Sprintf("Federation with %s suspended due to budget", instance))
        
    case ActionNotify:
        e.notifier.NotifyInstance(ctx, instance, "Approaching federation budget limit")
    }
    
    return nil
}
```

## 🔄 Federation Headers

### Cost Transparency Protocol

```go
// pkg/federation/delivery/headers.go
func (d *Delivery) addCostHeaders(req *http.Request, activity *activitypub.Activity) {
    // Add cost estimate headers
    req.Header.Set("X-Federation-Cost-Estimate", fmt.Sprintf("%.6f", d.estimateCost(activity)))
    req.Header.Set("X-Federation-Cost-Currency", "USD")
    
    // Add current usage
    dailyCost := d.tracker.GetDailyCost(req.Host)
    req.Header.Set("X-Federation-Daily-Cost", fmt.Sprintf("%.6f", dailyCost))
    
    // Add budget info if available
    if budget := d.getBudget(req.Host); budget != nil {
        req.Header.Set("X-Federation-Daily-Budget", fmt.Sprintf("%.2f", budget.DailyLimit))
        req.Header.Set("X-Federation-Budget-Remaining", fmt.Sprintf("%.2f", budget.BudgetRemaining))
    }
}

// Receiving side
func (h *InboxHandler) extractCostInfo(r *http.Request) *CostInfo {
    return &CostInfo{
        EstimatedCost:   parseFloat(r.Header.Get("X-Federation-Cost-Estimate")),
        DailyCost:      parseFloat(r.Header.Get("X-Federation-Daily-Cost")),
        DailyBudget:    parseFloat(r.Header.Get("X-Federation-Daily-Budget")),
        BudgetRemaining: parseFloat(r.Header.Get("X-Federation-Budget-Remaining")),
    }
}
```

## 💻 GraphQL API

### Cost Queries

```graphql
type FederationCostSummary {
  instance: String!
  period: DateTime!
  inboundCost: Float!
  outboundCost: Float!
  totalCost: Float!
  breakdown: CostBreakdown!
  budgetUsage: BudgetUsage
}

type CostBreakdown {
  storage: Float!
  dataTransfer: Float!
  mediaProcessing: Float!
  compute: Float!
}

type BudgetUsage {
  dailyLimit: Float!
  dailyUsed: Float!
  monthlyLimit: Float!
  monthlyUsed: Float!
  percentUsed: Float!
  status: BudgetStatus!
}

enum BudgetStatus {
  NORMAL
  WARNING
  THROTTLED
  SUSPENDED
}

extend type Query {
  federationCosts(
    instance: String
    startDate: DateTime!
    endDate: DateTime!
  ): [FederationCostSummary!]!
  
  federationBudget(instance: String!): FederationBudget
  
  costlyInstances(
    limit: Int
    period: TimePeriod
  ): [FederationCostSummary!]!
}

extend type Mutation {
  setFederationBudget(
    instance: String!
    dailyLimit: Float!
    monthlyLimit: Float!
    actions: [BudgetActionInput!]!
  ): FederationBudget!
  
  resetFederationThrottle(
    instance: String!
  ): ResetThrottlePayload!
}
```

## 📊 Admin Dashboard

### Cost Analytics

```go
type CostAnalytics struct {
    TotalDailyCost      float64
    TotalMonthlyCost    float64
    TopCostlyInstances  []InstanceCost
    CostTrend          []DailyCost
    ProjectedMonthly    float64
    BudgetAlerts       []BudgetAlert
}

func (a *AnalyticsService) GetFederationCostAnalytics(ctx context.Context) (*CostAnalytics, error) {
    analytics := &CostAnalytics{}
    
    // Current costs
    analytics.TotalDailyCost = a.storage.GetTotalDailyCost(ctx)
    analytics.TotalMonthlyCost = a.storage.GetMonthToDateCost(ctx)
    
    // Top instances by cost
    analytics.TopCostlyInstances = a.storage.GetTopCostlyInstances(ctx, 10)
    
    // Cost trend
    analytics.CostTrend = a.storage.GetDailyCostTrend(ctx, 30)
    
    // Projection
    analytics.ProjectedMonthly = a.projectMonthlyCost(analytics.CostTrend)
    
    // Budget alerts
    analytics.BudgetAlerts = a.checkAllBudgets(ctx)
    
    return analytics, nil
}
```

## 🤝 Federation Negotiation

### Cost-Based Peering

```go
// pkg/federation/peering/negotiation.go
type PeeringNegotiation struct {
    LocalInstance   string
    RemoteInstance  string
    ProposedTerms   PeeringTerms
    Status          NegotiationStatus
}

type PeeringTerms struct {
    MaxDailyCost     float64
    CostSplitRatio   float64 // 0.5 = equal split
    MediaProcessing  string   // "local", "remote", "origin"
    ThrottleRules    []ThrottleRule
    ReviewPeriod     time.Duration
}

func (n *Negotiator) ProposePeering(ctx context.Context, remoteInstance string) (*PeeringNegotiation, error) {
    // Analyze historical costs if available
    historicalCost := n.analyzer.GetHistoricalCost(ctx, remoteInstance)
    
    // Propose terms based on analysis
    terms := PeeringTerms{
        MaxDailyCost:    n.calculateFairBudget(historicalCost),
        CostSplitRatio:  0.5, // Start with equal split
        MediaProcessing: "origin", // Process at origin to save transfer
        ReviewPeriod:    30 * 24 * time.Hour,
    }
    
    // Send proposal
    return n.sendProposal(ctx, remoteInstance, terms)
}
```

## 🧪 Testing

### Cost Tracking Tests

```go
func TestCostTracking(t *testing.T) {
    tracker := NewFederationCostTracker()
    
    // Test various activity types
    activities := []struct {
        activity     *activitypub.Activity
        expectedCost float64
    }{
        {createNoteActivity(), 0.00065},  // 1 write
        {followActivity(), 0.00143},       // 2 writes + 1 read
        {likeActivity(), 0.00078},         // 1 write + 1 read
    }
    
    for _, tc := range activities {
        cost := tracker.TrackInboundActivity(ctx, tc.activity, "example.com")
        assert.InDelta(t, tc.expectedCost, cost, 0.00001)
    }
}

func TestBudgetEnforcement(t *testing.T) {
    enforcer := NewBudgetEnforcer()
    
    // Set budget with actions
    budget := &FederationBudget{
        DailyLimit: 1.00,
        Actions: []BudgetAction{
            {Threshold: 50, Action: ActionNotify},
            {Threshold: 80, Action: ActionThrottle},
            {Threshold: 100, Action: ActionSuspend},
        },
    }
    
    // Test enforcement at different thresholds
    enforcer.CheckAndEnforce(ctx, "example.com", 0.45) // No action
    enforcer.CheckAndEnforce(ctx, "example.com", 0.55) // Should notify
    enforcer.CheckAndEnforce(ctx, "example.com", 0.85) // Should throttle
    enforcer.CheckAndEnforce(ctx, "example.com", 1.05) // Should suspend
}
```

## 📈 Monitoring & Alerts

### CloudWatch Metrics

```go
func (m *MetricsPublisher) PublishFederationMetrics(ctx context.Context) {
    // Per-instance metrics
    for _, instance := range m.getActiveInstances(ctx) {
        m.putMetric("FederationCost", instance.DailyCost, "Instance", instance.Domain)
        m.putMetric("FederationBudgetUsage", instance.BudgetUsage, "Instance", instance.Domain)
    }
    
    // Aggregate metrics
    m.putMetric("TotalFederationCost", m.getTotalCost(ctx))
    m.putMetric("ActiveFederationPeers", m.getActivePeerCount(ctx))
    m.putMetric("ThrottledInstances", m.getThrottledCount(ctx))
}
```

## 🎯 Success Metrics

1. **Cost Visibility**: 100% of federation activities tracked
2. **Budget Compliance**: 95% of instances stay within budget
3. **Sustainability**: 20% reduction in federation costs
4. **Transparency**: Cost headers adopted by 10+ instances
5. **Peering Success**: 50% of peers negotiate cost terms

## 🏁 Conclusion

Lesser's Cost-Aware Federation provides:
- **Complete cost transparency** for instance operators
- **Automatic budget enforcement** to prevent surprises
- **Fair cost sharing** through negotiated peering
- **Sustainable growth** for the Fediverse
- **Innovation leadership** in federation protocols

This positions Lesser as the most economically sustainable ActivityPub implementation. 