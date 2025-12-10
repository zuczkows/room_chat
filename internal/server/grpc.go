package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/zuczkows/room-chat/internal/channels"
	"github.com/zuczkows/room-chat/internal/elastic"
	"github.com/zuczkows/room-chat/internal/protocol"
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

type GrpcServer struct {
	pb.UnimplementedRoomChatServer

	server   *grpc.Server
	users    *user.Users
	logger   *slog.Logger
	elastic  *elastic.MessageIndexer
	channels *channels.Channels
}

func NewGrpcServer(users *user.Users, logger *slog.Logger, elastic *elastic.MessageIndexer, channels *channels.Channels) *GrpcServer {
	gs := &GrpcServer{
		users:    users,
		logger:   logger,
		elastic:  elastic,
		channels: channels,
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
		return nil, status.Error(codes.Unauthenticated, string(protocol.MissingMetadata))
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, string(protocol.MissingAuthorization))
	}
	authHeader := authHeaders[0]
	username, password, ok := utils.ParseBasicAuth(authHeader)
	if !ok {
		return nil, status.Error(codes.PermissionDenied, string(protocol.MissingOrInvalidCredentials))
	}
	profile, err := s.users.Login(ctx, username, password)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserNotFound):
			s.logger.Info("login attempt with wrong username", slog.String("username", username))
			return nil, status.Error(codes.PermissionDenied, string(protocol.InvalidUsernameOrPassword))
		case errors.Is(err, user.ErrInvalidPassword):
			s.logger.Info("login attempt with wrong password", slog.String("username", username))
			return nil, status.Error(codes.PermissionDenied, string(protocol.InvalidUsernameOrPassword))
		default:
			s.logger.Error("login internal service error", slog.String("username", username), slog.Any("error", err))
			return nil, status.Error(codes.Internal, string(protocol.InternalServer))
		}
	}
	ctx = context.WithValue(ctx, userIDKey, profile.ID)
	newCtx := context.WithValue(ctx, usernameKey, username)
	return handler(newCtx, req)
}

func (s *GrpcServer) RegisterProfile(ctx context.Context, in *pb.RegisterProfileRequest) (*pb.RegisterProfileResponse, error) {
	if in.Username == "" {
		return nil, status.Error(codes.InvalidArgument, string(protocol.UserNameEmpty))
	}
	if in.Password == "" {
		return nil, status.Error(codes.InvalidArgument, string(protocol.PasswordEmpty))
	}

	req := protocol.CreateUserRequest{
		Username: in.Username,
		Password: in.Password,
		Nick:     in.Nick,
	}
	userID, err := s.users.Register(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserOrNickAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, string(protocol.UsernameNickTaken))
		case errors.Is(err, user.ErrMissingRequiredFields):
			return nil, status.Error(codes.InvalidArgument, string(protocol.MissingRequiredFields))
		default:
			s.logger.Error("Registration failed", slog.Any("error", err))
			return nil, status.Error(codes.Internal, string(protocol.InternalServer))
		}
	}
	return &pb.RegisterProfileResponse{
		Id: userID,
	}, nil
}

func (s *GrpcServer) UpdateProfile(ctx context.Context, in *pb.UpdateProfileRequest) (*pb.Empty, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		s.logger.Error("No authenticated user in context")
		return nil, status.Error(codes.Internal, string(protocol.InternalServer))
	}
	req := protocol.UpdateUserRequest{
		Nick: in.Nick,
	}

	_, err = s.users.UpdateProfile(ctx, userID, req)
	if err != nil {
		s.logger.Error("Profile update failed", slog.Any("error", err))
		switch {
		case errors.Is(err, user.ErrNickAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, string(protocol.NickAlreadyExists))
		default:
			return nil, status.Error(codes.Internal, string(protocol.InternalServer))
		}
	}

	s.logger.Debug("Profile updated successfully via gRPC", slog.Int64("userID", userID))
	return &pb.Empty{}, nil
}

func (s *GrpcServer) ListMessages(ctx context.Context, in *pb.ListMessagesRequest) (*pb.ListMessagesResponse, error) {
	authenticatedUsername, err := GetUsernameFromContext(ctx)
	if err != nil {
		s.logger.Error("No authenticated user in context")
		return nil, status.Error(codes.Internal, string(protocol.InternalServer))
	}

	isUserAMember, err := s.channels.IsUserAMember(in.Channel, authenticatedUsername)
	if err != nil {
		switch {
		case errors.Is(err, channels.ErrChannelDoesNotExist):
			// do not inform about not existing channel - information disclosure
			return nil, status.Error(codes.InvalidArgument, string(protocol.NotMemberOfChannel))
		default:
			return nil, status.Error(codes.Internal, string(protocol.InternalServer))
		}
	}
	if !isUserAMember {
		return nil, status.Error(codes.InvalidArgument, string(protocol.NotMemberOfChannel))
	}
	msgs, err := s.elastic.ListDocuments(in.Channel)
	if err != nil {
		s.logger.Error("Fetching document from ES failed", slog.Any("error", err))
		return nil, status.Error(codes.Internal, string(protocol.InternalServer))
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
