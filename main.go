package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	pb "taskape-backend/proto"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var publicEndpoints = []string{
	"/taskapebackend.BackendRequests/loginNewUser",
	"/taskapebackend.BackendRequests/validateToken",
	"/taskapebackend.BackendRequests/refreshToken",
}

type server struct {
	pb.BackendRequestsServer
	pool *pgxpool.Pool
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

func (s *server) CheckHandleAvailability(ctx context.Context, req *pb.CheckHandleRequest) (*pb.CheckHandleResponse, error) {
	if req.Handle == "" {
		return nil, fmt.Errorf("handle cannot be empty")
	}

	var exists bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE handle = $1)", req.Handle).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check handle availability: %v", err)
	}
	println(exists)

	return &pb.CheckHandleResponse{
		Available: !exists,
	}, nil
}

func generateTokens(phone string) (*TokenPair, error) {
	secretKey := os.Getenv("JWT_SECRET")
	if secretKey == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable not set")
	}

	accessClaims := jwt.MapClaims{
		"phone": phone,
		"exp":   time.Now().Add(time.Hour * 24).Unix(),
		"iat":   time.Now().Unix(),
		"type":  "access",
	}

	refreshClaims := jwt.MapClaims{
		"phone": phone,
		"exp":   time.Now().Add(time.Hour * 24 * 30).Unix(),
		"iat":   time.Now().Unix(),
		"type":  "refresh",
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)

	accessTokenString, err := accessToken.SignedString([]byte(secretKey))
	if err != nil {
		return nil, err
	}

	refreshTokenString, err := refreshToken.SignedString([]byte(secretKey))
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
	}, nil
}

func (s *server) UpdateTaskOrder(ctx context.Context, req *pb.UpdateTaskOrderRequest) (*pb.UpdateTaskOrderResponse, error) {
	tx, err := s.pool.Begin(ctx)
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

func (s *server) LoginNewUser(ctx context.Context, req *pb.NewUserLoginRequest) (*pb.NewUserLoginResponse, error) {
	print("LoginNewUser called with phone: ", req.Phone)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	checkQuery := `SELECT EXISTS(SELECT 1 FROM users WHERE phone = $1 AND handle IS NOT NULL)`
	var profileExists bool
	err = tx.QueryRow(ctx, checkQuery, req.Phone).Scan(&profileExists)
	if err != nil {
		return nil, fmt.Errorf("database query failed: %v", err)
	}
	var existingID int64 = -1
	if !profileExists {
		_, err = tx.Exec(ctx, "INSERT INTO users (phone) VALUES ($1)", req.Phone)
		if err != nil {
			return nil, fmt.Errorf("insert failed: %v", err)
		}
	} else {
		err = tx.QueryRow(ctx, "SELECT id FROM users WHERE phone = $1", req.Phone).Scan(&existingID)
		if err != nil {
			return nil, fmt.Errorf("failed to get user ID: %v", err)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	tokens, err := generateTokens(req.Phone)
	if err != nil {
		return nil, fmt.Errorf("token generation failed: %v", err)
	}

	return &pb.NewUserLoginResponse{
		Token:         tokens.AccessToken,
		RefreshToken:  tokens.RefreshToken,
		ProfileExists: profileExists,
		UserId:        existingID,
	}, nil
}

func (s *server) RegisterNewProfile(ctx context.Context, req *pb.RegisterNewProfileRequest) (*pb.RegisterNewProfileResponse, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	println("RegisterNewProfile called with phone: ", req.Phone)
	var existingID int64
	err = tx.QueryRow(ctx,
		"SELECT id FROM users WHERE phone = $1",
		req.Phone).Scan(&existingID)

	if err == pgx.ErrNoRows {
		err = tx.QueryRow(ctx,
			`INSERT INTO users (phone, handle, profile_picture, bio, color) 
             VALUES ($1, $2, $3, $4, $5) 
             RETURNING id`,
			req.Phone,
			req.Handle,
			req.ProfilePicture,
			req.Bio,
			req.Color).Scan(&existingID)

		if err != nil {
			return nil, fmt.Errorf("failed to insert new user: %v", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to query existing user: %v", err)
	} else {
		_, err = tx.Exec(ctx,
			`UPDATE users 
             SET handle = $1, profile_picture = $2, bio = $3, color = $4
             WHERE id = $5`,
			req.Handle,
			req.ProfilePicture,
			req.Bio,
			req.Color,
			existingID)

		if err != nil {
			return nil, fmt.Errorf("failed to update existing user: %v", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.RegisterNewProfileResponse{
		Success: true,
		Id:      existingID,
	}, nil
}

func (s *server) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	token, err := jwt.Parse(req.Token, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		return &pb.ValidateTokenResponse{Valid: false}, nil
	}

	return &pb.ValidateTokenResponse{Valid: true}, nil
}

func (s *server) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	refreshToken, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !refreshToken.Valid {
		return nil, fmt.Errorf("invalid refresh token")
	}

	claims, ok := refreshToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	phone, ok := claims["phone"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid phone in token")
	}

	newTokens, err := generateTokens(phone)
	if err != nil {
		return nil, fmt.Errorf("token generation failed: %v", err)
	}

	return &pb.RefreshTokenResponse{
		Token:        newTokens.AccessToken,
		RefreshToken: newTokens.RefreshToken,
	}, nil
}

func (s *server) CreateTask(ctx context.Context, req *pb.CreateTaskRequest) (*pb.CreateTaskResponse, error) {
	tx, err := s.pool.Begin(ctx)
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

func (s *server) CreateTasksBatch(ctx context.Context, req *pb.CreateTasksBatchRequest) (*pb.CreateTasksBatchResponse, error) {
	tx, err := s.pool.Begin(ctx)
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

func (s *server) UpdateTask(ctx context.Context, req *pb.UpdateTaskRequest) (*pb.UpdateTaskResponse, error) {
	tx, err := s.pool.Begin(ctx)
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

func (s *server) GetUserTasks(ctx context.Context, req *pb.GetUserTasksRequest) (*pb.GetUserTasksResponse, error) {
	log.Printf("GetUserTasks called for user ID: %s", req.UserId)

	if req.UserId == "" {
		log.Printf("ERROR: Empty user ID provided")
		return &pb.GetUserTasksResponse{
			Success: false,
			Error:   "User ID cannot be empty",
		}, nil
	}

	rows, err := s.pool.Query(ctx, `
        SELECT 
            id, user_id, name, description, created_at, deadline, author, "group", group_id,
            assigned_to, task_difficulty, custom_hours, mentioned_in_event,
            is_completed, proof_url, privacy_level, privacy_except_ids,
            flag_status, flag_color, flag_name, display_order
        FROM tasks
        WHERE user_id = $1
        ORDER BY display_order ASC, created_at DESC
    `, req.UserId)

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

	log.Printf("Successfully fetched %d tasks for user %s", len(tasks), req.UserId)
	return &pb.GetUserTasksResponse{
		Success: true,
		Tasks:   tasks,
	}, nil
}

func AuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	println(info.FullMethod)
	if slices.Contains(publicEndpoints, info.FullMethod) {
		return handler(ctx, req)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		print("unauthenticated request rejected\n")
		return nil, status.Errorf(codes.Unauthenticated, "no metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		print("unauthenticated request rejected, no token\n")
		return nil, status.Errorf(codes.Unauthenticated, "no auth token")
	}

	token, err := jwt.Parse(values[0], func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		print("unauthenticated request rejected, invalid token\n")
		return nil, status.Errorf(codes.Unauthenticated, "invalid token")
	}
	return handler(ctx, req)
}

func (s *server) SearchUsers(ctx context.Context, req *pb.SearchUsersRequest) (*pb.SearchUsersResponse, error) {
	if req.Query == "" {
		return nil, fmt.Errorf("search query is required")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.pool.Query(ctx, `
        SELECT id, handle, profile_picture, color
        FROM users
        WHERE handle ILIKE $1
        ORDER BY 
            CASE 
                WHEN handle ILIKE $2 THEN 0 -- Exact match
                WHEN handle ILIKE $3 THEN 1 -- Starts with query
                ELSE 2 -- Contains query
            END,
            handle
        LIMIT $4
    `, "%"+req.Query+"%", req.Query, req.Query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search users: %v", err)
	}
	defer rows.Close()

	var users []*pb.UserSearchResult
	for rows.Next() {
		var id, handle, profilePicture, color string
		err := rows.Scan(&id, &handle, &profilePicture, &color)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user search result: %v", err)
		}

		users = append(users, &pb.UserSearchResult{
			Id:             id,
			Handle:         handle,
			ProfilePicture: profilePicture,
			Color:          color,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user search results: %v", err)
	}

	return &pb.SearchUsersResponse{
		Users: users,
	}, nil
}

func (s *server) SendFriendRequest(ctx context.Context, req *pb.SendFriendRequestRequest) (*pb.SendFriendRequestResponse, error) {
	if req.SenderId == "" || req.ReceiverId == "" {
		return nil, fmt.Errorf("sender and receiver IDs are required")
	}

	var senderExists, receiverExists bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", req.SenderId).Scan(&senderExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check sender existence: %v", err)
	}

	err = s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", req.ReceiverId).Scan(&receiverExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check receiver existence: %v", err)
	}

	if !senderExists {
		return &pb.SendFriendRequestResponse{
			Success: false,
			Error:   "Sender user not found",
		}, nil
	}

	if !receiverExists {
		return &pb.SendFriendRequestResponse{
			Success: false,
			Error:   "Receiver user not found",
		}, nil
	}

	var alreadyFriends bool
	err = s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM user_friends WHERE user_id = $1 AND friend_id = $2)", req.SenderId, req.ReceiverId).Scan(&alreadyFriends)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing friendship: %v", err)
	}

	if alreadyFriends {
		return &pb.SendFriendRequestResponse{
			Success: false,
			Error:   "Users are already friends",
		}, nil
	}

	var requestId string
	var status string
	err = s.pool.QueryRow(ctx, `
    SELECT id, status FROM friend_requests 
    WHERE sender_id = $1 AND receiver_id = $2
`, req.SenderId, req.ReceiverId).Scan(&requestId, &status)

	if err != nil {
		if err == pgx.ErrNoRows || err.Error() == pgx.ErrNoRows.Error() {
			fmt.Printf("No existing request found, will create new one\n")
		} else {
			return nil, fmt.Errorf("failed to check existing request: %v", err)
		}
	}

	if err == nil {
		if status == "pending" {
			return &pb.SendFriendRequestResponse{
				Success:   true,
				RequestId: requestId,
				Error:     "Friend request already sent",
			}, nil
		} else if status == "rejected" {
			_, err = s.pool.Exec(ctx, `
            UPDATE friend_requests 
            SET status = 'pending', updated_at = NOW() 
            WHERE id = $1
        `, requestId)

			if err != nil {
				return nil, fmt.Errorf("failed to update friend request: %v", err)
			}

			return &pb.SendFriendRequestResponse{
				Success:   true,
				RequestId: requestId,
			}, nil
		}
	}

	err = s.pool.QueryRow(ctx, `
    INSERT INTO friend_requests (sender_id, receiver_id, status) 
    VALUES ($1, $2, 'pending') 
    RETURNING id
`, req.SenderId, req.ReceiverId).Scan(&requestId)

	if err != nil {
		return nil, fmt.Errorf("failed to create friend request: %v", err)
	}

	return &pb.SendFriendRequestResponse{
		Success:   true,
		RequestId: requestId,
	}, nil
}

func (s *server) RespondToFriendRequest(ctx context.Context, req *pb.RespondToFriendRequestRequest) (*pb.RespondToFriendRequestResponse, error) {
	if req.RequestId == "" || req.UserId == "" || req.Response == "" {
		return nil, fmt.Errorf("request ID, user ID, and response are required")
	}

	if req.Response != "accept" && req.Response != "reject" {
		return nil, fmt.Errorf("response must be 'accept' or 'reject'")
	}

	var senderId, receiverId string
	var status string
	err := s.pool.QueryRow(ctx, `
        SELECT sender_id, receiver_id, status 
        FROM friend_requests 
        WHERE id = $1
    `, req.RequestId).Scan(&senderId, &receiverId, &status)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &pb.RespondToFriendRequestResponse{
				Success: false,
				Error:   "Friend request not found",
			}, nil
		}
		return nil, fmt.Errorf("failed to get friend request: %v", err)
	}

	if receiverId != req.UserId {
		return &pb.RespondToFriendRequestResponse{
			Success: false,
			Error:   "User is not the receiver of this friend request",
		}, nil
	}

	if status != "pending" {
		return &pb.RespondToFriendRequestResponse{
			Success: false,
			Error:   fmt.Sprintf("Friend request is already %s", status),
		}, nil
	}

	_, err = s.pool.Exec(ctx, `
        UPDATE friend_requests 
        SET status = $1, updated_at = NOW() 
        WHERE id = $2
    `, req.Response, req.RequestId)

	if err != nil {
		return nil, fmt.Errorf("failed to update friend request: %v", err)
	}

	if req.Response == "accept" {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		_, err = tx.Exec(ctx, `
            INSERT INTO user_friends (user_id, friend_id) VALUES ($1, $2)
        `, receiverId, senderId)
		if err != nil {
			return nil, fmt.Errorf("failed to create friendship record (receiver->sender): %v", err)
		}

		_, err = tx.Exec(ctx, `
            INSERT INTO user_friends (user_id, friend_id) VALUES ($1, $2)
        `, senderId, receiverId)
		if err != nil {
			return nil, fmt.Errorf("failed to create friendship record (sender->receiver): %v", err)
		}

		err = tx.Commit(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %v", err)
		}
	}

	return &pb.RespondToFriendRequestResponse{
		Success: true,
	}, nil
}

func (s *server) GetUserFriends(ctx context.Context, req *pb.GetUserFriendsRequest) (*pb.GetUserFriendsResponse, error) {
	if req.UserId == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	rows, err := s.pool.Query(ctx, `
        SELECT u.id, u.handle, u.profile_picture, u.color
        FROM users u
        JOIN user_friends uf ON u.id = uf.friend_id
        WHERE uf.user_id = $1
    `, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("failed to query user friends: %v", err)
	}
	defer rows.Close()

	var friends []*pb.Friend
	for rows.Next() {
		var id, handle, profilePicture, color string
		err := rows.Scan(&id, &handle, &profilePicture, &color)
		if err != nil {
			return nil, fmt.Errorf("failed to scan friend row: %v", err)
		}

		friends = append(friends, &pb.Friend{
			Id:             id,
			Handle:         handle,
			ProfilePicture: profilePicture,
			Color:          color,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating friend rows: %v", err)
	}

	return &pb.GetUserFriendsResponse{
		Friends: friends,
	}, nil
}

func (s *server) GetFriendRequests(ctx context.Context, req *pb.GetFriendRequestsRequest) (*pb.GetFriendRequestsResponse, error) {
	if req.UserId == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	if req.Type != "incoming" && req.Type != "outgoing" {
		return nil, fmt.Errorf("type must be 'incoming' or 'outgoing'")
	}

	var query string
	var args []interface{}

	if req.Type == "incoming" {
		query = `
            SELECT fr.id, fr.sender_id, u.handle, fr.receiver_id, fr.status, fr.created_at
            FROM friend_requests fr
            JOIN users u ON fr.sender_id = u.id
            WHERE fr.receiver_id = $1 AND fr.status = 'pending'
            ORDER BY fr.created_at DESC
        `
		args = append(args, req.UserId)
	} else {
		query = `
            SELECT fr.id, fr.sender_id, u.handle, fr.receiver_id, fr.status, fr.created_at
            FROM friend_requests fr
            JOIN users u ON fr.receiver_id = u.id
            WHERE fr.sender_id = $1 AND fr.status = 'pending'
            ORDER BY fr.created_at DESC
        `
		args = append(args, req.UserId)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query friend requests: %v", err)
	}
	defer rows.Close()

	var requests []*pb.FriendRequest
	for rows.Next() {
		var id, senderId, senderHandle, receiverId, status string
		var createdAt time.Time

		err := rows.Scan(&id, &senderId, &senderHandle, &receiverId, &status, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan friend request row: %v", err)
		}

		requests = append(requests, &pb.FriendRequest{
			Id:           id,
			SenderId:     senderId,
			SenderHandle: senderHandle,
			ReceiverId:   receiverId,
			Status:       status,
			CreatedAt:    timestamppb.New(createdAt),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating friend request rows: %v", err)
	}

	return &pb.GetFriendRequestsResponse{
		Requests: requests,
	}, nil
}

func (s *server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	if req.UserId == "" {
		return &pb.GetUserResponse{
			Success: false,
			Error:   "User ID is required",
		}, nil
	}

	var id int64
	var handle, bio, profilePicture, color string

	userId, err := strconv.ParseInt(req.UserId, 10, 64)
	if err != nil {
		return &pb.GetUserResponse{
			Success: false,
			Error:   "Invalid user ID format",
		}, nil
	}

	err = s.pool.QueryRow(ctx,
		"SELECT id, handle, bio, profile_picture, color FROM users WHERE id = $1",
		userId).Scan(&id, &handle, &bio, &profilePicture, &color)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &pb.GetUserResponse{
				Success: false,
				Error:   "User not found",
			}, nil
		}
		return nil, fmt.Errorf("failed to query user: %v", err)
	}

	friendsRows, err := s.pool.Query(ctx, `
        SELECT u.id, u.handle, u.profile_picture, u.color
        FROM users u
        JOIN user_friends uf ON u.id = uf.friend_id
        WHERE uf.user_id = $1
    `, userId)
	if err != nil {
		return nil, fmt.Errorf("failed to query user friends: %v", err)
	}
	defer friendsRows.Close()

	var friends []*pb.Friend
	for friendsRows.Next() {
		var friendId, friendHandle, friendPic, friendColor string
		err := friendsRows.Scan(&friendId, &friendHandle, &friendPic, &friendColor)
		if err != nil {
			return nil, fmt.Errorf("failed to scan friend row: %v", err)
		}

		friends = append(friends, &pb.Friend{
			Id:             friendId,
			Handle:         friendHandle,
			ProfilePicture: friendPic,
			Color:          friendColor,
		})
	}

	incomingRows, err := s.pool.Query(ctx, `
        SELECT fr.id, fr.sender_id, u.handle, fr.receiver_id, fr.status, fr.created_at
        FROM friend_requests fr
        JOIN users u ON fr.sender_id = u.id
        WHERE fr.receiver_id = $1 AND fr.status = 'pending'
        ORDER BY fr.created_at DESC
    `, userId)
	if err != nil {
		return nil, fmt.Errorf("failed to query incoming friend requests: %v", err)
	}
	defer incomingRows.Close()

	var incomingRequests []*pb.FriendRequest
	for incomingRows.Next() {
		var reqId, senderId, senderHandle, receiverId, status string
		var createdAt time.Time

		err := incomingRows.Scan(&reqId, &senderId, &senderHandle, &receiverId, &status, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan incoming request row: %v", err)
		}

		incomingRequests = append(incomingRequests, &pb.FriendRequest{
			Id:           reqId,
			SenderId:     senderId,
			SenderHandle: senderHandle,
			ReceiverId:   receiverId,
			Status:       status,
			CreatedAt:    timestamppb.New(createdAt),
		})
	}

	outgoingRows, err := s.pool.Query(ctx, `
        SELECT fr.id, fr.sender_id, u.handle, fr.receiver_id, fr.status, fr.created_at
        FROM friend_requests fr
        JOIN users u ON fr.receiver_id = u.id
        WHERE fr.sender_id = $1 AND fr.status = 'pending'
        ORDER BY fr.created_at DESC
    `, userId)
	if err != nil {
		return nil, fmt.Errorf("failed to query outgoing friend requests: %v", err)
	}
	defer outgoingRows.Close()

	var outgoingRequests []*pb.FriendRequest
	for outgoingRows.Next() {
		var reqId, senderId, receiverHandle, receiverId, status string
		var createdAt time.Time

		err := outgoingRows.Scan(&reqId, &senderId, &receiverHandle, &receiverId, &status, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan outgoing request row: %v", err)
		}

		outgoingRequests = append(outgoingRequests, &pb.FriendRequest{
			Id:           reqId,
			SenderId:     senderId,
			SenderHandle: "self",
			ReceiverId:   receiverId,
			Status:       status,
			CreatedAt:    timestamppb.New(createdAt),
		})
	}

	return &pb.GetUserResponse{
		Success:          true,
		Id:               fmt.Sprintf("%d", id),
		Handle:           handle,
		Bio:              bio,
		ProfilePicture:   profilePicture,
		Color:            color,
		Friends:          friends,
		IncomingRequests: incomingRequests,
		OutgoingRequests: outgoingRequests,
	}, nil
}

func main() {
	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbPort := os.Getenv("DB_PORT")

	dbURL := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		dbUser, dbPass, dbHost, dbPort, dbName)

	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Unable to parse pool config: %v\n", err)
	}

	poolConfig.MaxConns = 10
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		log.Fatalf("Unable to create connection pool: %v\n", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer(grpc.UnaryInterceptor(AuthInterceptor))
	pb.RegisterBackendRequestsServer(s, &server{pool: pool})

	log.Println("Server started on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
