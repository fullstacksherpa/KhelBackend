package notifications

import (
	"context"

	"github.com/9ssi7/exponent"
)

// PushSender is just an abstraction over any push sender,
// but here it's directly tied to the exponent SDK types.
type PushSender interface {
	Publish(ctx context.Context, msgs []*exponent.Message) ([]*exponent.MessageResponse, error)
	PublishSingle(ctx context.Context, msg *exponent.Message) ([]*exponent.MessageResponse, error)
}
