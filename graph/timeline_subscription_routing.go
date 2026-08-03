package graph

import (
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/streaming"
)

const (
	timelineInputActorUsername = "actorUsername"
	timelineInputHashtag       = "hashtag"
	timelineInputListID        = "listId"
)

type timelineRoutingInputs struct {
	actorUsername *string
	hashtag       *string
	listID        *string
}

type validatedTimelineRoute struct {
	actorUsername string
	hashtag       string
	listID        string
}

func validateTimelineRoutingInputs(timelineType model.TimelineType, inputs timelineRoutingInputs) (validatedTimelineRoute, error) {
	if err := rejectUnexpectedTimelineRoutingInputs(timelineType, inputs); err != nil {
		return validatedTimelineRoute{}, err
	}

	switch timelineType {
	case model.TimelineTypeActor:
		if inputs.actorUsername == nil {
			return validatedTimelineRoute{}, ErrActorUsernameRequired
		}
		actorUsername := strings.TrimSpace(*inputs.actorUsername)
		if err := activitypub.ValidateUsername(actorUsername); err != nil {
			return validatedTimelineRoute{}, invalidTimelineRoutingInput(timelineInputActorUsername, timelineType, "a valid local actor username", err)
		}
		return validatedTimelineRoute{actorUsername: actorUsername}, nil
	case model.TimelineTypeHashtag:
		if inputs.hashtag == nil {
			return validatedTimelineRoute{}, ErrHashtagParameterRequired
		}
		hashtag, err := streaming.NormalizeHashtagStreamValue(*inputs.hashtag)
		if err != nil {
			return validatedTimelineRoute{}, invalidTimelineRoutingInput(timelineInputHashtag, timelineType, "a valid hashtag", err)
		}
		return validatedTimelineRoute{hashtag: hashtag}, nil
	case model.TimelineTypeList:
		if inputs.listID == nil {
			return validatedTimelineRoute{}, ErrListIDParameterRequired
		}
		listID := strings.TrimSpace(*inputs.listID)
		if err := common.ValidateStatusID(listID); err != nil {
			return validatedTimelineRoute{}, invalidTimelineRoutingInput(timelineInputListID, timelineType, "a valid list ID", err)
		}
		return validatedTimelineRoute{listID: listID}, nil
	case model.TimelineTypePublic, model.TimelineTypeLocal, model.TimelineTypeHome, model.TimelineTypeDirect:
		return validatedTimelineRoute{}, nil
	default:
		return validatedTimelineRoute{}, ErrUnsupportedTimelineTypeWithValue(timelineType)
	}
}

func rejectUnexpectedTimelineRoutingInputs(timelineType model.TimelineType, inputs timelineRoutingInputs) error {
	expected := ""
	switch timelineType {
	case model.TimelineTypeActor:
		expected = timelineInputActorUsername
	case model.TimelineTypeHashtag:
		expected = timelineInputHashtag
	case model.TimelineTypeList:
		expected = timelineInputListID
	case model.TimelineTypePublic, model.TimelineTypeLocal, model.TimelineTypeHome, model.TimelineTypeDirect:
		// These established routes do not accept an identifying input.
	default:
		return ErrUnsupportedTimelineTypeWithValue(timelineType)
	}

	for _, input := range []struct {
		name  string
		value *string
	}{
		{name: timelineInputActorUsername, value: inputs.actorUsername},
		{name: timelineInputHashtag, value: inputs.hashtag},
		{name: timelineInputListID, value: inputs.listID},
	} {
		if input.value != nil && input.name != expected {
			return apperrors.NewValidationError(input.name, fmt.Sprintf("not allowed for %s timeline", timelineType))
		}
	}

	return nil
}

func invalidTimelineRoutingInput(field string, timelineType model.TimelineType, requirement string, cause error) error {
	return apperrors.NewValidationError(field, fmt.Sprintf("must be %s for %s timeline", requirement, timelineType)).
		WithInternalError(cause)
}

func timelineStreamName(username string, timelineType model.TimelineType, inputs timelineRoutingInputs) (string, error) {
	route, err := validateTimelineRoutingInputs(timelineType, inputs)
	if err != nil {
		return "", err
	}

	switch timelineType {
	case model.TimelineTypeHome:
		username = strings.TrimSpace(username)
		if err := common.ValidateRequiredParam("username", username); err != nil {
			return "", ErrUsernameCannotBeEmpty
		}
		return streaming.UserStreamName(username), nil
	case model.TimelineTypePublic:
		return StreamNamePublic, nil
	case model.TimelineTypeLocal:
		return streaming.PublicLocalStream, nil
	case model.TimelineTypeDirect:
		username = strings.TrimSpace(username)
		if err := common.ValidateRequiredParam("username", username); err != nil {
			return "", ErrUsernameCannotBeEmpty
		}
		return streaming.DirectStreamName(username), nil
	case model.TimelineTypeActor:
		return streaming.PublicActorStreamName(route.actorUsername), nil
	case model.TimelineTypeHashtag:
		return streaming.HashtagStreamName(route.hashtag), nil
	case model.TimelineTypeList:
		return streaming.ListStreamName(route.listID), nil
	default:
		return "", ErrUnsupportedTimelineTypeWithValue(timelineType)
	}
}
