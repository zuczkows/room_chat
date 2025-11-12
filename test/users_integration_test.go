//go:build integration

package test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zuczkows/room-chat/internal/handlers"
	"github.com/zuczkows/room-chat/internal/user"
)

func TestUserHandlerPositive(t *testing.T) {
	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := handlers.NewUserHandler(userService, logger)

	t.Run("successful registration", func(t *testing.T) {
		createUserRequest := user.CreateUserRequest{
			Username: "test-user-1",
			Nick:     "test-nick-1",
			Password: "password2137!",
		}
		body, err := json.Marshal(createUserRequest)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleRegister(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestUserHandlerNegative(t *testing.T) {
	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := handlers.NewUserHandler(userService, logger)
	testUser1 := CreateTestUser1(t, userService)

	tests := []struct {
		name              string
		createUserRequest any
		expectedStatus    int
		expectedError     string
	}{
		{
			name: "duplicate username",
			createUserRequest: user.CreateUserRequest{
				Username: testUser1.Username,
				Nick:     "test-nick-1",
				Password: "password2137!",
			},
			expectedStatus: http.StatusConflict,
			expectedError:  handlers.ErrUsernameNickTaken.Error(),
		},
		{
			name: "duplicate Nick",
			createUserRequest: user.CreateUserRequest{
				Username: "newuser",
				Nick:     testUser1.Nick,
				Password: "password123",
			},
			expectedStatus: http.StatusConflict,
			expectedError:  handlers.ErrUsernameNickTaken.Error(),
		},
		{
			name: "missing required fields",
			createUserRequest: user.CreateUserRequest{
				Username: "",
				Nick:     "testnick",
				Password: "password123",
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedError:  handlers.ErrUserNameEmpty.Error(),
		},
		{
			name:              "invalid JSON",
			createUserRequest: "invalid json",
			expectedStatus:    http.StatusBadRequest,
			expectedError:     "Invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.createUserRequest)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleRegister(w, req)

			require.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedError)
		})
	}
}
