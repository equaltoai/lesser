// Package activitypubutil contains helpers for constructing ActivityPub actors.
package activitypubutil

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
)

// BuildLocalActor returns a sanitized ActivityPub actor for a local account.
// It merges existing actor data with canonical local identifiers derived from
// the provided username, base URL, and optional storage.User record. The
// returned actor is a deep copy and safe for callers to mutate.
func BuildLocalActor(username string, baseURL string, user *storage.User, existing *activitypub.Actor) *activitypub.Actor {
	clone := ensureActorInstance(existing)
	resolvedUsername := resolveUsername(clone, user, username)
	normalizedBase := chooseBaseURL(baseURL, user)

	ensureActorType(clone, user)
	applyActorIdentifiers(clone, normalizedBase, resolvedUsername)

	if user != nil {
		mergeUserProfile(clone, user, resolvedUsername)
	}

	return clone
}

// MergeActorMetadata copies metadata fields from src into dst when dst is missing values.
// Slice and pointer fields are cloned to avoid sharing mutable references.
func MergeActorMetadata(dst, src *activitypub.Actor) {
	if dst == nil || src == nil {
		return
	}

	copyScalarFields(dst, src)
	copyTemporalFields(dst, src)
	copySliceFields(dst, src)
	copyMediaFields(dst, src)
	copyEndpointFields(dst, src)
	copyKeyFields(dst, src)
	mergeBooleanFlags(dst, src)
}

// DerivePreferredUsername returns the most suitable preferred username by inspecting
// the actor's existing fields and falling back to the provided value.
func DerivePreferredUsername(actor *activitypub.Actor, fallback string) string {
	candidates := []string{
		extractCandidate(actor, func(a *activitypub.Actor) string {
			return a.PreferredUsername
		}),
		extractCandidate(actor, func(a *activitypub.Actor) string {
			return deriveFromID(a.ID)
		}),
		extractCandidate(actor, func(a *activitypub.Actor) string {
			return deriveFromURL(a.URL)
		}),
		fallback,
	}

	for _, candidate := range candidates {
		clean := sanitizeUsername(candidate)
		if clean != "" {
			return clean
		}
	}
	return ""
}

func extractCandidate(actor *activitypub.Actor, extractor func(*activitypub.Actor) string) string {
	if actor == nil {
		return ""
	}
	return extractor(actor)
}

func sanitizeUsername(value string) string {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return ""
	}
	candidate = strings.TrimPrefix(candidate, "@")
	if strings.Contains(candidate, "@") {
		parts := strings.Split(candidate, "@")
		if len(parts) > 0 {
			candidate = strings.TrimSpace(parts[0])
		}
	}
	candidate = strings.TrimSuffix(candidate, ".json")
	return candidate
}

func deriveFromID(identifier string) string {
	if identifier == "" {
		return ""
	}
	if !strings.Contains(identifier, "://") {
		return identifier
	}

	parsed, err := url.Parse(identifier)
	if err != nil {
		return identifier
	}

	return lastPathSegment(parsed.Path)
}

func deriveFromURL(raw string) string {
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	return lastPathSegment(parsed.Path)
}

func lastPathSegment(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return ""
	}
	segments := strings.Split(trimmed, "/")
	return segments[len(segments)-1]
}

func normalizeBaseURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		trimmed = "https://" + trimmed
	}

	return strings.TrimRight(trimmed, "/")
}

func cloneActor(actor *activitypub.Actor) *activitypub.Actor {
	if actor == nil {
		return nil
	}

	cloned := *actor
	cloned.Context = actor.Context.Clone()
	cloned.To = append([]string{}, actor.To...)
	cloned.CC = append([]string{}, actor.CC...)
	cloned.BTo = append([]string{}, actor.BTo...)
	cloned.BCC = append([]string{}, actor.BCC...)
	cloned.AlsoKnownAs = append([]string{}, actor.AlsoKnownAs...)
	cloned.Attachment = cloneAttachments(actor.Attachment)

	if actor.PublicKey != nil {
		cloned.PublicKey = clonePublicKey(actor.PublicKey)
	}
	if actor.Endpoints != nil {
		cloned.Endpoints = cloneEndpoints(actor.Endpoints)
	}
	if actor.Icon != nil {
		cloned.Icon = cloneImage(actor.Icon)
	}
	if actor.Image != nil {
		cloned.Image = cloneImage(actor.Image)
	}
	if actor.Published != nil {
		published := *actor.Published
		cloned.Published = &published
	}
	if actor.Updated != nil {
		updated := *actor.Updated
		cloned.Updated = &updated
	}
	return &cloned
}

func cloneAttachments(attachments []activitypub.Attachment) []activitypub.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	cloned := make([]activitypub.Attachment, len(attachments))
	copy(cloned, attachments)
	return cloned
}

func cloneImage(image *activitypub.Image) *activitypub.Image {
	if image == nil {
		return nil
	}
	cloned := *image
	if image.Published != nil {
		published := *image.Published
		cloned.Published = &published
	}
	if image.Updated != nil {
		updated := *image.Updated
		cloned.Updated = &updated
	}
	return &cloned
}

func cloneEndpoints(endpoints *activitypub.Endpoints) *activitypub.Endpoints {
	if endpoints == nil {
		return nil
	}
	cloned := *endpoints
	return &cloned
}

func clonePublicKey(key *activitypub.PublicKey) *activitypub.PublicKey {
	if key == nil {
		return nil
	}
	cloned := *key
	return &cloned
}

func ensureActorInstance(existing *activitypub.Actor) *activitypub.Actor {
	clone := cloneActor(existing)
	if clone != nil {
		return clone
	}
	return &activitypub.Actor{}
}

func resolveUsername(actor *activitypub.Actor, user *storage.User, fallback string) string {
	candidate := strings.TrimSpace(fallback)
	if user != nil && candidate == "" {
		candidate = strings.TrimSpace(user.Username)
	}
	return strings.ToLower(strings.TrimSpace(DerivePreferredUsername(actor, candidate)))
}

func chooseBaseURL(baseURL string, user *storage.User) string {
	normalized := normalizeBaseURL(baseURL)
	if normalized != "" {
		return normalized
	}
	if user == nil {
		return ""
	}
	return normalizeBaseURL(user.URL)
}

func ensureActorType(actor *activitypub.Actor, user *storage.User) {
	if actor == nil {
		return
	}

	if actor.Context == nil {
		actor.Context = activitypub.Context.Clone()
	}

	if strings.TrimSpace(actor.Type) == "" {
		actor.Type = activitypub.PersonType
	}

	if user != nil && user.IsAgent {
		actor.Type = activitypub.ServiceType
		applyAgentManifest(actor, user)
	}
}

func applyAgentManifest(actor *activitypub.Actor, user *storage.User) {
	if actor == nil || user == nil {
		return
	}

	manifest := actor.AgentManifest
	if manifest == nil {
		manifest = &activitypub.AgentManifest{Type: "Agent"}
	}
	if strings.TrimSpace(manifest.Type) == "" {
		manifest.Type = "Agent"
	}

	if v := strings.TrimSpace(user.AgentVersion); v != "" {
		manifest.Version = v
	}
	if v := strings.TrimSpace(user.AgentType); v != "" {
		manifest.Purpose = v
	}
	if v := normalizeOperatedBy(user.AgentOwner); v != "" {
		manifest.OperatedBy = v
	}

	if user.AgentCapabilities != nil {
		manifest.Capabilities = &activitypub.AgentCapabilities{
			CanPost:           user.AgentCapabilities.CanPost,
			CanReply:          user.AgentCapabilities.CanReply,
			CanBoost:          user.AgentCapabilities.CanBoost,
			CanFollow:         user.AgentCapabilities.CanFollow,
			CanDM:             user.AgentCapabilities.CanDM,
			RestrictedDomains: append([]string(nil), user.AgentCapabilities.RestrictedDomains...),
			MaxPostsPerHour:   user.AgentCapabilities.MaxPostsPerHour,
			RequiresApproval:  user.AgentCapabilities.RequiresApproval,
		}
	}

	actor.AgentManifest = manifest
}

func normalizeOperatedBy(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "@") {
		return trimmed
	}
	return "@" + trimmed
}

func applyActorIdentifiers(actor *activitypub.Actor, base, username string) {
	if base == "" || username == "" {
		return
	}

	actor.ID = fmt.Sprintf("%s/users/%s", base, username)
	actor.URL = fmt.Sprintf("%s/@%s", base, username)
	actor.Inbox = fmt.Sprintf("%s/users/%s/inbox", base, username)
	actor.Outbox = fmt.Sprintf("%s/users/%s/outbox", base, username)
	actor.Followers = fmt.Sprintf("%s/users/%s/followers", base, username)
	actor.Following = fmt.Sprintf("%s/users/%s/following", base, username)
	actor.Liked = fmt.Sprintf("%s/users/%s/liked", base, username)

	if actor.Endpoints == nil {
		actor.Endpoints = &activitypub.Endpoints{}
	}
	actor.Endpoints.SharedInbox = fmt.Sprintf("%s/inbox", base)

	actor.PreferredUsername = username

	if actor.PublicKey != nil {
		actor.PublicKey.Owner = actor.ID
		actor.PublicKey.ID = actor.ID + "#main-key"
	}
}

func mergeUserProfile(actor *activitypub.Actor, user *storage.User, username string) {
	if strings.TrimSpace(actor.Name) == "" {
		actor.Name = preferFirst(
			strings.TrimSpace(user.DisplayName),
			username,
			strings.TrimSpace(user.Username),
		)
	}

	if strings.TrimSpace(actor.Summary) == "" {
		actor.Summary = strings.TrimSpace(user.Note)
	}

	if actor.URL == "" {
		actor.URL = strings.TrimSpace(user.URL)
	}

	if actor.PreferredUsername == "" {
		actor.PreferredUsername = strings.TrimSpace(user.Username)
	}

	assignTimeIfUnset(&actor.Published, user.CreatedAt)
	assignTimeIfUnset(&actor.Updated, user.UpdatedAt)

	if len(actor.Attachment) == 0 {
		actor.Attachment = attachmentsFromFields(user.Fields)
	}

	if actor.Icon == nil {
		actor.Icon = buildImage(user.Avatar)
	}
	if actor.Image == nil {
		actor.Image = buildImage(user.Header)
	}

	if !actor.ManuallyApprovesFollowers {
		actor.ManuallyApprovesFollowers = user.Locked
	}
	if !actor.Discoverable {
		actor.Discoverable = user.Discoverable
	}
}

func preferFirst(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func assignTimeIfUnset(target **time.Time, source time.Time) {
	if *target != nil || source.IsZero() {
		return
	}
	t := source
	*target = &t
}

func attachmentsFromFields(fields []map[string]string) []activitypub.Attachment {
	if len(fields) == 0 {
		return nil
	}

	attachments := make([]activitypub.Attachment, 0, len(fields))
	for _, field := range fields {
		name := strings.TrimSpace(field["name"])
		value := strings.TrimSpace(field["value"])
		if name == "" && value == "" {
			continue
		}
		attachments = append(attachments, activitypub.Attachment{
			Type:  "PropertyValue",
			Name:  name,
			Value: field["value"],
		})
	}
	if len(attachments) == 0 {
		return nil
	}
	return attachments
}

func buildImage(source string) *activitypub.Image {
	if strings.TrimSpace(source) == "" {
		return nil
	}
	return &activitypub.Image{
		BaseObject: activitypub.BaseObject{
			Type: activitypub.ImageType,
		},
		URL: strings.TrimSpace(source),
	}
}

func copyScalarFields(dst, src *activitypub.Actor) {
	if dst.Type == "" {
		dst.Type = src.Type
	}
	if dst.ID == "" {
		dst.ID = src.ID
	}
	if dst.URL == "" {
		dst.URL = src.URL
	}
	if dst.Inbox == "" {
		dst.Inbox = src.Inbox
	}
	if dst.Outbox == "" {
		dst.Outbox = src.Outbox
	}
	if dst.Followers == "" {
		dst.Followers = src.Followers
	}
	if dst.Following == "" {
		dst.Following = src.Following
	}
	if dst.Liked == "" {
		dst.Liked = src.Liked
	}
	if dst.PreferredUsername == "" {
		dst.PreferredUsername = src.PreferredUsername
	}
	if dst.Name == "" {
		dst.Name = src.Name
	}
	if dst.Summary == "" {
		dst.Summary = src.Summary
	}
	if dst.Context == nil && src.Context != nil {
		dst.Context = src.Context.Clone()
	}
}

func copyTemporalFields(dst, src *activitypub.Actor) {
	if dst.Published == nil && src.Published != nil {
		cloneTime := *src.Published
		dst.Published = &cloneTime
	}
	if dst.Updated == nil && src.Updated != nil {
		cloneTime := *src.Updated
		dst.Updated = &cloneTime
	}
}

func copySliceFields(dst, src *activitypub.Actor) {
	if len(dst.To) == 0 && len(src.To) > 0 {
		dst.To = append([]string{}, src.To...)
	}
	if len(dst.CC) == 0 && len(src.CC) > 0 {
		dst.CC = append([]string{}, src.CC...)
	}
	if len(dst.BTo) == 0 && len(src.BTo) > 0 {
		dst.BTo = append([]string{}, src.BTo...)
	}
	if len(dst.BCC) == 0 && len(src.BCC) > 0 {
		dst.BCC = append([]string{}, src.BCC...)
	}
	if len(dst.AlsoKnownAs) == 0 && len(src.AlsoKnownAs) > 0 {
		dst.AlsoKnownAs = append([]string{}, src.AlsoKnownAs...)
	}
	if len(dst.Attachment) == 0 && len(src.Attachment) > 0 {
		dst.Attachment = cloneAttachments(src.Attachment)
	}
}

func copyMediaFields(dst, src *activitypub.Actor) {
	if dst.Icon == nil && src.Icon != nil {
		dst.Icon = cloneImage(src.Icon)
	}
	if dst.Image == nil && src.Image != nil {
		dst.Image = cloneImage(src.Image)
	}
	if dst.Icon != nil && dst.Icon.MediaType == "" && src.Icon != nil && src.Icon.MediaType != "" {
		dst.Icon.MediaType = src.Icon.MediaType
	}
	if dst.Image != nil && dst.Image.MediaType == "" && src.Image != nil && src.Image.MediaType != "" {
		dst.Image.MediaType = src.Image.MediaType
	}
}

func copyEndpointFields(dst, src *activitypub.Actor) {
	if src.Endpoints == nil {
		return
	}
	if dst.Endpoints == nil {
		dst.Endpoints = cloneEndpoints(src.Endpoints)
		return
	}
	if dst.Endpoints.SharedInbox == "" {
		dst.Endpoints.SharedInbox = src.Endpoints.SharedInbox
	}
}

func copyKeyFields(dst, src *activitypub.Actor) {
	if src.PublicKey == nil {
		return
	}
	if dst.PublicKey == nil {
		dst.PublicKey = clonePublicKey(src.PublicKey)
		return
	}
	if dst.PublicKey.Owner == "" {
		dst.PublicKey.Owner = src.PublicKey.Owner
	}
	if dst.PublicKey.ID == "" {
		dst.PublicKey.ID = src.PublicKey.ID
	}
}

func mergeBooleanFlags(dst, src *activitypub.Actor) {
	if !dst.ManuallyApprovesFollowers && src.ManuallyApprovesFollowers {
		dst.ManuallyApprovesFollowers = true
	}
	if !dst.Discoverable && src.Discoverable {
		dst.Discoverable = true
	}
}
