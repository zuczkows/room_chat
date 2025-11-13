package server

import (
	"context"
	"errors"
	"log/slog"

	"github.com/zuczkows/room-chat/internal/user"
	"github.com/zuczkows/room-chat/internal/utils"
	pb "github.com/zuczkows/room-chat/protobuf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextKey string

const UserContextKey contextKey = "user"

const (
	ErrUsernameNickTaken         = "Username or nickname is already taken."
	ErrInternalServer            = "Something went wrong on our side."
	ErrMissingRequiredFields     = "Some required fields are missing."
	ErrUserNameEmpty             = "Username can not be empty."
	ErrInvalidUsernameOrPassword = "Invalid username or password."
	ErrNickAlreadyExists         = "Nick already exists."
)

type GrpcServer struct {
	pb.RoomChatServer
	userService *user.Service
	logger      *slog.Logger
}

func NewGrpcServer(userService *user.Service, logger *slog.Logger) *grpc.Server {
	grpcServer := &GrpcServer{userService: userService, logger: logger}
	server := grpc.NewServer(grpc.UnaryInterceptor(grpcServer.AuthInterceptor))
	pb.RegisterRoomChatServer(server, grpcServer)

	return server
}
func (s *GrpcServer) AuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// NOTE(zuczkows): Move to some helper with map for protected and unprotected methods when there will be more than 2 RPC methosd
	if info.FullMethod == "/room_chat.RoomChat/RegisterProfile" {
		return handler(ctx, req)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing metadata")
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization header")
	}
	authHeader := authHeaders[0]
	username, password, ok := utils.ParseBasicAuth(authHeader)
	if !ok {
		return nil, status.Errorf(codes.PermissionDenied, "Missing or invalid credentials.")
	}
	profile, err := s.userService.Login(ctx, username, password)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserNotFound):
			s.logger.Info("login attempt with wrong username", slog.String("username", username))
			return nil, status.Errorf(codes.PermissionDenied, ErrInvalidUsernameOrPassword)
		case errors.Is(err, user.ErrInvalidPassword):
			s.logger.Info("login attempt with wrong password", slog.String("username", username))
			return nil, status.Errorf(codes.PermissionDenied, ErrInvalidUsernameOrPassword)
		default:
			s.logger.Error("login internal service error", slog.String("username", username), slog.Any("error", err))
			return nil, status.Errorf(codes.Internal, ErrInternalServer)
		}
	}
	newCtx := context.WithValue(ctx, UserContextKey, profile.ID)
	return handler(newCtx, req)
}

func getUserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(UserContextKey).(int64)
	return userID, ok
}

func (s *GrpcServer) RegisterProfile(ctx context.Context, in *pb.RegisterProfileRequest) (*pb.RegisterProfileResponse, error) {
	if in.Username == "" {
		return nil, status.Errorf(codes.InvalidArgument, ErrUserNameEmpty)
	}

	req := user.CreateUserRequest{
		Username: in.Username,
		Password: in.Password,
		Nick:     in.Nick,
	}
	userID, err := s.userService.Register(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserOrNickAlreadyExists):
			return nil, status.Errorf(codes.AlreadyExists, ErrUsernameNickTaken)
		case errors.Is(err, user.ErrMissingRequiredFields):
			return nil, status.Errorf(codes.InvalidArgument, ErrMissingRequiredFields)
		default:
			s.logger.Error("Registration failed", slog.Any("error", err))
			return nil, status.Errorf(codes.Internal, ErrInternalServer)
		}
	}
	return &pb.RegisterProfileResponse{
		Id: userID,
	}, nil
}

func (s *GrpcServer) UpdateProfile(ctx context.Context, in *pb.UpdateProfileRequest) (*pb.Empty, error) {
	userID, ok := getUserIDFromContext(ctx)
	if !ok {
		s.logger.Error("No authenticated user in context")
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}
	req := user.UpdateUserRequest{
		Nick: in.Nick,
	}

	_, err := s.userService.UpdateProfile(ctx, userID, req)
	if err != nil {
		s.logger.Error("Profile update failed", slog.Any("error", err))
		switch {
		case errors.Is(err, user.ErrNickAlreadyExists):
			return nil, status.Errorf(codes.AlreadyExists, ErrNickAlreadyExists)
		default:
			return nil, status.Errorf(codes.Internal, ErrInternalServer)
		}
	}

	s.logger.Info("Profile updated successfully via gRPC", slog.Int64("userID", userID))
	return &pb.Empty{}, nil
}
