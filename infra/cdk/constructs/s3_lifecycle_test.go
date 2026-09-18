package constructs

import (
	"strings"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
)

// restoringStorageClasses are the storage classes S3 refuses to serve directly:
// GetObject fails with InvalidObjectState until the object is restored. An
// object the instance reads back must never reach one of these. GLACIER_IR is
// deliberately absent — it is the instant-retrieval tier and stays servable.
var restoringStorageClasses = map[string]struct{}{
	"GLACIER":      {},
	"DEEP_ARCHIVE": {},
}

// readBackPrefixes are the media bucket key prefixes the instance reads back:
// avatars/ on the public serve route, published/ through the media CDN,
// media/ through the media and export pipelines, imports/ through the import
// processor, and exports/ through presigned downloads. A lifecycle rule that can
// match objects under any of these must keep them in an instantly-retrievable
// class.
var readBackPrefixes = []string{
	"avatars/",
	"exports/",
	"imports/",
	"media/",
	"published/",
}

type recordingLifecycleBucket struct {
	awss3.Bucket
	rules []*awss3.LifecycleRule
}

func (b *recordingLifecycleBucket) AddLifecycleRule(rule *awss3.LifecycleRule) {
	if rule == nil {
		return
	}
	b.rules = append(b.rules, rule)
}

func mediaBucketRules(t *testing.T, environment string) []*awss3.LifecycleRule {
	t.Helper()

	bucket := &recordingLifecycleBucket{}
	ApplyS3LifecyclePolicies(&S3LifecycleConfig{
		Environment: environment,
		Bucket:      bucket,
		BucketType:  "media",
	})
	if len(bucket.rules) == 0 {
		t.Fatalf("media bucket lifecycle produced no rules for environment %q", environment)
	}
	return bucket.rules
}

func ruleByID(t *testing.T, rules []*awss3.LifecycleRule, id string) *awss3.LifecycleRule {
	t.Helper()

	for _, rule := range rules {
		if rule != nil && rule.Id != nil && *rule.Id == id {
			return rule
		}
	}
	t.Fatalf("media bucket lifecycle has no rule %q", id)
	return nil
}

func ruleID(rule *awss3.LifecycleRule) string {
	if rule == nil || rule.Id == nil {
		return ""
	}
	return *rule.Id
}

func rulePrefix(rule *awss3.LifecycleRule) string {
	if rule == nil || rule.Prefix == nil {
		return ""
	}
	return *rule.Prefix
}

func transitionStorageClass(t *testing.T, transition *awss3.Transition) string {
	t.Helper()

	if transition == nil || transition.StorageClass == nil {
		t.Fatal("media bucket lifecycle rule has a transition with no storage class")
	}
	value := transition.StorageClass.Value()
	if value == nil {
		t.Fatal("media bucket lifecycle transition storage class has no value")
	}
	return *value
}

// appliesToReadBackObject reports whether a rule can match an object the
// instance reads back. A rule with prefix P matches every key starting with P, so
// it reaches a read-back prefix R when R starts with P (the prefix-less rule
// therefore reaches all of them).
func appliesToReadBackObject(rule *awss3.LifecycleRule) bool {
	prefix := rulePrefix(rule)
	if prefix == "" {
		return true
	}
	for _, readBack := range readBackPrefixes {
		if strings.HasPrefix(readBack, prefix) {
			return true
		}
	}
	return false
}

// TestMediaBucketLifecycleKeepsReadBackObjectsServable pins the fix for the
// bucket-wide Glacier Flexible transition. That filter-less rule moved every
// object to GLACIER at 180 days, and a prefix-scoped rule cannot suppress a
// bucket-wide transition (S3 applies every matching rule), so any avatar older
// than 180 days 500'd on the public serve route. A filter-less rule is therefore
// only allowed to use storage classes S3 still serves on demand.
func TestMediaBucketLifecycleKeepsReadBackObjectsServable(t *testing.T) {
	for _, environment := range []string{"production", "staging", "development"} {
		for _, rule := range mediaBucketRules(t, environment) {
			if !appliesToReadBackObject(rule) || rule.Transitions == nil {
				continue
			}
			for _, transition := range *rule.Transitions {
				class := transitionStorageClass(t, transition)
				if _, restoring := restoringStorageClasses[class]; restoring {
					t.Fatalf("environment %q: rule %q (prefix %q) transitions read-back objects to %s, which S3 will not serve until restored",
						environment, ruleID(rule), rulePrefix(rule), class)
				}
			}
		}
	}
}

// TestMediaBucketStorageClassRuleStopsAtInstantRetrieval pins the exact shape of
// the bucket-wide rule so the 180-day Glacier Flexible transition cannot return
// unnoticed.
func TestMediaBucketStorageClassRuleStopsAtInstantRetrieval(t *testing.T) {
	expected := []struct {
		class string
		days  float64
	}{
		{class: "STANDARD_IA", days: 30},
		{class: "GLACIER_IR", days: 90},
	}

	rule := ruleByID(t, mediaBucketRules(t, "production"), "optimize-storage-class")
	if prefix := rulePrefix(rule); prefix != "" {
		t.Fatalf("optimize-storage-class prefix = %q, want a bucket-wide rule", prefix)
	}
	if rule.Transitions == nil {
		t.Fatal("optimize-storage-class must declare its transitions explicitly")
	}
	transitions := *rule.Transitions
	if len(transitions) != len(expected) {
		t.Fatalf("optimize-storage-class has %d transitions, want %d", len(transitions), len(expected))
	}

	for i, want := range expected {
		transition := transitions[i]
		if got := transitionStorageClass(t, transition); got != want.class {
			t.Fatalf("transition %d storage class = %s, want %s", i, got, want.class)
		}
		if transition.TransitionAfter == nil {
			t.Fatalf("transition %d has no TransitionAfter", i)
		}
		if got := *transition.TransitionAfter.ToDays(nil); got != want.days {
			t.Fatalf("transition %d days = %v, want %v", i, got, want.days)
		}
	}
}

// TestMediaBucketAvatarRuleStaysInstantlyRetrievable covers the avatars/ prefix
// on its own: it keeps its own IA transition, never carries a restore-only class,
// and never expires, so a live avatar URL keeps being served indefinitely.
func TestMediaBucketAvatarRuleStaysInstantlyRetrievable(t *testing.T) {
	rule := ruleByID(t, mediaBucketRules(t, "production"), "optimize-avatar-storage")
	if prefix := rulePrefix(rule); prefix != "avatars/" {
		t.Fatalf("optimize-avatar-storage prefix = %q, want avatars/", prefix)
	}
	if rule.Expiration != nil {
		t.Fatal("avatars/ must not expire: a live avatar URL would start 404ing")
	}
	if rule.Transitions == nil {
		t.Fatal("optimize-avatar-storage must declare its transitions explicitly")
	}
	transitions := *rule.Transitions
	if len(transitions) != 1 {
		t.Fatalf("optimize-avatar-storage has %d transitions, want 1", len(transitions))
	}
	if got := transitionStorageClass(t, transitions[0]); got != "STANDARD_IA" {
		t.Fatalf("avatars/ transition storage class = %s, want STANDARD_IA", got)
	}
	if transitions[0].TransitionAfter == nil {
		t.Fatal("avatars/ transition has no TransitionAfter")
	}
	if got := *transitions[0].TransitionAfter.ToDays(nil); got != 60 {
		t.Fatalf("avatars/ transition days = %v, want 60", got)
	}
}
