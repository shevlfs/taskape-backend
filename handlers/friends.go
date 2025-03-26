package handlers

import (
	"context"
	"fmt"
	"time"

	pb "taskape-backend/proto"

	"github.com/jackc/pgx"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *FriendHandler) SearchUsers(ctx context.Context, req *pb.SearchUsersRequest) (*pb.SearchUsersResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	var query string
	var args []interface{}

	if req.Query == "" {
		query = `
            SELECT id, handle, profile_picture, color
            FROM users
            ORDER BY handle
            LIMIT $1
        `
		args = append(args, limit)
	} else {
		query = `
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
        `
		args = append(args, "%"+req.Query+"%", req.Query, req.Query+"%", limit)
	}

	rows, err := h.Pool.Query(ctx, query, args...)
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

func (h *FriendHandler) SendFriendRequest(ctx context.Context, req *pb.SendFriendRequestRequest) (*pb.SendFriendRequestResponse, error) {
	if req.SenderId == "" || req.ReceiverId == "" {
		return nil, fmt.Errorf("sender and receiver IDs are required")
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	var senderExists, receiverExists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", req.SenderId).Scan(&senderExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check sender existence: %v", err)
	}

	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", req.ReceiverId).Scan(&receiverExists)
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
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM user_friends WHERE user_id = $1 AND friend_id = $2)", req.SenderId, req.ReceiverId).Scan(&alreadyFriends)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing friendship: %v", err)
	}

	if alreadyFriends {
		return &pb.SendFriendRequestResponse{
			Success: false,
			Error:   "Users are already friends",
		}, nil
	}

	var existingRequestId string
	err = tx.QueryRow(ctx, `
        SELECT id FROM friend_requests 
        WHERE sender_id = $1 AND receiver_id = $2
    `, req.SenderId, req.ReceiverId).Scan(&existingRequestId)

	if err != nil && err != pgx.ErrNoRows && err.Error() != pgx.ErrNoRows.Error() {
		return nil, fmt.Errorf("failed to check existing request: %v", err)
	}

	if err == nil || (err != pgx.ErrNoRows && err.Error() != pgx.ErrNoRows.Error()) {
		_, err = tx.Exec(ctx, `
            DELETE FROM friend_requests 
            WHERE id = $1
        `, existingRequestId)

		if err != nil {
			return nil, fmt.Errorf("failed to delete existing request: %v", err)
		}
	}

	var requestId string
	err = tx.QueryRow(ctx, `
        INSERT INTO friend_requests (sender_id, receiver_id, status) 
        VALUES ($1, $2, 'pending') 
        RETURNING id
    `, req.SenderId, req.ReceiverId).Scan(&requestId)

	if err != nil {
		return nil, fmt.Errorf("failed to create friend request: %v", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.SendFriendRequestResponse{
		Success:   true,
		RequestId: requestId,
	}, nil
}

func (h *FriendHandler) RespondToFriendRequest(ctx context.Context, req *pb.RespondToFriendRequestRequest) (*pb.RespondToFriendRequestResponse, error) {
	if req.RequestId == "" || req.UserId == "" || req.Response == "" {
		return nil, fmt.Errorf("request ID, user ID, and response are required")
	}

	if req.Response != "accept" && req.Response != "reject" {
		return nil, fmt.Errorf("response must be 'accept' or 'reject'")
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	var senderId, receiverId string
	var status string
	err = tx.QueryRow(ctx, `
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

	_, err = tx.Exec(ctx, `
        DELETE FROM friend_requests
        WHERE id = $1
    `, req.RequestId)

	if err != nil {
		return nil, fmt.Errorf("failed to delete friend request: %v", err)
	}

	if req.Response == "accept" {
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
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &pb.RespondToFriendRequestResponse{
		Success: true,
	}, nil
}

func (h *FriendHandler) GetUserFriends(ctx context.Context, req *pb.GetUserFriendsRequest) (*pb.GetUserFriendsResponse, error) {
	if req.UserId == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	rows, err := h.Pool.Query(ctx, `
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

func (h *FriendHandler) GetFriendRequests(ctx context.Context, req *pb.GetFriendRequestsRequest) (*pb.GetFriendRequestsResponse, error) {
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

	rows, err := h.Pool.Query(ctx, query, args...)
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
