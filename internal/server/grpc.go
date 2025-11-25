package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/zuczkows/room-chat/internal/chat"
	apperrors "github.com/zuczkows/room-chat/internal/errors"
	"github.com/zuczkows/room-chat/internal/storage"
	"github.com/zuczkows/room-chat/internal/user"
	"github.com/zuczkows/room-chat/internal/utils"
	pb "github.com/zuczkows/room-chat/protobuf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type GrpcConfig struct {
	Host string
	Port int
}

type contextKey string

const (
	UserIDKey   contextKey = "userID"
	UsernameKey contextKey = "username"
)

type GrpcServer struct {
	pb.UnimplementedRoomChatServer

	server         *grpc.Server
	userService    *user.Service
	logger         *slog.Logger
	storage        *storage.MessageIndexer
	channelManager *chat.ChannelManager
}

func NewGrpcServer(userService *user.Service, logger *slog.Logger, storage *storage.MessageIndexer, channelManager *chat.ChannelManager) *GrpcServer {
	gs := &GrpcServer{
		userService:    userService,
		logger:         logger,
		storage:        storage,
		channelManager: channelManager,
	}
	gs.server = grpc.NewServer(
		grpc.UnaryInterceptor(gs.AuthInterceptor),
	)
	pb.RegisterRoomChatServer(gs.server, gs)
	return gs
}

func (gs *GrpcServer) Start(cfg GrpcConfig) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	gs.logger.Info("Starting gRPC server", slog.String("address", addr))
	if err := gs.server.Serve(lis); err != nil {
		gs.logger.Error("failed to serve gRPC", slog.Any("error", err))
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	return nil
}

func (s *GrpcServer) AuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// NOTE(zuczkows): Move to some helper with map for protected and unprotected methods when there will be more than 2 RPC methosd
	if info.FullMethod == "/room_chat.RoomChat/RegisterProfile" {
		return handler(ctx, req)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, apperrors.MissingMetadata)
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Errorf(codes.Unauthenticated, apperrors.MissingAuthorization)
	}
	authHeader := authHeaders[0]
	username, password, ok := utils.ParseBasicAuth(authHeader)
	if !ok {
		return nil, status.Errorf(codes.PermissionDenied, apperrors.MissingOrInvalidCredentials)
	}
	profile, err := s.userService.Login(ctx, username, password)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserNotFound):
			s.logger.Info("login attempt with wrong username", slog.String("username", username))
			return nil, status.Errorf(codes.PermissionDenied, apperrors.InvalidUsernameOrPassword)
		case errors.Is(err, user.ErrInvalidPassword):
			s.logger.Info("login attempt with wrong password", slog.String("username", username))
			return nil, status.Errorf(codes.PermissionDenied, apperrors.InvalidUsernameOrPassword)
		default:
			s.logger.Error("login internal service error", slog.String("username", username), slog.Any("error", err))
			return nil, status.Errorf(codes.Internal, apperrors.InternalServer)
		}
	}
	ctx = context.WithValue(ctx, UserIDKey, profile.ID)
	newCtx := context.WithValue(ctx, UsernameKey, username)
	return handler(newCtx, req)
}

func getUserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(UserIDKey).(int64)
	return userID, ok
}

func GetUsernameFromContext(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(UsernameKey).(string)
	return username, ok
}

func (s *GrpcServer) RegisterProfile(ctx context.Context, in *pb.RegisterProfileRequest) (*pb.RegisterProfileResponse, error) {
	if in.Username == "" {
		return nil, status.Errorf(codes.InvalidArgument, apperrors.UserNameEmpty)
	}
	if in.Password == "" {
		return nil, status.Errorf(codes.InvalidArgument, apperrors.PasswordEmpty)
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
			return nil, status.Errorf(codes.AlreadyExists, apperrors.UsernameNickTaken)
		case errors.Is(err, user.ErrMissingRequiredFields):
			return nil, status.Errorf(codes.InvalidArgument, apperrors.MissingRequiredFields)
		default:
			s.logger.Error("Registration failed", slog.Any("error", err))
			return nil, status.Errorf(codes.Internal, apperrors.InternalServer)
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
		return nil, status.Errorf(codes.Unauthenticated, apperrors.AuthenticationRequired)
	}
	req := user.UpdateUserRequest{
		Nick: in.Nick,
	}

	_, err := s.userService.UpdateProfile(ctx, userID, req)
	if err != nil {
		s.logger.Error("Profile update failed", slog.Any("error", err))
		switch {
		case errors.Is(err, user.ErrNickAlreadyExists):
			return nil, status.Errorf(codes.AlreadyExists, apperrors.NickAlreadyExists)
		default:
			return nil, status.Errorf(codes.Internal, apperrors.InternalServer)
		}
	}

	s.logger.Debug("Profile updated successfully via gRPC", slog.Int64("userID", userID))
	return &pb.Empty{}, nil
}

func (s *GrpcServer) ListMessages(ctx context.Context, in *pb.ListMessagesRequest) (*pb.ListMessagesResponse, error) {
	authenticatedUsername, ok := GetUsernameFromContext(ctx)
	if !ok {
		s.logger.Error("No authenticated user in context")
		return nil, status.Errorf(codes.Unauthenticated, apperrors.AuthenticationRequired)
	}
	channel := in.Channel

	isUserAMember := s.channelManager.IsUserAMember(channel, authenticatedUsername)
	if !isUserAMember {
		return nil, status.Errorf(codes.InvalidArgument, apperrors.NotMemberOfChannel)
	}
	msgs, err := s.storage.ListDocuments(channel)
	if err != nil {
		return nil, status.Errorf(codes.Internal, apperrors.InternalServer)
	}

	protoMessages := make([]*pb.Message, 0, len(msgs))
	for _, m := range msgs {
		protoMessages = append(protoMessages, &pb.Message{
			Id:        m.ID,
			ChannelId: m.ChannelID,
			AuthorId:  m.AuthorID,
			Content:   m.Content,
			CreatedAt: timestamppb.New(m.CreatedAt),
		})
	}

	return &pb.ListMessagesResponse{
		Messages: protoMessages,
	}, nil

}
