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
