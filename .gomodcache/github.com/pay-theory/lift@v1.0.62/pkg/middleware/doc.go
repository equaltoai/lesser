// Package middleware contains production‑ready middleware for Lift applications,
// including request correlation, structured logging, panic recovery, error
// formatting, input validation, JWT authentication, rate limiting, idempotency,
// retries, circuit breaking, load shedding, security headers, and service mesh
// helpers. Middleware composes via the Lift Middleware type:
//
//	app.Use(middleware.RequestID())
//	app.Use(middleware.Logger())
//	app.Use(middleware.Recover())
//	app.Use(middleware.ErrorHandler())
//	app.Use(middleware.JWTAuth(jwtConfig))
//
// Middleware is evaluated in registration order (last added runs closest to the
// handler). Functions here are safe defaults for serverless workloads.
package middleware
