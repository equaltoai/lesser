// This patch updates HandleGetStatusContext in cmd/api/handlers/statuses.go
// Replace lines 1400-1432 (the descendants section) with:

	// Get descendants (replies to this status)
	descendants := []models.Status{}
	replies, _, err := h.store.GetReplies(ctx, objectID, 20, "")
	if err == nil {
		for _, reply := range replies {
			// Get actor for reply
			var replyActor *activitypub.Actor
			var attributedTo string
			
			// Extract attributedTo from reply object
			switch o := reply.(type) {
			case *activitypub.Note:
				attributedTo = o.AttributedTo
			case map[string]interface{}:
				if attr, ok := o["attributedTo"].(string); ok {
					attributedTo = attr
				}
			default:
				// Try reflection for other types
				v := reflect.ValueOf(reply)
				if v.Kind() == reflect.Ptr {
					v = v.Elem()
				}
				if v.Kind() == reflect.Struct {
					if attrField := v.FieldByName("AttributedTo"); attrField.IsValid() && attrField.Kind() == reflect.String {
						attributedTo = attrField.String()
					}
				}
			}
			
			// Get actor if we have attributedTo
			if attributedTo != "" {
				username := h.converter.ExtractUsernameFromActorID(attributedTo)
				if username != "" {
					replyActor, _ = h.store.GetActor(ctx, username)
				}
			}
			
			// Convert to status
			status := h.converter.ObjectToStatus(reply, replyActor)
			
			// Get interaction counts for the reply
			replyObjectID := status.URI
			if replyObjectID == "" {
				// Extract from status ID if URI is not set
				replyObjectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), status.ID)
			}
			likeCount, _ := h.store.CountObjectLikes(ctx, replyObjectID)
			announceCount, _ := h.store.CountObjectAnnounces(ctx, replyObjectID)
			replyCount, _ := h.store.CountReplies(ctx, replyObjectID)
			
			status.FavouritesCount = likeCount
			status.ReblogsCount = announceCount
			status.RepliesCount = replyCount
			
			descendants = append(descendants, status)
		}
		
		h.logger.Debug("found descendants",
			zap.String("status_id", statusID),
			zap.Int("count", len(descendants)))
	} else {
		h.logger.Warn("failed to get replies",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

// Also update the main status in HandleGetStatus to include reply count:
// Around line 1206, after getting like and announce counts:

	// Get interaction counts
	likeCount, _ := h.store.CountObjectLikes(ctx, objectID)
	announceCount, _ := h.store.CountObjectAnnounces(ctx, objectID)
	replyCount, _ := h.store.CountReplies(ctx, objectID) // Add this line
	status.FavouritesCount = likeCount
	status.ReblogsCount = announceCount
	status.RepliesCount = replyCount // Add this line

// And in HandleCreateStatus when creating a reply (around line 216):

	// Handle reply
	if req.InReplyToID != "" {
		note.InReplyTo = req.InReplyToID
		
		// Increment reply count for parent status
		if err := h.store.IncrementReplyCount(ctx, req.InReplyToID); err != nil {
			h.logger.Warn("failed to increment reply count",
				zap.String("parent_status_id", req.InReplyToID),
				zap.Error(err))
		}

		// Record reply engagement for trending
		if err := h.store.RecordStatusEngagement(ctx, req.InReplyToID, "reply", actor.ID); err != nil {
			h.logger.Warn("failed to record reply engagement",
				zap.String("parent_status_id", req.InReplyToID),
				zap.Error(err))
		}
	}