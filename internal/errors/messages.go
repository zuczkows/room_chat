package apperrors

import (
	"encoding/json"
	"net/http"
)

const (
	UsernameNickTaken           = "Username or nickname is already taken."
	InternalServer              = "Something went wrong on our side."
	MissingRequiredFields       = "Some required fields are missing."
	UserNameEmpty               = "Username can not be empty."
	PasswordEmpty               = "Password can not be empty."
	InvalidUsernameOrPassword   = "Invalid username or password."
	NickAlreadyExists           = "Nick already exists."
	AuthenticationRequired      = "Authentication required."
	MissingOrInvalidCredentials = "Missing or invalid credentials."
	MissingMetadata             = "Missing metadata."
	MissingAuthorization        = "Missing authorization header."
	NotMemberOfChannel          = "You are not a member of this channel."
	InvalidJSON                 = "Invalid JSON."
	AuthorizationRequired       = "Authorization required."
	InvalidCredentials          = "Invalid credentials."
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func SendError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	json.NewEncoder(w).Encode(ErrorResponse{
		Error: msg,
	})
}
