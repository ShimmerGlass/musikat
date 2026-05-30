package notification

import (
	"context"

	"github.com/shimmerglass/musikat/database"
)

var _ Notifier = (*Noop)(nil)

type Noop struct{}

func (n *Noop) Send(ctx context.Context, to database.User, message string) error {
	return nil
}
