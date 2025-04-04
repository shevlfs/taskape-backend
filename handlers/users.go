package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	pb "taskape-backend/proto"

	"github.com/google/uuid"
	"github.com/jackc/pgx"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *UserHandler) CheckHandleAvailability(ctx context.Context, req *pb.CheckHandleRequest) (*pb.CheckHandleResponse, error) {
	if req.Handle == "" {
		return nil, fmt.Errorf("handle cannot be empty")
	}

	var exists bool
	err := h.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE handle = $1)", req.Handle).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check handle availability: %v", err)
	}
	println(exists)

	return &pb.CheckHandleResponse{
		Available: !exists,
	}, nil
}

func (h *UserHandler) RegisterNewProfile(ctx context.Context, req *pb.RegisterNewProfileRequest) (*pb.RegisterNewProfileResponse, error) {
	tx, err := h.Pool.Begin(ctx)
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

func (h *UserHandler) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
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

	err = h.Pool.QueryRow(ctx,
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

	friendsRows, err := h.Pool.Query(ctx, `
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

	incomingRows, err := h.Pool.Query(ctx, `
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

	outgoingRows, err := h.Pool.Query(ctx, `
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



func (h *UserHandler) GetUsersBatch(ctx context.Context, req *pb.GetUsersBatchRequest) (*pb.GetUsersBatchResponse, error) {
	if len(req.UserIds) == 0 {
		return &pb.GetUsersBatchResponse{
			Success: false,
			Error:   "No user IDs provided",
		}, nil
	}

	
	maxUsers := 50
	if len(req.UserIds) > maxUsers {
		req.UserIds = req.UserIds[:maxUsers]
	}

	
	query := `
		SELECT id, handle, bio, profile_picture, color
		FROM users
		WHERE id = ANY($1)
	`

	
	userIDInts := make([]int64, 0, len(req.UserIds))
	for _, id := range req.UserIds {
		userID, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			continue 
		}
		userIDInts = append(userIDInts, userID)
	}

	
	rows, err := h.Pool.Query(ctx, query, userIDInts)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %v", err)
	}
	defer rows.Close()

	
	var users []*pb.UserResponse
	for rows.Next() {
		var id int64
		var handle, bio, profilePicture, color string

		err := rows.Scan(&id, &handle, &bio, &profilePicture, &color)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user row: %v", err)
		}

		users = append(users, &pb.UserResponse{
			Id:             fmt.Sprintf("%d", id),
			Handle:         handle,
			Bio:            bio,
			ProfilePicture: profilePicture,
			Color:          color,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user rows: %v", err)
	}

	return &pb.GetUsersBatchResponse{
		Success: true,
		Users:   users,
	}, nil
}



func (h *UserHandler) EditUserProfile(ctx context.Context, req *pb.EditUserProfileRequest) (*pb.EditUserProfileResponse, error) {
	if req.UserId == "" {
		return &pb.EditUserProfileResponse{
			Success: false,
			Error:   "User ID is required",
		}, nil
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	
	var exists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", req.UserId).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check if user exists: %v", err)
	}

	if !exists {
		return &pb.EditUserProfileResponse{
			Success: false,
			Error:   "User not found",
		}, nil
	}

	
	if req.Handle != "" {
		var currentHandle string
		err = tx.QueryRow(ctx, "SELECT handle FROM users WHERE id = $1", req.UserId).Scan(&currentHandle)
		if err != nil {
			return nil, fmt.Errorf("failed to get current handle: %v", err)
		}

		if currentHandle != req.Handle {
			
			var handleExists bool
			err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE handle = $1 AND id != $2)", req.Handle, req.UserId).Scan(&handleExists)
			if err != nil {
				return nil, fmt.Errorf("failed to check handle availability: %v", err)
			}

			if handleExists {
				return &pb.EditUserProfileResponse{
					Success: false,
					Error:   "Handle is already taken",
				}, nil
			}
		}
	}

	
	updateFields := []string{}
	args := []interface{}{req.UserId} 
	argCount := 2                     

	if req.Handle != "" {
		updateFields = append(updateFields, fmt.Sprintf("handle = $%d", argCount))
		args = append(args, req.Handle)
		argCount++
	}

	if req.Bio != "" {
		updateFields = append(updateFields, fmt.Sprintf("bio = $%d", argCount))
		args = append(args, req.Bio)
		argCount++
	}

	if req.Color != "" {
		updateFields = append(updateFields, fmt.Sprintf("color = $%d", argCount))
		args = append(args, req.Color)
		argCount++
	}

	if req.ProfilePicture != "" {
		updateFields = append(updateFields, fmt.Sprintf("profile_picture = $%d", argCount))
		args = append(args, req.ProfilePicture)
		argCount++
	}

	
	if len(updateFields) == 0 {
		return &pb.EditUserProfileResponse{
			Success: false,
			Error:   "No fields to update",
		}, nil
	}

	updateQuery := fmt.Sprintf("UPDATE users SET %s WHERE id = $1", strings.Join(updateFields, ", "))
	_, err = tx.Exec(ctx, updateQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update user profile: %v", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.EditUserProfileResponse{
		Success: true,
	}, nil
}

func (h *UserHandler) CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.CreateGroupResponse, error) {
	if req.CreatorId == "" || req.GroupName == "" {
		return &pb.CreateGroupResponse{
			Success: false,
			Error:   "Creator ID and group name are required",
		}, nil
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	
	var creatorExists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", req.CreatorId).Scan(&creatorExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check if creator exists: %v", err)
	}

	if !creatorExists {
		return &pb.CreateGroupResponse{
			Success: false,
			Error:   "Creator user not found",
		}, nil
	}

	groupID := uuid.New().String()

	_, err = tx.Exec(ctx, `
		INSERT INTO groups (
			id, name, description, color, creator_id, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6
		)
	`, groupID, req.GroupName, req.Description, req.Color, req.CreatorId, time.Now())

	if err != nil {
		return nil, fmt.Errorf("failed to create group: %v", err)
	}

	
	_, err = tx.Exec(ctx, `
		INSERT INTO group_members (
			group_id, user_id, role, joined_at
		) VALUES (
			$1, $2, 'admin', $3
		)
	`, groupID, req.CreatorId, time.Now())

	if err != nil {
		return nil, fmt.Errorf("failed to add creator to group: %v", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.CreateGroupResponse{
		Success: true,
		GroupId: groupID,
	}, nil
}

func (h *UserHandler) GetGroupTasks(ctx context.Context, req *pb.GetGroupTasksRequest) (*pb.GetGroupTasksResponse, error) {
	if req.GroupId == "" {
		return &pb.GetGroupTasksResponse{
			Success: false,
			Error:   "Group ID is required",
		}, nil
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	
	var groupExists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM groups WHERE id = $1)", req.GroupId).Scan(&groupExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check if group exists: %v", err)
	}

	if !groupExists {
		return &pb.GetGroupTasksResponse{
			Success: false,
			Error:   "Group not found",
		}, nil
	}

	
	var isMember bool
	if req.RequesterId != "" {
		err = tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM group_members 
				WHERE group_id = $1 AND user_id = $2
			)
		`, req.GroupId, req.RequesterId).Scan(&isMember)
		if err != nil {
			return nil, fmt.Errorf("failed to check group membership: %v", err)
		}
	}

	
	if !isMember && req.RequesterId != "" {
		return &pb.GetGroupTasksResponse{
			Success: false,
			Error:   "Requester is not a member of this group",
		}, nil
	}

	
	rows, err := tx.Query(ctx, `
		SELECT 
			id, user_id, name, description, created_at, deadline, author, "group", group_id,
			assigned_to, task_difficulty, custom_hours, mentioned_in_event,
			is_completed, proof_url, privacy_level, privacy_except_ids,
			flag_status, flag_color, flag_name, display_order, 
			needs_confirmation, is_confirmed, confirmation_user_id, confirmed_at
		FROM tasks
		WHERE group_id = $1
	`, req.GroupId)
	if err != nil {
		return nil, fmt.Errorf("failed to query group tasks: %v", err)
	}
	defer rows.Close()

	var tasks []*pb.Task
	for rows.Next() {
		var (
			id, userID, name, description, author, group, groupID, taskDifficulty, privacyLevel string
			createdAt                                                                           time.Time
			deadlinePtr                                                                         *time.Time
			assignedTo, privacyExceptIDs                                                        []string
			customHours                                                                         int32
			customHoursPtr                                                                      *int32
			mentionedInEvent, isCompleted, flagStatus                                           bool
			proofURL, flagColor, flagName                                                       string
			proofURLPtr, flagColorPtr, flagNamePtr                                              *string
			displayOrder                                                                        int32
			needsConfirmation, isConfirmed                                                      bool
			confirmationUserID                                                                  string
			confirmationUserIDPtr                                                               *string
			confirmedAt                                                                         *time.Time
		)

		err := rows.Scan(
			&id, &userID, &name, &description, &createdAt, &deadlinePtr, &author, &group, &groupID,
			&assignedTo, &taskDifficulty, &customHoursPtr, &mentionedInEvent,
			&isCompleted, &proofURLPtr, &privacyLevel, &privacyExceptIDs,
			&flagStatus, &flagColorPtr, &flagNamePtr, &displayOrder,
			&needsConfirmation, &isConfirmed, &confirmationUserIDPtr, &confirmedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task row: %v", err)
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
		if confirmationUserIDPtr != nil {
			confirmationUserID = *confirmationUserIDPtr
		}

		
		createdAtProto := timestamppb.New(createdAt)
		var deadlineProto, confirmedAtProto *timestamppb.Timestamp
		if deadlinePtr != nil {
			deadlineProto = timestamppb.New(*deadlinePtr)
		}
		if confirmedAt != nil {
			confirmedAtProto = timestamppb.New(*confirmedAt)
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
				IsCompleted:        isCompleted,
				ProofUrl:           proofURL,
				NeedsConfirmation:  needsConfirmation,
				IsConfirmed:        isConfirmed,
				ConfirmationUserId: confirmationUserID,
				ConfirmedAt:        confirmedAtProto,
			},
			Privacy: &pb.PrivacySettings{
				Level:     privacyLevel,
				ExceptIds: privacyExceptIDs,
			},
			FlagStatus:   flagStatus,
			FlagColor:    flagColor,
			FlagName:     flagName,
			DisplayOrder: displayOrder,
		}

		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating task rows: %v", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.GetGroupTasksResponse{
		Success: true,
		Tasks:   tasks,
	}, nil
}

func (h *UserHandler) InviteToGroup(ctx context.Context, req *pb.InviteToGroupRequest) (*pb.InviteToGroupResponse, error) {
	if req.GroupId == "" || req.InviterId == "" || req.InviteeId == "" {
		return &pb.InviteToGroupResponse{
			Success: false,
			Error:   "Group ID, inviter ID, and invitee ID are required",
		}, nil
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	
	var groupExists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM groups WHERE id = $1)", req.GroupId).Scan(&groupExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check if group exists: %v", err)
	}

	if !groupExists {
		return &pb.InviteToGroupResponse{
			Success: false,
			Error:   "Group not found",
		}, nil
	}

	
	var inviterRole string
	err = tx.QueryRow(ctx, `
		SELECT role FROM group_members 
		WHERE group_id = $1 AND user_id = $2
	`, req.GroupId, req.InviterId).Scan(&inviterRole)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &pb.InviteToGroupResponse{
				Success: false,
				Error:   "Inviter is not a member of this group",
			}, nil
		}
		return nil, fmt.Errorf("failed to check inviter role: %v", err)
	}

	
	if inviterRole != "admin" && inviterRole != "member" {
		return &pb.InviteToGroupResponse{
			Success: false,
			Error:   "Inviter does not have permission to invite others",
		}, nil
	}

	
	var inviteeExists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", req.InviteeId).Scan(&inviteeExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check if invitee exists: %v", err)
	}

	if !inviteeExists {
		return &pb.InviteToGroupResponse{
			Success: false,
			Error:   "Invitee user not found",
		}, nil
	}

	
	var alreadyMember bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM group_members 
			WHERE group_id = $1 AND user_id = $2
		)
	`, req.GroupId, req.InviteeId).Scan(&alreadyMember)
	if err != nil {
		return nil, fmt.Errorf("failed to check if invitee is already a member: %v", err)
	}

	if alreadyMember {
		return &pb.InviteToGroupResponse{
			Success: false,
			Error:   "Invitee is already a member of this group",
		}, nil
	}

	
	var pendingInviteExists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM group_invites 
			WHERE group_id = $1 AND invitee_id = $2 AND status = 'pending'
		)
	`, req.GroupId, req.InviteeId).Scan(&pendingInviteExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check for pending invites: %v", err)
	}

	if pendingInviteExists {
		return &pb.InviteToGroupResponse{
			Success: false,
			Error:   "There's already a pending invite for this user",
		}, nil
	}

	
	inviteID := uuid.New().String()

	
	_, err = tx.Exec(ctx, `
		INSERT INTO group_invites (
			id, group_id, inviter_id, invitee_id, status, created_at
		) VALUES (
			$1, $2, $3, $4, 'pending', $5
		)
	`, inviteID, req.GroupId, req.InviterId, req.InviteeId, time.Now())

	if err != nil {
		return nil, fmt.Errorf("failed to create group invite: %v", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.InviteToGroupResponse{
		Success:  true,
		InviteId: inviteID,
	}, nil
}

func (h *UserHandler) AcceptGroupInvite(ctx context.Context, req *pb.AcceptGroupInviteRequest) (*pb.AcceptGroupInviteResponse, error) {
	if req.InviteId == "" || req.UserId == "" {
		return &pb.AcceptGroupInviteResponse{
			Success: false,
			Error:   "Invite ID and user ID are required",
		}, nil
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	
	var groupID, inviteeID string
	var status string
	err = tx.QueryRow(ctx, `
		SELECT group_id, invitee_id, status
		FROM group_invites
		WHERE id = $1
	`, req.InviteId).Scan(&groupID, &inviteeID, &status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &pb.AcceptGroupInviteResponse{
				Success: false,
				Error:   "Invite not found",
			}, nil
		}
		return nil, fmt.Errorf("failed to get invite details: %v", err)
	}

	
	if inviteeID != req.UserId {
		return &pb.AcceptGroupInviteResponse{
			Success: false,
			Error:   "Only the invitee can accept or reject this invite",
		}, nil
	}

	
	if status != "pending" {
		return &pb.AcceptGroupInviteResponse{
			Success: false,
			Error:   fmt.Sprintf("Invite is already %s", status),
		}, nil
	}

	
	newStatus := "rejected"
	if req.Accept {
		newStatus = "accepted"
	}

	_, err = tx.Exec(ctx, `
		UPDATE group_invites
		SET status = $1, responded_at = $2
		WHERE id = $3
	`, newStatus, time.Now(), req.InviteId)
	if err != nil {
		return nil, fmt.Errorf("failed to update invite status: %v", err)
	}

	
	if req.Accept {
		
		var groupExists bool
		err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM groups WHERE id = $1)", groupID).Scan(&groupExists)
		if err != nil {
			return nil, fmt.Errorf("failed to check if group exists: %v", err)
		}

		if !groupExists {
			return &pb.AcceptGroupInviteResponse{
				Success: false,
				Error:   "Group no longer exists",
			}, nil
		}

		
		_, err = tx.Exec(ctx, `
			INSERT INTO group_members (
				group_id, user_id, role, joined_at
			) VALUES (
				$1, $2, 'member', $3
			)
		`, groupID, req.UserId, time.Now())
		if err != nil {
			return nil, fmt.Errorf("failed to add user to group: %v", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.AcceptGroupInviteResponse{
		Success: true,
	}, nil
}

func (h *UserHandler) KickUserFromGroup(ctx context.Context, req *pb.KickUserFromGroupRequest) (*pb.KickUserFromGroupResponse, error) {
	if req.GroupId == "" || req.AdminId == "" || req.UserId == "" {
		return &pb.KickUserFromGroupResponse{
			Success: false,
			Error:   "Group ID, admin ID, and user ID are required",
		}, nil
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	
	var groupExists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM groups WHERE id = $1)", req.GroupId).Scan(&groupExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check if group exists: %v", err)
	}

	if !groupExists {
		return &pb.KickUserFromGroupResponse{
			Success: false,
			Error:   "Group not found",
		}, nil
	}

	
	var creatorId string
	err = tx.QueryRow(ctx, "SELECT creator_id FROM groups WHERE id = $1", req.GroupId).Scan(&creatorId)
	if err != nil {
		return nil, fmt.Errorf("failed to get group creator: %v", err)
	}

	if creatorId != req.AdminId {
		return &pb.KickUserFromGroupResponse{
			Success: false,
			Error:   "Only the group creator can remove members",
		}, nil
	}

	
	var isMember bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM group_members 
			WHERE group_id = $1 AND user_id = $2
		)
	`, req.GroupId, req.UserId).Scan(&isMember)
	if err != nil {
		return nil, fmt.Errorf("failed to check if user is a member: %v", err)
	}

	if !isMember {
		return &pb.KickUserFromGroupResponse{
			Success: false,
			Error:   "User is not a member of this group",
		}, nil
	}

	
	if req.UserId == creatorId {
		return &pb.KickUserFromGroupResponse{
			Success: false,
			Error:   "Cannot remove the group creator",
		}, nil
	}

	
	_, err = tx.Exec(ctx, `
		DELETE FROM group_members
		WHERE group_id = $1 AND user_id = $2
	`, req.GroupId, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("failed to remove user from group: %v", err)
	}

	
	_, err = tx.Exec(ctx, `
		UPDATE group_invites
		SET status = 'cancelled', responded_at = $1
		WHERE group_id = $2 AND invitee_id = $3 AND status = 'pending'
	`, time.Now(), req.GroupId, req.UserId)
	if err != nil {
		log.Printf("Warning: Failed to cancel pending invites: %v", err)
		
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.KickUserFromGroupResponse{
		Success: true,
	}, nil
}



func (h *UserHandler) GetUserStreak(ctx context.Context, req *pb.GetUserStreakRequest) (*pb.GetUserStreakResponse, error) {
	if req.UserId == "" {
		return &pb.GetUserStreakResponse{
			Success: false,
			Error:   "User ID is required",
		}, nil
	}

	userIDInt, err := strconv.Atoi(req.UserId)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID format: %v", err)
	}

	
	var userExists bool
	err = h.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", userIDInt).Scan(&userExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check if user exists: %v", err)
	}

	if !userExists {
		return &pb.GetUserStreakResponse{
			Success: false,
			Error:   "User not found",
		}, nil
	}

	
	var currentStreak, longestStreak int32
	var lastCompletedDate, streakStartDate *time.Time

	err = h.Pool.QueryRow(ctx, `
		SELECT 
			COALESCE(current_streak, 0), 
			COALESCE(longest_streak, 0), 
			last_completed_date,
			(SELECT MIN(confirmed_at) FROM tasks 
			 WHERE user_id = $1::text 
			 AND is_completed = true 
			 AND confirmed_at > NOW() - INTERVAL '1 day' * current_streak)
		FROM user_streaks 
		WHERE user_id = $1
	`, userIDInt).Scan(&currentStreak, &longestStreak, &lastCompletedDate, &streakStartDate)

	if err != nil {
		if err == pgx.ErrNoRows {
			
			return &pb.GetUserStreakResponse{
				Success: true,
				Streak: &pb.UserStreakData{
					CurrentStreak: 0,
					LongestStreak: 0,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to get user streak: %v", err)
	}

	streakData := &pb.UserStreakData{
		CurrentStreak: currentStreak,
		LongestStreak: longestStreak,
	}

	if lastCompletedDate != nil {
		streakData.LastCompletedDate = timestamppb.New(*lastCompletedDate)
	}

	if streakStartDate != nil {
		streakData.StreakStartDate = timestamppb.New(*streakStartDate)
	}

	return &pb.GetUserStreakResponse{
		Success: true,
		Streak:  streakData,
	}, nil
}
