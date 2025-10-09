package user

import "time"

type Profile struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Nick      string    `json:"nick"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// no email verification for this app so no email in struct
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nick     string `json:"nick"`
}

// only nick can be updated at this point
type UpdateUserRequest struct {
	Nick string `json:"nick"`
}
