package steward

// The steward's room address, stored beside the seat.
//
// It is a small separate file rather than a field on the journal, because the
// journal is the AUTHORITY on who holds the seat and must stay replayable from
// nothing. A room address is neither authority nor history — it is a live
// pointer that is meaningless once the room closes, and putting it in the
// journal would mean replaying a stream of dead addresses to learn the current
// one.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/qiangli/coreutils/pkg/role"
)

func seatContactPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "seat-room.json"), nil
}

func saveSeatContact(c *role.Contact) error {
	p, err := seatContactPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

func loadSeatContact() (*role.Contact, error) {
	p, err := seatContactPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var c role.Contact
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func clearSeatContact() error {
	p, err := seatContactPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
