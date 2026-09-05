package state

import (
	"camera-appliance/camera-manager/internal/fingerprint"
	"camera-appliance/camera-manager/internal/matcher"
	"context"
)

// ReconcileDevice serializes matching and persistence, preserving the primary
// key when discovery enriches an existing physical camera's identity.
func (s *Store) ReconcileDevice(ctx context.Context, discovered Device) (Device, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, deviceSelect)
	if err != nil {
		return Device{}, err
	}
	existing := map[string]fingerprint.Fingerprint{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			rows.Close()
			return Device{}, err
		}
		existing[d.ID] = d.Fingerprint()
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return Device{}, rowsErr
	}
	id, err := matcher.ResolveIdentity(existing, discovered.Fingerprint())
	if err != nil {
		return Device{}, err
	}
	if id != "" {
		discovered.ID = id
	} else if discovered.ID == "" {
		discovered.ID = fingerprint.DeviceID(discovered.Fingerprint())
	}
	if err := upsertDevice(ctx, tx, discovered); err != nil {
		return Device{}, err
	}
	saved, err := scanDevice(tx.QueryRowContext(ctx, deviceSelect+" WHERE id=?", discovered.ID))
	if err != nil {
		return Device{}, err
	}
	if err := tx.Commit(); err != nil {
		return Device{}, err
	}
	return saved, nil
}
