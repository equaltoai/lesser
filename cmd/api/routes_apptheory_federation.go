package main

import (
	"github.com/pay-theory/lift/pkg/lift"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func configureFederationDiscoveryRoutesAppTheory(app *apptheory.App) {
	baseMiddleware := standardLiftMiddlewaresForAppTheory()

	app.Get("/.well-known/nodeinfo", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleNodeInfoWellKnownLift),
		baseMiddleware,
	))
	app.Get("/nodeinfo/2.0", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleNodeInfoLift),
		baseMiddleware,
	))
	app.Get("/.well-known/reputation-keys", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetReputationKeysLift),
		baseMiddleware,
	))
}

