package main

import (
	"testing"

	liftHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap/zaptest"
)

func TestConfigureLiftRoutes(t *testing.T) {
	logger = zaptest.NewLogger(t)
	liftHandler = &liftHandlers.Handler{}

	app := lift.New()
	configureLiftRoutes(app)
}
