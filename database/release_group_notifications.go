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

func (d *DB) DeleteReleaseGroupNotification(ctx context.Context, rgID, userID string) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	_, err := d.gq.
		Delete(tableReleaseGroupNotifications).
		Where(
			goqu.C("user_id").Eq(userID),
			goqu.C("release_group_mb_id").Eq(rgID),
		).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete release group notification: %w", err)
	}

	return nil
}

// NotifiedReleaseGroupIDs returns the set of release group MBIDs the user has
// already been notified about.
func (d *DB) NotifiedReleaseGroupIDs(ctx context.Context, userID string) (map[string]struct{}, error) {
	ids := []string{}
	err := d.gq.
		Select("release_group_mb_id").
		From(tableReleaseGroupNotifications).
		Where(goqu.C("user_id").Eq(userID)).
		Executor().ScanValsContext(ctx, &ids)
	if err != nil {
		return nil, fmt.Errorf("notified release groups: %w", err)
	}

	res := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		res[id] = struct{}{}
	}

	return res, nil
}
