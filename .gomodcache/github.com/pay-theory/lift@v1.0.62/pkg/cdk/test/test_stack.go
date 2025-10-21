package test

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
)

// TestStack is a simple helper to create an App and a Stack for CDK assertions
type TestStack struct {
	app   awscdk.App
	stack awscdk.Stack
}

// NewTestStack creates a new CDK app and stack for testing
func NewTestStack() *TestStack {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)
	return &TestStack{app: app, stack: stack}
}

// Stack returns the underlying CDK stack
func (t *TestStack) Stack() awscdk.Stack { return t.stack }
