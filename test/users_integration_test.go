//go:build integration

package test

import (
	"bytes"
	"encoding/json"
	"log"
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

func TestUserHandler(t *testing.T) {

	db, cleanup, err := SetupDB()
	if err != nil {
		log.Printf("failed to setup database: %s", err)
	}
	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := handlers.NewUserHandler(userService, logger)
	defer cleanup()

	tests := []struct {
		name              string
		createUserRequest any
		expectedStatus    int
		expectedError     string
	}{
		{
			name: "successful registration",
			createUserRequest: user.CreateUserRequest{
				Username: "test-user-1",
				Nick:     "test-nick-1",
				Password: "password2137!",
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "duplicate username",
			createUserRequest: user.CreateUserRequest{
				Username: "test-user-1",
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
				Nick:     "test-nick-1",
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

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == http.StatusCreated {
				var resp handlers.RegisterResponse
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Greater(t, resp.ID, int64(0))
			}

			if tt.expectedError != "" {
				assert.Contains(t, w.Body.String(), tt.expectedError)
			}
		})
	}
}
