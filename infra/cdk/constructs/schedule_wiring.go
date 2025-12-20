package constructs

import (
	"cdk/inventory"
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type ScheduleWiringProps struct {
	Functions   *LambdaFunctions
	Environment string
}

func CreateScheduleWiring(scope constructs.Construct, props *ScheduleWiringProps) {
	for _, spec := range inventory.LambdaInventory.Lambdas {
		if len(spec.ScheduleTriggers) == 0 {
			continue
		}

		validateScheduleCapable(spec)

		target := props.Functions.Must(spec.Name)
		for idx, trig := range spec.ScheduleTriggers {
			ruleId := fmt.Sprintf("%sScheduleRule%d", sanitizeScheduleId(spec.Name), idx)
			ruleName := fmt.Sprintf("lesser-%s-%s-schedule-%d", props.Environment, spec.Name, idx)

			rule := awsevents.NewRule(scope, jsii.String(ruleId), &awsevents.RuleProps{
				RuleName:    jsii.String(ruleName),
				Schedule:    awsevents.Schedule_Expression(jsii.String(trig.Expression)),
				Enabled:     jsii.Bool(true),
				Description: jsii.String(fmt.Sprintf("Inventory-driven schedule for %s (%s)", spec.Name, trig.Expression)),
			})

			targetProps := &awseventstargets.LambdaFunctionProps{}
			if trig.Input != "" {
				targetProps.Event = awsevents.RuleTargetInput_FromText(jsii.String(trig.Input))
			}

			rule.AddTarget(awseventstargets.NewLambdaFunction(target, targetProps))
		}
	}
}

func validateScheduleCapable(spec inventory.LambdaSpec) {
	switch spec.Type {
	case inventory.LambdaTypeProcessorScheduled, inventory.LambdaTypeHybrid:
		return
	default:
		panic(fmt.Sprintf("lambda %s has schedule triggers but type %s does not support schedules (R3)", spec.Name, spec.Type))
	}
}

func sanitizeScheduleId(name string) string {
	id := strings.ReplaceAll(name, "-", "")
	id = strings.ReplaceAll(id, "_", "")
	if id == "" {
		return "Schedule"
	}
	return id
}