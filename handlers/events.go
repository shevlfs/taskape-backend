package handlers

import (
	"context"
	"fmt"
	"math/rand"
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

func (h *EventHandler) GetUserEvents(ctx context.Context, req *pb.GetUserEventsRequest) (*pb.GetUserEventsResponse, error) {
	if req.UserId == "" {
		return &pb.GetUserEventsResponse{
			Success: false,
			Error:   "user id is required",
		}, nil
	}

	limit := 50
	if req.Limit > 0 {
		limit = int(req.Limit)
	}

	// Convert userID to integer for database queries
	userIDInt, err := strconv.Atoi(req.UserId)
	if err != nil {
		return nil, fmt.Errorf("invalid user id format: %v", err)
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// First get the list of users that the requesting user is allowed to see events from
	// This includes the user themselves and their friends
	var allowedUserIDs []int
	allowedUserIDs = append(allowedUserIDs, userIDInt) // User can always see their own events

	// Get friends list if viewing someone else's events
	friendRows, err := tx.Query(ctx, `
		SELECT friend_id FROM user_friends WHERE user_id = $1
	`, userIDInt)
	if err != nil {
		return nil, fmt.Errorf("failed to query friends: %v", err)
	}
	defer friendRows.Close()

	for friendRows.Next() {
		var friendID int
		if err := friendRows.Scan(&friendID); err != nil {
			return nil, fmt.Errorf("failed to scan friend row: %v", err)
		}
		allowedUserIDs = append(allowedUserIDs, friendID)
	}

	// Check if we need to create new events for users
	// We'll only do this for recent friends (last 24 hours)
	currentTime := time.Now()
	currentTime.Add(-24 * time.Hour)

	// Build the query to get events with privacy filtering
	query := `
		SELECT 
			e.id, e.user_id, e.target_user_id, e.event_type, e.event_size, 
			e.created_at, e.expires_at, e.task_ids, e.streak_days, 
			e.likes_count, e.comments_count
		FROM events e
		WHERE (
			-- User can see their own events
			(e.user_id = $1 AND e.target_user_id = $1)
			
			-- Or events targeted to them from their friends
			OR (e.target_user_id = $1 AND e.user_id = ANY($2))
			
			-- Or events from their friends that are public
			OR (e.user_id = ANY($2) AND e.target_user_id = e.user_id)
		)
		AND (e.expires_at IS NULL OR e.expires_at > $3 OR $4 = true)
	`

	// If we're limiting to just one event per user, we need a different approach
	var events []*pb.Event

	if req.Limit == 1 {
		// For each allowed user, get their most recent event
		for _, userID := range allowedUserIDs {
			// Skip getting events if it's not the user we're specifically requesting
			if req.UserId != strconv.Itoa(userID) && !contains(allowedUserIDs, userID) {
				continue
			}

			row := tx.QueryRow(ctx, `
				SELECT 
					id, user_id, target_user_id, event_type, event_size, 
					created_at, expires_at, task_ids, streak_days, 
					likes_count, comments_count
				FROM events
				WHERE user_id = $1
				AND (expires_at IS NULL OR expires_at > $2 OR $3 = true)
				ORDER BY created_at DESC
				LIMIT 1
			`, userID, currentTime, req.IncludeExpired)

			event, err := scanEvent(row)
			if err != nil {
				if err == pgx.ErrNoRows {
					// No event found for this user, we might need to generate one
					if userID != userIDInt {
						newEvent, genErr := h.generateEventForUser(ctx, tx, userID, userIDInt)
						if genErr == nil && newEvent != nil {
							events = append(events, newEvent)
						}
					}
					continue
				}
				return nil, fmt.Errorf("error scanning event: %v", err)
			}

			// Check privacy settings for task-related events
			if canViewEvent(event, userIDInt, allowedUserIDs) {
				events = append(events, event)
			}
		}
	} else {
		// Standard query for multiple events
		rows, err := tx.Query(ctx, query+`
			ORDER BY created_at DESC
			LIMIT $5
		`, userIDInt, allowedUserIDs, currentTime, req.IncludeExpired, limit)

		if err != nil {
			return nil, fmt.Errorf("failed to query events: %v", err)
		}
		defer rows.Close()

		for rows.Next() {
			event, err := scanEventRow(rows)
			if err != nil {
				return nil, fmt.Errorf("error scanning event row: %v", err)
			}

			// Check privacy settings for task-related events
			if canViewEvent(event, userIDInt, allowedUserIDs) {
				events = append(events, event)
			}
		}

		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating event rows: %v", err)
		}

		// If we didn't get any events, try to generate some
		if len(events) == 0 {
			for _, friendID := range allowedUserIDs {
				if friendID == userIDInt {
					continue // Skip generating events for self
				}
				newEvent, err := h.generateEventForUser(ctx, tx, friendID, userIDInt)
				if err == nil && newEvent != nil {
					events = append(events, newEvent)
					if len(events) >= limit {
						break
					}
				}
			}
		}
	}

	// For each event, get the liked by user IDs
	for i, event := range events {
		likeRows, err := tx.Query(ctx, `
			SELECT user_id::text 
			FROM event_likes 
			WHERE event_id = $1::uuid
		`, event.Id)

		if err != nil {
			continue // Skip if we can't get likes
		}

		var likedByUserIds []string
		for likeRows.Next() {
			var userID string
			if err := likeRows.Scan(&userID); err == nil {
				likedByUserIds = append(likedByUserIds, userID)
			}
		}
		likeRows.Close()

		events[i].LikedByUserIds = likedByUserIds
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.GetUserEventsResponse{
		Success: true,
		Events:  events,
	}, nil
}

// Helper function to scan an event from a pgx.Row
func scanEvent(row pgx.Row) (*pb.Event, error) {
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
	if err != nil {
		return nil, err
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
		LikedByUserIds: []string{},
	}

	if expiresAtPtr != nil {
		event.ExpiresAt = timestamppb.New(*expiresAtPtr)
	}

	return event, nil
}

// Helper function to scan an event from a pgx.Rows
func scanEventRow(rows pgx.Rows) (*pb.Event, error) {
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

	err := rows.Scan(
		&id, &userID, &targetUserID, &eventTypeStr, &eventSizeStr,
		&createdAt, &expiresAtPtr, &taskIDsArray, &streakDays,
		&likesCount, &commentsCount,
	)
	if err != nil {
		return nil, err
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
		LikedByUserIds: []string{},
	}

	if expiresAtPtr != nil {
		event.ExpiresAt = timestamppb.New(*expiresAtPtr)
	}

	return event, nil
}

// Helper function to check if an event should be visible based on privacy settings
func canViewEvent(event *pb.Event, viewerID int, friendIDs []int) bool {
	// If it's the user's own event or it's targeted to them, they can see it
	viewerIDStr := strconv.Itoa(viewerID)
	if event.UserId == viewerIDStr || event.TargetUserId == viewerIDStr {
		return true
	}

	// Check if the viewer is a friend of the event owner
	eventUserID, _ := strconv.Atoi(event.UserId)
	isFriend := contains(friendIDs, eventUserID)
	if !isFriend {
		return false
	}

	// For task-related events, check task privacy settings
	if event.Type == pb.EventType_NEW_TASKS_ADDED ||
		event.Type == pb.EventType_NEWLY_RECEIVED ||
		event.Type == pb.EventType_NEWLY_COMPLETED ||
		event.Type == pb.EventType_REQUIRES_CONFIRMATION {
		// TODO: Implement task privacy checking by querying the tasks
		// This would require additional database queries to check each task's privacy
		// For now, we'll allow friends to see all task-related events
		return true
	}

	// Streak events and deadline events are visible to friends
	return true
}

// Helper function to generate an event for a user
func (h *EventHandler) generateEventForUser(ctx context.Context, tx pgx.Tx, userID int, _ int) (*pb.Event, error) {
	// Check if the user already has a recent event (last 24 hours)
	var recentEventExists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM events 
			WHERE user_id = $1 
			AND created_at > NOW() - INTERVAL '24 hours'
		)
	`, userID).Scan(&recentEventExists)

	if err != nil {
		return nil, fmt.Errorf("failed to check recent events: %v", err)
	}

	if recentEventExists {
		return nil, nil // User already has a recent event, no need to generate
	}

	// Get the user's current streak
	var streakDays int
	err = tx.QueryRow(ctx, `
		WITH daily_completions AS (
			SELECT 
				DISTINCT date_trunc('day', confirmed_at) AS completion_date
			FROM tasks
			WHERE user_id = $1::text
			AND is_completed = true
			AND confirmed_at IS NOT NULL
			ORDER BY completion_date DESC
		),
		date_diffs AS (
			SELECT 
				completion_date,
				lag(completion_date, 1) OVER (ORDER BY completion_date DESC) AS prev_date,
				completion_date - lag(completion_date, 1) OVER (ORDER BY completion_date DESC) AS day_diff
			FROM daily_completions
		)
		SELECT COUNT(*)
		FROM date_diffs
		WHERE day_diff = INTERVAL '1 day'
		OR day_diff IS NULL
		LIMIT 1
	`, userID).Scan(&streakDays)

	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("failed to calculate streak: %v", err)
	}

	// If the user has a streak, create a streak event
	if streakDays > 0 {
		now := time.Now()
		expiresAt := now.Add(24 * time.Hour)
		eventID := uuid.New().String()

		// Insert the event
		_, err = tx.Exec(ctx, `
			INSERT INTO events (
				id, user_id, target_user_id, event_type, event_size, 
				created_at, expires_at, streak_days, likes_count, comments_count, task_ids
			) VALUES (
				$1, $2, $3, 'n_day_streak', 'medium', $4, $5, $6, 0, 0, '{}'
			)
		`, eventID, userID, userID, now, expiresAt, streakDays)

		if err != nil {
			return nil, fmt.Errorf("failed to insert streak event: %v", err)
		}

		// Create and return the event object
		return &pb.Event{
			Id:            eventID,
			UserId:        strconv.Itoa(userID),
			TargetUserId:  strconv.Itoa(userID),
			Type:          pb.EventType_N_DAY_STREAK,
			Size:          pb.EventSize_MEDIUM,
			CreatedAt:     timestamppb.New(now),
			ExpiresAt:     timestamppb.New(expiresAt),
			TaskIds:       []string{},
			StreakDays:    int32(streakDays),
			LikesCount:    0,
			CommentsCount: 0,
		}, nil
	}

	// Otherwise, check for recent task activity
	var taskID string
	var taskName string
	err = tx.QueryRow(ctx, `
		SELECT id, name
		FROM tasks
		WHERE user_id = $1::text
		AND privacy_level = 'everyone'
		ORDER BY created_at DESC
		LIMIT 1
	`, userID).Scan(&taskID, &taskName)

	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("failed to get recent task: %v", err)
	}

	if err == nil {
		// Create a new task added event
		now := time.Now()
		expiresAt := now.Add(24 * time.Hour)
		eventID := uuid.New().String()

		_, err = tx.Exec(ctx, `
			INSERT INTO events (
				id, user_id, target_user_id, event_type, event_size, 
				created_at, expires_at, task_ids, likes_count, comments_count
			) VALUES (
				$1, $2, $3, 'new_tasks_added', 'medium', $4, $5, $6, 0, 0
			)
		`, eventID, userID, userID, now, expiresAt, []string{taskID})

		if err != nil {
			return nil, fmt.Errorf("failed to insert task event: %v", err)
		}

		// Create and return the event object
		return &pb.Event{
			Id:            eventID,
			UserId:        strconv.Itoa(userID),
			TargetUserId:  strconv.Itoa(userID),
			Type:          pb.EventType_NEW_TASKS_ADDED,
			Size:          pb.EventSize_MEDIUM,
			CreatedAt:     timestamppb.New(now),
			ExpiresAt:     timestamppb.New(expiresAt),
			TaskIds:       []string{taskID},
			LikesCount:    0,
			CommentsCount: 0,
		}, nil
	}

	// No suitable event could be generated
	return nil, nil
}

// Helper function to check if a slice contains a value
func contains(slice []int, val int) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

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
