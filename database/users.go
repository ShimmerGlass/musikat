package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	tableUsers = "users"
)

var (
	ErrUserNotFound = fmt.Errorf("user not found")
	ErrUserExists   = fmt.Errorf("user already exists")
)

type User struct {
	ID       string `db:"id"`
	Name     string `db:"name"`
	Password string `db:"password"`

	SubsonicUser string `db:"subsonic_user"`
	SubsonicPass string `db:"subsonic_pass"`

	XMPPJID string `db:"xmpp_jid"`

	Admin bool `db:"admin"`
}

// CreateUser inserts a new user. It returns ErrUserExists if the id is taken.
func (d *DB) CreateUser(ctx context.Context, user User) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	_, err := d.gq.
		Insert(tableUsers).
		Rows(user).
		Executor().ExecContext(ctx)
	if err != nil {
		var sqlErr *sqlite.Error
		if errors.As(err, &sqlErr) && sqlErr.Code()&0xff == sqlite3.SQLITE_CONSTRAINT {
			return ErrUserExists
		}
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

var ErrSetupDone = fmt.Errorf("setup already completed")

// CreateFirstUser inserts user only if there are no users yet, atomically. It
// returns ErrSetupDone otherwise.
func (d *DB) CreateFirstUser(ctx context.Context, user User) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	count, err := d.gq.From(tableUsers).CountContext(ctx)
	if err != nil {
		return fmt.Errorf("create first user: %w", err)
	}
	if count > 0 {
		return ErrSetupDone
	}

	_, err = d.gq.
		Insert(tableUsers).
		Rows(user).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("create first user: %w", err)
	}

	return nil
}

// UpdateUser updates an existing user. It returns ErrUserNotFound if there is
// no user with that id.
func (d *DB) UpdateUser(ctx context.Context, user User) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	res, err := d.gq.
		Update(tableUsers).
		Set(goqu.Record{
			"name":          user.Name,
			"password":      user.Password,
			"subsonic_user": user.SubsonicUser,
			"subsonic_pass": user.SubsonicPass,
			"xmpp_jid":      user.XMPPJID,
			"admin":         user.Admin,
		}).
		Where(goqu.C("id").Eq(user.ID)).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if n == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (d *DB) Users(ctx context.Context) ([]User, error) {
	res := []User{}

	err := d.gq.
		Select("*").
		From(tableUsers).
		Executor().ScanStructsContext(ctx, &res)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	return res, nil
}

func (d *DB) User(ctx context.Context, id string) (User, error) {
	user := User{}

	ok, err := d.gq.
		Select("*").
		From(tableUsers).
		Where(goqu.C("id").Eq(id)).
		Executor().ScanStructContext(ctx, &user)
	if err != nil {
		return User{}, fmt.Errorf("users by id: %w", err)
	}
	if !ok {
		return User{}, ErrUserNotFound
	}

	return user, nil
}

// UserDelete removes a user along with their watches and notifications.
func (d *DB) UserDelete(ctx context.Context, id string) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	tx, err := d.gq.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete user: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, table := range []string{tableArtistWatches, tableReleaseGroupNotifications, tableUsers} {
		col := "user_id"
		if table == tableUsers {
			col = "id"
		}
		_, err = tx.
			Delete(table).
			Where(goqu.C(col).Eq(id)).
			Executor().ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("delete user: %s: %w", table, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("delete user: commit: %w", err)
	}

	return nil
}
