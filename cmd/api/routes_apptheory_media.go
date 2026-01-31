package main

import (
	"time"

	lesserLift "github.com/equaltoai/lesser/pkg/lift"
	"github.com/equaltoai/lesser/pkg/ratelimit"
	"github.com/pay-theory/lift/pkg/lift"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func configureMediaRoutesAppTheory(app *apptheory.App) {
	baseMiddleware := standardLiftMiddlewaresForAppTheory()

	redirect := lift.HandlerFunc(func(ctx *lift.Context) error {
		return lesserLift.Redirect(ctx, "/l/", false)
	})

	app.Get("/", liftHandlerToAppTheory(redirect, baseMiddleware))
	app.Handle("HEAD", "/", liftHandlerToAppTheory(redirect, baseMiddleware))

	app.Get("/api/oembed", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleOEmbedLift),
		baseMiddleware,
	))
	app.Get("/embed/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleEmbedPageLift),
		baseMiddleware,
	))

	// Media endpoints.
	app.Post("/api/v1/media", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUploadMediaLift),
			20, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/media/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetMediaLift),
		baseMiddleware,
	))
	app.Put("/api/v1/media/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUpdateMediaLift),
		baseMiddleware,
	))
}
