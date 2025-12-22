package constructs

import (
	"cdk/inventory"
	"fmt"
	"strings"
)

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
