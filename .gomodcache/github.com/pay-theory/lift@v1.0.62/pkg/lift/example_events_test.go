package lift_test

import (
	"github.com/pay-theory/lift/pkg/lift"
)

// Example showing SQS handler registration.
func ExampleApp_SQS() {
	app := lift.New()
	_ = app.Handle("SQS", "my-queue", func(ctx *lift.Context) error {
		// parse message from ctx.Request.Body
		return nil
	})
}

// Example showing S3 handler registration.
func ExampleApp_S3() {
	app := lift.New()
	_ = app.Handle("S3", "my-bucket", func(ctx *lift.Context) error {
		// examine ctx.Request.Records or Detail for S3 keys
		return nil
	})
}

// Example showing EventBridge handler registration.
func ExampleApp_EventBridge() {
	app := lift.New()
	_ = app.Handle("EventBridge", "my.app.source", func(ctx *lift.Context) error {
		// read ctx.Request.Detail and process event
		return nil
	})
}
