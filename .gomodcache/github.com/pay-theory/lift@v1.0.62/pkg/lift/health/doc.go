// Package health contains HTTP endpoints and middleware for service health.
// It provides liveness/readiness/component checks with JSON or plain‑text
// responses, optional CORS support, and a health header middleware for quick
// status propagation. The endpoints accept a HealthManager implementation and
// can log via an optional structured logger.
package health
