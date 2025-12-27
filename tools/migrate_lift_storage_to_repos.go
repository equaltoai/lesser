// Package main provides migration tooling for converting Lift storage calls to DynamORM repository patterns.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MethodMapping represents a storage method to repository method mapping
type MethodMapping struct {
	Pattern     string // Regex pattern to match
	Replacement string // Replacement string
	Description string // Description for logging
}

// Define all the method mappings based on our analysis
var methodMappings = []MethodMapping{
	// Actor methods
	{`h\.store\.GetActor\(`, `h.repos.Actor().GetActor(`, "GetActor"},
	{`h\.store\.GetActorByID\(`, `h.repos.Actor().GetActorByID(`, "GetActorByID"},
	{`h\.store\.GetActorByNumericID\(`, `h.repos.Actor().GetActorByNumericID(`, "GetActorByNumericID"},
	{`h\.store\.GetActorWithMetadata\(`, `h.repos.Actor().GetActorWithMetadata(`, "GetActorWithMetadata"},
	{`h\.store\.CreateActor\(`, `h.repos.Actor().CreateActor(`, "CreateActor"},
	{`h\.store\.UpdateActor\(`, `h.repos.Actor().UpdateActor(`, "UpdateActor"},
	{`h\.store\.UpdateActorLastStatusTime\(`, `h.repos.Actor().UpdateLastStatusTime(`, "UpdateActorLastStatusTime"},
	{`h\.store\.SetActorFields\(`, `h.repos.Actor().SetFields(`, "SetActorFields"},
	{`h\.store\.SearchActors\(`, `h.repos.Actor().Search(`, "SearchActors"},

	// Object methods
	{`h\.store\.GetObject\(`, `h.repos.Object().GetObject(`, "GetObject"},
	{`h\.store\.CreateObject\(`, `h.repos.Object().Create(`, "CreateObject"},
	{`h\.store\.UpdateObject\(`, `h.repos.Object().Update(`, "UpdateObject"},
	{`h\.store\.DeleteObject\(`, `h.repos.Object().Delete(`, "DeleteObject"},
	{`h\.store\.GetObjectsByActor\(`, `h.repos.Object().GetByActor(`, "GetObjectsByActor"},
	{`h\.store\.CountObjectLikes\(`, `h.repos.Like().CountForObject(`, "CountObjectLikes"},
	{`h\.store\.CountObjectAnnounces\(`, `h.repos.Object().CountAnnounces(`, "CountObjectAnnounces"},
	{`h\.store\.GetObjectLikes\(`, `h.repos.Like().GetForObject(`, "GetObjectLikes"},
	{`h\.store\.GetObjectAnnounces\(`, `h.repos.Object().GetAnnounces(`, "GetObjectAnnounces"},

	// User/Account methods
	{`h\.store\.GetUser\(`, `h.repos.Account().GetUser(`, "GetUser"},
	{`h\.store\.GetUserByEmail\(`, `h.repos.Account().GetUserByEmail(`, "GetUserByEmail"},
	{`h\.store\.CreateUser\(`, `h.repos.Account().CreateUser(`, "CreateUser"},
	{`h\.store\.UpdateUser\(`, `h.repos.Account().UpdateUser(`, "UpdateUser"},
	{`h\.store\.DeleteUser\(`, `h.repos.Account().DeleteAccount(`, "DeleteUser -> DeleteAccount"},
	{`h\.store\.AuthenticateUser\(`, `h.repos.Account().AuthenticateUser(`, "AuthenticateUser"},
	{`h\.store\.GetUserPreferences\(`, `h.repos.Account().GetPreferences(`, "GetUserPreferences"},
	{`h\.store\.UpdateUserPreferences\(`, `h.repos.Account().UpdatePreferences(`, "UpdateUserPreferences"},

	// Relationship methods
	{`h\.store\.IsFollowing\(`, `h.repos.Relationship().GetRelationship(`, "IsFollowing -> GetRelationship"},
	{`h\.store\.CreateFollow\(`, `h.repos.Relationship().CreateRelationship(`, "CreateFollow -> CreateRelationship"},
	{`h\.store\.RemoveFollow\(`, `h.repos.Relationship().DeleteRelationship(`, "RemoveFollow -> DeleteRelationship"},
	{`h\.store\.AcceptFollow\(`, `h.repos.Relationship().AcceptFollowRequest(`, "AcceptFollow -> AcceptFollowRequest"},
	{`h\.store\.GetFollowers\(`, `h.repos.Relationship().GetFollowers(`, "GetFollowers"},
	{`h\.store\.GetFollowing\(`, `h.repos.Relationship().GetFollowing(`, "GetFollowing"},
	{`h\.store\.HasFollowRequest\(`, `h.repos.Relationship().HasFollowRequest(`, "HasFollowRequest"},
	{`h\.store\.GetFollowRequest\(`, `h.repos.Relationship().GetFollowRequest(`, "GetFollowRequest"},
	{`h\.store\.GetPendingFollowRequests\(`, `h.repos.Relationship().GetPendingFollowRequests(`, "GetPendingFollowRequests"},
	{`h\.store\.AcceptFollowRequest\(`, `h.repos.Relationship().AcceptFollowRequest(`, "AcceptFollowRequest"},
	{`h\.store\.RejectFollowRequest\(`, `h.repos.Relationship().RejectFollowRequest(`, "RejectFollowRequest"},

	// Block methods
	{`h\.store\.CreateBlock\(`, `h.repos.Relationship().CreateBlock(`, "CreateBlock"},
	{`h\.store\.DeleteBlock\(`, `h.repos.Relationship().DeleteBlock(`, "DeleteBlock"},
	{`h\.store\.GetBlock\(`, `h.repos.Relationship().GetBlock(`, "GetBlock"},
	{`h\.store\.GetBlockedActors\(`, `h.repos.Relationship().GetBlockedActors(`, "GetBlockedActors"},

	// Mute methods
	{`h\.store\.CreateMute\(`, `h.repos.Relationship().CreateMute(`, "CreateMute"},
	{`h\.store\.DeleteMute\(`, `h.repos.Relationship().DeleteMute(`, "DeleteMute"},
	{`h\.store\.GetMute\(`, `h.repos.Relationship().GetMute(`, "GetMute"},
	{`h\.store\.GetMutedActors\(`, `h.repos.Relationship().GetMutedActors(`, "GetMutedActors"},

	// List methods
	{`h\.store\.CreateList\(`, `h.repos.List().Create(`, "CreateList"},
	{`h\.store\.GetList\(`, `h.repos.List().Get(`, "GetList"},
	{`h\.store\.UpdateList\(`, `h.repos.List().Update(`, "UpdateList"},
	{`h\.store\.DeleteList\(`, `h.repos.List().Delete(`, "DeleteList"},
	{`h\.store\.GetListsForUser\(`, `h.repos.List().GetForUser(`, "GetListsForUser"},
	{`h\.store\.GetListAccounts\(`, `h.repos.List().GetAccounts(`, "GetListAccounts"},
	{`h\.store\.AddAccountsToList\(`, `h.repos.List().AddAccounts(`, "AddAccountsToList"},
	{`h\.store\.RemoveAccountsFromList\(`, `h.repos.List().RemoveAccounts(`, "RemoveAccountsFromList"},

	// Timeline methods
	{`h\.store\.GetHomeTimeline\(`, `h.repos.Timeline().GetHomeTimeline(`, "GetHomeTimeline"},
	{`h\.store\.GetPublicTimeline\(`, `h.repos.Timeline().GetPublicTimeline(`, "GetPublicTimeline"},
	{`h\.store\.GetHashtagTimeline\(`, `h.repos.Timeline().GetHashtagTimeline(`, "GetHashtagTimeline"},
	{`h\.store\.GetListTimeline\(`, `h.repos.Timeline().GetListTimeline(`, "GetListTimeline"},
	{`h\.store\.AddToTimeline\(`, `h.repos.Timeline().AddEntry(`, "AddToTimeline"},
	{`h\.store\.RemoveFromTimeline\(`, `h.repos.Timeline().RemoveEntry(`, "RemoveFromTimeline"},

	// Notification methods
	{`h\.store\.CreateNotification\(`, `h.repos.Notification().Create(`, "CreateNotification"},
	{`h\.store\.GetNotifications\(`, `h.repos.Notification().GetNotifications(`, "GetNotifications"},
	{`h\.store\.MarkNotificationRead\(`, `h.repos.Notification().MarkAsRead(`, "MarkNotificationRead"},
	{`h\.store\.MarkAllNotificationsRead\(`, `h.repos.Notification().MarkAllAsRead(`, "MarkAllNotificationsRead"},
	{`h\.store\.DeleteNotification\(`, `h.repos.Notification().Delete(`, "DeleteNotification"},
	{`h\.store\.GetUnreadNotificationCount\(`, `h.repos.Notification().CountUnread(`, "GetUnreadNotificationCount"},

	// Like methods
	{`h\.store\.CreateLike\(`, `h.repos.Like().Create(`, "CreateLike"},
	{`h\.store\.DeleteLike\(`, `h.repos.Like().Delete(`, "DeleteLike"},
	{`h\.store\.GetLike\(`, `h.repos.Like().Get(`, "GetLike"},
	{`h\.store\.GetLikedObjects\(`, `h.repos.Like().GetLikedObjects(`, "GetLikedObjects"},

	// Bookmark methods
	{`h\.store\.CreateBookmark\(`, `h.repos.Account().AddBookmark(`, "CreateBookmark"},
	{`h\.store\.RemoveBookmark\(`, `h.repos.Account().RemoveBookmark(`, "RemoveBookmark"},
	{`h\.store\.GetBookmarks\(`, `h.repos.Account().GetBookmarks(`, "GetBookmarks"},
	{`h\.store\.IsBookmarked\(`, `h.repos.Account().IsBookmarked(`, "IsBookmarked"},

	// Conversation methods
	{`h\.store\.GetConversation\(`, `h.repos.Conversation().Get(`, "GetConversation"},
	{`h\.store\.GetUserConversations\(`, `h.repos.Conversation().GetForUser(`, "GetUserConversations"},
	{`h\.store\.CreateConversation\(`, `h.repos.Conversation().Create(`, "CreateConversation"},
	{`h\.store\.DeleteConversation\(`, `h.repos.Conversation().Delete(`, "DeleteConversation"},
	{`h\.store\.MarkConversationRead\(`, `h.repos.Conversation().MarkAsRead(`, "MarkConversationRead"},
	{`h\.store\.CreateConversationMute\(`, `h.repos.Conversation().Mute(`, "CreateConversationMute"},
	{`h\.store\.DeleteConversationMute\(`, `h.repos.Conversation().Unmute(`, "DeleteConversationMute"},
	{`h\.store\.IsConversationMuted\(`, `h.repos.Conversation().IsMuted(`, "IsConversationMuted"},

	// Media methods
	{`h\.store\.CreateMedia\(`, `h.repos.Media().Create(`, "CreateMedia"},
	{`h\.store\.GetMedia\(`, `h.repos.Media().Get(`, "GetMedia"},
	{`h\.store\.UpdateMedia\(`, `h.repos.Media().Update(`, "UpdateMedia"},
	{`h\.store\.DeleteMedia\(`, `h.repos.Media().Delete(`, "DeleteMedia"},
	{`h\.store\.GetMediaByUser\(`, `h.repos.Media().GetByUser(`, "GetMediaByUser"},

	// OAuth methods
	{`h\.store\.GetOAuthApp\(`, `h.repos.Account().GetOAuthApp(`, "GetOAuthApp"},
	{`h\.store\.SaveOAuthState\(`, `h.repos.Account().SaveOAuthState(`, "SaveOAuthState"},
	{`h\.store\.GetOAuthState\(`, `h.repos.Account().GetOAuthState(`, "GetOAuthState"},
	{`h\.store\.CreateAuthorizationCode\(`, `h.repos.Account().CreateAuthorizationCode(`, "CreateAuthorizationCode"},
	{`h\.store\.GetUserAppConsent\(`, `h.repos.Account().GetUserAppConsent(`, "GetUserAppConsent"},

	// Activity methods
	{`h\.store\.CreateActivity\(`, `h.repos.Activity().Create(`, "CreateActivity"},
	{`h\.store\.GetActivity\(`, `h.repos.Activity().Get(`, "GetActivity"},
	{`h\.store\.GetActivitiesByActor\(`, `h.repos.Activity().GetByActor(`, "GetActivitiesByActor"},

	// Search methods
	{`h\.store\.SearchAccounts\(`, `h.repos.Search().SearchAccounts(`, "SearchAccounts"},
	{`h\.store\.SearchStatuses\(`, `h.repos.Search().SearchStatuses(`, "SearchStatuses"},
	{`h\.store\.SearchHashtags\(`, `h.repos.Search().SearchHashtags(`, "SearchHashtags"},

	// Community Note methods
	{`h\.store\.CreateCommunityNote\(`, `h.repos.CommunityNote().Create(`, "CreateCommunityNote"},
	{`h\.store\.GetCommunityNote\(`, `h.repos.CommunityNote().Get(`, "GetCommunityNote"},
	{`h\.store\.GetVisibleCommunityNotes\(`, `h.repos.CommunityNote().GetVisible(`, "GetVisibleCommunityNotes"},
	{`h\.store\.CreateCommunityNoteVote\(`, `h.repos.CommunityNote().CreateVote(`, "CreateCommunityNoteVote"},
	{`h\.store\.GetCommunityNotesByAuthor\(`, `h.repos.CommunityNote().GetByAuthor(`, "GetCommunityNotesByAuthor"},
	{`h\.store\.CheckCommunityNoteRateLimit\(`, `h.repos.CommunityNote().CheckRateLimit(`, "CheckCommunityNoteRateLimit"},

	// Instance/Federation methods
	{`h\.store\.GetInstance\(`, `h.repos.Instance().Get(`, "GetInstance"},
	{`h\.store\.CreateInstance\(`, `h.repos.Instance().Create(`, "CreateInstance"},
	{`h\.store\.UpdateInstance\(`, `h.repos.Instance().Update(`, "UpdateInstance"},
	{`h\.store\.GetBlockedInstances\(`, `h.repos.Instance().GetBlocked(`, "GetBlockedInstances"},

	// Special methods - Different repositories
	{`h\.store\.GetStatusesByLink\(`, `h.repos.Status().GetByLink(`, "GetStatusesByLink"},
	{`h\.store\.GetActiveUserCount\(`, `h.repos.Analytics().GetActiveUserCount(`, "GetActiveUserCount"},
	{`h\.store\.GetTotalUserCount\(`, `h.repos.Analytics().GetTotalUserCount(`, "GetTotalUserCount"},
	{`h\.store\.GetStorageUsage\(`, `h.repos.Analytics().GetStorageUsage(`, "GetStorageUsage"},
	{`h\.store\.GetStorageHistory\(`, `h.repos.Analytics().GetStorageHistory(`, "GetStorageHistory"},
	{`h\.store\.GetUserGrowthHistory\(`, `h.repos.Analytics().GetUserGrowthHistory(`, "GetUserGrowthHistory"},
	{`h\.store\.RecordStatusEngagement\(`, `h.repos.Analytics().RecordEngagement(`, "RecordStatusEngagement"},
	{`h\.store\.GetAccountSuggestions\(`, `h.repos.Social().GetSuggestions(`, "GetAccountSuggestions"},
	{`h\.store\.RemoveAccountSuggestion\(`, `h.repos.Social().RemoveSuggestion(`, "RemoveAccountSuggestion"},

	// Methods in non-obvious locations
	{`h\.store\.GetAnnounce\(`, `h.repos.Social().GetAnnounce(`, "GetAnnounce"},
	{`h\.store\.DeleteAnnounce\(`, `h.repos.Social().DeleteAnnounce(`, "DeleteAnnounce"},
	{`h\.store\.CreateStatusPin\(`, `h.repos.Social().CreateStatusPin(`, "CreateStatusPin"},
	{`h\.store\.DeleteStatusPin\(`, `h.repos.Social().DeleteStatusPin(`, "DeleteStatusPin"},
	{`h\.store\.GetStatusPins\(`, `h.repos.Social().GetStatusPins(`, "GetStatusPins"},
	{`h\.store\.IsEndorsed\(`, `h.repos.Relationship().IsEndorsed(`, "IsEndorsed"},
	{`h\.store\.GetAccountNote\(`, `h.repos.User().GetAccountNote(`, "GetAccountNote"},
	{`h\.store\.SetAccountNote\(`, `h.repos.User().UpdateAccountNote(`, "SetAccountNote -> UpdateAccountNote"},

	// Count methods with parameter type issues
	{`h\.store\.GetFollowersCount\(`, `h.repos.Relationship().CountFollowers(`, "GetFollowersCount"},
	{`h\.store\.GetFollowingCount\(`, `h.repos.Relationship().CountFollowing(`, "GetFollowingCount"},
	{`h\.store\.GetStatusCount\(`, `h.repos.Status().CountStatusesByAuthor(`, "GetStatusCount"},
}

// ComplexMapping represents mappings that need special handling
type ComplexMapping struct {
	Pattern     string
	Handler     func(line string) (string, error)
	Description string
}

// Complex mappings that need special handling
var complexMappings = []ComplexMapping{
	// Add complex transformations here if needed
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run migrate_lift_storage_to_repos.go <directory>")
		fmt.Println("Example: go run migrate_lift_storage_to_repos.go /Users/aronprice/lesser/cmd/api/lift")
		os.Exit(1)
	}

	directory := os.Args[1]

	// Find all Go files in the directory
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		fmt.Printf("Error finding files: %v\n", err)
		os.Exit(1)
	}

	totalReplacements := 0
	filesModified := 0

	for _, file := range files {
		replacements, err := processFile(file)
		if err != nil {
			fmt.Printf("Error processing %s: %v\n", file, err)
			continue
		}
		if replacements > 0 {
			filesModified++
			totalReplacements += replacements
			fmt.Printf("✓ %s: %d replacements\n", filepath.Base(file), replacements)
		}
	}

	fmt.Printf("\nMigration complete!\n")
	fmt.Printf("Files modified: %d\n", filesModified)
	fmt.Printf("Total replacements: %d\n", totalReplacements)
}

func processFile(filename string) (int, error) {
	// Read the file
	content, err := os.ReadFile(filename) // #nosec G304 -- controlled input in migration tool
	if err != nil {
		return 0, err
	}

	originalContent := string(content)
	modifiedContent := originalContent
	replacements := 0

	// Apply simple method mappings
	for _, mapping := range methodMappings {
		re := regexp.MustCompile(mapping.Pattern)
		matches := re.FindAllStringIndex(modifiedContent, -1)
		if len(matches) > 0 {
			modifiedContent = re.ReplaceAllString(modifiedContent, mapping.Replacement)
			replacements += len(matches)
		}
	}

	// Apply complex mappings if any
	if len(complexMappings) > 0 {
		lines := strings.Split(modifiedContent, "\n")
		for i, line := range lines {
			for _, mapping := range complexMappings {
				if matched, _ := regexp.MatchString(mapping.Pattern, line); matched {
					newLine, err := mapping.Handler(line)
					if err == nil {
						lines[i] = newLine
						replacements++
					}
				}
			}
		}
		modifiedContent = strings.Join(lines, "\n")
	}

	// Only write if changes were made
	if replacements > 0 {
		// Create backup
		backupName := filename + ".backup"
		err = os.WriteFile(backupName, content, 0600)
		if err != nil {
			return 0, fmt.Errorf("failed to create backup: %w", err)
		}

		// Write modified content
		err = os.WriteFile(filename, []byte(modifiedContent), 0600)
		if err != nil {
			return 0, fmt.Errorf("failed to write file: %w", err)
		}
	}

	return replacements, nil
}
