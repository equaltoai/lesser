// Package factories provides timeline factory for test data generation  
package factories

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// TimelineFactory creates timeline data for testing
type TimelineFactory struct {
	domain         string
	actorFactory   *ActorFactory
	activityFactory *ActivityFactory
	sequence       int
}

// NewTimelineFactory creates a new timeline factory
func NewTimelineFactory(domain string) *TimelineFactory {
	return &TimelineFactory{
		domain:          domain,
		actorFactory:    NewActorFactory(domain),
		activityFactory: NewActivityFactory(domain),
		sequence:        1,
	}
}

// TimelineScenario represents different timeline scenarios for testing
type TimelineScenario string

const (
	// EmptyTimeline has no posts
	EmptyTimeline TimelineScenario = "empty"
	// SimpleTimeline has basic posts from followed users
	SimpleTimeline TimelineScenario = "simple"
	// MixedTimeline has posts, replies, boosts, and likes
	MixedTimeline TimelineScenario = "mixed"
	// HighVolumeTimeline has many posts for performance testing
	HighVolumeTimeline TimelineScenario = "high_volume"
	// ConversationTimeline has threaded conversations
	ConversationTimeline TimelineScenario = "conversation"
)

// TimelineData represents a complete timeline setup for testing
type TimelineData struct {
	User        *activitypub.Actor
	Following   []*activitypub.Actor
	Followers   []*activitypub.Actor
	Activities  []*activitypub.Activity
	Scenario    TimelineScenario
}

// CreateTimelineScenario creates a complete timeline scenario
func (f *TimelineFactory) CreateTimelineScenario(username string, scenario TimelineScenario) *TimelineData {
	data := &TimelineData{
		Scenario: scenario,
	}

	switch scenario {
	case EmptyTimeline:
		data = f.createEmptyTimeline(username)
	case SimpleTimeline:
		data = f.createSimpleTimeline(username)
	case MixedTimeline:
		data = f.createMixedTimeline(username)
	case HighVolumeTimeline:
		data = f.createHighVolumeTimeline(username)
	case ConversationTimeline:
		data = f.createConversationTimeline(username)
	}

	return data
}

// createEmptyTimeline creates a timeline with no content
func (f *TimelineFactory) createEmptyTimeline(username string) *TimelineData {
	user := f.actorFactory.CreateActor(ActorOptions{Username: username})
	
	return &TimelineData{
		User:       user,
		Following:  []*activitypub.Actor{},
		Followers:  []*activitypub.Actor{},
		Activities: []*activitypub.Activity{},
		Scenario:   EmptyTimeline,
	}
}

// createSimpleTimeline creates a basic timeline with simple posts
func (f *TimelineFactory) createSimpleTimeline(username string) *TimelineData {
	user := f.actorFactory.CreateActor(ActorOptions{Username: username})
	
	// Create 3 users that this user follows
	following := f.actorFactory.CreateActorBatch(3, "following_")
	
	// Create some simple posts from followed users
	activities := make([]*activitypub.Activity, 0)
	
	baseTime := time.Now().Add(-2 * time.Hour)
	
	for i, followedUser := range following {
		for j := 0; j < 2; j++ { // 2 posts per followed user
			content := fmt.Sprintf("This is post #%d from %s", j+1, followedUser.PreferredUsername)
			publishTime := baseTime.Add(time.Duration(i*30+j*10) * time.Minute)
			
			note := f.activityFactory.CreateNote(content, followedUser.ID, NoteOptions{
				Published: &publishTime,
			})
			
			activity := f.activityFactory.CreateActivity(ActivityOptions{
				Type:      "Create",
				Actor:     followedUser.ID,
				Object:    note,
				Published: &publishTime,
			})
			
			activities = append(activities, activity)
		}
	}

	return &TimelineData{
		User:       user,
		Following:  following,
		Followers:  []*activitypub.Actor{},
		Activities: activities,
		Scenario:   SimpleTimeline,
	}
}

// createMixedTimeline creates a timeline with various activity types
func (f *TimelineFactory) createMixedTimeline(username string) *TimelineData {
	user := f.actorFactory.CreateActor(ActorOptions{Username: username})
	
	// Create users to follow
	following := f.actorFactory.CreateActorBatch(5, "mixed_following_")
	
	activities := make([]*activitypub.Activity, 0)
	baseTime := time.Now().Add(-4 * time.Hour)
	
	for i, followedUser := range following {
		// Create original post
		content := fmt.Sprintf("Original post from %s about testing", followedUser.PreferredUsername)
		publishTime := baseTime.Add(time.Duration(i*60) * time.Minute)
		
		note := f.activityFactory.CreateNote(content, followedUser.ID, NoteOptions{
			Published: &publishTime,
		})
		
		createActivity := f.activityFactory.CreateActivity(ActivityOptions{
			Type:      "Create",
			Actor:     followedUser.ID,
			Object:    note,
			Published: &publishTime,
		})
		activities = append(activities, createActivity)
		
		// Add some likes from other users
		if i > 0 {
			likeTime := publishTime.Add(5 * time.Minute)
			likeActivity := f.activityFactory.CreateLike(
				following[i-1].ID, 
				note.ID,
				ActivityOptions{Published: &likeTime},
			)
			activities = append(activities, likeActivity)
		}
		
		// Add some boosts/announces
		if i > 1 {
			announceTime := publishTime.Add(10 * time.Minute)
			announceActivity := f.activityFactory.CreateAnnounce(
				following[i-2].ID,
				note.ID,
				ActivityOptions{Published: &announceTime},
			)
			activities = append(activities, announceActivity)
		}
		
		// Add reply
		if i < 3 {
			replyTime := publishTime.Add(15 * time.Minute)
			replyContent := fmt.Sprintf("Reply to %s's post", followedUser.PreferredUsername)
			replyNote := f.activityFactory.CreateNote(replyContent, following[(i+1)%len(following)].ID, NoteOptions{
				InReplyTo: note.ID,
				Published: &replyTime,
			})
			
			replyActivity := f.activityFactory.CreateActivity(ActivityOptions{
				Type:      "Create",
				Actor:     following[(i+1)%len(following)].ID,
				Object:    replyNote,
				Published: &replyTime,
			})
			activities = append(activities, replyActivity)
		}
	}

	return &TimelineData{
		User:       user,
		Following:  following,
		Followers:  []*activitypub.Actor{},
		Activities: activities,
		Scenario:   MixedTimeline,
	}
}

// createHighVolumeTimeline creates a timeline with many posts for performance testing
func (f *TimelineFactory) createHighVolumeTimeline(username string) *TimelineData {
	user := f.actorFactory.CreateActor(ActorOptions{Username: username})
	
	// Create many users to follow
	following := f.actorFactory.CreateActorBatch(20, "volume_following_")
	
	activities := make([]*activitypub.Activity, 0, 1000) // Pre-allocate for 1000 activities
	baseTime := time.Now().Add(-24 * time.Hour)
	
	// Create 50 activities per followed user
	for _, followedUser := range following {
		for j := 0; j < 50; j++ {
			content := fmt.Sprintf("High volume post #%d from %s", j+1, followedUser.PreferredUsername)
			publishTime := baseTime.Add(time.Duration(j*5) * time.Minute)
			
			note := f.activityFactory.CreateNote(content, followedUser.ID, NoteOptions{
				Published: &publishTime,
			})
			
			activity := f.activityFactory.CreateActivity(ActivityOptions{
				Type:      "Create",
				Actor:     followedUser.ID,
				Object:    note,
				Published: &publishTime,
			})
			
			activities = append(activities, activity)
		}
	}

	return &TimelineData{
		User:       user,
		Following:  following,
		Followers:  []*activitypub.Actor{},
		Activities: activities,
		Scenario:   HighVolumeTimeline,
	}
}

// createConversationTimeline creates a timeline with threaded conversations
func (f *TimelineFactory) createConversationTimeline(username string) *TimelineData {
	user := f.actorFactory.CreateActor(ActorOptions{Username: username})
	
	// Create conversation participants
	following := f.actorFactory.CreateActorBatch(4, "conv_participant_")
	
	activities := make([]*activitypub.Activity, 0)
	
	// Create multiple conversation threads
	for i := 0; i < 3; i++ {
		threadActivities := f.activityFactory.CreateThread(
			following[0].PreferredUsername,
			fmt.Sprintf("Starting conversation thread #%d about important topics", i+1),
			[]ThreadReply{
				{Actor: following[1].PreferredUsername, Content: "Great point! I'd like to add..."},
				{Actor: following[2].PreferredUsername, Content: "I disagree because..."},
				{Actor: following[0].PreferredUsername, Content: "Thanks for the feedback, let me clarify..."},
				{Actor: following[3].PreferredUsername, Content: "This reminds me of..."},
			},
		)
		activities = append(activities, threadActivities...)
	}
	
	// Create a longer conversation
	conversationMessages := []string{
		"What does everyone think about the new features?",
		"I really like the improved performance",
		"The UI changes are great too",
		"I had some issues with the initial setup though",
		"Really? What kind of issues?",
		"The configuration was unclear",
		"We should improve the documentation",
		"I can help with that if needed",
		"That would be awesome, thanks!",
	}
	
	conversationActivities := f.activityFactory.CreateConversation(
		[]string{following[0].PreferredUsername, following[1].PreferredUsername, following[2].PreferredUsername},
		conversationMessages,
	)
	
	activities = append(activities, conversationActivities...)

	return &TimelineData{
		User:       user,
		Following:  following,
		Followers:  []*activitypub.Actor{},
		Activities: activities,
		Scenario:   ConversationTimeline,
	}
}

// CreateCustomTimeline allows creating a timeline with specific parameters
func (f *TimelineFactory) CreateCustomTimeline(username string, followingCount int, postsPerUser int) *TimelineData {
	user := f.actorFactory.CreateActor(ActorOptions{Username: username})
	
	following := f.actorFactory.CreateActorBatch(followingCount, "custom_following_")
	
	activities := make([]*activitypub.Activity, 0, followingCount*postsPerUser)
	baseTime := time.Now().Add(-6 * time.Hour)
	
	for i, followedUser := range following {
		for j := 0; j < postsPerUser; j++ {
			content := fmt.Sprintf("Custom post #%d from %s", j+1, followedUser.PreferredUsername)
			publishTime := baseTime.Add(time.Duration(i*postsPerUser+j) * time.Minute)
			
			note := f.activityFactory.CreateNote(content, followedUser.ID, NoteOptions{
				Published: &publishTime,
			})
			
			activity := f.activityFactory.CreateActivity(ActivityOptions{
				Type:      "Create",
				Actor:     followedUser.ID,
				Object:    note,
				Published: &publishTime,
			})
			
			activities = append(activities, activity)
		}
	}

	return &TimelineData{
		User:       user,
		Following:  following,
		Followers:  []*activitypub.Actor{},
		Activities: activities,
		Scenario:   "custom",
	}
}

// CreateTimelineWithMedia creates a timeline that includes media attachments
func (f *TimelineFactory) CreateTimelineWithMedia(username string) *TimelineData {
	user := f.actorFactory.CreateActor(ActorOptions{Username: username})
	
	following := f.actorFactory.CreateActorBatch(3, "media_following_")
	
	activities := make([]*activitypub.Activity, 0)
	baseTime := time.Now().Add(-2 * time.Hour)
	
	for i, followedUser := range following {
		// Create post with image attachment
		content := fmt.Sprintf("Check out this image from %s", followedUser.PreferredUsername)
		publishTime := baseTime.Add(time.Duration(i*30) * time.Minute)
		
		attachment := activitypub.Attachment{
			Type:      "Document",
			MediaType: "image/jpeg",
			URL:       fmt.Sprintf("https://%s/media/image_%d.jpg", f.domain, i),
			Name:      fmt.Sprintf("Test image %d", i),
		}
		
		note := f.activityFactory.CreateNote(content, followedUser.ID, NoteOptions{
			Published:   &publishTime,
			Attachments: []activitypub.Attachment{attachment},
		})
		
		activity := f.activityFactory.CreateActivity(ActivityOptions{
			Type:      "Create",
			Actor:     followedUser.ID,
			Object:    note,
			Published: &publishTime,
		})
		
		activities = append(activities, activity)
	}

	return &TimelineData{
		User:       user,
		Following:  following,
		Followers:  []*activitypub.Actor{},
		Activities: activities,
		Scenario:   "media",
	}
}

// CreateNotificationData creates notification data for testing (returns maps to avoid type issues)
func (f *TimelineFactory) CreateNotificationData(username string, count int) []map[string]interface{} {
	user := f.actorFactory.CreateActor(ActorOptions{Username: username})
	otherUsers := f.actorFactory.CreateActorBatch(5, "notif_user_")
	
	notifications := make([]map[string]interface{}, count)
	notificationTypes := []string{"mention", "follow", "favourite", "reblog", "follow_request"}
	
	baseTime := time.Now().Add(-1 * time.Hour)
	
	for i := 0; i < count; i++ {
		notifType := notificationTypes[i%len(notificationTypes)]
		fromUser := otherUsers[i%len(otherUsers)]
		
		notifications[i] = map[string]interface{}{
			"id":         fmt.Sprintf("notif_%d", i+1),
			"type":       notifType,
			"user_id":    user.ID,
			"actor_id":   fromUser.ID,
			"created_at": baseTime.Add(time.Duration(i*5) * time.Minute),
		}
		
		// Add specific data based on notification type
		switch notifType {
		case "mention", "reblog":
			// Create a status that was mentioned or reblogged
			note := f.activityFactory.CreateNote(
				fmt.Sprintf("Test status for %s notification", notifType),
				fromUser.ID,
			)
			notifications[i]["target_id"] = note.ID
		}
	}
	
	return notifications
}

// Reset resets all internal factories
func (f *TimelineFactory) Reset() {
	f.actorFactory.Reset()
	f.activityFactory.Reset()
	f.sequence = 1
}

// GetSequence returns the current sequence number
func (f *TimelineFactory) GetSequence() int {
	return f.sequence
}