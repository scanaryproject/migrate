// Package migrate reads migrations from sources and runs them against databases.
// Sources are defined by the `source.Driver` and databases by the `database.Driver`
// interface. The driver interfaces are kept "dumb", all migration logic is kept
// in this package.
//
// The model is Liquibase-like: the database keeps one ledger row per applied
// migration (version, filename, md5sum, time_applied). Up and Down move exactly
// one migration per call. Before every step the ledger is validated against the
// local source: every applied version must still exist locally with the same
// md5, and no local version below the highest applied one may be unapplied.
package migrate

import (
	"crypto/md5" // #nosec G501 -- checksum for change detection, not security
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	iurl "github.com/golang-migrate/migrate/v4/internal/url"
	"github.com/golang-migrate/migrate/v4/source"
)

// DefaultLockTimeout sets the max time a database driver has to acquire a lock.
var DefaultLockTimeout = 15 * time.Second

var (
	ErrNoChange    = errors.New("no change")
	ErrLocked      = errors.New("database locked")
	ErrLockTimeout = errors.New("timeout: can't acquire database lock")
)

// ErrChecksumMismatch: an applied migration's local up file changed since it was applied.
type ErrChecksumMismatch struct {
	Version  uint
	Filename string
	Applied  string
	Local    string
}

func (e ErrChecksumMismatch) Error() string {
	return fmt.Sprintf("checksum mismatch for version %d (%s): applied md5 %s, local md5 %s", e.Version, e.Filename, e.Applied, e.Local)
}

// ErrMissingLocal: ledger has a version with no local up migration.
type ErrMissingLocal struct {
	Version  uint
	Filename string
}

func (e ErrMissingLocal) Error() string {
	return fmt.Sprintf("version %d (%s) is applied in database but has no local up migration", e.Version, e.Filename)
}

// ErrOutOfOrder: a local version lower than the highest applied one was never applied.
type ErrOutOfOrder struct {
	Version    uint
	MaxApplied uint
}

func (e ErrOutOfOrder) Error() string {
	return fmt.Sprintf("local version %d is not applied but version %d already is; migrations must be applied in order", e.Version, e.MaxApplied)
}

// ErrNoDownMigration: Down requested but the source has no down file for the version.
type ErrNoDownMigration struct {
	Version uint
}

func (e ErrNoDownMigration) Error() string {
	return fmt.Sprintf("no down migration for version %d", e.Version)
}

// Status is the result of comparing ledger and source.
type Status struct {
	// Applied rows from the ledger, ascending by version.
	Applied []database.AppliedMigration
	// Pending local versions above the highest applied one, ascending.
	Pending []uint
}

type Migrate struct {
	sourceName         string
	sourceDrv          source.Driver
	databaseDriverName string
	databaseDrv        database.Driver

	// Log accepts a Logger interface
	Log Logger

	// LockTimeout defaults to DefaultLockTimeout,
	// but can be set per Migrate instance.
	LockTimeout time.Duration

	isLockedMu *sync.Mutex
	isLocked   bool
}

// New returns a new Migrate instance from a source URL and a database URL.
// The URL scheme is defined by each driver.
func New(sourceURL, databaseURL string) (*Migrate, error) {
	m := newCommon()

	sourceName, err := iurl.SchemeFromURL(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse scheme from source URL: %w", err)
	}
	m.sourceName = sourceName

	databaseDriverName, err := iurl.SchemeFromURL(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse scheme from database URL: %w", err)
	}
	m.databaseDriverName = databaseDriverName

	sourceDrv, err := source.Open(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open source, %q: %w", sourceURL, err)
	}
	m.sourceDrv = sourceDrv

	databaseDrv, err := database.Open(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	m.databaseDrv = databaseDrv

	return m, nil
}

// NewWithDatabaseInstance returns a new Migrate instance from a source URL
// and an existing database instance. The source URL scheme is defined by each driver.
// Use any string that can serve as an identifier during logging as databaseDriverName.
// You are responsible for closing the underlying database client if necessary.
func NewWithDatabaseInstance(sourceURL string, databaseDriverName string, databaseInstance database.Driver) (*Migrate, error) {
	m := newCommon()

	sourceName, err := iurl.SchemeFromURL(sourceURL)
	if err != nil {
		return nil, err
	}
	m.sourceName = sourceName
	m.databaseDriverName = databaseDriverName

	sourceDrv, err := source.Open(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open source, %q: %w", sourceURL, err)
	}
	m.sourceDrv = sourceDrv
	m.databaseDrv = databaseInstance

	return m, nil
}

// NewWithSourceInstance returns a new Migrate instance from an existing source instance
// and a database URL. The database URL scheme is defined by each driver.
// Use any string that can serve as an identifier during logging as sourceName.
// You are responsible for closing the underlying source client if necessary.
func NewWithSourceInstance(sourceName string, sourceInstance source.Driver, databaseURL string) (*Migrate, error) {
	m := newCommon()

	databaseDriverName, err := iurl.SchemeFromURL(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse scheme from database URL: %w", err)
	}
	m.databaseDriverName = databaseDriverName
	m.sourceName = sourceName

	databaseDrv, err := database.Open(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	m.databaseDrv = databaseDrv
	m.sourceDrv = sourceInstance

	return m, nil
}

// NewWithInstance returns a new Migrate instance from an existing source and
// database instance. Use any string that can serve as an identifier during logging
// as sourceName and databaseDriverName. You are responsible for closing down
// the underlying source and database client if necessary.
func NewWithInstance(sourceName string, sourceInstance source.Driver, databaseDriverName string, databaseInstance database.Driver) (*Migrate, error) {
	m := newCommon()

	m.sourceName = sourceName
	m.databaseDriverName = databaseDriverName
	m.sourceDrv = sourceInstance
	m.databaseDrv = databaseInstance

	return m, nil
}

func newCommon() *Migrate {
	return &Migrate{
		LockTimeout: DefaultLockTimeout,
		isLockedMu:  &sync.Mutex{},
	}
}

// Close closes the source and the database.
func (m *Migrate) Close() (source error, database error) {
	databaseSrvClose := make(chan error)
	sourceSrvClose := make(chan error)

	m.logVerbosePrintf("Closing source and database\n")

	go func() {
		databaseSrvClose <- m.databaseDrv.Close()
	}()

	go func() {
		sourceSrvClose <- m.sourceDrv.Close()
	}()

	return <-sourceSrvClose, <-databaseSrvClose
}

// Up validates the ledger and applies exactly one pending migration.
// Returns ErrNoChange when nothing is pending.
func (m *Migrate) Up() error {
	if err := m.lock(); err != nil {
		return err
	}
	return m.unlockErr(m.up())
}

func (m *Migrate) up() error {
	st, err := m.status()
	if err != nil {
		return err
	}
	if len(st.Pending) == 0 {
		return ErrNoChange
	}
	v := st.Pending[0]

	body, filename, err := m.readUp(v)
	if err != nil {
		return err
	}

	start := time.Now()
	row := database.AppliedMigration{Version: v, Filename: filename, MD5Sum: md5hex(body)}
	if err := m.databaseDrv.Apply(row, body); err != nil {
		return err
	}
	m.logPrintf("%d/u %s (%v)\n", v, filename, time.Since(start))
	return nil
}

// Down validates the ledger and reverts exactly the highest applied migration.
// Returns ErrNoChange when the ledger is empty.
func (m *Migrate) Down() error {
	if err := m.lock(); err != nil {
		return err
	}
	return m.unlockErr(m.down())
}

func (m *Migrate) down() error {
	st, err := m.status()
	if err != nil {
		return err
	}
	if len(st.Applied) == 0 {
		return ErrNoChange
	}
	v := st.Applied[len(st.Applied)-1].Version

	r, filename, err := m.sourceDrv.ReadDown(v)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoDownMigration{v}
	}
	if err != nil {
		return err
	}
	body, err := readAll(r)
	if err != nil {
		return err
	}

	start := time.Now()
	if err := m.databaseDrv.Revert(v, body); err != nil {
		return err
	}
	m.logPrintf("%d/d %s (%v)\n", v, filename, time.Since(start))
	return nil
}

// Status validates the ledger against the source and reports applied and pending versions.
func (m *Migrate) Status() (Status, error) {
	if err := m.lock(); err != nil {
		return Status{}, err
	}
	st, err := m.status()
	return st, m.unlockErr(err)
}

// Force records version as applied (with the md5 of the local up file) without running it.
// The version must exist locally and must not be applied yet.
func (m *Migrate) Force(version uint) error {
	if err := m.lock(); err != nil {
		return err
	}
	return m.unlockErr(m.force(version))
}

func (m *Migrate) force(version uint) error {
	applied, err := m.databaseDrv.Applied()
	if err != nil {
		return err
	}
	for _, a := range applied {
		if a.Version == version {
			return fmt.Errorf("version %d is already applied", version)
		}
	}
	body, filename, err := m.readUp(version)
	if err != nil {
		return err
	}
	row := database.AppliedMigration{Version: version, Filename: filename, MD5Sum: md5hex(body)}
	if err := m.databaseDrv.Apply(row, nil); err != nil {
		return err
	}
	m.logPrintf("%d/u %s (forced, not run)\n", version, filename)
	return nil
}

// Unforce deletes the ledger row for version without running the down migration.
func (m *Migrate) Unforce(version uint) error {
	if err := m.lock(); err != nil {
		return err
	}
	return m.unlockErr(m.unforce(version))
}

func (m *Migrate) unforce(version uint) error {
	applied, err := m.databaseDrv.Applied()
	if err != nil {
		return err
	}
	for _, a := range applied {
		if a.Version == version {
			if err := m.databaseDrv.Revert(version, nil); err != nil {
				return err
			}
			m.logPrintf("%d/d %s (unforced, not run)\n", version, a.Filename)
			return nil
		}
	}
	return fmt.Errorf("version %d is not applied", version)
}

// Drop deletes everything in the database.
func (m *Migrate) Drop() error {
	if err := m.lock(); err != nil {
		return err
	}
	if err := m.databaseDrv.Drop(); err != nil {
		return m.unlockErr(err)
	}
	return m.unlock()
}

// status loads ledger and source, validates them, and computes pending versions.
// Caller must hold the lock.
func (m *Migrate) status() (Status, error) {
	applied, err := m.databaseDrv.Applied()
	if err != nil {
		return Status{}, err
	}
	local, err := m.localVersions()
	if err != nil {
		return Status{}, err
	}

	// every applied version must exist locally with the same md5
	for _, a := range applied {
		body, _, err := m.readUp(a.Version)
		if errors.Is(err, os.ErrNotExist) {
			return Status{}, ErrMissingLocal{a.Version, a.Filename}
		}
		if err != nil {
			return Status{}, err
		}
		if sum := md5hex(body); sum != a.MD5Sum {
			return Status{}, ErrChecksumMismatch{a.Version, a.Filename, a.MD5Sum, sum}
		}
	}

	st := Status{Applied: applied}
	if len(applied) == 0 {
		st.Pending = local
		return st, nil
	}

	// no local version at or below max applied may be unapplied
	maxApplied := applied[len(applied)-1].Version
	isApplied := make(map[uint]bool, len(applied))
	for _, a := range applied {
		isApplied[a.Version] = true
	}
	st.Pending = make([]uint, 0, len(local))
	for _, v := range local {
		switch {
		case v > maxApplied:
			st.Pending = append(st.Pending, v)
		case !isApplied[v]:
			return Status{}, ErrOutOfOrder{v, maxApplied}
		}
	}
	return st, nil
}

// localVersions lists every version known to the source, ascending.
func (m *Migrate) localVersions() ([]uint, error) {
	versions := make([]uint, 0)
	v, err := m.sourceDrv.First()
	if errors.Is(err, os.ErrNotExist) {
		return versions, nil
	}
	if err != nil {
		return nil, err
	}
	for {
		versions = append(versions, v)
		v, err = m.sourceDrv.Next(v)
		if errors.Is(err, os.ErrNotExist) {
			return versions, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// readUp reads the whole up migration body for version.
// Returns an error wrapping os.ErrNotExist if the source has no up file.
func (m *Migrate) readUp(version uint) (body []byte, filename string, err error) {
	r, filename, err := m.sourceDrv.ReadUp(version)
	if err != nil {
		return nil, "", err
	}
	body, err = readAll(r)
	return body, filename, err
}

func readAll(r io.ReadCloser) (body []byte, err error) {
	defer func() {
		if cerr := r.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()
	return io.ReadAll(r)
}

func md5hex(b []byte) string {
	sum := md5.Sum(b) // #nosec G401 -- change detection only
	return hex.EncodeToString(sum[:])
}

// lock is a thread safe helper function to lock the database.
// It should be called as late as possible when running migrations.
func (m *Migrate) lock() error {
	m.isLockedMu.Lock()
	defer m.isLockedMu.Unlock()

	if m.isLocked {
		return ErrLocked
	}

	// create done channel, used in the timeout goroutine
	done := make(chan bool, 1)
	defer func() {
		done <- true
	}()

	// use errchan to signal error back to this context
	errchan := make(chan error, 2)

	// start timeout goroutine
	timeout := time.After(m.LockTimeout)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-timeout:
				errchan <- ErrLockTimeout
				return
			}
		}
	}()

	// now try to acquire the lock
	go func() {
		if err := m.databaseDrv.Lock(); err != nil {
			errchan <- err
		} else {
			errchan <- nil
		}
	}()

	// wait until we either receive ErrLockTimeout or error from Lock operation
	err := <-errchan
	if err == nil {
		m.isLocked = true
	}
	return err
}

// unlock is a thread safe helper function to unlock the database.
// It should be called as early as possible when no more migrations are
// expected to be executed.
func (m *Migrate) unlock() error {
	m.isLockedMu.Lock()
	defer m.isLockedMu.Unlock()

	if err := m.databaseDrv.Unlock(); err != nil {
		// BUG: Can potentially create a deadlock. Add a timeout.
		return err
	}

	m.isLocked = false
	return nil
}

// unlockErr calls unlock and returns a combined error
// if a prevErr is not nil.
func (m *Migrate) unlockErr(prevErr error) error {
	if err := m.unlock(); err != nil {
		prevErr = errors.Join(prevErr, err)
	}
	return prevErr
}

// logPrintf writes to m.Log if not nil
func (m *Migrate) logPrintf(format string, v ...interface{}) {
	if m.Log != nil {
		m.Log.Printf(format, v...)
	}
}

// logVerbosePrintf writes to m.Log if not nil. Use for verbose logging output.
func (m *Migrate) logVerbosePrintf(format string, v ...interface{}) {
	if m.Log != nil && m.Log.Verbose() {
		m.Log.Printf(format, v...)
	}
}
