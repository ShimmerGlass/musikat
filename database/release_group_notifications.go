package database

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9"
)

const tableReleaseGroupNotifications = "release_group_notifications"

func (d *DB) AddReleaseGroupNotification(ctx context.Context, rgID, userID string) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	_, err := d.gq.
		Insert(tableReleaseGroupNotifications).
		Rows(goqu.Record{
			"user_id":             userID,
			"release_group_mb_id": rgID,
		}).
		OnConflict(goqu.DoNothing()).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("add release group notification: %w", err)
	}

	return nil
}

func (d *DB) HasReleaseGroupNotification(ctx context.Context, rgID, userID string) (bool, error) {
	var count int
	_, err := d.gq.
		Select(goqu.L("count(*)")).
		From(tableReleaseGroupNotifications).
		Where(
			goqu.C("user_id").Eq(userID),
			goqu.C("release_group_mb_id").Eq(rgID),
		).
		Executor().ScanValContext(ctx, &count)
	if err != nil {
		return false, fmt.Errorf("has release group notification: %w", err)
	}

	return count > 0, nil
}
