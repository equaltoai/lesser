# Lift Event Patterns for Lesser

This package provides standardized patterns and helpers for handling AWS Lambda event types with the Lift framework.

## Overview

Lesser relies on Lift for HTTP routing and supported event triggers, and this package adds reusable helpers for common event-driven patterns used across the codebase.

This package provides reusable patterns and helpers to integrate these event types with Lift's middleware and error handling.

## Patterns

Note: DynamoDB stream triggers are handled directly by Lift runtime (`app.DynamoDB` + `ctx.DynamoDBRecords()`); no extra wrapper is required here.

### SQS Pattern
Used by services that process messages from SQS queues.

### EventBridge / Schedules
Use Lift runtime directly via `app.EventBridge(...)` (no Lesser wrapper required).

## Usage

Each pattern provides:
1. Type-safe event extraction from Lift context
2. Proper error handling and logging
3. Middleware support
4. Batch processing utilities
5. Retry logic where appropriate
