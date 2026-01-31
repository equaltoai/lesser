package main

import (
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	appmiddleware "github.com/equaltoai/lesser/pkg/middleware"
	"github.com/pay-theory/lift/pkg/lift"
	liftMiddleware "github.com/pay-theory/lift/pkg/middleware"
)

func standardLiftMiddlewaresForAppTheory() []lift.Middleware {
	var out []lift.Middleware

	log := logger
	if log == nil && lambdaCtx != nil {
		log = lambdaCtx.Logger
	}

	if log != nil {
		out = append(out, appmiddleware.PanicRecovery(log))
	}

	out = append(out, liftMiddleware.TimeoutMiddleware(liftMiddleware.TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
	}))

	if logger != nil {
		out = append(out, createLoggingMiddleware(logger))
		out = append(out, createInstanceLockMiddlewareFn(repos, logger))
		out = append(out, common.CreateAPIErrorMiddleware(logger))
	}

	if costTrackingService != nil {
		out = append(out, createCentralizedCostTrackingMiddleware())
	}
	if tracingManager != nil {
		out = append(out, createTracingMiddleware())
	}
	if emfMetrics != nil {
		out = append(out, createEMFMetricsMiddleware())
	}

	out = append(out, createLatencyTrackingMiddleware())

	return out
}

