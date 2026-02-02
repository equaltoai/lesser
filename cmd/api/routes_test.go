package main

import (
	"testing"

	apiHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap/zaptest"
)

func TestConfigureRoutes(t *testing.T) {
	logger = zaptest.NewLogger(t)
	apiHandler = &apiHandlers.Handler{}

	app := apptheory.New()
	configureRoutes(app)
}
