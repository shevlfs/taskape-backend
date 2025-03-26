package handlers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	pb "taskape-backend/proto"

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
