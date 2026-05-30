package notification

import (
	"context"

	"github.com/shimmerglass/musikat/database"
)

type Notifier interface {
	Send(ctx context.Context, to database.User, message string) error
}
