package stub

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
)

func init() {
	database.Register("stub", &Stub{})
}

type Stub struct {
	Url               string
	Instance          interface{}
	Rows              []database.AppliedMigration
	MigrationSequence []string
	LastRunMigration  []byte
	isLocked          atomic.Bool

	Config *Config
}

func (s *Stub) Open(url string) (database.Driver, error) {
	return &Stub{
		Url:               url,
		MigrationSequence: make([]string, 0),
		Config:            &Config{},
	}, nil
}

type Config struct{}

func WithInstance(instance interface{}, config *Config) (database.Driver, error) {
	return &Stub{
		Instance:          instance,
		MigrationSequence: make([]string, 0),
		Config:            config,
	}, nil
}

func (s *Stub) Close() error {
	return nil
}

func (s *Stub) Lock() error {
	if !s.isLocked.CompareAndSwap(false, true) {
		return database.ErrLocked
	}
	return nil
}

func (s *Stub) Unlock() error {
	if !s.isLocked.CompareAndSwap(true, false) {
		return database.ErrNotLocked
	}
	return nil
}

func (s *Stub) Applied() ([]database.AppliedMigration, error) {
	out := make([]database.AppliedMigration, len(s.Rows))
	copy(out, s.Rows)
	return out, nil
}

func (s *Stub) Apply(m database.AppliedMigration, body []byte) error {
	for _, r := range s.Rows {
		if r.Version == m.Version {
			return fmt.Errorf("stub: version %d already applied", m.Version)
		}
	}
	if body != nil {
		s.LastRunMigration = body
		s.MigrationSequence = append(s.MigrationSequence, string(body))
	}
	if m.AppliedAt.IsZero() {
		m.AppliedAt = time.Now()
	}
	// keep sorted by version
	i := 0
	for i < len(s.Rows) && s.Rows[i].Version < m.Version {
		i++
	}
	s.Rows = append(s.Rows, database.AppliedMigration{})
	copy(s.Rows[i+1:], s.Rows[i:])
	s.Rows[i] = m
	return nil
}

func (s *Stub) Revert(version uint, body []byte) error {
	for i, r := range s.Rows {
		if r.Version == version {
			if body != nil {
				s.LastRunMigration = body
				s.MigrationSequence = append(s.MigrationSequence, string(body))
			}
			s.Rows = append(s.Rows[:i], s.Rows[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("stub: version %d not applied", version)
}

const DROP = "DROP"

func (s *Stub) Drop() error {
	s.Rows = nil
	s.LastRunMigration = nil
	s.MigrationSequence = append(s.MigrationSequence, DROP)
	return nil
}

func (s *Stub) EqualSequence(seq []string) bool {
	return reflect.DeepEqual(seq, s.MigrationSequence)
}
