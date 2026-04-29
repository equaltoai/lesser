// Package handlers provides WebSocket command handlers for system/utility commands
package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	mediatools "github.com/equaltoai/lesser/pkg/media"
	"github.com/equaltoai/lesser/pkg/services/lists"
	mediasvc "github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// SystemCommandHandler handles WebSocket commands for system-level operations
type SystemCommandHandler struct {
	*streaming.BaseCommandHandler
	notesService         notesTimelineAPI
	listsService         listsAPI
	mediaService         mediaUploader
	notificationsService notificationsAPI
}

type notesTimelineAPI interface {
	GetTimeline(ctx context.Context, query *notes.GetTimelineQuery) (*notes.Result, error)
}

type listsAPI interface {
	CreateList(ctx context.Context, cmd *lists.CreateListCommand) (*lists.ListResult, error)
	UpdateList(ctx context.Context, cmd *lists.UpdateListCommand) (*lists.ListResult, error)
	DeleteList(ctx context.Context, cmd *lists.DeleteListCommand) error
	AddToList(ctx context.Context, cmd *lists.AddToListCommand) (*lists.MembershipResult, error)
	RemoveFromList(ctx context.Context, cmd *lists.RemoveFromListCommand) (*lists.MembershipResult, error)
}

type notificationsAPI interface {
	MarkAsRead(ctx context.Context, cmd *notifications.MarkAsReadCommand) (*notifications.NotificationResult, error)
	ClearNotifications(ctx context.Context, cmd *notifications.ClearCommand) (*notifications.ClearResult, error)
	ListNotifications(ctx context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error)
}

type mediaUploader interface {
	UploadMedia(ctx context.Context, cmd *mediasvc.UploadMediaCommand) (*mediasvc.Result, error)
}

const maxWebSocketUploadBytes = 10 * 1024 * 1024

type wsUploadPayload struct {
	Data        []byte
	Filename    string
	MimeType    string
	Description string
	Focus       string
	Sensitive   bool
	SpoilerText string
	Category    models.MediaCategory
}

// NewSystemCommandHandler creates a new system command handler
func NewSystemCommandHandler(
	notesService *notes.Service,
	listsService *lists.Service,
	mediaService mediaUploader,
	notificationsService *notifications.Service,
	logger *zap.Logger,
) *SystemCommandHandler {
	return &SystemCommandHandler{
		BaseCommandHandler:   streaming.NewBaseCommandHandler(logger),
		notesService:         notesService,
		listsService:         listsService,
		mediaService:         mediaService,
		notificationsService: notificationsService,
	}
}

// GetSupportedCommands returns the list of commands this handler supports
func (sch *SystemCommandHandler) GetSupportedCommands() []string {
	return []string{
		// List commands
		streaming.CmdCreateList,
		streaming.CmdUpdateList,
		streaming.CmdDeleteList,
		streaming.CmdAddToList,
		streaming.CmdRemoveFromList,
		// Media commands
		streaming.CmdUploadMedia,
		// Notification commands
		streaming.CmdMarkNotificationRead,
		streaming.CmdMarkAllNotificationsRead,
		streaming.CmdDismissNotification,
		// Timeline commands
		streaming.CmdSubscribeTimeline,
		streaming.CmdUnsubscribeTimeline,
		// System commands
		streaming.CmdGetServerInfo,
		streaming.CmdGetTimeline,
		streaming.CmdGetNotifications,
	}
}

// HandleCommand processes system-related WebSocket commands
func (sch *SystemCommandHandler) HandleCommand(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	switch cmd.Type {
	// List commands
	case streaming.CmdCreateList:
		return sch.handleCreateList(ctx, conn, cmd)
	case streaming.CmdUpdateList:
		return sch.handleUpdateList(ctx, conn, cmd)
	case streaming.CmdDeleteList:
		return sch.handleDeleteList(ctx, conn, cmd)
	case streaming.CmdAddToList:
		return sch.handleAddToList(ctx, conn, cmd)
	case streaming.CmdRemoveFromList:
		return sch.handleRemoveFromList(ctx, conn, cmd)
	// Media commands
	case streaming.CmdUploadMedia:
		return sch.handleUploadMedia(ctx, conn, cmd)
	// Notification commands
	case streaming.CmdMarkNotificationRead:
		return sch.handleMarkNotificationRead(ctx, conn, cmd)
	case streaming.CmdMarkAllNotificationsRead:
		return sch.handleMarkAllNotificationsRead(ctx, conn, cmd)
	case streaming.CmdDismissNotification:
		return sch.handleDismissNotification(ctx, conn, cmd)
	// Timeline commands
	case streaming.CmdSubscribeTimeline:
		return sch.handleSubscribeTimeline(ctx, conn, cmd)
	case streaming.CmdUnsubscribeTimeline:
		return sch.handleUnsubscribeTimeline(ctx, conn, cmd)
	// System commands
	case streaming.CmdGetServerInfo:
		return sch.handleGetServerInfo(ctx, conn, cmd)
	case streaming.CmdGetTimeline:
		return sch.handleGetTimeline(ctx, conn, cmd)
	case streaming.CmdGetNotifications:
		return sch.handleGetNotifications(ctx, conn, cmd)
	default:
		return sch.CreateErrorResponse(cmd.ID, "UNSUPPORTED_COMMAND",
			"Unsupported system command", fmt.Sprintf("Command %s not supported by system handler", cmd.Type)), nil
	}
}

// List Command Handlers

// handleCreateList handles creating a new list
func (sch *SystemCommandHandler) handleCreateList(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"title"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	title := sch.GetString(cmd.Payload, "title", "")
	repliesPolicy := sch.GetString(cmd.Payload, "replies_policy", "list")

	createCmd := &lists.CreateListCommand{
		Username:      conn.UserID, // conn.UserID is the username
		Title:         title,
		RepliesPolicy: repliesPolicy,
		CreatorID:     conn.UserID,
	}

	result, err := sch.listsService.CreateList(ctx, createCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CREATE_LIST_FAILED",
			"Failed to create list", err.Error()), nil
	}

	data, err := sch.ConvertToJSON(result.List)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUpdateList handles updating an existing list
func (sch *SystemCommandHandler) handleUpdateList(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	listID := sch.GetString(cmd.Payload, "id", "")
	title := sch.GetString(cmd.Payload, "title", "")
	repliesPolicy := sch.GetString(cmd.Payload, "replies_policy", "")

	updateCmd := &lists.UpdateListCommand{
		ListID:        listID,
		Title:         title,
		RepliesPolicy: repliesPolicy,
		UpdaterID:     conn.UserID,
	}

	result, err := sch.listsService.UpdateList(ctx, updateCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "UPDATE_LIST_FAILED",
			"Failed to update list", err.Error()), nil
	}

	data, err := sch.ConvertToJSON(result.List)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleDeleteList handles deleting a list
func (sch *SystemCommandHandler) handleDeleteList(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	listID := sch.GetString(cmd.Payload, "id", "")

	deleteCmd := &lists.DeleteListCommand{
		ListID:    listID,
		DeleterID: conn.UserID,
	}

	err := sch.listsService.DeleteList(ctx, deleteCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "DELETE_LIST_FAILED",
			"Failed to delete list", err.Error()), nil
	}

	data := map[string]interface{}{
		"deleted": true,
		"id":      listID,
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleListMembership handles adding or removing accounts from a list
func (sch *SystemCommandHandler) handleListMembership(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command, isAdd bool) (*streaming.CommandResponse, error) {
	config := &streaming.ListCommandValidationConfig{
		RequiredFields: []string{"id", "account_id"},
	}
	if validationErr := sch.ValidateListCommand(conn, cmd, config); validationErr != nil {
		return validationErr, nil
	}

	listID := sch.GetString(cmd.Payload, "id", "")
	accountID := sch.GetString(cmd.Payload, "account_id", "")

	var result *lists.MembershipResult
	var err error

	if isAdd {
		addCmd := &lists.AddToListCommand{
			ListID:         listID,
			MemberUsername: accountID, // account_id is the username
			AdderID:        conn.UserID,
		}
		result, err = sch.listsService.AddToList(ctx, addCmd)
		if err != nil {
			return sch.CreateErrorResponse(cmd.ID, "ADD_TO_LIST_FAILED",
				"Failed to add accounts to list", err.Error()), nil
		}
	} else {
		removeCmd := &lists.RemoveFromListCommand{
			ListID:         listID,
			MemberUsername: accountID, // account_id is the username
			RemoverID:      conn.UserID,
		}
		result, err = sch.listsService.RemoveFromList(ctx, removeCmd)
		if err != nil {
			return sch.CreateErrorResponse(cmd.ID, "REMOVE_FROM_LIST_FAILED",
				"Failed to remove accounts from list", err.Error()), nil
		}
	}

	var resultKey string
	if isAdd {
		resultKey = "added"
	} else {
		resultKey = "removed"
	}

	data := map[string]interface{}{
		resultKey:    result.Success,
		"list_id":    listID,
		"account_id": accountID,
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleAddToList handles adding accounts to a list
func (sch *SystemCommandHandler) handleAddToList(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return sch.handleListMembership(ctx, conn, cmd, true)
}

// handleRemoveFromList handles removing accounts from a list
func (sch *SystemCommandHandler) handleRemoveFromList(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return sch.handleListMembership(ctx, conn, cmd, false)
}

// Media Command Handlers

// handleUploadMedia handles media uploads via WebSocket commands.
func (sch *SystemCommandHandler) handleUploadMedia(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if sch.mediaService == nil {
		return sch.CreateErrorResponse(cmd.ID, "UPLOAD_MEDIA_UNAVAILABLE",
			"Media service unavailable",
			"Upload service is not configured"), nil
	}

	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"file_data"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	payload, errResp := sch.parseUploadPayload(cmd.Payload, cmd.ID)
	if errResp != nil {
		return errResp, nil
	}

	userID := conn.UserID
	if userID == "" {
		userID = conn.Username
	}

	result, err := sch.mediaService.UploadMedia(ctx, &mediasvc.UploadMediaCommand{
		UserID:        userID,
		FileName:      payload.Filename,
		ContentType:   payload.MimeType,
		FileData:      payload.Data,
		Description:   payload.Description,
		Focus:         payload.Focus,
		Sensitive:     payload.Sensitive,
		SpoilerText:   payload.SpoilerText,
		MediaCategory: payload.Category,
	})
	if err != nil {
		sch.Logger().Error("media upload failed",
			zap.String("user", userID),
			zap.String("filename", payload.Filename),
			zap.Error(err))
		return sch.CreateErrorResponse(cmd.ID, "UPLOAD_MEDIA_FAILED",
			"Failed to upload media",
			err.Error()), nil
	}

	if result == nil || result.Media == nil {
		sch.Logger().Error("media upload returned empty result",
			zap.String("user", userID))
		return sch.CreateErrorResponse(cmd.ID, "UPLOAD_MEDIA_FAILED",
			"Failed to upload media",
			"empty result"), nil
	}

	sch.Logger().Info("media uploaded via websocket",
		zap.String("user", userID),
		zap.String("media_id", result.Media.MediaID),
		zap.String("filename", payload.Filename),
		zap.String("content_type", payload.MimeType),
		zap.Int64("size", result.Media.FileSize))

	return sch.CreateSuccessResponse(cmd.ID, buildWebSocketMediaResponse(result.Media)), nil
}

func (sch *SystemCommandHandler) parseUploadPayload(payload map[string]interface{}, commandID string) (*wsUploadPayload, *streaming.CommandResponse) {
	fileData := sch.GetString(payload, "file_data", "")
	if fileData == "" {
		return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
			"File data is required",
			"Provide base64-encoded file data")
	}

	if comma := strings.Index(fileData, ","); comma != -1 {
		fileData = fileData[comma+1:]
	}
	fileData = strings.TrimSpace(fileData)

	if len(fileData) > base64.StdEncoding.EncodedLen(maxWebSocketUploadBytes) {
		return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
			"Uploaded file is too large",
			fmt.Sprintf("WebSocket media uploads are limited to %d bytes", maxWebSocketUploadBytes))
	}

	decoded, err := base64.StdEncoding.DecodeString(fileData)
	if err != nil {
		sch.Logger().Warn("failed to decode media payload", zap.Error(err))
		return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
			"Invalid media payload",
			err.Error())
	}

	if len(decoded) == 0 {
		return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
			"Uploaded file is empty",
			"Ensure the media payload contains data")
	}

	if len(decoded) > maxWebSocketUploadBytes {
		return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
			"Uploaded file is too large",
			fmt.Sprintf("WebSocket media uploads are limited to %d bytes", maxWebSocketUploadBytes))
	}

	filename := sch.GetString(payload, "file_name", "")
	if filename == "" {
		filename = sch.GetString(payload, "filename", "")
	}
	if strings.TrimSpace(filename) == "" {
		filename = fmt.Sprintf("upload-%d", time.Now().Unix())
	}

	mimeType := strings.TrimSpace(sch.GetString(payload, "mime_type", ""))
	if mimeType == "" {
		mimeType = strings.TrimSpace(sch.GetString(payload, "content_type", ""))
	}
	if mimeType == "" {
		mimeType = strings.TrimSpace(sch.GetString(payload, "contentType", ""))
	}
	if mimeType == "" {
		sniff := len(decoded)
		if sniff > 512 {
			sniff = 512
		}
		mimeType = http.DetectContentType(decoded[:sniff])
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	if err := mediasvc.ValidateSVGUpload(mimeType, decoded); err != nil {
		sch.Logger().Warn("unsafe SVG media upload rejected", zap.Error(err))
		return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
			"Invalid SVG media",
			err.Error())
	}

	description := strings.TrimSpace(sch.GetString(payload, "description", ""))
	if description != "" {
		if err := common.ValidateMediaDescription(description); err != nil {
			sch.Logger().Warn("invalid media description", zap.Error(err))
			return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
				"Invalid media description",
				err.Error())
		}
	}

	spoilerText := strings.TrimSpace(sch.GetString(payload, "spoiler_text", ""))
	if spoilerText == "" {
		spoilerText = strings.TrimSpace(sch.GetString(payload, "spoilerText", ""))
	}
	if spoilerText != "" {
		if err := common.ValidateSpoilerText(spoilerText); err != nil {
			sch.Logger().Warn("invalid media spoiler text", zap.Error(err))
			return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
				"Invalid spoiler text",
				err.Error())
		}
	}

	mediaTypeValue := sch.GetString(payload, "media_type", "")
	if mediaTypeValue == "" {
		mediaTypeValue = sch.GetString(payload, "mediaType", "")
	}
	category, categoryErr := resolveMediaCategory(mediaTypeValue, mimeType)
	if categoryErr != nil {
		sch.Logger().Warn("invalid media category", zap.Error(categoryErr))
		return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
			"Invalid media category",
			categoryErr.Error())
	}

	sensitive := sch.GetBool(payload, "sensitive", false)

	filename, err = ensureFilenameForMime(filename, mimeType)
	if err != nil {
		return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
			"Invalid filename",
			err.Error())
	}

	focus, focusErr := parseFocusValue(payload["focus"])
	if focusErr != nil {
		sch.Logger().Warn("invalid media focus", zap.Error(focusErr))
		return nil, sch.CreateErrorResponse(commandID, "UPLOAD_MEDIA_INVALID",
			"Invalid media focus",
			focusErr.Error())
	}

	return &wsUploadPayload{
		Data:        decoded,
		Filename:    filename,
		MimeType:    mimeType,
		Description: description,
		Focus:       focus,
		Sensitive:   sensitive,
		SpoilerText: spoilerText,
		Category:    category,
	}, nil
}

func parseFocusValue(value interface{}) (string, error) {
	if value == nil {
		return "", nil
	}

	switch v := value.(type) {
	case string:
		focus := strings.TrimSpace(v)
		if focus == "" {
			return "", nil
		}
		if err := common.ValidateMediaFocus(focus); err != nil {
			return "", err
		}
		return focus, nil
	case map[string]interface{}:
		xVal, xOK := v["x"].(float64)
		yVal, yOK := v["y"].(float64)
		if !xOK || !yOK {
			return "", errors.New("focus requires x and y coordinates")
		}
		focus := fmt.Sprintf("%.2f,%.2f", xVal, yVal)
		if err := common.ValidateMediaFocus(focus); err != nil {
			return "", err
		}
		return focus, nil
	default:
		return "", errors.New("focus must be a string or {x,y} object")
	}
}

func resolveMediaCategory(category string, mimeType string) (models.MediaCategory, error) {
	category = strings.TrimSpace(category)
	if category == "" {
		return models.DetermineMediaCategory(mimeType), nil
	}

	if normalized, ok := models.NormalizeMediaCategory(category); ok {
		return normalized, nil
	}

	return "", fmt.Errorf("unsupported media category '%s'", category)
}

func ensureFilenameForMime(filename, mimeType string) (string, error) {
	trimmed := strings.TrimSpace(filename)

	result, err := mediatools.EnsureFilenameHasExtension(trimmed, mimeType)
	if err != nil {
		return "", err
	}

	return result, nil
}

func buildWebSocketMediaResponse(record *models.Media) map[string]interface{} {
	mediaData := map[string]interface{}{
		"id":             record.MediaID,
		"url":            record.CDNUrl,
		"mime_type":      record.ContentType,
		"size":           record.FileSize,
		"description":    record.Description,
		"created_at":     record.CreatedAt.Format(time.RFC3339),
		"sensitive":      record.IsNSFW,
		"media_category": string(record.MediaCategory),
	}

	if record.Blurhash != "" {
		mediaData["blurhash"] = record.Blurhash
	}
	if record.Focus != "" {
		mediaData["focus"] = record.Focus
	}
	if record.CDNUrl != "" {
		mediaData["preview_url"] = record.CDNUrl
	}
	if record.SpoilerText != "" {
		mediaData["spoiler_text"] = record.SpoilerText
	}

	return map[string]interface{}{
		"media":     mediaData,
		"upload_id": record.MediaID,
		"warnings":  []string{},
	}
}

// Notification Command Handlers

func (sch *SystemCommandHandler) handleMarkNotificationRead(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	config := &streaming.CommandHandlerConfig{
		RequiredFields:  []string{"id"},
		ParameterName:   "id",
		ErrorCodePrefix: "MARK_READ",
		OperationName:   "mark notification as read",
		ResultExtractor: func(result interface{}) interface{} {
			return result.(*notifications.NotificationResult).Notification
		},
		ServiceCall: func(ctx context.Context, conn *streaming.ConnectionInfo, notificationID string) (interface{}, error) {
			markCmd := &notifications.MarkAsReadCommand{
				NotificationID: notificationID,
				UserID:         conn.UserID,
			}
			return sch.notificationsService.MarkAsRead(ctx, markCmd)
		},
	}

	return sch.ExecuteStandardCommandFlow(ctx, conn, cmd, config)
}

func (sch *SystemCommandHandler) handleMarkAllNotificationsRead(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Clear all notifications (marking them as read)
	clearCmd := &notifications.ClearCommand{
		UserID:   conn.UserID,
		ClearAll: true,
	}

	result, err := sch.notificationsService.ClearNotifications(ctx, clearCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "MARK_ALL_READ_FAILED",
			"Failed to mark all notifications as read", err.Error()), nil
	}

	data := map[string]interface{}{
		"marked_count": result.ClearedCount,
		"success":      result.ClearedCount > 0,
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

func (sch *SystemCommandHandler) handleDismissNotification(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	notificationID := sch.GetString(cmd.Payload, "id", "")

	// Clear (dismiss) specific notification
	clearCmd := &notifications.ClearCommand{
		UserID:          conn.UserID,
		NotificationIDs: []string{notificationID},
	}

	result, err := sch.notificationsService.ClearNotifications(ctx, clearCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "DISMISS_FAILED",
			"Failed to dismiss notification", err.Error()), nil
	}

	data := map[string]interface{}{
		"dismissed": result.ClearedCount > 0,
		"id":        notificationID,
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleSubscribeTimeline handles subscribing to a timeline for real-time updates
func (sch *SystemCommandHandler) handleSubscribeTimeline(_ context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"timeline"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	timeline := sch.GetString(cmd.Payload, "timeline", "home")

	// Subscribe to the timeline (this would typically update connection subscriptions)
	// For now, we'll return success and rely on the streaming infrastructure
	// to actually push timeline updates

	data := map[string]interface{}{
		"subscribed": true,
		"timeline":   timeline,
		"user_id":    conn.UserID,
	}

	// Store subscription in connection metadata
	if conn.Metadata == nil {
		conn.Metadata = make(map[string]interface{})
	}

	subscriptions, ok := conn.Metadata["subscriptions"].([]string)
	if !ok {
		subscriptions = []string{}
	}

	// Add timeline subscription if not already present
	timelineKey := fmt.Sprintf("timeline:%s", timeline)
	found := false
	for _, sub := range subscriptions {
		if sub == timelineKey {
			found = true
			break
		}
	}

	if !found {
		subscriptions = append(subscriptions, timelineKey)
		conn.Metadata["subscriptions"] = subscriptions
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUnsubscribeTimeline handles unsubscribing from a timeline
func (sch *SystemCommandHandler) handleUnsubscribeTimeline(_ context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"timeline"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	timeline := sch.GetString(cmd.Payload, "timeline", "home")

	// Unsubscribe from the timeline
	data := map[string]interface{}{
		"unsubscribed": true,
		"timeline":     timeline,
		"user_id":      conn.UserID,
	}

	// Remove subscription from connection metadata
	if conn.Metadata != nil {
		subscriptions, ok := conn.Metadata["subscriptions"].([]string)
		if ok {
			timelineKey := fmt.Sprintf("timeline:%s", timeline)
			newSubs := []string{}
			for _, sub := range subscriptions {
				if sub != timelineKey {
					newSubs = append(newSubs, sub)
				}
			}
			conn.Metadata["subscriptions"] = newSubs
		}
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleGetServerInfo returns server information
func (sch *SystemCommandHandler) handleGetServerInfo(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Server info doesn't require authentication

	// Get server configuration and stats
	serverInfo := map[string]interface{}{
		"version":     "1.0.0",
		"server_name": "Lesser",
		"description": "A serverless ActivityPub implementation",
		"features": []string{
			"mastodon_api",
			"activitypub",
			"websocket_streaming",
			"graphql",
			"oauth2",
			"webauthn",
		},
		"stats": map[string]interface{}{
			"user_count":   0, // Would need to query from DB
			"status_count": 0, // Would need to query from DB
			"domain_count": 0, // Would need to query from DB
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	return sch.CreateSuccessResponse(cmd.ID, serverInfo), nil
}

// handleGetTimeline handles timeline retrieval requests
func (sch *SystemCommandHandler) handleGetTimeline(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	timeline := sch.GetString(cmd.Payload, "timeline", "home")
	limit := sch.GetInt(cmd.Payload, "limit", 20)
	sinceID := sch.GetString(cmd.Payload, "since_id", "")
	maxID := sch.GetString(cmd.Payload, "max_id", "")

	// Query timeline from notes service
	query := &notes.GetTimelineQuery{
		UserID:   conn.UserID,
		Timeline: timeline,
		Limit:    limit,
		SinceID:  sinceID,
		MaxID:    maxID,
	}

	result, err := sch.notesService.GetTimeline(ctx, query)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "TIMELINE_FETCH_FAILED",
			"Failed to fetch timeline", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleGetNotifications handles notification retrieval requests
func (sch *SystemCommandHandler) handleGetNotifications(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	limit := sch.GetInt(cmd.Payload, "limit", 20)
	// sinceID is not used in the current implementation
	// sinceID := sch.GetString(cmd.Payload, "since_id", "")
	maxID := sch.GetString(cmd.Payload, "max_id", "")
	excludeTypes := sch.GetStringSlice(cmd.Payload, "exclude_types")
	includeTypes := sch.GetStringSlice(cmd.Payload, "include_types")

	// Query notifications from service
	query := &notifications.ListNotificationsQuery{
		UserID:       conn.UserID,
		Types:        includeTypes,
		ExcludeTypes: excludeTypes,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: maxID, // Use max_id as cursor
		},
	}

	result, err := sch.notificationsService.ListNotifications(ctx, query)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "NOTIFICATIONS_FETCH_FAILED",
			"Failed to fetch notifications", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}
