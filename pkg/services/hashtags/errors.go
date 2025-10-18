package hashtags

import (
	"errors"
)

// Error variables for hashtag service operations
var (
	// Query errors
	ErrGetHashtag              = errors.New("failed to get hashtag")
	ErrGetFollowedHashtags     = errors.New("failed to get followed hashtags")
	ErrGetHashtagTimeline      = errors.New("failed to get hashtag timeline")
	ErrGetMultiHashtagTimeline = errors.New("failed to get multi-hashtag timeline")
	ErrGetHashtagStats         = errors.New("failed to get hashtag statistics")
	ErrCheckFollowingHashtag   = errors.New("failed to check if following hashtag")

	// Mutation errors
	ErrFollowHashtag              = errors.New("failed to follow hashtag")
	ErrUnfollowHashtag            = errors.New("failed to unfollow hashtag")
	ErrUpdateHashtagNotifications = errors.New("failed to update hashtag notification settings")
	ErrMuteHashtag                = errors.New("failed to mute hashtag")
	ErrUnmuteHashtag              = errors.New("failed to unmute hashtag")

	// Validation errors
	ErrHashtagNameRequired  = errors.New("hashtag name is required")
	ErrHashtagNameTooLong   = errors.New("hashtag name is too long")
	ErrInvalidHashtagFormat = errors.New("invalid hashtag format")
	ErrInvalidMode          = errors.New("invalid timeline mode, must be ANY or ALL")

	// Resource errors
	ErrHashtagNotFound        = errors.New("hashtag not found")
	ErrHashtagAlreadyFollowed = errors.New("hashtag is already followed")
	ErrHashtagNotFollowed     = errors.New("hashtag is not followed")

	// Infrastructure errors
	ErrPublisherNotAvailable = errors.New("publisher not available for hashtag events")
)
