package followers

import "time"

var (
	QueryTimeoutDuration = time.Second * 5
)

type Follower struct {
	UserID     int64  `json:"user_id"`
	FollowerID int64  `json:"follower_id"`
	CreatedAt  string `json:"created_at"`
}
