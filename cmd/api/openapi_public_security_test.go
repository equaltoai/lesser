package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIWebAuthnSignupEndpointsDeclareExplicitAnonymousSecurity(t *testing.T) {
	repoRoot := authSurfaceRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs/contracts/openapi.yaml")) // #nosec G304 -- repo contract artifact.
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi: %v", err)
	}

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/auth/webauthn/signup/begin"},
		{method: http.MethodPost, path: "/api/v1/auth/webauthn/signup/finish"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(strings.ToLower(tt.method)+" "+tt.path, func(t *testing.T) {
			op := openAPIOperationYAMLNode(t, &doc, tt.path, tt.method)
			security, ok := openAPIMapNodeValue(op, "security")
			if !ok {
				t.Fatalf("%s %s missing explicit security declaration", tt.method, tt.path)
			}
			if security.Kind != yaml.SequenceNode {
				t.Fatalf("%s %s security node kind = %d, want sequence", tt.method, tt.path, security.Kind)
			}
			if len(security.Content) != 0 {
				t.Fatalf("%s %s security content = %#v, want empty sequence", tt.method, tt.path, security.Content)
			}
		})
	}
}

func openAPIOperationYAMLNode(t *testing.T, doc *yaml.Node, path, method string) *yaml.Node {
	t.Helper()

	root := openAPIRequiredMapNodeValue(t, doc.Content[0], "paths")
	pathNode := openAPIRequiredMapNodeValue(t, root, path)
	return openAPIRequiredMapNodeValue(t, pathNode, strings.ToLower(method))
}

func openAPIRequiredMapNodeValue(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()

	value, ok := openAPIMapNodeValue(node, key)
	if !ok {
		t.Fatalf("yaml map missing key %q", key)
	}
	return value
}

func openAPIMapNodeValue(node *yaml.Node, key string) (*yaml.Node, bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], true
		}
	}
	return nil, false
}
