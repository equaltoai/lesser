package main

import (
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func configureAPIRoutesAppTheory(app *apptheory.App) {
	configureAuthRoutesAppTheory(app)
	configureAccountsRoutesAppTheory(app)
	configureStatusesRoutesAppTheory(app)
	configureModerationRoutesAppTheory(app)
	configureMediaRoutesAppTheory(app)
	configureFederationDiscoveryRoutesAppTheory(app)
}
