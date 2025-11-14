//go:build integration

package test

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zuczkows/room-chat/internal/server"
	"github.com/zuczkows/room-chat/internal/user"
	pb "github.com/zuczkows/room-chat/protobuf"
)

const (
	grpcAddr = "0.0.0.0"
	grpcPort = "50051"
)

func TestGrpc(t *testing.T) {
	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	testUser1 := CreateTestUser1(t, userService)

	grpcServer := server.NewGrpcServer(userService, logger)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	require.NoError(t, err)
	logger.Info("Starting gRPC server", slog.String("address", ":50051"))
	go func() {
		err := grpcServer.Serve(lis)
		require.NoError(t, err)
	}()
	time.Sleep(100 * time.Millisecond)

	grpcConn, err := grpc.NewClient(fmt.Sprintf("%s:%s", grpcAddr, grpcPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer grpcConn.Close()

	client := pb.NewRoomChatClient(grpcConn)

	t.Run("successful registration", func(t *testing.T) {
		req := &pb.RegisterProfileRequest{
			Username: "test-grpc-1",
			Password: "password-grpc",
			Nick:     "test-grpc",
		}
		resp, err := client.RegisterProfile(context.Background(), req)
		require.NoError(t, err)
		require.Greater(t, resp.Id, int64(0))
	})

	t.Run("username already exists", func(t *testing.T) {
		req := &pb.RegisterProfileRequest{
			Username: testUser1.Username,
			Password: "password-grpc",
			Nick:     "test-grpc",
		}
		_, err := client.RegisterProfile(context.Background(), req)
		require.Error(t, err)
		require.ErrorContains(t, err, server.ErrUsernameNickTaken)
		require.Equal(t, codes.AlreadyExists, status.Code(err))
	})

	t.Run("missing required argument", func(t *testing.T) {
		req := &pb.RegisterProfileRequest{
			Username: "missing-required-argument",
			Nick:     "missing-required-argument",
		}
		_, err := client.RegisterProfile(context.Background(), req)
		require.Error(t, err)
		require.ErrorContains(t, err, server.ErrUPasswordEmpty)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("UpdateProfile without authorization", func(t *testing.T) {
		req := &pb.UpdateProfileRequest{
			Nick: "without auth",
		}
		_, err := client.UpdateProfile(context.Background(), req)
		require.Error(t, err)
		require.ErrorContains(t, err, server.ErrMissingAuthorization)
		require.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("UpdateProfile with invalid credentials", func(t *testing.T) {
		auth := "Basic " + basicAuth("user-2137", "2137")
		ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", auth)

		req := &pb.UpdateProfileRequest{
			Nick: "invalid credentials",
		}

		_, err := client.UpdateProfile(ctx, req)
		require.Error(t, err)
		require.ErrorContains(t, err, server.ErrInvalidUsernameOrPassword)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})
	t.Run("UpdateProfile with valid credentials", func(t *testing.T) {
		auth := "Basic " + basicAuth(testUser1.Username, testUser1.Password)
		ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", auth)

		req := &pb.UpdateProfileRequest{
			Nick: "New Nick",
		}

		_, err := client.UpdateProfile(ctx, req)
		require.NoError(t, err)
	})
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}
