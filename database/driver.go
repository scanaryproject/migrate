// Package database provides the Driver interface.
// All database drivers must implement this interface, register themselves,
// optionally provide a `WithInstance` function and pass the tests
// in package database/testing.
package database

import (
	"fmt"
	"sync"
	"time"

	iurl "github.com/golang-migrate/migrate/v4/internal/url"
)

var (
	ErrLocked    = fmt.Errorf("can't acquire lock")
	ErrNotLocked = fmt.Errorf("can't unlock, as not currently locked")
)

// NoTransactionMarker, when found at the very start of a migration body,
// tells the driver to run the body outside of a transaction
// (e.g. for CREATE INDEX CONCURRENTLY). The ledger row is then written
// in a separate statement after the body succeeded.
const NoTransactionMarker = "-- migrate:no-transaction"

// AppliedMigration is one row of the migrations ledger table.
type AppliedMigration struct {
	Version   uint
	Filename  string
	MD5Sum    string // lowercase hex of md5 over the up migration body
	AppliedAt time.Time
}

var driversMu sync.RWMutex
var drivers = make(map[string]Driver)

// Driver is the interface every database driver must implement.
//
// How to implement a database driver?
//  1. Implement this interface.
//  2. Optionally, add a function named `WithInstance`.
//     This function should accept an existing DB instance and a Config{} struct
//     and return a driver instance.
//  3. Add a test that calls database/testing.go:Test()
//  4. Add own tests for Open(), WithInstance() (when provided) and Close().
//     All other functions are tested by tests in database/testing.
//  5. Call Register in init().
//  6. Create a internal/cli/build_<driver-name>.go file
//  7. Add driver name in 'DATABASE' variable in Makefile
//
// Guidelines:
//   - Don't try to correct user input. Don't assume things.
//     When in doubt, return an error and explain the situation to the user.
//   - All configuration input must come from the URL string in func Open()
//     or the Config{} struct in WithInstance. Don't os.Getenv().
type Driver interface {
	// Open returns a new driver instance configured with parameters
	// coming from the URL string. Migrate will call this function
	// only once per instance.
	Open(url string) (Driver, error)

	// Close closes the underlying database instance managed by the driver.
	// Migrate will call this function only once per instance.
	Close() error

	// Lock should acquire a database lock so that only one migration process
	// can run at a time. Migrate will call this function before Apply/Revert.
	// If the implementation can't provide this functionality, return nil.
	// Return database.ErrLocked if database is already locked.
	Lock() error

	// Unlock should release the lock.
	Unlock() error

	// Applied returns every recorded migration ordered by version ascending.
	// An empty ledger returns an empty slice and no error.
	Applied() ([]AppliedMigration, error)

	// Apply runs body (the up migration) and records m in the ledger.
	// body == nil means "record only" (used by force).
	// Where the database supports it, body and the ledger insert run in
	// one transaction, unless body starts with NoTransactionMarker.
	Apply(m AppliedMigration, body []byte) error

	// Revert runs body (the down migration) and deletes the ledger row for version.
	// body == nil means "delete only" (used by unforce).
	// Same transaction semantics as Apply.
	Revert(version uint, body []byte) error

	// Drop deletes everything in the database.
	// Note that this is a breaking action, a new call to Open() is necessary to
	// ensure subsequent calls work as expected.
	Drop() error
}

// Open returns a new driver instance.
func Open(url string) (Driver, error) {
	scheme, err := iurl.SchemeFromURL(url)
	if err != nil {
		return nil, err
	}

	driversMu.RLock()
	d, ok := drivers[scheme]
	driversMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("database driver: unknown driver %v (forgotten import?)", scheme)
	}

	return d.Open(url)
}

// Register globally registers a driver.
func Register(name string, driver Driver) {
	driversMu.Lock()
	defer driversMu.Unlock()
	if driver == nil {
		panic("Register driver is nil")
	}
	if _, dup := drivers[name]; dup {
		panic("Register called twice for driver " + name)
	}
	drivers[name] = driver
}

// List lists the registered drivers
func List() []string {
	driversMu.RLock()
	defer driversMu.RUnlock()
	names := make([]string, 0, len(drivers))
	for n := range drivers {
		names = append(names, n)
	}
	return names
}
