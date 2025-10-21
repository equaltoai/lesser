package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
)

// synthesizeTemplate returns the synthesized template for a stack
func synthesizeTemplate(t *testing.T, stack awscdk.Stack) assertions.Template {
	t.Helper()
	return assertions.Template_FromStack(stack, nil)
}

// assertResourceExists asserts that at least one resource of the given type exists
func assertResourceExists(t *testing.T, template assertions.Template, resourceType string) {
	t.Helper()
	resources := findResourcesByType(template, resourceType)
	if len(resources) == 0 {
		t.Fatalf("Expected resource of type %s to exist", resourceType)
	}
}

// findResourcesByType finds all resources of a given type in the template
func findResourcesByType(template assertions.Template, resourceType string) map[string]map[string]interface{} {
	templateMap := template.ToJSON()
	if templateMap == nil {
		return nil
	}
	resources, ok := (*templateMap)["Resources"].(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string]map[string]interface{})
	for key, value := range resources {
		if resMap, ok := value.(map[string]interface{}); ok {
			if resType, ok := resMap["Type"].(string); ok && resType == resourceType {
				result[key] = resMap
			}
		}
	}
	return result
}

// assertResourceCount asserts the count of resources of a given type
func assertResourceCount(t *testing.T, template assertions.Template, resourceType string, expectedCount int) {
	t.Helper()
	resources := findResourcesByType(template, resourceType)
	if len(resources) != expectedCount {
		t.Fatalf("Expected %d resources of type %s, got %d", expectedCount, resourceType, len(resources))
	}
}

// intPtr helper returns a pointer to an int
func intPtr(i int) *int { return &i }
