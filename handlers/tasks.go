package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	pb "taskape-backend/proto"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *TaskHandler) CreateTask(ctx context.Context, req *pb.CreateTaskRequest) (*pb.CreateTaskResponse, error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	taskID := req.Task.Id
	if taskID == "" {
		taskID = uuid.New().String()
	}

	var deadline *time.Time
	if req.Task.Deadline != nil {
		t := req.Task.Deadline.AsTime()
		deadline = &t
	}

	assignedToJSON, err := json.Marshal(req.Task.AssignedTo)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal assigned_to: %v", err)
	}

	exceptIDsJSON, err := json.Marshal(req.Task.Privacy.ExceptIds)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal privacy_except_ids: %v", err)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO tasks (
            id, user_id, name, description, deadline, author, "group", group_id, 
            assigned_to, task_difficulty, custom_hours, mentioned_in_event,
            is_completed, proof_url, privacy_level, privacy_except_ids,
            flag_status, flag_color, flag_name, display_order,
            needs_confirmation, proof_description
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
            $17, $18, $19, $20, $21, $22
        )`,
		taskID,
		req.Task.UserId,
		req.Task.Name,
		req.Task.Description,
		deadline,
		req.Task.Author,
		req.Task.Group,
		req.Task.GroupId,
		assignedToJSON,
		req.Task.TaskDifficulty,
		req.Task.CustomHours,
		req.Task.MentionedInEvent,
		req.Task.Completion.IsCompleted,
		req.Task.Completion.ProofUrl,
		req.Task.Privacy.Level,
		exceptIDsJSON,
		req.Task.FlagStatus,
		req.Task.FlagColor,
		req.Task.FlagName,
		req.Task.DisplayOrder,
		req.Task.ProofNeeded,
		req.Task.ProofDescription,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to insert task: %v", err)
	}

	// Determine if we need to create an event
	// 1. If the task is assigned to others - create a "newly received" event for them
	// 2. If the task is for the user who created it - create a "new tasks added" event

	var eventType string
	var eventTargetUserID string

	if len(req.Task.AssignedTo) > 0 && req.Task.UserId != req.Task.Author {
		// This is a task assigned to someone by another user
		eventType = "newly_received"
		eventTargetUserID = req.Task.UserId

		// Check if there's a recent "newly received" event we can update
		var recentEventID string
		err = tx.QueryRow(ctx, `
			SELECT id FROM events
			WHERE user_id = $1
			AND target_user_id = $2
			AND event_type = $3
			AND created_at > NOW() - INTERVAL '30 minutes'
			ORDER BY created_at DESC
			LIMIT 1
		`, req.Task.Author, eventTargetUserID, eventType).Scan(&recentEventID)

		if err == nil {
			// Found a recent event, update it to include this task
			_, err = tx.Exec(ctx, `
				UPDATE events
				SET task_ids = array_append(task_ids, $1)
				WHERE id = $2
			`, taskID, recentEventID)

			if err != nil {
				// Log but don't fail the transaction
				log.Printf("failed to update event with new task: %v", err)
			} else {
				log.Printf("successfully added task %s to existing event %s", taskID, recentEventID)
			}
		} else if err == pgx.ErrNoRows {
			// No recent event found, create a new one
			err = createNewEventForTask(ctx, tx, req.Task.Author, eventTargetUserID, eventType, taskID)
			if err != nil {
				log.Printf("failed to create new event for task: %v", err)
			} else {
				log.Printf("successfully created new event for task %s", taskID)
			}
		}
	} else if req.Task.UserId == req.Task.Author {
		// This is a task the user created for themselves
		eventType = "new_tasks_added"
		eventTargetUserID = req.Task.UserId

		// Check if there's a recent "new tasks added" event we can update
		var recentEventID string
		err = tx.QueryRow(ctx, `
			SELECT id FROM events
			WHERE user_id = $1
			AND target_user_id = $2
			AND event_type = $3
			AND created_at > NOW() - INTERVAL '30 minutes'
			ORDER BY created_at DESC
			LIMIT 1
		`, req.Task.UserId, eventTargetUserID, eventType).Scan(&recentEventID)

		if err == nil {
			// Found a recent event, update it to include this task
			_, err = tx.Exec(ctx, `
				UPDATE events
				SET task_ids = array_append(task_ids, $1)
				WHERE id = $2
			`, taskID, recentEventID)

			if err != nil {
				// Log but don't fail the transaction
				log.Printf("failed to update event with new task: %v", err)
			} else {
				log.Printf("successfully added task %s to existing event %s", taskID, recentEventID)
			}
		} else if err == pgx.ErrNoRows {
			// No recent event found, create a new one
			err = createNewEventForTask(ctx, tx, req.Task.UserId, eventTargetUserID, eventType, taskID)
			if err != nil {
				log.Printf("failed to create new event for task: %v", err)
			} else {
				log.Printf("successfully created new event for task %s", taskID)
			}
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.CreateTaskResponse{
		Success: true,
		TaskId:  taskID,
	}, nil
}

func createNewEventForTask(ctx context.Context, tx pgx.Tx, userID string, targetUserID string, eventType string, taskID string) error {
	eventID := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)

	// Choose a random event size
	sizes := []string{"small", "medium", "large"}
	eventSize := sizes[rand.Intn(len(sizes))]

	log.Printf("Creating new event: user=%s, target=%s, type=%s, task=%s",
		userID, targetUserID, eventType, taskID)

	_, err := tx.Exec(ctx, `
		INSERT INTO events (
			id, user_id, target_user_id, event_type, event_size, 
			created_at, expires_at, task_ids, likes_count, comments_count
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, 0, 0
		)
	`, eventID, userID, targetUserID, eventType, eventSize, now, expiresAt,
		pq.Array([]string{taskID}))

	return err
}

func (h *TaskHandler) CreateTasksBatch(ctx context.Context, req *pb.CreateTasksBatchRequest) (*pb.CreateTasksBatchResponse, error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	taskIDs := make([]string, len(req.Tasks))

	// Group tasks by assignee to create appropriate events
	tasksForSelf := []string{}
	tasksForOthers := make(map[string][]string) // map[assigneeID][]taskID

	for i, task := range req.Tasks {
		taskID := task.Id
		if taskID == "" {
			taskID = uuid.New().String()
		}
		taskIDs[i] = taskID

		var deadline *time.Time
		if task.Deadline != nil {
			t := task.Deadline.AsTime()
			deadline = &t
		}

		assignedTo := task.AssignedTo
		if assignedTo == nil {
			assignedTo = []string{}
		}

		privacyExceptIds := task.Privacy.ExceptIds
		if privacyExceptIds == nil {
			privacyExceptIds = []string{}
		}

		// Insert the task
		_, err = tx.Exec(ctx, `
            INSERT INTO tasks (
                id, user_id, name, description, deadline, author, "group", group_id, 
                assigned_to, task_difficulty, custom_hours, mentioned_in_event,
                is_completed, proof_url, privacy_level, privacy_except_ids,
                flag_status, flag_color, flag_name, display_order,
                needs_confirmation, proof_description
            ) VALUES (
                $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
                $17, $18, $19, $20, $21, $22
            )`,
			taskID,
			task.UserId,
			task.Name,
			task.Description,
			deadline,
			task.Author,
			task.Group,
			task.GroupId,
			assignedTo,
			task.TaskDifficulty,
			task.CustomHours,
			task.MentionedInEvent,
			task.Completion.IsCompleted,
			task.Completion.ProofUrl,
			task.Privacy.Level,
			privacyExceptIds,
			task.FlagStatus,
			task.FlagColor,
			task.FlagName,
			task.DisplayOrder,
			task.ProofNeeded,
			task.ProofDescription,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to insert task %d: %v", i, err)
		}

		// Group tasks for events
		if task.UserId == task.Author {
			// Task created by user for themselves
			tasksForSelf = append(tasksForSelf, strings.ToLower(taskID))
		} else if len(task.AssignedTo) > 0 {
			// Task assigned to others
			for _, assigneeID := range task.AssignedTo {
				if _, exists := tasksForOthers[assigneeID]; !exists {
					tasksForOthers[assigneeID] = []string{}
				}
				tasksForOthers[assigneeID] = append(tasksForOthers[assigneeID], strings.ToLower(taskID))
			}
		}
	}

	// Create events for tasks
	// 1. For tasks created by user for themselves
	if len(tasksForSelf) > 0 {
		// Check if there's a recent "new tasks added" event to update
		userID := req.Tasks[0].UserId // Assuming all tasks have same userID in batch
		var recentEventID string
		err = tx.QueryRow(ctx, `
			SELECT id FROM events
			WHERE user_id = $1
			AND target_user_id = $1
			AND event_type = 'new_tasks_added'
			AND created_at > NOW() - INTERVAL '30 minutes'
			ORDER BY created_at DESC
			LIMIT 1
		`, userID).Scan(&recentEventID)

		if err == nil {
			// Update existing event with new tasks
			for _, taskID := range tasksForSelf {
				_, err = tx.Exec(ctx, `
					UPDATE events
					SET task_ids = array_append(task_ids, $1)
					WHERE id = $2
				`, taskID, recentEventID)

				if err != nil {
					log.Printf("failed to update event with new task: %v", err)
				}
			}
		} else if err == pgx.ErrNoRows {
			// Create new event for self-tasks
			eventID := uuid.New().String()
			now := time.Now()
			expiresAt := now.Add(24 * time.Hour)
			sizes := []string{"small", "medium", "large"}
			eventSize := sizes[rand.Intn(len(sizes))]

			_, err = tx.Exec(ctx, `
				INSERT INTO events (
					id, user_id, target_user_id, event_type, event_size, 
					created_at, expires_at, task_ids, likes_count, comments_count
				) VALUES (
					$1, $2, $3, 'new_tasks_added', $4, $5, $6, $7, 0, 0
				)
			`, eventID, userID, userID, eventSize, now, expiresAt, tasksForSelf)

			if err != nil {
				log.Printf("failed to create event for self tasks: %v", err)
			}
		}
	}

	// 2. For tasks assigned to others
	for assigneeID, assignedTasks := range tasksForOthers {
		authorID := req.Tasks[0].Author // Assuming same author for all tasks in batch

		// Check for recent "newly received" event for this assignee
		var recentEventID string
		err = tx.QueryRow(ctx, `
			SELECT id FROM events
			WHERE user_id = $1
			AND target_user_id = $2
			AND event_type = 'newly_received'
			AND created_at > NOW() - INTERVAL '30 minutes'
			ORDER BY created_at DESC
			LIMIT 1
		`, authorID, assigneeID).Scan(&recentEventID)

		if err == nil {
			// Update existing event with new tasks
			for _, taskID := range assignedTasks {
				_, err = tx.Exec(ctx, `
					UPDATE events
					SET task_ids = array_append(task_ids, $1)
					WHERE id = $2
				`, taskID, recentEventID)

				if err != nil {
					log.Printf("failed to update event with assigned task: %v", err)
				}
			}
		} else if err == pgx.ErrNoRows {
			// Create new event for assigned tasks
			eventID := uuid.New().String()
			now := time.Now()
			expiresAt := now.Add(24 * time.Hour)

			sizes := []string{"small", "medium", "large"}
			eventSize := sizes[rand.Intn(len(sizes))]

			_, err = tx.Exec(ctx, `
				INSERT INTO events (
					id, user_id, target_user_id, event_type, event_size, 
					created_at, expires_at, task_ids, likes_count, comments_count
				) VALUES (
					$1, $2, $3, 'newly_received', $4, $5, $6, $7, 0, 0
				)
			`, eventID, authorID, assigneeID, eventSize, now, expiresAt, assignedTasks)

			if err != nil {
				log.Printf("failed to create event for assigned tasks: %v", err)
			}
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.CreateTasksBatchResponse{
		Success: true,
		TaskIds: taskIDs,
	}, nil
}

func (h *TaskHandler) UpdateTaskOrder(ctx context.Context, req *pb.UpdateTaskOrderRequest) (*pb.UpdateTaskOrderResponse, error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	for _, taskOrder := range req.Tasks {
		_, err := tx.Exec(ctx, `
            UPDATE tasks 
            SET display_order = $1
            WHERE id = $2 AND user_id = $3
        `, taskOrder.DisplayOrder, taskOrder.TaskId, req.UserId)

		if err != nil {
			return nil, fmt.Errorf("failed to update task order: %v", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.UpdateTaskOrderResponse{
		Success: true,
	}, nil
}

func (h *TaskHandler) GetUserTasks(ctx context.Context, req *pb.GetUserTasksRequest) (*pb.GetUserTasksResponse, error) {
	log.Printf("GetUserTasks called for user ID: %s, requester ID: %s", req.UserId, req.RequesterId)

	if req.UserId == "" {
		log.Printf("ERROR: Empty user ID provided")
		return &pb.GetUserTasksResponse{
			Success: false,
			Error:   "User ID cannot be empty",
		}, nil
	}

	canViewAllTasks := req.UserId == req.RequesterId

	if !canViewAllTasks && req.RequesterId != "" {
		var isFriend bool
		err := h.Pool.QueryRow(ctx, `
            SELECT EXISTS(
                SELECT 1 FROM user_friends 
                WHERE user_id = $1::INTEGER AND friend_id = $2::INTEGER
            )
        `, req.UserId, req.RequesterId).Scan(&isFriend)

		if err != nil {
			log.Printf("ERROR checking friendship: %v", err)
		} else {
			log.Printf("Friendship status between %s and %s: %v", req.UserId, req.RequesterId, isFriend)
		}
	}

	var query string
	var args []interface{}

	if req.UserId == req.RequesterId {
		query = `
            SELECT 
                id, user_id, name, description, created_at, deadline, author, "group", group_id,
                assigned_to, task_difficulty, custom_hours, mentioned_in_event,
                is_completed, proof_url, privacy_level, privacy_except_ids,
                flag_status, flag_color, flag_name, display_order
            FROM tasks
            WHERE user_id = $1
        `
		args = append(args, req.UserId)
	} else if req.RequesterId == "" {
		query = `
            SELECT 
                id, user_id, name, description, created_at, deadline, author, "group", group_id,
                assigned_to, task_difficulty, custom_hours, mentioned_in_event,
                is_completed, proof_url, privacy_level, privacy_except_ids,
                flag_status, flag_color, flag_name, display_order
            FROM tasks
            WHERE user_id = $1 AND privacy_level = 'everyone'
        `
		args = append(args, req.UserId)
	} else {
		query = `
            SELECT 
                id, user_id, name, description, created_at, deadline, author, "group", group_id,
                assigned_to, task_difficulty, custom_hours, mentioned_in_event,
                is_completed, proof_url, privacy_level, privacy_except_ids,
                flag_status, flag_color, flag_name, display_order
            FROM tasks
            WHERE user_id = $1 
            AND (
                -- Everyone can see tasks with 'everyone' privacy level
                privacy_level = 'everyone'
                
                -- Friends can see 'friends-only' tasks
                OR (
                    privacy_level = 'friends-only' 
                    AND EXISTS(
                        SELECT 1 FROM user_friends 
                        WHERE user_id = $1::INTEGER 
                        AND friend_id = $2::INTEGER
                    )
                )
                
                -- People not in the 'except' list can see these tasks
                OR (
                    privacy_level = 'except' 
                    AND NOT($2::TEXT = ANY(privacy_except_ids))
                )
            )
            -- Never show 'noone' tasks to others
            AND privacy_level != 'noone'
        `
		args = append(args, req.UserId, req.RequesterId)
	}

	query += " ORDER BY display_order ASC, created_at DESC"

	rows, err := h.Pool.Query(ctx, query, args...)
	if err != nil {
		log.Printf("ERROR querying tasks for user %s: %v", req.UserId, err)
		return &pb.GetUserTasksResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to query tasks: %v", err),
		}, nil
	}
	defer rows.Close()

	var tasks []*pb.Task
	for rows.Next() {
		var (
			id, userID, name, description, author, group, groupID, taskDifficulty, privacyLevel string
			createdAt                                                                           time.Time
			deadlinePtr                                                                         *time.Time
			assignedTo, exceptIDs                                                               []string
			customHours                                                                         int32
			customHoursPtr                                                                      *int32
			mentionedInEvent, isCompleted, flagStatus                                           bool
			proofURL, flagColor, flagName                                                       string
			proofURLPtr, flagColorPtr, flagNamePtr                                              *string
			displayOrder                                                                        int32
		)

		err := rows.Scan(
			&id, &userID, &name, &description, &createdAt, &deadlinePtr, &author, &group, &groupID,
			&assignedTo, &taskDifficulty, &customHoursPtr, &mentionedInEvent,
			&isCompleted, &proofURLPtr, &privacyLevel, &exceptIDs,
			&flagStatus, &flagColorPtr, &flagNamePtr, &displayOrder,
		)
		if err != nil {
			log.Printf("ERROR scanning task row: %v", err)
			return &pb.GetUserTasksResponse{
				Success: false,
				Error:   fmt.Sprintf("Failed to scan task row: %v", err),
			}, nil
		}

		createdAtProto := timestamppb.New(createdAt)
		var deadlineProto *timestamppb.Timestamp
		if deadlinePtr != nil {
			deadlineProto = timestamppb.New(*deadlinePtr)
		}

		if customHoursPtr != nil {
			customHours = *customHoursPtr
		}

		if proofURLPtr != nil {
			proofURL = *proofURLPtr
		}

		if flagColorPtr != nil {
			flagColor = *flagColorPtr
		}

		if flagNamePtr != nil {
			flagName = *flagNamePtr
		}

		task := &pb.Task{
			Id:               id,
			UserId:           userID,
			Name:             name,
			Description:      description,
			CreatedAt:        createdAtProto,
			Deadline:         deadlineProto,
			Author:           author,
			Group:            group,
			GroupId:          groupID,
			AssignedTo:       assignedTo,
			TaskDifficulty:   taskDifficulty,
			CustomHours:      customHours,
			MentionedInEvent: mentionedInEvent,
			Completion: &pb.CompletionStatus{
				IsCompleted: isCompleted,
				ProofUrl:    proofURL,
			},
			Privacy: &pb.PrivacySettings{
				Level:     privacyLevel,
				ExceptIds: exceptIDs,
			},
			FlagStatus:   flagStatus,
			FlagColor:    flagColor,
			FlagName:     flagName,
			DisplayOrder: displayOrder,
		}

		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		log.Printf("ERROR iterating task rows: %v", err)
		return &pb.GetUserTasksResponse{
			Success: false,
			Error:   fmt.Sprintf("Error iterating task rows: %v", err),
		}, nil
	}

	log.Printf("Successfully fetched %d tasks for user %s (requested by %s)",
		len(tasks), req.UserId, req.RequesterId)
	return &pb.GetUserTasksResponse{
		Success: true,
		Tasks:   tasks,
	}, nil
}

func updateUserStreak(ctx context.Context, tx pgx.Tx, userID string) error {
	userIDInt, err := strconv.Atoi(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID format: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// First check if the user already has a streak record
	var exists bool
	err = tx.QueryRow(ctx, `
        SELECT EXISTS(SELECT 1 FROM user_streaks WHERE user_id = $1)
    `, userIDInt).Scan(&exists)

	if err != nil {
		return fmt.Errorf("failed to check if streak exists: %v", err)
	}

	if !exists {
		// Create a new streak record for this user
		_, err = tx.Exec(ctx, `
            INSERT INTO user_streaks (
                user_id, current_streak, longest_streak, 
                last_completed_date, last_streak_event_date
            ) VALUES ($1, 1, 1, $2, NULL)
        `, userIDInt, today)

		if err != nil {
			return fmt.Errorf("failed to create new user streak: %v", err)
		}
		return nil
	}

	// User has an existing streak record - update it
	var currentStreak int
	var longestStreak int
	var lastCompletedDate *time.Time
	var lastStreakEventDate *time.Time

	err = tx.QueryRow(ctx, `
        SELECT 
            current_streak, 
            longest_streak, 
            last_completed_date, 
            last_streak_event_date
        FROM user_streaks 
        WHERE user_id = $1
    `, userIDInt).Scan(&currentStreak, &longestStreak, &lastCompletedDate, &lastStreakEventDate)

	if err != nil {
		return fmt.Errorf("failed to get user streak: %v", err)
	}

	// Determine if streak continues or resets
	newStreak := currentStreak
	if lastCompletedDate == nil {
		// First completion ever
		newStreak = 1
	} else if lastCompletedDate.Format("2006-01-02") == yesterday {
		// Continuing streak from yesterday
		newStreak = currentStreak + 1
	} else if lastCompletedDate.Format("2006-01-02") == today {
		// Already completed today, streak unchanged
		return nil
	} else {
		// Streak broken, starting new one
		newStreak = 1
	}

	// Update longest streak if needed
	if newStreak > longestStreak {
		longestStreak = newStreak
	}

	// Update the streak record
	_, err = tx.Exec(ctx, `
        UPDATE user_streaks 
        SET 
            current_streak = $1, 
            longest_streak = $2, 
            last_completed_date = $3
        WHERE user_id = $4
    `, newStreak, longestStreak, today, userIDInt)

	if err != nil {
		return fmt.Errorf("failed to update user streak: %v", err)
	}

	// Determine if we should create a streak event based on requirements:
	// - Streak is more than 2 days old AND
	// - Either first event about this streak OR last event was 3+ days ago
	shouldCreateEvent := false
	log.Println("Updating streak for userid: ", userIDInt, " len:", newStreak)
	if newStreak > 2 {
		if lastStreakEventDate == nil {
			// First event about this streak
			shouldCreateEvent = true
		} else {
			// Check if last event was 3+ days ago
			daysSinceLastEvent := int(time.Now().Sub(*lastStreakEventDate).Hours() / 24)
			if daysSinceLastEvent >= 3 {
				shouldCreateEvent = true
			}
		}
	}

	if shouldCreateEvent {
		// Create streak event
		err = createStreakEvent(ctx, tx, userID, newStreak)
		if err != nil {
			return fmt.Errorf("failed to create streak event: %v", err)
		}

		// Update last_streak_event_date
		_, err = tx.Exec(ctx, `
            UPDATE user_streaks 
            SET last_streak_event_date = $1
            WHERE user_id = $2
        `, today, userIDInt)

		if err != nil {
			return fmt.Errorf("failed to update last streak event date: %v", err)
		}
	}

	return nil
}

func createStreakEvent(ctx context.Context, tx pgx.Tx, userID string, streakDays int) error {
	eventID := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(30 * 24 * time.Hour) // Streak events last 30 days

	_, err := tx.Exec(ctx, `
        INSERT INTO events (
            id, user_id, target_user_id, event_type, event_size, 
            created_at, expires_at, streak_days, likes_count, comments_count,
            task_ids
        ) VALUES (
            $1, $2, $3, 'n_day_streak', 'medium', $4, $5, $6, 0, 0, '{}'
        )
    `, eventID, userID, userID, now, expiresAt, streakDays)

	return err
}

func (h *TaskHandler) UpdateTask(ctx context.Context, req *pb.UpdateTaskRequest) (*pb.UpdateTaskResponse, error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Get current task state to check for completion state change
	var oldIsCompleted bool
	var oldNeedsConfirmation bool
	err = tx.QueryRow(ctx, `
		SELECT is_completed, COALESCE(needs_confirmation, false)
		FROM tasks 
		WHERE id = $1 AND user_id = $2
	`, req.Task.Id, req.Task.UserId).Scan(&oldIsCompleted, &oldNeedsConfirmation)

	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("failed to check task status: %v", err)
	}

	// Task not found
	if err == pgx.ErrNoRows {
		return &pb.UpdateTaskResponse{
			Success: false,
			Error:   "no task found with the provided id and user id",
		}, nil
	}

	var deadline *time.Time
	if req.Task.Deadline != nil {
		t := req.Task.Deadline.AsTime()
		deadline = &t
	}

	assignedToArray := "{}"
	if len(req.Task.AssignedTo) > 0 {
		assignedToArray = fmt.Sprintf("{%s}", strings.Join(quoteStrings(req.Task.AssignedTo), ","))
	}

	exceptIDsArray := "{}"
	if len(req.Task.Privacy.ExceptIds) > 0 {
		exceptIDsArray = fmt.Sprintf("{%s}", strings.Join(quoteStrings(req.Task.Privacy.ExceptIds), ","))
	}

	// Determine if task is being completed and what kind of event to create
	isBeingCompleted := !oldIsCompleted && req.Task.Completion.IsCompleted
	needsConfirmation := req.Task.Completion.NeedsConfirmation

	// Update the task
	result, err := tx.Exec(ctx, `
    UPDATE tasks SET
        name = $1,
        description = $2,
        deadline = $3,
        assigned_to = $4,
        task_difficulty = $5,
        custom_hours = $6,
        is_completed = $7,
        proof_url = $8,
        privacy_level = $9,
        privacy_except_ids = $10,
        flag_status = $11, 
        flag_color = $12,
        flag_name = $13,
        display_order = $14,
		proof_needed = $18,
		proof_description = $19,
        needs_confirmation = $15
    WHERE id = $16 AND user_id = $17
    RETURNING id
    `,
		req.Task.Name,
		req.Task.Description,
		deadline,
		assignedToArray,
		req.Task.TaskDifficulty,
		req.Task.CustomHours,
		req.Task.Completion.IsCompleted,
		req.Task.Completion.ProofUrl,
		req.Task.Privacy.Level,
		exceptIDsArray,
		req.Task.FlagStatus,
		req.Task.FlagColor,
		req.Task.FlagName,
		req.Task.DisplayOrder,
		needsConfirmation,
		req.Task.Id,
		req.Task.UserId, req.Task.ProofNeeded,
		req.Task.ProofDescription,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update task: %v", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return &pb.UpdateTaskResponse{
			Success: false,
			Error:   "no task found with the provided id and user id",
		}, nil
	}

	// Handle event creation based on task status changes
	if isBeingCompleted || req.Task.Completion.NeedsConfirmation {
		if req.Task.Completion.NeedsConfirmation {
			// Create "requires_confirmation" event
			eventID := uuid.New().String()
			now := time.Now()
			expiresAt := now.Add(3 * 24 * time.Hour)
			sizes := []string{"medium", "large"}
			eventSize := sizes[rand.Intn(len(sizes))]

			_, err = tx.Exec(ctx, `
				INSERT INTO events (
					id, user_id, target_user_id, event_type, event_size, 
					created_at, expires_at, task_ids, likes_count, comments_count
				) VALUES (
					$1, $2, $3, 'requires_confirmation', $4, $5, $6, $7, 0, 0
				)
			`, eventID, req.Task.UserId, req.Task.UserId, eventSize, now, expiresAt, []string{strings.ToLower(req.Task.Id)})

			if err != nil {
				log.Printf("failed to create requires_confirmation event: %v", err)
			}
		} else {
			// Create "newly_completed" event
			eventID := uuid.New().String()
			now := time.Now()
			expiresAt := now.Add(24 * time.Hour)

			// Choose a random event size
			sizes := []string{"small", "medium", "large"}
			eventSize := sizes[rand.Intn(len(sizes))]

			_, err = tx.Exec(ctx, `
				INSERT INTO events (
					id, user_id, target_user_id, event_type, event_size, 
					created_at, expires_at, task_ids, likes_count, comments_count
				) VALUES (
					$1, $2, $3, 'newly_completed', $4, $5, $6, $7, 0, 0
				)
			`, eventID, req.Task.UserId, req.Task.UserId, eventSize, now, expiresAt, []string{req.Task.Id})

			if err != nil {
				log.Printf("failed to create newly_completed event: %v", err)
			}

			// Update streak for non-confirmation tasks
			// Check if this is the first task completed today
			today := time.Now().Format("2006-01-02")
			var completedTodayCount int

			err := tx.QueryRow(ctx, `
				SELECT COUNT(*) FROM tasks 
				WHERE user_id = $1 
				AND is_completed = true
				AND DATE(COALESCE(confirmed_at, created_at)) = $2::date
				AND id != $3
			`, req.Task.UserId, today, req.Task.Id).Scan(&completedTodayCount)

			if err != nil {
				log.Printf("failed to check completed tasks count: %v", err)
			} else if completedTodayCount == 0 {
				// This is the first completion today, update streak
				err = updateUserStreak(ctx, tx, req.Task.UserId)
				if err != nil {
					log.Printf("failed to update user streak: %v", err)
				}
			}
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.UpdateTaskResponse{
		Success: true,
	}, nil
}

func (h *TaskHandler) ConfirmTaskCompletion(ctx context.Context, req *pb.ConfirmTaskCompletionRequest) (*pb.ConfirmTaskCompletionResponse, error) {
	if req.TaskId == "" || req.ConfirmerId == "" {
		return &pb.ConfirmTaskCompletionResponse{
			Success: false,
			Error:   "task id and confirmer id are required",
		}, nil
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Get task details
	var userID string
	var needsConfirmation bool
	var isCompleted bool
	var previouslyConfirmed bool
	err = tx.QueryRow(ctx, `
        SELECT user_id, COALESCE(needs_confirmation, false), is_completed, COALESCE(is_confirmed, false) 
        FROM tasks 
        WHERE id = $1
    `, req.TaskId).Scan(&userID, &needsConfirmation, &isCompleted, &previouslyConfirmed)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &pb.ConfirmTaskCompletionResponse{
				Success: false,
				Error:   "task not found",
			}, nil
		}
		return nil, fmt.Errorf("failed to get task: %v", err)
	}

	if !needsConfirmation {
		return &pb.ConfirmTaskCompletionResponse{
			Success: false,
			Error:   "task does not require confirmation",
		}, nil
	}

	if !isCompleted {
		return &pb.ConfirmTaskCompletionResponse{
			Success: false,
			Error:   "task is not marked as completed yet",
		}, nil
	}

	_, err = tx.Exec(ctx, `
        UPDATE tasks 
        SET is_confirmed = $1, 
            confirmation_user_id = $2, 
            confirmed_at = NOW() 
        WHERE id = $3
    `, req.IsConfirmed, req.ConfirmerId, req.TaskId)

	if err != nil {
		return nil, fmt.Errorf("failed to update task confirmation: %v", err)
	}

	// Clean up any "requires_confirmation" events for this task
	_, err = tx.Exec(ctx, `
		UPDATE events
		SET expires_at = NOW() - INTERVAL '1 second' -- Expire immediately
		WHERE event_type = 'requires_confirmation'
		AND $1 = ANY(task_ids)
	`, req.TaskId)

	if err != nil {
		log.Printf("failed to expire requires_confirmation event: %v", err)
	}

	// If task is confirmed (approved), create a newly_completed event and update streak
	if req.IsConfirmed && !previouslyConfirmed {
		// Create "newly_completed" event
		eventID := uuid.New().String()
		now := time.Now()
		expiresAt := now.Add(24 * time.Hour)

		// Choose a random event size
		sizes := []string{"small", "medium", "large"}
		eventSize := sizes[rand.Intn(len(sizes))]

		_, err = tx.Exec(ctx, `
			INSERT INTO events (
				id, user_id, target_user_id, event_type, event_size, 
				created_at, expires_at, task_ids, likes_count, comments_count
			) VALUES (
				$1, $2, $3, 'newly_completed', $4, $5, $6, $7, 0, 0
			)
		`, eventID, userID, userID, eventSize, now, expiresAt, []string{req.TaskId})

		if err != nil {
			log.Printf("failed to create newly_completed event: %v", err)
		}

		// Update streak if this is the first confirmed task today
		today := time.Now().Format("2006-01-02")
		var completedTodayCount int

		err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM tasks 
			WHERE user_id = $1 
			AND is_completed = true
			AND is_confirmed = true
			AND DATE(confirmed_at) = $2::date
			AND id != $3 -- Don't count the current task
		`, userID, today, req.TaskId).Scan(&completedTodayCount)

		if err != nil {
			log.Printf("failed to check completed tasks count: %v", err)
		} else if completedTodayCount == 0 {
			// This is the first confirmed completion today, update streak
			err = updateUserStreak(ctx, tx, userID)
			if err != nil {
				log.Printf("failed to update user streak: %v", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.ConfirmTaskCompletionResponse{
		Success: true,
	}, nil
}

func quoteStrings(strs []string) []string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf("\"%s\"", s)
	}
	return quoted
}
