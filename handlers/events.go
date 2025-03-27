package handlers

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "taskape-backend/proto"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	EventTypeNewTasksAdded        = "new_tasks_added"
	EventTypeNewlyReceived        = "newly_received"
	EventTypeNewlyCompleted       = "newly_completed"
	EventTypeRequiresConfirmation = "requires_confirmation"
	EventTypeNDayStreak           = "n_day_streak"
	EventTypeDeadlineComingUp     = "deadline_coming_up"
)

// CreateEventForTasks creates an event for a user's tasks with the specified type
func (h *TaskHandler) CreateEventForTasks(ctx context.Context, userID string, targetUserID string, eventType string, taskIDs []string) error {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Generate a random event size (small, medium, large)
	sizes := []string{"small", "medium", "large"}
	eventSize := sizes[rand.Intn(len(sizes))]

	// Set expiration to 24 hours from now
	expiresAt := time.Now().Add(24 * time.Hour)

	eventID := uuid.New().String()
	taskIDsArray := "{" + stringArrayToPostgresArray(taskIDs) + "}"

	_, err = tx.Exec(ctx, `
		INSERT INTO events (
			id, user_id, target_user_id, event_type, event_size, 
			created_at, expires_at, task_ids, likes_count, comments_count
		) VALUES (
			$1, $2, $3, $4, $5, 
			NOW(), $6, $7, 0, 0
		)
	`, eventID, userID, targetUserID, eventType, eventSize, expiresAt, taskIDsArray)

	if err != nil {
		return fmt.Errorf("failed to insert event: %v", err)
	}

	return tx.Commit(ctx)
}

// stringArrayToPostgresArray converts a string slice to a comma-separated string for Postgres array
func stringArrayToPostgresArray(arr []string) string {
	if len(arr) == 0 {
		return ""
	}

	result := ""
	for i, str := range arr {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf("\"%s\"", str)
	}
	return result
}

func (h *EventHandler) createRandomEventForFriend(ctx context.Context, friendID int, targetUserID int) (*pb.Event, error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Get some random tasks for this friend, if available
	var taskIDs []string
	taskRows, err := tx.Query(ctx, `
		SELECT id FROM tasks 
		WHERE user_id = $1::text AND privacy_level = 'everyone'
		ORDER BY RANDOM() LIMIT 3
	`, strconv.Itoa(friendID))

	if err != nil {
		return nil, fmt.Errorf("failed to get tasks: %v", err)
	}

	for taskRows.Next() {
		var taskID string
		if err := taskRows.Scan(&taskID); err != nil {
			taskRows.Close()
			return nil, fmt.Errorf("failed to scan task ID: %v", err)
		}
		taskIDs = append(taskIDs, taskID)
	}
	taskRows.Close()

	// Choose a random event type
	eventTypes := []string{
		"new_tasks_added",
		"n_day_streak",
		"deadline_coming_up",
	}
	eventType := eventTypes[rand.Intn(len(eventTypes))]

	// If no tasks found and we chose a task-related event type, fallback to streak
	if len(taskIDs) == 0 && (eventType == "new_tasks_added" || eventType == "deadline_coming_up") {
		eventType = "n_day_streak"
	}

	// Create the event
	eventID := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)
	streakDays := rand.Intn(10) + 1 // Random streak between 1-10 days

	// Choose random event size
	sizes := []string{"small", "medium", "large"}
	eventSize := sizes[rand.Intn(len(sizes))]

	// Insert the event with task IDs
	_, err = tx.Exec(ctx, `
		INSERT INTO events (
			id, user_id, target_user_id, event_type, event_size, 
			created_at, expires_at, task_ids, streak_days, 
			likes_count, comments_count
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, 0, 0
		)
	`, eventID, friendID, targetUserID, eventType, eventSize, now, expiresAt, taskIDs, streakDays)

	if err != nil {
		return nil, fmt.Errorf("failed to insert event: %v", err)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	// Convert event type to protobuf enum
	pbEventType := pb.EventType_NEW_TASKS_ADDED
	switch eventType {
	case "new_tasks_added":
		pbEventType = pb.EventType_NEW_TASKS_ADDED
	case "newly_received":
		pbEventType = pb.EventType_NEWLY_RECEIVED
	case "newly_completed":
		pbEventType = pb.EventType_NEWLY_COMPLETED
	case "requires_confirmation":
		pbEventType = pb.EventType_REQUIRES_CONFIRMATION
	case "n_day_streak":
		pbEventType = pb.EventType_N_DAY_STREAK
	case "deadline_coming_up":
		pbEventType = pb.EventType_DEADLINE_COMING_UP
	}

	// Convert event size to protobuf enum
	pbEventSize := pb.EventSize_MEDIUM
	switch eventSize {
	case "small":
		pbEventSize = pb.EventSize_SMALL
	case "medium":
		pbEventSize = pb.EventSize_MEDIUM
	case "large":
		pbEventSize = pb.EventSize_LARGE
	}

	// Create and return the protobuf event
	event := &pb.Event{
		Id:             eventID,
		UserId:         strconv.Itoa(friendID),
		TargetUserId:   strconv.Itoa(targetUserID),
		Type:           pbEventType,
		Size:           pbEventSize,
		CreatedAt:      timestamppb.New(now),
		ExpiresAt:      timestamppb.New(expiresAt),
		TaskIds:        taskIDs,
		StreakDays:     int32(streakDays),
		LikesCount:     0,
		CommentsCount:  0,
		LikedByUserIds: []string{},
	}

	return event, nil
}

// GetUserEvents retrieves events for a user, ensuring one event per friend
func (h *EventHandler) GetUserEvents(ctx context.Context, req *pb.GetUserEventsRequest) (*pb.GetUserEventsResponse, error) {
	if req.UserId == "" {
		return &pb.GetUserEventsResponse{
			Success: false,
			Error:   "User ID is required",
		}, nil
	}

	limit := 50
	if req.Limit > 0 {
		limit = int(req.Limit)
	}

	// Convert userID to integer
	userIDInt, err := strconv.Atoi(req.UserId)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID format: %v", err)
	}

	// First, get the user's friend IDs
	friendRows, err := h.Pool.Query(ctx, `
		SELECT friend_id FROM user_friends WHERE user_id = $1
	`, userIDInt)
	if err != nil {
		return nil, fmt.Errorf("failed to query friends: %v", err)
	}
	defer friendRows.Close()

	var friendIDs []int
	for friendRows.Next() {
		var friendID int
		if err := friendRows.Scan(&friendID); err != nil {
			return nil, fmt.Errorf("failed to scan friend row: %v", err)
		}
		friendIDs = append(friendIDs, friendID)
	}

	// Check if user has any friends
	if len(friendIDs) == 0 {
		return &pb.GetUserEventsResponse{
			Success: true,
			Events:  []*pb.Event{},
		}, nil
	}

	// Define how recent an event should be to not require a new one
	threeHoursAgo := time.Now().Add(-3 * time.Hour)

	var events []*pb.Event
	eventsMap := make(map[int]*pb.Event) // Map of friendID to their event

	// For each friend, get their most recent event or create a new one
	for _, friendID := range friendIDs {
		// Try to get a recent event - use separate query for liked_by_user_ids to avoid type issues
		row := h.Pool.QueryRow(ctx, `
			SELECT 
				id, user_id, target_user_id, event_type, event_size, 
				created_at, expires_at, task_ids, streak_days, 
				likes_count, comments_count
			FROM events
			WHERE user_id = $1 
			AND target_user_id = $2
			AND created_at > $3
			ORDER BY created_at DESC
			LIMIT 1
		`, friendID, userIDInt, threeHoursAgo)

		var (
			id            string
			userID        int
			targetUserID  int
			eventTypeStr  string
			eventSizeStr  string
			createdAt     time.Time
			expiresAtPtr  *time.Time
			taskIDsArray  []string
			streakDays    int32
			likesCount    int32
			commentsCount int32
		)

		err := row.Scan(
			&id, &userID, &targetUserID, &eventTypeStr, &eventSizeStr,
			&createdAt, &expiresAtPtr, &taskIDsArray, &streakDays,
			&likesCount, &commentsCount,
		)

		if err == nil {
			// Get liked_by_user_ids separately to avoid type issues
			var likedByUserIds []string
			likeRows, err := h.Pool.Query(ctx, `
				SELECT user_id::text 
				FROM event_likes 
				WHERE event_id = $1::uuid
			`, id)

			if err == nil {
				defer likeRows.Close()
				for likeRows.Next() {
					var userID string
					if err := likeRows.Scan(&userID); err == nil {
						likedByUserIds = append(likedByUserIds, userID)
					}
				}
			}

			// Convert string event type to enum
			eventType := pb.EventType_NEW_TASKS_ADDED
			switch eventTypeStr {
			case "new_tasks_added":
				eventType = pb.EventType_NEW_TASKS_ADDED
			case "newly_received":
				eventType = pb.EventType_NEWLY_RECEIVED
			case "newly_completed":
				eventType = pb.EventType_NEWLY_COMPLETED
			case "requires_confirmation":
				eventType = pb.EventType_REQUIRES_CONFIRMATION
			case "n_day_streak":
				eventType = pb.EventType_N_DAY_STREAK
			case "deadline_coming_up":
				eventType = pb.EventType_DEADLINE_COMING_UP
			}

			// Convert string event size to enum
			eventSize := pb.EventSize_MEDIUM
			switch eventSizeStr {
			case "small":
				eventSize = pb.EventSize_SMALL
			case "medium":
				eventSize = pb.EventSize_MEDIUM
			case "large":
				eventSize = pb.EventSize_LARGE
			}

			event := &pb.Event{
				Id:             id,
				UserId:         strconv.Itoa(userID),
				TargetUserId:   strconv.Itoa(targetUserID),
				Type:           eventType,
				Size:           eventSize,
				CreatedAt:      timestamppb.New(createdAt),
				TaskIds:        taskIDsArray,
				StreakDays:     streakDays,
				LikesCount:     likesCount,
				CommentsCount:  commentsCount,
				LikedByUserIds: likedByUserIds,
			}

			if expiresAtPtr != nil {
				event.ExpiresAt = timestamppb.New(*expiresAtPtr)
			}

			eventsMap[friendID] = event
		} else if err == pgx.ErrNoRows {
			// No recent event found, create a new one
			newEvent, err := h.createRandomEventForFriend(ctx, friendID, userIDInt)
			if err != nil {
				return nil, fmt.Errorf("failed to create event for friend %d: %v", friendID, err)
			}
			eventsMap[friendID] = newEvent
		} else {
			return nil, fmt.Errorf("error querying recent event for friend %d: %v", friendID, err)
		}
	}

	// Collect all events into a slice
	for _, event := range eventsMap {
		events = append(events, event)
	}

	// Limit the total number of events if needed
	if len(events) > limit {
		// Sort by creation time first to get the most recent ones
		sort.Slice(events, func(i, j int) bool {
			return events[i].CreatedAt.AsTime().After(events[j].CreatedAt.AsTime())
		})
		events = events[:limit]
	}

	return &pb.GetUserEventsResponse{
		Success: true,
		Events:  events,
	}, nil
}

// LikeEvent adds a like to an event
// LikeEvent adds a like to an event
func (h *EventHandler) LikeEvent(ctx context.Context, req *pb.LikeEventRequest) (*pb.LikeEventResponse, error) {
	if req.EventId == "" || req.UserId == "" {
		return &pb.LikeEventResponse{
			Success: false,
			Error:   "Event ID and User ID are required",
		}, nil
	}

	// Convert user ID to integer
	userIDInt, err := strconv.Atoi(req.UserId)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID format: %v", err)
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Check if event exists
	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM events WHERE id = $1::uuid)
	`, req.EventId).Scan(&exists)

	if err != nil {
		return nil, fmt.Errorf("failed to check if event exists: %v", err)
	}

	if !exists {
		return &pb.LikeEventResponse{
			Success: false,
			Error:   "Event not found",
		}, nil
	}

	// Check if user has already liked this event
	var alreadyLiked bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM event_likes WHERE event_id = $1::uuid AND user_id = $2)
	`, req.EventId, userIDInt).Scan(&alreadyLiked)

	if err != nil {
		return nil, fmt.Errorf("failed to check if user already liked event: %v", err)
	}

	if alreadyLiked {
		// User has already liked this event
		var likesCount int32
		err = tx.QueryRow(ctx, `
			SELECT likes_count FROM events WHERE id = $1::uuid
		`, req.EventId).Scan(&likesCount)

		if err != nil {
			return nil, fmt.Errorf("failed to get likes count: %v", err)
		}

		return &pb.LikeEventResponse{
			Success:    true,
			LikesCount: likesCount,
		}, nil
	}

	// Add the like
	_, err = tx.Exec(ctx, `
		INSERT INTO event_likes (event_id, user_id)
		VALUES ($1::uuid, $2)
	`, req.EventId, userIDInt)

	if err != nil {
		return nil, fmt.Errorf("failed to like event: %v", err)
	}

	// Update the likes count in the events table
	_, err = tx.Exec(ctx, `
		UPDATE events 
		SET likes_count = likes_count + 1
		WHERE id = $1::uuid
	`, req.EventId)

	if err != nil {
		return nil, fmt.Errorf("failed to update likes count: %v", err)
	}

	// Get updated likes count
	var likesCount int32
	err = tx.QueryRow(ctx, `
		SELECT likes_count FROM events WHERE id = $1::uuid
	`, req.EventId).Scan(&likesCount)

	if err != nil {
		return nil, fmt.Errorf("failed to get updated likes count: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.LikeEventResponse{
		Success:    true,
		LikesCount: likesCount,
	}, nil
}

// UnlikeEvent removes a like from an event
// UnlikeEvent removes a like from an event
func (h *EventHandler) UnlikeEvent(ctx context.Context, req *pb.UnlikeEventRequest) (*pb.UnlikeEventResponse, error) {
	if req.EventId == "" || req.UserId == "" {
		return &pb.UnlikeEventResponse{
			Success: false,
			Error:   "Event ID and User ID are required",
		}, nil
	}

	// Convert user ID to integer
	userIDInt, err := strconv.Atoi(req.UserId)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID format: %v", err)
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Check if the like exists
	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM event_likes WHERE event_id = $1::uuid AND user_id = $2)
	`, req.EventId, userIDInt).Scan(&exists)

	if err != nil {
		return nil, fmt.Errorf("failed to check if like exists: %v", err)
	}

	if !exists {
		// User hasn't liked this event
		var likesCount int32
		err = tx.QueryRow(ctx, `
			SELECT likes_count FROM events WHERE id = $1::uuid
		`, req.EventId).Scan(&likesCount)

		if err != nil {
			return nil, fmt.Errorf("failed to get likes count: %v", err)
		}

		return &pb.UnlikeEventResponse{
			Success:    true,
			LikesCount: likesCount,
		}, nil
	}

	// Remove the like
	_, err = tx.Exec(ctx, `
		DELETE FROM event_likes 
		WHERE event_id = $1::uuid AND user_id = $2
	`, req.EventId, userIDInt)

	if err != nil {
		return nil, fmt.Errorf("failed to unlike event: %v", err)
	}

	// Update the likes count in the events table
	_, err = tx.Exec(ctx, `
		UPDATE events 
		SET likes_count = GREATEST(likes_count - 1, 0)
		WHERE id = $1::uuid
	`, req.EventId)

	if err != nil {
		return nil, fmt.Errorf("failed to update likes count: %v", err)
	}

	// Get updated likes count
	var likesCount int32
	err = tx.QueryRow(ctx, `
		SELECT likes_count FROM events WHERE id = $1::uuid
	`, req.EventId).Scan(&likesCount)

	if err != nil {
		return nil, fmt.Errorf("failed to get updated likes count: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.UnlikeEventResponse{
		Success:    true,
		LikesCount: likesCount,
	}, nil
}

// AddEventComment adds a comment to an event
func (h *EventHandler) AddEventComment(ctx context.Context, req *pb.AddEventCommentRequest) (*pb.AddEventCommentResponse, error) {
	if req.EventId == "" || req.UserId == "" || req.Content == "" {
		return &pb.AddEventCommentResponse{
			Success: false,
			Error:   "Event ID, User ID, and Content are required",
		}, nil
	}

	// Trim and check if content is empty after trimming
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return &pb.AddEventCommentResponse{
			Success: false,
			Error:   "Comment content cannot be empty",
		}, nil
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Check if event exists
	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM events WHERE id = $1)
	`, req.EventId).Scan(&exists)

	if err != nil {
		return nil, fmt.Errorf("failed to check if event exists: %v", err)
	}

	if !exists {
		return &pb.AddEventCommentResponse{
			Success: false,
			Error:   "Event not found",
		}, nil
	}

	// Add the comment
	commentID := uuid.New().String()
	now := time.Now()

	_, err = tx.Exec(ctx, `
		INSERT INTO event_comments (id, event_id, user_id, content, created_at, is_edited)
		VALUES ($1, $2, $3, $4, $5, FALSE)
	`, commentID, req.EventId, req.UserId, content, now)

	if err != nil {
		return nil, fmt.Errorf("failed to add comment: %v", err)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	// Create response
	comment := &pb.EventComment{
		Id:        commentID,
		EventId:   req.EventId,
		UserId:    req.UserId,
		Content:   content,
		CreatedAt: timestamppb.New(now),
		IsEdited:  false,
	}

	return &pb.AddEventCommentResponse{
		Success: true,
		Comment: comment,
	}, nil
}

// GetEventComments retrieves comments for an event
func (h *EventHandler) GetEventComments(ctx context.Context, req *pb.GetEventCommentsRequest) (*pb.GetEventCommentsResponse, error) {
	if req.EventId == "" {
		return &pb.GetEventCommentsResponse{
			Success: false,
			Error:   "Event ID is required",
		}, nil
	}

	// Check if event exists
	var exists bool
	err := h.Pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM events WHERE id = $1)
	`, req.EventId).Scan(&exists)

	if err != nil {
		return nil, fmt.Errorf("failed to check if event exists: %v", err)
	}

	if !exists {
		return &pb.GetEventCommentsResponse{
			Success: false,
			Error:   "Event not found",
		}, nil
	}

	// Set default limit if not provided
	limit := 20
	if req.Limit > 0 {
		limit = int(req.Limit)
	}

	// Set default offset if not provided
	offset := 0
	if req.Offset > 0 {
		offset = int(req.Offset)
	}

	// Get total count of non-deleted comments
	var totalCount int32
	err = h.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM event_comments 
		WHERE event_id = $1 AND NOT deleted
	`, req.EventId).Scan(&totalCount)

	if err != nil {
		return nil, fmt.Errorf("failed to get total comments count: %v", err)
	}

	// Query comments
	rows, err := h.Pool.Query(ctx, `
		SELECT id, event_id, user_id, content, created_at, is_edited, edited_at
		FROM event_comments
		WHERE event_id = $1 AND NOT deleted
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3
	`, req.EventId, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("failed to query comments: %v", err)
	}
	defer rows.Close()

	var comments []*pb.EventComment
	for rows.Next() {
		var (
			id, eventId, userId, content string
			createdAt                    time.Time
			isEdited                     bool
			editedAtPtr                  *time.Time
		)

		if err := rows.Scan(
			&id, &eventId, &userId, &content, &createdAt, &isEdited, &editedAtPtr,
		); err != nil {
			return nil, fmt.Errorf("failed to scan comment row: %v", err)
		}

		comment := &pb.EventComment{
			Id:        id,
			EventId:   eventId,
			UserId:    userId,
			Content:   content,
			CreatedAt: timestamppb.New(createdAt),
			IsEdited:  isEdited,
		}

		if editedAtPtr != nil {
			comment.EditedAt = timestamppb.New(*editedAtPtr)
		}

		comments = append(comments, comment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating comment rows: %v", err)
	}

	return &pb.GetEventCommentsResponse{
		Success:    true,
		Comments:   comments,
		TotalCount: totalCount,
	}, nil
}

// DeleteEventComment deletes a comment from an event
func (h *EventHandler) DeleteEventComment(ctx context.Context, req *pb.DeleteEventCommentRequest) (*pb.DeleteEventCommentResponse, error) {
	if req.CommentId == "" || req.UserId == "" {
		return &pb.DeleteEventCommentResponse{
			Success: false,
			Error:   "Comment ID and User ID are required",
		}, nil
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Check if comment exists and belongs to the user
	var exists bool
	var commentUserId string
	err = tx.QueryRow(ctx, `
		SELECT user_id, EXISTS(SELECT 1 FROM event_comments WHERE id = $1 AND NOT deleted)
		FROM event_comments WHERE id = $1
	`, req.CommentId).Scan(&commentUserId, &exists)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &pb.DeleteEventCommentResponse{
				Success: false,
				Error:   "Comment not found",
			}, nil
		}
		return nil, fmt.Errorf("failed to check comment: %v", err)
	}

	if !exists {
		return &pb.DeleteEventCommentResponse{
			Success: false,
			Error:   "Comment not found or already deleted",
		}, nil
	}

	// Verify the user is the comment author (or implement admin check here if needed)
	if commentUserId != req.UserId {
		return &pb.DeleteEventCommentResponse{
			Success: false,
			Error:   "You can only delete your own comments",
		}, status.Error(codes.PermissionDenied, "You can only delete your own comments")
	}

	// Soft delete the comment (mark as deleted)
	_, err = tx.Exec(ctx, `
		UPDATE event_comments 
		SET deleted = TRUE
		WHERE id = $1
	`, req.CommentId)

	if err != nil {
		return nil, fmt.Errorf("failed to delete comment: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.DeleteEventCommentResponse{
		Success: true,
	}, nil
}
