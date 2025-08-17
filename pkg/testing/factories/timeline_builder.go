package factories

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
)

// TimelineBuilder builds timeline test data using the builder pattern
type TimelineBuilder struct {
	*BaseBuilder
	actorBuilder    *ActorBuilder
	activityBuilder *ActivityBuilder
	noteBuilder     *NoteBuilder
	timeline        *TimelineData
}

// NewTimelineBuilder creates a new timeline builder
func NewTimelineBuilder(domain string) *TimelineBuilder {
	return &TimelineBuilder{
		BaseBuilder:     NewBaseBuilder(domain),
		actorBuilder:    NewActorBuilder(domain),
		activityBuilder: NewActivityBuilder(domain),
		noteBuilder:     NewNoteBuilder(domain),
		timeline: &TimelineData{
			Following:  []*activitypub.Actor{},
			Followers:  []*activitypub.Actor{},
			Activities: []*activitypub.Activity{},
		},
	}
}

// Reset resets the builder to create a new timeline
func (b *TimelineBuilder) Reset() *TimelineBuilder {
	b.timeline = &TimelineData{
		Following:  []*activitypub.Actor{},
		Followers:  []*activitypub.Actor{},
		Activities: []*activitypub.Activity{},
		Scenario:   SimpleTimeline,
	}
	return b
}

// WithUser sets the primary user for the timeline
func (b *TimelineBuilder) WithUser(username string) *TimelineBuilder {
	b.timeline.User = b.actorBuilder.
		Reset().
		WithUsername(username).
		Build()
	return b
}

// WithScenario sets the timeline scenario type
func (b *TimelineBuilder) WithScenario(scenario TimelineScenario) *TimelineBuilder {
	b.timeline.Scenario = scenario
	return b
}

// WithFollowing adds users that the primary user follows
func (b *TimelineBuilder) WithFollowing(count int) *TimelineBuilder {
	for i := 0; i < count; i++ {
		actor := b.actorBuilder.
			Reset().
			WithUsername(fmt.Sprintf("following_%d", i+1)).
			Build()
		b.timeline.Following = append(b.timeline.Following, actor)
	}
	return b
}

// WithFollowers adds followers for the primary user
func (b *TimelineBuilder) WithFollowers(count int) *TimelineBuilder {
	for i := 0; i < count; i++ {
		actor := b.actorBuilder.
			Reset().
			WithUsername(fmt.Sprintf("follower_%d", i+1)).
			Build()
		b.timeline.Followers = append(b.timeline.Followers, actor)
	}
	return b
}

// WithPosts adds posts from followed users
func (b *TimelineBuilder) WithPosts(count int) *TimelineBuilder {
	if err := common.ValidateSliceNotEmpty("following", b.timeline.Following); err != nil {
		// Create some following if none exist
		b.WithFollowing(3)
	}

	baseTime := b.GenerateTimestamp()
	for i := 0; i < count; i++ {
		// Pick an author from following list
		author := b.timeline.Following[i%len(b.timeline.Following)]
		
		// Create a note
		note := b.noteBuilder.
			Reset().
			WithContent(fmt.Sprintf("Post #%d from %s", i+1, author.PreferredUsername)).
			WithAttributedTo(author.ID).
			WithPublished(baseTime.Add(time.Duration(i) * 10 * time.Minute)).
			AsPublic().
			Build()
		
		// Create a Create activity for the note
		activity := b.activityBuilder.
			Reset().
			WithType("Create").
			WithActor(author.ID).
			WithObject(note).
			WithPublished(baseTime.Add(time.Duration(i) * 10 * time.Minute)).
			Build()
		
		b.timeline.Activities = append(b.timeline.Activities, activity)
	}
	return b
}

// WithReplies adds reply activities
func (b *TimelineBuilder) WithReplies(count int) *TimelineBuilder {
	if err := common.ValidateSliceNotEmpty("activities", b.timeline.Activities); err != nil {
		// Need some posts to reply to
		b.WithPosts(3)
	}

	for i := 0; i < count && i < len(b.timeline.Activities); i++ {
		originalActivity := b.timeline.Activities[i]
		
		// Get a replier (could be from followers or following)
		var replier *activitypub.Actor
		if err := common.ValidateSliceNotEmpty("followers", b.timeline.Followers); err == nil {
			replier = b.timeline.Followers[i%len(b.timeline.Followers)]
		} else {
			replier = b.actorBuilder.
				Reset().
				WithUsername(fmt.Sprintf("replier_%d", i+1)).
				Build()
		}
		
		// Create reply note
		var originalNoteID string
		if note, ok := originalActivity.Object.(*activitypub.Note); ok {
			originalNoteID = note.ID
		}
		
		replyNote := b.noteBuilder.
			Reset().
			WithContent(fmt.Sprintf("Reply to post #%d", i+1)).
			WithAttributedTo(replier.ID).
			WithInReplyTo(originalNoteID).
			AsPublic().
			Build()
		
		// Create reply activity
		replyActivity := b.activityBuilder.
			Reset().
			WithType("Create").
			WithActor(replier.ID).
			WithObject(replyNote).
			Build()
		
		b.timeline.Activities = append(b.timeline.Activities, replyActivity)
	}
	return b
}

// WithBoosts adds boost activities
func (b *TimelineBuilder) WithBoosts(count int) *TimelineBuilder {
	if err := common.ValidateSliceNotEmpty("activities", b.timeline.Activities); err != nil {
		// Need some posts to boost
		b.WithPosts(3)
	}

	for i := 0; i < count && i < len(b.timeline.Activities); i++ {
		originalActivity := b.timeline.Activities[i]
		
		// Get a booster
		var booster *activitypub.Actor
		if err := common.ValidateSliceNotEmpty("following", b.timeline.Following); err == nil {
			booster = b.timeline.Following[(i+1)%len(b.timeline.Following)]
		} else {
			booster = b.actorBuilder.
				Reset().
				WithUsername(fmt.Sprintf("booster_%d", i+1)).
				Build()
		}
		
		// Create boost activity
		boostActivity := b.activityBuilder.
			Reset().
			WithType("Announce").
			WithActor(booster.ID).
			WithObject(originalActivity.Object).
			WithTo("https://www.w3.org/ns/activitystreams#Public").
			Build()
		
		b.timeline.Activities = append(b.timeline.Activities, boostActivity)
	}
	return b
}

// WithLikes adds like activities
func (b *TimelineBuilder) WithLikes(count int) *TimelineBuilder {
	if err := common.ValidateSliceNotEmpty("activities", b.timeline.Activities); err != nil {
		// Need some posts to like
		b.WithPosts(3)
	}

	for i := 0; i < count && i < len(b.timeline.Activities); i++ {
		originalActivity := b.timeline.Activities[i]
		
		// Get a liker
		var liker *activitypub.Actor
		if err := common.ValidateSliceNotEmpty("followers", b.timeline.Followers); err == nil {
			liker = b.timeline.Followers[i%len(b.timeline.Followers)]
		} else {
			liker = b.actorBuilder.
				Reset().
				WithUsername(fmt.Sprintf("liker_%d", i+1)).
				Build()
		}
		
		// Create like activity
		likeActivity := b.activityBuilder.
			Reset().
			WithType("Like").
			WithActor(liker.ID).
			WithObject(originalActivity.Object).
			Build()
		
		b.timeline.Activities = append(b.timeline.Activities, likeActivity)
	}
	return b
}

// Build creates the timeline with defaults for any unset values
func (b *TimelineBuilder) Build() *TimelineData {
	// Apply defaults
	if b.timeline.User == nil {
		b.timeline.User = b.actorBuilder.
			Reset().
			WithUsername("testuser").
			Build()
	}
	
	if err := common.ValidateRequiredParam("scenario", string(b.timeline.Scenario)); err != nil {
		b.timeline.Scenario = SimpleTimeline
	}
	
	// Create a copy to return
	result := *b.timeline
	
	// Reset for next build
	b.Reset()
	
	return &result
}

// BuildEmptyTimeline creates an empty timeline
func (b *TimelineBuilder) BuildEmptyTimeline(username string) *TimelineData {
	return b.Reset().
		WithUser(username).
		WithScenario(EmptyTimeline).
		Build()
}

// BuildSimpleTimeline creates a simple timeline with basic posts
func (b *TimelineBuilder) BuildSimpleTimeline(username string) *TimelineData {
	return b.Reset().
		WithUser(username).
		WithScenario(SimpleTimeline).
		WithFollowing(3).
		WithFollowers(2).
		WithPosts(10).
		Build()
}

// BuildMixedTimeline creates a timeline with mixed content
func (b *TimelineBuilder) BuildMixedTimeline(username string) *TimelineData {
	return b.Reset().
		WithUser(username).
		WithScenario(MixedTimeline).
		WithFollowing(5).
		WithFollowers(4).
		WithPosts(10).
		WithReplies(3).
		WithBoosts(2).
		WithLikes(5).
		Build()
}

// BuildHighVolumeTimeline creates a high volume timeline for performance testing
func (b *TimelineBuilder) BuildHighVolumeTimeline(username string) *TimelineData {
	return b.Reset().
		WithUser(username).
		WithScenario(HighVolumeTimeline).
		WithFollowing(20).
		WithFollowers(15).
		WithPosts(100).
		WithReplies(20).
		WithBoosts(10).
		WithLikes(50).
		Build()
}

// BuildConversationTimeline creates a timeline with conversation threads
func (b *TimelineBuilder) BuildConversationTimeline(username string) *TimelineData {
	builder := b.Reset().
		WithUser(username).
		WithScenario(ConversationTimeline).
		WithFollowing(3).
		WithFollowers(3)
	
	// Create conversation threads
	for i := 0; i < 3; i++ {
		// Original post
		builder.WithPosts(1)
		// Multiple replies to create a thread
		builder.WithReplies(4)
	}
	
	return builder.Build()
}