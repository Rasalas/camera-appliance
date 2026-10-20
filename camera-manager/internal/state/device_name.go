package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DeviceNameMaxLength keeps names readable on viewer tiles and in lists.
const DeviceNameMaxLength = 40

var ErrInvalidDeviceName = errors.New("ungültiger Kameraname")

type deviceNameError string

func (e deviceNameError) Error() string { return string(e) }
func (e deviceNameError) Unwrap() error { return ErrInvalidDeviceName }

// NormalizeDeviceName trims the name; an empty result clears the custom name.
func NormalizeDeviceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > DeviceNameMaxLength {
		return "", deviceNameError(fmt.Sprintf("Der Name darf höchstens %d Zeichen lang sein.", DeviceNameMaxLength))
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", deviceNameError("Der Name darf keine Zeilenumbrüche enthalten.")
	}
	return name, nil
}

// SetDeviceName stores the display name for the stable device ID.
// It returns sql.ErrNoRows when the device does not exist.
func (s *Store) SetDeviceName(ctx context.Context, id, name string) (Device, error) {
	name, err := NormalizeDeviceName(name)
	if err != nil {
		return Device{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE devices SET name=nullif(?,'') WHERE id=?`, name, id)
	if err != nil {
		return Device{}, err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return Device{}, err
	} else if changed == 0 {
		return Device{}, sql.ErrNoRows
	}
	return s.Device(ctx, id)
}

// ensureDeviceNameColumn upgrades databases created before display names.
func ensureDeviceNameColumn(ctx context.Context, db querier) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('devices') WHERE name='name'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE devices ADD COLUMN name TEXT`)
	return err
}

type querier interface {
	executor
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
