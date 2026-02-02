package monitoring

import "testing"

func TestValidateHealthCheckEvent(t *testing.T) {
	t.Parallel()

	if err := ValidateHealthCheckEvent(HealthCheckEvent{}); err == nil {
		t.Fatalf("expected error for missing action/components")
	}

	if err := ValidateHealthCheckEvent(HealthCheckEvent{Action: "check_health"}); err == nil {
		t.Fatalf("expected error for missing components")
	}

	if err := ValidateHealthCheckEvent(HealthCheckEvent{
		Action: "check_health",
		Components: []ComponentCheckConfig{
			{Type: "", Identifier: "x"},
		},
	}); err == nil {
		t.Fatalf("expected error for missing component type")
	}

	if err := ValidateHealthCheckEvent(HealthCheckEvent{
		Action: "check_health",
		Components: []ComponentCheckConfig{
			{Type: "dynamodb", Identifier: ""},
		},
	}); err == nil {
		t.Fatalf("expected error for missing component identifier")
	}

	if err := ValidateHealthCheckEvent(HealthCheckEvent{
		Action: "check_health",
		Components: []ComponentCheckConfig{
			{Type: "redis", Identifier: "x"},
		},
	}); err == nil {
		t.Fatalf("expected error for invalid component type")
	}

	if err := ValidateHealthCheckEvent(HealthCheckEvent{
		Action: "check_health",
		Components: []ComponentCheckConfig{
			{Type: "dynamodb", Identifier: "table"},
			{Type: "lambda", Identifier: "fn"},
			{Type: "sqs", Identifier: "queue"},
		},
	}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
