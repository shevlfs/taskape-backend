// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.29.3
// source: proto/backendrequests.proto

package taskape_proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	BackendRequests_LoginNewUser_FullMethodName            = "/taskapebackend.BackendRequests/loginNewUser"
	BackendRequests_ValidateToken_FullMethodName           = "/taskapebackend.BackendRequests/validateToken"
	BackendRequests_RefreshToken_FullMethodName            = "/taskapebackend.BackendRequests/refreshToken"
	BackendRequests_VerifyUserToken_FullMethodName         = "/taskapebackend.BackendRequests/verifyUserToken"
	BackendRequests_RegisterNewProfile_FullMethodName      = "/taskapebackend.BackendRequests/registerNewProfile"
	BackendRequests_CreateTask_FullMethodName              = "/taskapebackend.BackendRequests/CreateTask"
	BackendRequests_CreateTasksBatch_FullMethodName        = "/taskapebackend.BackendRequests/CreateTasksBatch"
	BackendRequests_GetUserTasks_FullMethodName            = "/taskapebackend.BackendRequests/GetUserTasks"
	BackendRequests_CheckHandleAvailability_FullMethodName = "/taskapebackend.BackendRequests/CheckHandleAvailability"
)

// BackendRequestsClient is the client API for BackendRequests service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type BackendRequestsClient interface {
	LoginNewUser(ctx context.Context, in *NewUserLoginRequest, opts ...grpc.CallOption) (*NewUserLoginResponse, error)
	ValidateToken(ctx context.Context, in *ValidateTokenRequest, opts ...grpc.CallOption) (*ValidateTokenResponse, error)
	RefreshToken(ctx context.Context, in *RefreshTokenRequest, opts ...grpc.CallOption) (*RefreshTokenResponse, error)
	VerifyUserToken(ctx context.Context, in *VerifyUserRequest, opts ...grpc.CallOption) (*VerifyUserResponse, error)
	RegisterNewProfile(ctx context.Context, in *RegisterNewProfileRequest, opts ...grpc.CallOption) (*RegisterNewProfileResponse, error)
	CreateTask(ctx context.Context, in *CreateTaskRequest, opts ...grpc.CallOption) (*CreateTaskResponse, error)
	CreateTasksBatch(ctx context.Context, in *CreateTasksBatchRequest, opts ...grpc.CallOption) (*CreateTasksBatchResponse, error)
	GetUserTasks(ctx context.Context, in *GetUserTasksRequest, opts ...grpc.CallOption) (*GetUserTasksResponse, error)
	CheckHandleAvailability(ctx context.Context, in *CheckHandleRequest, opts ...grpc.CallOption) (*CheckHandleResponse, error)
}

type backendRequestsClient struct {
	cc grpc.ClientConnInterface
}

func NewBackendRequestsClient(cc grpc.ClientConnInterface) BackendRequestsClient {
	return &backendRequestsClient{cc}
}

func (c *backendRequestsClient) LoginNewUser(ctx context.Context, in *NewUserLoginRequest, opts ...grpc.CallOption) (*NewUserLoginResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(NewUserLoginResponse)
	err := c.cc.Invoke(ctx, BackendRequests_LoginNewUser_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *backendRequestsClient) ValidateToken(ctx context.Context, in *ValidateTokenRequest, opts ...grpc.CallOption) (*ValidateTokenResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ValidateTokenResponse)
	err := c.cc.Invoke(ctx, BackendRequests_ValidateToken_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *backendRequestsClient) RefreshToken(ctx context.Context, in *RefreshTokenRequest, opts ...grpc.CallOption) (*RefreshTokenResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(RefreshTokenResponse)
	err := c.cc.Invoke(ctx, BackendRequests_RefreshToken_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *backendRequestsClient) VerifyUserToken(ctx context.Context, in *VerifyUserRequest, opts ...grpc.CallOption) (*VerifyUserResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(VerifyUserResponse)
	err := c.cc.Invoke(ctx, BackendRequests_VerifyUserToken_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *backendRequestsClient) RegisterNewProfile(ctx context.Context, in *RegisterNewProfileRequest, opts ...grpc.CallOption) (*RegisterNewProfileResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(RegisterNewProfileResponse)
	err := c.cc.Invoke(ctx, BackendRequests_RegisterNewProfile_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *backendRequestsClient) CreateTask(ctx context.Context, in *CreateTaskRequest, opts ...grpc.CallOption) (*CreateTaskResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CreateTaskResponse)
	err := c.cc.Invoke(ctx, BackendRequests_CreateTask_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *backendRequestsClient) CreateTasksBatch(ctx context.Context, in *CreateTasksBatchRequest, opts ...grpc.CallOption) (*CreateTasksBatchResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CreateTasksBatchResponse)
	err := c.cc.Invoke(ctx, BackendRequests_CreateTasksBatch_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *backendRequestsClient) GetUserTasks(ctx context.Context, in *GetUserTasksRequest, opts ...grpc.CallOption) (*GetUserTasksResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetUserTasksResponse)
	err := c.cc.Invoke(ctx, BackendRequests_GetUserTasks_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *backendRequestsClient) CheckHandleAvailability(ctx context.Context, in *CheckHandleRequest, opts ...grpc.CallOption) (*CheckHandleResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CheckHandleResponse)
	err := c.cc.Invoke(ctx, BackendRequests_CheckHandleAvailability_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BackendRequestsServer is the server API for BackendRequests service.
// All implementations must embed UnimplementedBackendRequestsServer
// for forward compatibility.
type BackendRequestsServer interface {
	LoginNewUser(context.Context, *NewUserLoginRequest) (*NewUserLoginResponse, error)
	ValidateToken(context.Context, *ValidateTokenRequest) (*ValidateTokenResponse, error)
	RefreshToken(context.Context, *RefreshTokenRequest) (*RefreshTokenResponse, error)
	VerifyUserToken(context.Context, *VerifyUserRequest) (*VerifyUserResponse, error)
	RegisterNewProfile(context.Context, *RegisterNewProfileRequest) (*RegisterNewProfileResponse, error)
	CreateTask(context.Context, *CreateTaskRequest) (*CreateTaskResponse, error)
	CreateTasksBatch(context.Context, *CreateTasksBatchRequest) (*CreateTasksBatchResponse, error)
	GetUserTasks(context.Context, *GetUserTasksRequest) (*GetUserTasksResponse, error)
	CheckHandleAvailability(context.Context, *CheckHandleRequest) (*CheckHandleResponse, error)
	mustEmbedUnimplementedBackendRequestsServer()
}

// UnimplementedBackendRequestsServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedBackendRequestsServer struct{}

func (UnimplementedBackendRequestsServer) LoginNewUser(context.Context, *NewUserLoginRequest) (*NewUserLoginResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method LoginNewUser not implemented")
}
func (UnimplementedBackendRequestsServer) ValidateToken(context.Context, *ValidateTokenRequest) (*ValidateTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ValidateToken not implemented")
}
func (UnimplementedBackendRequestsServer) RefreshToken(context.Context, *RefreshTokenRequest) (*RefreshTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RefreshToken not implemented")
}
func (UnimplementedBackendRequestsServer) VerifyUserToken(context.Context, *VerifyUserRequest) (*VerifyUserResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method VerifyUserToken not implemented")
}
func (UnimplementedBackendRequestsServer) RegisterNewProfile(context.Context, *RegisterNewProfileRequest) (*RegisterNewProfileResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RegisterNewProfile not implemented")
}
func (UnimplementedBackendRequestsServer) CreateTask(context.Context, *CreateTaskRequest) (*CreateTaskResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateTask not implemented")
}
func (UnimplementedBackendRequestsServer) CreateTasksBatch(context.Context, *CreateTasksBatchRequest) (*CreateTasksBatchResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateTasksBatch not implemented")
}
func (UnimplementedBackendRequestsServer) GetUserTasks(context.Context, *GetUserTasksRequest) (*GetUserTasksResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetUserTasks not implemented")
}
func (UnimplementedBackendRequestsServer) CheckHandleAvailability(context.Context, *CheckHandleRequest) (*CheckHandleResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CheckHandleAvailability not implemented")
}
func (UnimplementedBackendRequestsServer) mustEmbedUnimplementedBackendRequestsServer() {}
func (UnimplementedBackendRequestsServer) testEmbeddedByValue()                         {}

// UnsafeBackendRequestsServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to BackendRequestsServer will
// result in compilation errors.
type UnsafeBackendRequestsServer interface {
	mustEmbedUnimplementedBackendRequestsServer()
}

func RegisterBackendRequestsServer(s grpc.ServiceRegistrar, srv BackendRequestsServer) {
	// If the following call pancis, it indicates UnimplementedBackendRequestsServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&BackendRequests_ServiceDesc, srv)
}

func _BackendRequests_LoginNewUser_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(NewUserLoginRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BackendRequestsServer).LoginNewUser(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: BackendRequests_LoginNewUser_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BackendRequestsServer).LoginNewUser(ctx, req.(*NewUserLoginRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BackendRequests_ValidateToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ValidateTokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BackendRequestsServer).ValidateToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: BackendRequests_ValidateToken_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BackendRequestsServer).ValidateToken(ctx, req.(*ValidateTokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BackendRequests_RefreshToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RefreshTokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BackendRequestsServer).RefreshToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: BackendRequests_RefreshToken_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BackendRequestsServer).RefreshToken(ctx, req.(*RefreshTokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BackendRequests_VerifyUserToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(VerifyUserRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BackendRequestsServer).VerifyUserToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: BackendRequests_VerifyUserToken_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BackendRequestsServer).VerifyUserToken(ctx, req.(*VerifyUserRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BackendRequests_RegisterNewProfile_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RegisterNewProfileRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BackendRequestsServer).RegisterNewProfile(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: BackendRequests_RegisterNewProfile_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BackendRequestsServer).RegisterNewProfile(ctx, req.(*RegisterNewProfileRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BackendRequests_CreateTask_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateTaskRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BackendRequestsServer).CreateTask(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: BackendRequests_CreateTask_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BackendRequestsServer).CreateTask(ctx, req.(*CreateTaskRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BackendRequests_CreateTasksBatch_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateTasksBatchRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BackendRequestsServer).CreateTasksBatch(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: BackendRequests_CreateTasksBatch_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BackendRequestsServer).CreateTasksBatch(ctx, req.(*CreateTasksBatchRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BackendRequests_GetUserTasks_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetUserTasksRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BackendRequestsServer).GetUserTasks(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: BackendRequests_GetUserTasks_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BackendRequestsServer).GetUserTasks(ctx, req.(*GetUserTasksRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BackendRequests_CheckHandleAvailability_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CheckHandleRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BackendRequestsServer).CheckHandleAvailability(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: BackendRequests_CheckHandleAvailability_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BackendRequestsServer).CheckHandleAvailability(ctx, req.(*CheckHandleRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// BackendRequests_ServiceDesc is the grpc.ServiceDesc for BackendRequests service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var BackendRequests_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "taskapebackend.BackendRequests",
	HandlerType: (*BackendRequestsServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "loginNewUser",
			Handler:    _BackendRequests_LoginNewUser_Handler,
		},
		{
			MethodName: "validateToken",
			Handler:    _BackendRequests_ValidateToken_Handler,
		},
		{
			MethodName: "refreshToken",
			Handler:    _BackendRequests_RefreshToken_Handler,
		},
		{
			MethodName: "verifyUserToken",
			Handler:    _BackendRequests_VerifyUserToken_Handler,
		},
		{
			MethodName: "registerNewProfile",
			Handler:    _BackendRequests_RegisterNewProfile_Handler,
		},
		{
			MethodName: "CreateTask",
			Handler:    _BackendRequests_CreateTask_Handler,
		},
		{
			MethodName: "CreateTasksBatch",
			Handler:    _BackendRequests_CreateTasksBatch_Handler,
		},
		{
			MethodName: "GetUserTasks",
			Handler:    _BackendRequests_GetUserTasks_Handler,
		},
		{
			MethodName: "CheckHandleAvailability",
			Handler:    _BackendRequests_CheckHandleAvailability_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/backendrequests.proto",
}
