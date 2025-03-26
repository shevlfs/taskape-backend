package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	pb "taskape-backend/proto"

	"github.com/google/uuid"
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
            flag_status, flag_color, flag_name, display_order
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
            $17, $18, $19, $20
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
	)

	if err != nil {
		return nil, fmt.Errorf("failed to insert task: %v", err)
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

func (h *TaskHandler) CreateTasksBatch(ctx context.Context, req *pb.CreateTasksBatchRequest) (*pb.CreateTasksBatchResponse, error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	taskIDs := make([]string, len(req.Tasks))

	for i, task := range req.Tasks {
		taskID := task.Id
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

		_, err = tx.Exec(ctx, `
            INSERT INTO tasks (
                id, user_id, name, description, deadline, author, "group", group_id, 
                assigned_to, task_difficulty, custom_hours, mentioned_in_event,
                is_completed, proof_url, privacy_level, privacy_except_ids
            ) VALUES (
                $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
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
		)

		if err != nil {
			return nil, fmt.Errorf("failed to insert task %d: %v", i, err)
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

func (h *TaskHandler) UpdateTask(ctx context.Context, req *pb.UpdateTaskRequest) (*pb.UpdateTaskResponse, error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	var deadline *time.Time
	if req.Task.Deadline != nil {
		t := req.Task.Deadline.AsTime()
		deadline = &t
	}

	assignedToArray := "{}"
	if len(req.Task.AssignedTo) > 0 {
		assignedToArray = fmt.Sprintf("{%s}", strings.Join(req.Task.AssignedTo, ","))
	}

	exceptIDsArray := "{}"
	if len(req.Task.Privacy.ExceptIds) > 0 {
		exceptIDsArray = fmt.Sprintf("{%s}", strings.Join(req.Task.Privacy.ExceptIds, ","))
	}

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
        display_order = $14
    WHERE id = $15 AND user_id = $16
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
		req.Task.Id,
		req.Task.UserId,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update task: %v", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return &pb.UpdateTaskResponse{
			Success: false,
			Error:   "No task found with the provided ID and user ID",
		}, nil
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.UpdateTaskResponse{
		Success: true,
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
		// If viewing own tasks, get everything
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
		// If no requester_id (anonymous), only get public tasks
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
		// For authenticated users (potential friends)
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

	// Execute the query
	rows, err := h.Pool.Query(ctx, query, args...)
	if err != nil {
		log.Printf("ERROR querying tasks for user %s: %v", req.UserId, err)
		return &pb.GetUserTasksResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to query tasks: %v", err),
		}, nil
	}
	defer rows.Close()

	// Process the results (the rest of the function remains the same)
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
