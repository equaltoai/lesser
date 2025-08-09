1. Authentication Method Priority/Usage

  - Which authentication methods are primary vs fallback? (OAuth, WebAuthn,
  API keys, etc.) - webauthn is THE authentication, we require passkey for access
  - Do all operations require authentication, or are some endpoints public? - follow mastodon conventions
  - How often do we expect token refresh vs new authentication? - varies on use case

  2. Real-time Subscription Scope

  - Which of the 6 subscription resolvers are actually needed for MVP?
    - ModerationQueueUpdate
    - ThreatIntelligence
    - PerformanceAlert
    - InfrastructureEvent
  - Should we implement all or prioritize specific ones?
    - we want ALL for mvp

  3. Metrics Aggregation Frequency
  - metrics should be processed from streams as they arrive

  4. Cost Tracking Granularity

  - Do we need per-user cost tracking, or is aggregate sufficient? per user
  - Should federation costs be tracked per instance or just totals? per intance
  - Any specific cost alerting thresholds you want set? not yet

  5. Data Archival Strategy

  - For the 1+ year reporting data retention, should older data go to S3? - glacier
  - Any specific compliance requirements driving the 7-year cost data
  retention? - I never set this requirement, I specified 1 year
  - How should we handle data export for users who leave? - we need to engineer a solution that meets the capabilities of mastodon

  6. Error Handling in Streams

  - If the DynamoDB Streams processor fails, should we have DLQ/retry logic? - use a dynamodb oriented retry if possible
  - How critical is it that every operational event becomes a reporting
  record? - not critical but impactful if lots of loss
  