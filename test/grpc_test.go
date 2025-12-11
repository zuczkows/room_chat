//go:build integration

package test

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zuczkows/room-chat/internal/utils"
	pb "github.com/zuczkows/room-chat/protobuf"
	"github.com/zuczkows/room-chat/test/internal/websocket"
)

func TestGrpc(t *testing.T) {
	select {
	case err := <-grpcErrCh:
		t.Fatalf("gRPC server failed to start: %v", err)
	default:
	}
	testUser1 := CreateTestUser1(t, users)

	grpcConn, err := grpc.NewClient(fmt.Sprintf("%s:%d", grpcAddr, grpcPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer grpcConn.Close()

	client := pb.NewRoomChatClient(grpcConn)

	wsUser1, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user")
	require.NoError(t, err)
	defer wsUser1.Close()

	accessTokenUser1 := utils.GetEncodedBase64Token(testUser1.Username, testUser1.Password)
	_, err = wsUser1.Login(accessTokenUser1)
	require.NoError(t, err)

	channelUser1 := "channel-user-1"
	_, err = wsUser1.Join(channelUser1)
	require.NoError(t, err)

	testMessageUser1 := "testMessageUser1"
	_, err = wsUser1.SendMessage(testMessageUser1, channelUser1)
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond) // ES time to index

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
		AssertGrpcError(t, err, "Username or nickname is already taken.", codes.AlreadyExists)
	})

	t.Run("missing required argument", func(t *testing.T) {
		req := &pb.RegisterProfileRequest{
			Username: "missing-required-argument",
			Nick:     "missing-required-argument",
		}
		_, err := client.RegisterProfile(context.Background(), req)
		AssertGrpcError(t, err, "Password cannot be empty.", codes.InvalidArgument)
	})

	t.Run("UpdateProfile without authorization", func(t *testing.T) {
		req := &pb.UpdateProfileRequest{
			Nick: "without auth",
		}
		_, err := client.UpdateProfile(context.Background(), req)
		AssertGrpcError(t, err, "Missing authorization header.", codes.Unauthenticated)
	})

	t.Run("UpdateProfile with invalid credentials", func(t *testing.T) {
		auth := "Basic " + basicAuth("user-2137", "2137")
		ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", auth)

		req := &pb.UpdateProfileRequest{
			Nick: "invalid credentials",
		}

		_, err := client.UpdateProfile(ctx, req)
		AssertGrpcError(t, err, "Invalid username or password.", codes.PermissionDenied)
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

	t.Run("user can get messages from channel he belongs to", func(t *testing.T) {
		auth := "Basic " + basicAuth(testUser1.Username, testUser1.Password)
		ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", auth)

		req := &pb.ListMessagesRequest{
			Channel: channelUser1,
		}
		response, err := client.ListMessages(ctx, req)
		require.NoError(t, err)

		require.Equal(t, testMessageUser1, response.Messages[0].Content)
	})

	t.Run("ListMessages without authorization", func(t *testing.T) {
		req := &pb.ListMessagesRequest{
			Channel: "without auth",
		}
		_, err := client.ListMessages(context.Background(), req)
		AssertGrpcError(t, err, "Missing authorization header.", codes.Unauthenticated)
	})

}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func AssertGrpcError(t *testing.T, err error, expectedMessage string, expectedCode codes.Code) {
	t.Helper()
	require.Error(t, err)
	e, ok := status.FromError(err)
	if ok {
		require.Equal(t, expectedMessage, e.Message())
		require.Equal(t, expectedCode, e.Code())
	} else {
		require.Failf(t, "error is not a gRPC error", "got: %v", err)
	}
}
