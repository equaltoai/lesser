package main

import (
	"os"

	"github.com/pay-theory/lift/pkg/lift"
)

// configureLiftRoutes sets up routes that use native Lift handlers
// This allows gradual migration from Lambda handlers to Lift handlers
func configureLiftRoutes(app *lift.App) {
	// Check if we should use Lift handlers (can be controlled via environment variable)
	useLiftHandlers := os.Getenv("USE_LIFT_HANDLERS") == "true"
	
	if useLiftHandlers {
		// OAuth endpoints with native Lift implementation
		app.GET("/oauth/authorize", lift.HandlerFunc(liftHandler.HandleOAuthAuthorizeLift))
		
		// NodeInfo endpoints with native Lift implementation
		app.GET("/.well-known/nodeinfo", lift.HandlerFunc(liftHandler.HandleNodeInfoWellKnownLift))
		app.GET("/nodeinfo/2.0", lift.HandlerFunc(liftHandler.HandleNodeInfoLift))
		
		// Relationships endpoint with native Lift implementation
		app.GET("/accounts/relationships", lift.HandlerFunc(liftHandler.HandleGetRelationshipsLift))
		
		// Data exports with native Lift implementation
		app.POST("/exports", lift.HandlerFunc(liftHandler.HandleCreateExportLift))
		app.GET("/exports/{id}", lift.HandlerFunc(liftHandler.HandleGetExportStatusLift))
		app.GET("/exports", lift.HandlerFunc(liftHandler.HandleListExportsLift))
		
		// Add more Lift handlers here as they are implemented
		// app.POST("/oauth/token", lift.HandlerFunc(liftHandler.HandleOAuthTokenLift))
		// app.POST("/accounts", lift.HandlerFunc(liftHandler.HandleRegistrationLift))
	}
}