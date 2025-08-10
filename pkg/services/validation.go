package services

import (
	"strings"
)

// validationService implements ValidationService
type validationService struct {
	config *ServiceConfig
}

// NewValidationService creates a new validation service
func NewValidationService(config *ServiceConfig) ValidationService {
	return &validationService{
		config: config,
	}
}

// ValidateCreatePost validates post creation input
func (v *validationService) ValidateCreatePost(input *CreatePostInput) error {
	if input.Content == "" {
		return NewValidationError("Post content is required")
	}

	if len(input.Content) > 500 {
		return NewValidationError("Post content must not exceed 500 characters")
	}

	if input.Visibility != "" && !isValidVisibility(input.Visibility) {
		return NewValidationError("Invalid visibility setting")
	}

	if len(input.MediaIDs) > 4 {
		return NewValidationError("Maximum 4 media attachments allowed")
	}

	return nil
}

// ValidateFollowInput validates follow operation input
func (v *validationService) ValidateFollowInput(input *FollowInput) error {
	if input.TargetActorID == "" {
		return NewValidationError("Target actor ID is required")
	}

	if strings.TrimSpace(input.TargetActorID) == "" {
		return NewValidationError("Target actor ID cannot be empty")
	}

	return nil
}

// ValidateLikeInput validates like operation input
func (v *validationService) ValidateLikeInput(input *LikeInput) error {
	if input.ObjectID == "" {
		return NewValidationError("Object ID is required")
	}

	if strings.TrimSpace(input.ObjectID) == "" {
		return NewValidationError("Object ID cannot be empty")
	}

	return nil
}

// ValidateDeletePost validates post deletion input
func (v *validationService) ValidateDeletePost(input *DeletePostInput) error {
	if input.ObjectID == "" {
		return NewValidationError("Object ID is required")
	}

	if strings.TrimSpace(input.ObjectID) == "" {
		return NewValidationError("Object ID cannot be empty")
	}

	return nil
}

// ValidateUpdatePost validates post update input
func (v *validationService) ValidateUpdatePost(input *UpdatePostInput) error {
	if input.ObjectID == "" {
		return NewValidationError("Object ID is required")
	}

	if input.Content != "" && len(input.Content) > 500 {
		return NewValidationError("Post content must not exceed 500 characters")
	}

	if input.Visibility != "" && !isValidVisibility(input.Visibility) {
		return NewValidationError("Invalid visibility setting")
	}

	return nil
}

// Helper functions
func isValidVisibility(visibility string) bool {
	validVisibilities := map[string]bool{
		"public":   true,
		"unlisted": true,
		"private":  true,
		"direct":   true,
	}
	return validVisibilities[visibility]
}