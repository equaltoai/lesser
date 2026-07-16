package stacks

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
)

func TestInstancePlaneEnabledRequiresBodyOrSoulEnablement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config map[string]interface{}
		want   bool
	}{
		{
			name: "dark by default",
			config: map[string]interface{}{
				"bodyEnabled":          true,
				"instancePlaneEnabled": false,
			},
			want: false,
		},
		{
			name: "fails closed without body or soul",
			config: map[string]interface{}{
				"instancePlaneEnabled": true,
			},
			want: false,
		},
		{
			name: "enabled with body path",
			config: map[string]interface{}{
				"bodyEnabled":          true,
				"instancePlaneEnabled": true,
			},
			want: true,
		},
		{
			name: "enabled with legacy soul path",
			config: map[string]interface{}{
				"soulEnabled":          "true",
				"instancePlaneEnabled": "yes",
			},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := awscdk.NewApp(nil)
			stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)
			apiStack := &LesserApiStack{
				Stack:         stack,
				Configuration: tc.config,
			}

			if got := apiStack.instancePlaneEnabled(); got != tc.want {
				t.Fatalf("instancePlaneEnabled()=%v, want %v", got, tc.want)
			}
		})
	}
}
