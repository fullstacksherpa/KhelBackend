package notifications

import (
	"context"

	"github.com/9ssi7/exponent"
)

type ExpoAdapter struct {
	client *exponent.Client
}

func NewExpoAdapter(c *exponent.Client) *ExpoAdapter {
	return &ExpoAdapter{client: c}
}

func (a *ExpoAdapter) Publish(ctx context.Context, msgs []*exponent.Message) ([]*exponent.MessageResponse, error) {
	return a.client.Publish(ctx, msgs)
}

func (a *ExpoAdapter) PublishSingle(ctx context.Context, msg *exponent.Message) ([]*exponent.MessageResponse, error) {
	return a.client.PublishSingle(ctx, msg)
}
