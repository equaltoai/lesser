package theorydb

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/agents"
)

var agentsCapabilitiesType = reflect.TypeOf(agents.Capabilities{})

type agentCapabilitiesConverter struct{}

func (agentCapabilitiesConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	if value == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	var caps agents.Capabilities
	switch v := value.(type) {
	case agents.Capabilities:
		caps = v
	case *agents.Capabilities:
		if v == nil {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		caps = *v
	default:
		rv := reflect.ValueOf(value)
		if rv.IsValid() && rv.Kind() == reflect.Ptr && rv.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return nil, fmt.Errorf("agentCapabilitiesConverter: expected agents.Capabilities, got %T", value)
	}

	restricted := make([]types.AttributeValue, 0, len(caps.RestrictedDomains))
	for _, domain := range caps.RestrictedDomains {
		restricted = append(restricted, &types.AttributeValueMemberS{Value: domain})
	}

	return &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"canPost":           &types.AttributeValueMemberBOOL{Value: caps.CanPost},
		"canReply":          &types.AttributeValueMemberBOOL{Value: caps.CanReply},
		"canBoost":          &types.AttributeValueMemberBOOL{Value: caps.CanBoost},
		"canFollow":         &types.AttributeValueMemberBOOL{Value: caps.CanFollow},
		"canDM":             &types.AttributeValueMemberBOOL{Value: caps.CanDM},
		"restrictedDomains": &types.AttributeValueMemberL{Value: restricted},
		"maxPostsPerHour":   &types.AttributeValueMemberN{Value: strconv.Itoa(caps.MaxPostsPerHour)},
		"requiresApproval":  &types.AttributeValueMemberBOOL{Value: caps.RequiresApproval},
	}}, nil
}

func (agentCapabilitiesConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	dest, ok := target.(*agents.Capabilities)
	if !ok {
		return fmt.Errorf("agentCapabilitiesConverter: target must be *agents.Capabilities, got %T", target)
	}
	if dest == nil {
		return fmt.Errorf("agentCapabilitiesConverter: target is nil")
	}

	switch v := av.(type) {
	case nil:
		*dest = agents.Capabilities{}
		return nil
	case *types.AttributeValueMemberNULL:
		*dest = agents.Capabilities{}
		return nil
	case *types.AttributeValueMemberM:
		decodeCapabilitiesMap(v.Value, dest)
		return nil
	case *types.AttributeValueMemberS:
		raw := strings.TrimSpace(v.Value)
		if raw == "" || raw == "null" {
			*dest = agents.Capabilities{}
			return nil
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return fmt.Errorf("agentCapabilitiesConverter: unmarshal JSON string: %w", err)
		}
		decodeCapabilitiesAnyMap(payload, dest)
		return nil
	default:
		// Be permissive: treat unexpected shapes as empty rather than failing the read.
		*dest = agents.Capabilities{}
		return nil
	}
}

func decodeCapabilitiesMap(m map[string]types.AttributeValue, dest *agents.Capabilities) {
	if dest == nil {
		return
	}

	if b, ok := boolFromAttributeMap(m, "canPost", "can_post"); ok {
		dest.CanPost = b
	}
	if b, ok := boolFromAttributeMap(m, "canReply", "can_reply"); ok {
		dest.CanReply = b
	}
	if b, ok := boolFromAttributeMap(m, "canBoost", "can_boost"); ok {
		dest.CanBoost = b
	}
	if b, ok := boolFromAttributeMap(m, "canFollow", "can_follow"); ok {
		dest.CanFollow = b
	}
	if b, ok := boolFromAttributeMap(m, "canDM", "canDm", "can_dm"); ok {
		dest.CanDM = b
	}

	if v, ok := intFromAttributeMap(m, "maxPostsPerHour", "max_posts_per_hour"); ok {
		dest.MaxPostsPerHour = v
	}
	if b, ok := boolFromAttributeMap(m, "requiresApproval", "requires_approval"); ok {
		dest.RequiresApproval = b
	}

	if domains, ok := stringSliceFromAttributeMap(m, "restrictedDomains", "restricted_domains"); ok {
		dest.RestrictedDomains = domains
	}
}

func decodeCapabilitiesAnyMap(m map[string]any, dest *agents.Capabilities) {
	if dest == nil {
		return
	}

	if b, ok := boolFromAnyMap(m, "canPost", "can_post"); ok {
		dest.CanPost = b
	}
	if b, ok := boolFromAnyMap(m, "canReply", "can_reply"); ok {
		dest.CanReply = b
	}
	if b, ok := boolFromAnyMap(m, "canBoost", "can_boost"); ok {
		dest.CanBoost = b
	}
	if b, ok := boolFromAnyMap(m, "canFollow", "can_follow"); ok {
		dest.CanFollow = b
	}
	if b, ok := boolFromAnyMap(m, "canDM", "canDm", "can_dm"); ok {
		dest.CanDM = b
	}
	if v, ok := intFromAnyMap(m, "maxPostsPerHour", "max_posts_per_hour"); ok {
		dest.MaxPostsPerHour = v
	}
	if b, ok := boolFromAnyMap(m, "requiresApproval", "requires_approval"); ok {
		dest.RequiresApproval = b
	}
	if domains, ok := stringSliceFromAnyMap(m, "restrictedDomains", "restricted_domains"); ok {
		dest.RestrictedDomains = domains
	}
}

func firstAttributeValue(m map[string]types.AttributeValue, keys ...string) (types.AttributeValue, bool) {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if av, ok := m[key]; ok {
			return av, true
		}
	}
	return nil, false
}

func boolFromAttributeMap(m map[string]types.AttributeValue, keys ...string) (bool, bool) {
	av, ok := firstAttributeValue(m, keys...)
	if !ok || av == nil {
		return false, false
	}

	switch v := av.(type) {
	case *types.AttributeValueMemberBOOL:
		return v.Value, true
	case *types.AttributeValueMemberN:
		n, err := strconv.ParseInt(strings.TrimSpace(v.Value), 10, 64)
		if err != nil {
			return false, false
		}
		return n != 0, true
	case *types.AttributeValueMemberS:
		s := strings.TrimSpace(v.Value)
		if b, err := strconv.ParseBool(s); err == nil {
			return b, true
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n != 0, true
		}
		return false, false
	default:
		return false, false
	}
}

func intFromAttributeMap(m map[string]types.AttributeValue, keys ...string) (int, bool) {
	av, ok := firstAttributeValue(m, keys...)
	if !ok || av == nil {
		return 0, false
	}

	switch v := av.(type) {
	case *types.AttributeValueMemberN:
		n, err := strconv.ParseInt(strings.TrimSpace(v.Value), 10, 64)
		if err != nil {
			return 0, false
		}
		return int(n), true // #nosec G115 -- MaxPostsPerHour is bounded by business logic.
	case *types.AttributeValueMemberS:
		n, err := strconv.ParseInt(strings.TrimSpace(v.Value), 10, 64)
		if err != nil {
			return 0, false
		}
		return int(n), true // #nosec G115 -- MaxPostsPerHour is bounded by business logic.
	default:
		return 0, false
	}
}

func stringSliceFromAttributeMap(m map[string]types.AttributeValue, keys ...string) ([]string, bool) {
	av, ok := firstAttributeValue(m, keys...)
	if !ok || av == nil {
		return nil, false
	}

	switch v := av.(type) {
	case *types.AttributeValueMemberSS:
		out := make([]string, 0, len(v.Value))
		out = append(out, v.Value...)
		return out, true
	case *types.AttributeValueMemberL:
		out := make([]string, 0, len(v.Value))
		for _, elem := range v.Value {
			s, ok := elem.(*types.AttributeValueMemberS)
			if !ok {
				continue
			}
			out = append(out, s.Value)
		}
		return out, true
	default:
		return nil, false
	}
}

func firstAny(m map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func boolFromAnyMap(m map[string]any, keys ...string) (bool, bool) {
	raw, ok := firstAny(m, keys...)
	if !ok {
		return false, false
	}
	switch v := raw.(type) {
	case bool:
		return v, true
	case float64:
		return v != 0, true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return false, false
		}
		return n != 0, true
	case string:
		s := strings.TrimSpace(v)
		if b, err := strconv.ParseBool(s); err == nil {
			return b, true
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n != 0, true
		}
		return false, false
	default:
		return false, false
	}
}

func intFromAnyMap(m map[string]any, keys ...string) (int, bool) {
	raw, ok := firstAny(m, keys...)
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true // #nosec G115 -- MaxPostsPerHour is bounded by business logic.
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return int(n), true // #nosec G115 -- MaxPostsPerHour is bounded by business logic.
	default:
		return 0, false
	}
}

func stringSliceFromAnyMap(m map[string]any, keys ...string) ([]string, bool) {
	raw, ok := firstAny(m, keys...)
	if !ok {
		return nil, false
	}

	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, elem := range v {
			s, ok := elem.(string)
			if !ok {
				continue
			}
			out = append(out, s)
		}
		return out, true
	case []string:
		out := make([]string, 0, len(v))
		out = append(out, v...)
		return out, true
	default:
		return nil, false
	}
}
