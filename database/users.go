package database

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9"
)

const (
	tableUsers = "users"
)

var ErrUserNotFound = fmt.Errorf("user not found")

type User struct {
	ID       string `db:"id"`
	Name     string `db:"name"`
	Password string `db:"password"`

	SubsonicUser string `db:"subsonic_user"`
	SubsonicPass string `db:"subsonic_pass"`

	XMPPJID string `db:"xmpp_jid"`
}

func (d *DB) AddUser(ctx context.Context, user User) error {
	_, err := d.gq.
		Insert(tableUsers).
		Rows(user).
		OnConflict(goqu.DoUpdate("id", goqu.Record{
			"name":          goqu.L("excluded.name"),
			"password":      goqu.L("excluded.password"),
			"subsonic_user": goqu.L("excluded.subsonic_user"),
			"subsonic_pass": goqu.L("excluded.subsonic_pass"),
			"xmpp_jid":      goqu.L("excluded.xmpp_jid"),
		})).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("add user: %w", err)
	}

	return nil
}

func (d *DB) ListUsers(ctx context.Context) ([]User, error) {
	scanner, err := d.gq.
		Select("*").
		From(tableUsers).
		Executor().ScannerContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: select: %w", err)
	}
	defer scanner.Close()

	return scan[User](scanner)
}

func (d *DB) UserByID(ctx context.Context, id string) (User, error) {
	user := User{}

	ok, err := d.gq.
		Select("*").
		From(tableUsers).
		Where(goqu.C("id").Eq(id)).
		Executor().ScanStructContext(ctx, &user)
	if err != nil {
		return User{}, fmt.Errorf("list users: select: %w", err)
	}
	if !ok {
		return User{}, ErrUserNotFound
	}

	return user, nil
}

func (d *DB) UserDelete(ctx context.Context, id string) error {
	_, err := d.gq.
		Delete(tableUsers).
		Where(goqu.C("id").Eq(id)).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return nil
}
