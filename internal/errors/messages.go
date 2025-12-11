package apperrors

import (
	"encoding/json"
	"net/http"

	"github.com/zuczkows/room-chat/internal/protocol"
)

type ErrorResponse struct {
	Error protocol.UserErrMessage `json:"error"`
}

func SendError(w http.ResponseWriter, status int, msg protocol.UserErrMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	json.NewEncoder(w).Encode(ErrorResponse{
		Error: msg,
	})
}
