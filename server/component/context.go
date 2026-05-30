package component

import (
	"context"

	"github.com/shimmerglass/musikat/database"
)

type ctxKey string

const (
	ctxKeyUser ctxKey = "user"
	ctxKeyURL  ctxKey = "url"
)

func CtxWithUser(ctx context.Context, user database.User) context.Context {
	return context.WithValue(ctx, ctxKeyUser, user)
}

func UserFromCtx(ctx context.Context) database.User {
	return ctx.Value(ctxKeyUser).(database.User)
}

func CtxWithURL(ctx context.Context, url string) context.Context {
	return context.WithValue(ctx, ctxKeyURL, url)
}

func URLFromCtx(ctx context.Context) string {
	return ctx.Value(ctxKeyURL).(string)
}
