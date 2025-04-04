package main

import (
	"context"
	"log"
	"net"

	"taskape-backend/handlers"
	"taskape-backend/middleware"
	pb "taskape-backend/proto"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

type Server struct {
	pb.BackendRequestsServer
	authHandler   *handlers.AuthHandler
	userHandler   *handlers.UserHandler
	taskHandler   *handlers.TaskHandler
	friendHandler *handlers.FriendHandler
	eventHandler  *handlers.EventHandler
}

func NewServer(pool *pgxpool.Pool) *Server {
	return &Server{
		authHandler:   handlers.NewAuthHandler(pool),
		userHandler:   handlers.NewUserHandler(pool),
		taskHandler:   handlers.NewTaskHandler(pool),
		friendHandler: handlers.NewFriendHandler(pool),
		eventHandler:  handlers.NewEventHandler(pool),
	}
}

func (s *Server) LoginNewUser(ctx context.Context, req *pb.NewUserLoginRequest) (*pb.NewUserLoginResponse, error) {
	return s.authHandler.LoginNewUser(ctx, req)
}

func (s *Server) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	return s.authHandler.ValidateToken(ctx, req)
}

func (s *Server) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	return s.authHandler.RefreshToken(ctx, req)
}

func (s *Server) VerifyUserToken(ctx context.Context, req *pb.VerifyUserRequest) (*pb.VerifyUserResponse, error) {
	return s.authHandler.VerifyUserToken(ctx, req)
}

func (s *Server) RegisterNewProfile(ctx context.Context, req *pb.RegisterNewProfileRequest) (*pb.RegisterNewProfileResponse, error) {
	return s.userHandler.RegisterNewProfile(ctx, req)
}

func (s *Server) CheckHandleAvailability(ctx context.Context, req *pb.CheckHandleRequest) (*pb.CheckHandleResponse, error) {
	return s.userHandler.CheckHandleAvailability(ctx, req)
}

func (s *Server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	return s.userHandler.GetUser(ctx, req)
}

func (s *Server) CreateTask(ctx context.Context, req *pb.CreateTaskRequest) (*pb.CreateTaskResponse, error) {
	return s.taskHandler.CreateTask(ctx, req)
}

func (s *Server) CreateTasksBatch(ctx context.Context, req *pb.CreateTasksBatchRequest) (*pb.CreateTasksBatchResponse, error) {
	return s.taskHandler.CreateTasksBatch(ctx, req)
}

func (s *Server) GetUserTasks(ctx context.Context, req *pb.GetUserTasksRequest) (*pb.GetUserTasksResponse, error) {
	return s.taskHandler.GetUserTasks(ctx, req)
}

func (s *Server) UpdateTask(ctx context.Context, req *pb.UpdateTaskRequest) (*pb.UpdateTaskResponse, error) {
	return s.taskHandler.UpdateTask(ctx, req)
}

func (s *Server) UpdateTaskOrder(ctx context.Context, req *pb.UpdateTaskOrderRequest) (*pb.UpdateTaskOrderResponse, error) {
	return s.taskHandler.UpdateTaskOrder(ctx, req)
}

func (s *Server) SearchUsers(ctx context.Context, req *pb.SearchUsersRequest) (*pb.SearchUsersResponse, error) {
	return s.friendHandler.SearchUsers(ctx, req)
}

func (s *Server) SendFriendRequest(ctx context.Context, req *pb.SendFriendRequestRequest) (*pb.SendFriendRequestResponse, error) {
	return s.friendHandler.SendFriendRequest(ctx, req)
}

func (s *Server) RespondToFriendRequest(ctx context.Context, req *pb.RespondToFriendRequestRequest) (*pb.RespondToFriendRequestResponse, error) {
	return s.friendHandler.RespondToFriendRequest(ctx, req)
}

func (s *Server) GetUserFriends(ctx context.Context, req *pb.GetUserFriendsRequest) (*pb.GetUserFriendsResponse, error) {
	return s.friendHandler.GetUserFriends(ctx, req)
}

func (s *Server) GetFriendRequests(ctx context.Context, req *pb.GetFriendRequestsRequest) (*pb.GetFriendRequestsResponse, error) {
	return s.friendHandler.GetFriendRequests(ctx, req)
}

func (s *Server) GetUserEvents(ctx context.Context, req *pb.GetUserEventsRequest) (*pb.GetUserEventsResponse, error) {
	return s.eventHandler.GetUserEvents(ctx, req)
}

func (s *Server) GetUserRelatedEvents(ctx context.Context, req *pb.GetUserRelatedEventsRequest) (*pb.GetUserRelatedEventsResponse, error) {
	return s.eventHandler.GetUserRelatedEvents(ctx, req)
}

func (s *Server) ConfirmTaskCompletion(ctx context.Context, req *pb.ConfirmTaskCompletionRequest) (*pb.ConfirmTaskCompletionResponse, error) {
	return s.taskHandler.ConfirmTaskCompletion(ctx, req)
}

func (s *Server) LikeEvent(ctx context.Context, req *pb.LikeEventRequest) (*pb.LikeEventResponse, error) {
	return s.eventHandler.LikeEvent(ctx, req)
}

func (s *Server) UnlikeEvent(ctx context.Context, req *pb.UnlikeEventRequest) (*pb.UnlikeEventResponse, error) {
	return s.eventHandler.UnlikeEvent(ctx, req)
}

func (s *Server) AddEventComment(ctx context.Context, req *pb.AddEventCommentRequest) (*pb.AddEventCommentResponse, error) {
	return s.eventHandler.AddEventComment(ctx, req)
}

func (s *Server) GetEventComments(ctx context.Context, req *pb.GetEventCommentsRequest) (*pb.GetEventCommentsResponse, error) {
	return s.eventHandler.GetEventComments(ctx, req)
}

func (s *Server) DeleteEventComment(ctx context.Context, req *pb.DeleteEventCommentRequest) (*pb.DeleteEventCommentResponse, error) {
	return s.eventHandler.DeleteEventComment(ctx, req)
}

func (s *Server) GetUsersBatch(ctx context.Context, req *pb.GetUsersBatchRequest) (*pb.GetUsersBatchResponse, error) {
	return s.userHandler.GetUsersBatch(ctx, req)
}

func (s *Server) GetUsersTasksBatch(ctx context.Context, req *pb.GetUsersTasksBatchRequest) (*pb.GetUsersTasksBatchResponse, error) {
	return s.taskHandler.GetUsersTasksBatch(ctx, req)
}

func (s *Server) EditUserProfile(ctx context.Context, req *pb.EditUserProfileRequest) (*pb.EditUserProfileResponse, error) {
	return s.userHandler.EditUserProfile(ctx, req)
}

func (s *Server) CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.CreateGroupResponse, error) {
	return s.userHandler.CreateGroup(ctx, req)
}

func (s *Server) GetGroupTasks(ctx context.Context, req *pb.GetGroupTasksRequest) (*pb.GetGroupTasksResponse, error) {
	return s.userHandler.GetGroupTasks(ctx, req)
}

func (s *Server) InviteToGroup(ctx context.Context, req *pb.InviteToGroupRequest) (*pb.InviteToGroupResponse, error) {
	return s.userHandler.InviteToGroup(ctx, req)
}

func (s *Server) AcceptGroupInvite(ctx context.Context, req *pb.AcceptGroupInviteRequest) (*pb.AcceptGroupInviteResponse, error) {
	return s.userHandler.AcceptGroupInvite(ctx, req)
}

func (s *Server) KickUserFromGroup(ctx context.Context, req *pb.KickUserFromGroupRequest) (*pb.KickUserFromGroupResponse, error) {
	return s.userHandler.KickUserFromGroup(ctx, req)
}

func (s *Server) GetUserStreak(ctx context.Context, req *pb.GetUserStreakRequest) (*pb.GetUserStreakResponse, error) {
	return s.userHandler.GetUserStreak(ctx, req)
}

func (s *Server) Start(port string) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(middleware.AuthInterceptor),
	)

	pb.RegisterBackendRequestsServer(grpcServer, s)

	log.Printf("Server started on :%s", port)
	return grpcServer.Serve(lis)
}
