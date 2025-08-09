package main

import (
	"os"

	"github.com/pay-theory/lift/pkg/lift"
)

const (
	// Environment variable values
	envTrue = "true"
)

// configureLiftRoutes sets up routes that use native Lift handlers
// This allows gradual migration from Lambda handlers to Lift handlers
func configureLiftRoutes(app *lift.App) {
	// Check if we should use Lift handlers (can be controlled via environment variable)
	useLiftHandlers := os.Getenv("USE_LIFT_HANDLERS") == envTrue

	if useLiftHandlers {
		// OAuth endpoints with native Lift implementation
		_ = app.GET("/oauth/authorize", lift.HandlerFunc(liftHandler.HandleOAuthAuthorizeLift))

		// NodeInfo endpoints with native Lift implementation
		_ = app.GET("/.well-known/nodeinfo", lift.HandlerFunc(liftHandler.HandleNodeInfoWellKnownLift))
		_ = app.GET("/nodeinfo/2.0", lift.HandlerFunc(liftHandler.HandleNodeInfoLift))

		// Relationships endpoint with native Lift implementation
		_ = app.GET("/accounts/relationships", lift.HandlerFunc(liftHandler.HandleGetRelationshipsLift))

		// Data exports with native Lift implementation
		_ = app.POST("/exports", lift.HandlerFunc(liftHandler.HandleCreateExportLift))
		_ = app.GET("/exports/{id}", lift.HandlerFunc(liftHandler.HandleGetExportStatusLift))
		_ = app.GET("/exports", lift.HandlerFunc(liftHandler.HandleListExportsLift))

		// Community Notes endpoints with native Lift implementation
		_ = app.POST("/notes", lift.HandlerFunc(liftHandler.HandleCreateNoteLift))
		_ = app.GET("/notes/{object_id}", lift.HandlerFunc(liftHandler.HandleGetNotesLift))
		_ = app.POST("/notes/{id}/vote", lift.HandlerFunc(liftHandler.HandleVoteNoteLift))
		_ = app.GET("/accounts/{id}/notes", lift.HandlerFunc(liftHandler.HandleGetUserNotesLift))

	}
}
