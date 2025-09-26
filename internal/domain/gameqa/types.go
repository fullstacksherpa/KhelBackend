package gameqa

import (
	"errors"
	"time"
)

var (
	ErrQuestionNotFound = errors.New("question not found")
	ErrNotFound         = errors.New("resource not found")
	ErrReplyNotFound    = errors.New("reply not found")
)

type Question struct {
	ID        int64     `json:"id"`
	GameID    int64     `json:"game_id"`
	UserID    int64     `json:"user_id"`
	Question  string    `json:"question"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Reply struct {
	ID         int64     `json:"id"`
	QuestionID int64     `json:"question_id"`
	AdminID    int64     `json:"admin_id"`
	Reply      string    `json:"reply"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type QuestionWithReplies struct {
	ID        int64     `json:"id"`
	Question  string    `json:"question"`
	CreatedAt time.Time `json:"created_at"`
	Replies   []Reply   `json:"replies,omitempty"`
}
