package activitypub

// IsPublicAddressedActivity reports whether an activity is addressed to the
// ActivityStreams Public collection in a recipient-visible field. It treats
// either top-level activity addressing or embedded object addressing as public.
func IsPublicAddressedActivity(activity *Activity) bool {
	if activity == nil {
		return false
	}
	if hasPublicAddress(activity.To) || hasPublicAddress(activity.CC) {
		return true
	}
	return isPublicAddressedObject(activity.Object)
}

func isPublicAddressedObject(object any) bool {
	switch obj := object.(type) {
	case nil:
		return false
	case *Note:
		return obj != nil && (hasPublicAddress(obj.To) || hasPublicAddress(obj.CC))
	case Note:
		return hasPublicAddress(obj.To) || hasPublicAddress(obj.CC)
	case map[string]any:
		return hasPublicAddressValue(obj["to"]) || hasPublicAddressValue(obj["cc"])
	default:
		return false
	}
}

func hasPublicAddressValue(value any) bool {
	switch recipients := value.(type) {
	case string:
		return recipients == PublicAddress
	case []string:
		return hasPublicAddress(recipients)
	case []any:
		for _, recipient := range recipients {
			if s, ok := recipient.(string); ok && s == PublicAddress {
				return true
			}
		}
	}
	return false
}

func hasPublicAddress(recipients []string) bool {
	for _, recipient := range recipients {
		if recipient == PublicAddress {
			return true
		}
	}
	return false
}
