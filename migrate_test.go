package migrate

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"reflect"
	"testing"

	dStub "github.com/golang-migrate/migrate/v4/database/stub"
	"github.com/golang-migrate/migrate/v4/source"
	sStub "github.com/golang-migrate/migrate/v4/source/stub"
)

// sourceStubMigrations hold the following migrations:
// u = up migration, d = down migration, n = version
//
//	|  1  |  -  |  3  |  4  |  7  |
//	| u d |  -  | u d | u d | u d |
var sourceStubMigrations *source.Migrations

const (
	srcDrvNameStub = "stub"
	dbDrvNameStub  = "stub"
)

func init() {
	sourceStubMigrations = source.NewMigrations()
	sourceStubMigrations.Append(&source.Migration{Version: 1, Direction: source.Up, Identifier: "CREATE 1"})
	sourceStubMigrations.Append(&source.Migration{Version: 1, Direction: source.Down, Identifier: "DROP 1"})
	sourceStubMigrations.Append(&source.Migration{Version: 3, Direction: source.Up, Identifier: "CREATE 3"})
	sourceStubMigrations.Append(&source.Migration{Version: 3, Direction: source.Down, Identifier: "DROP 3"})
	sourceStubMigrations.Append(&source.Migration{Version: 4, Direction: source.Up, Identifier: "CREATE 4"})
	sourceStubMigrations.Append(&source.Migration{Version: 4, Direction: source.Down, Identifier: "DROP 4"})
	sourceStubMigrations.Append(&source.Migration{Version: 7, Direction: source.Up, Identifier: "CREATE 7"})
	sourceStubMigrations.Append(&source.Migration{Version: 7, Direction: source.Down, Identifier: "DROP 7"})
}

type DummyInstance struct{ Name string }

func TestNew(t *testing.T) {
	m, err := New("stub://", "stub://")
	if err != nil {
		t.Fatal(err)
	}

	if m.sourceName != srcDrvNameStub {
		t.Errorf("expected stub, got %v", m.sourceName)
	}
	if m.sourceDrv == nil {
		t.Error("expected sourceDrv not to be nil")
	}

	if m.databaseDriverName != dbDrvNameStub {
		t.Errorf("expected stub, got %v", m.databaseDriverName)
	}
	if m.databaseDrv == nil {
		t.Error("expected databaseDrv not to be nil")
	}
}

func ExampleNew() {
	// Read migrations from /home/mattes/migrations and connect to a local postgres database.
	m, err := New("file:///home/mattes/migrations", "postgres://mattes:secret@localhost:5432/database?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	// Apply the next pending migration ...
	if err := m.Up(); err != nil && !errors.Is(err, ErrNoChange) {
		log.Fatal(err)
	}
}

func TestNewWithDatabaseInstance(t *testing.T) {
	dummyDb := &DummyInstance{"database"}
	dbInst, err := dStub.WithInstance(dummyDb, &dStub.Config{})
	if err != nil {
		t.Fatal(err)
	}

	m, err := NewWithDatabaseInstance("stub://", dbDrvNameStub, dbInst)
	if err != nil {
		t.Fatal(err)
	}

	if m.sourceName != srcDrvNameStub {
		t.Errorf("expected stub, got %v", m.sourceName)
	}
	if m.sourceDrv == nil {
		t.Error("expected sourceDrv not to be nil")
	}

	if m.databaseDriverName != dbDrvNameStub {
		t.Errorf("expected stub, got %v", m.databaseDriverName)
	}
	if m.databaseDrv == nil {
		t.Error("expected databaseDrv not to be nil")
	}
}

func ExampleNewWithDatabaseInstance() {
	// Create and use an existing database instance.
	db, err := sql.Open("postgres", "postgres://mattes:secret@localhost:5432/database?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	// Create driver instance from db.
	// Check each driver if it supports the WithInstance function.
	// `import "github.com/golang-migrate/migrate/v4/database/postgres"`
	instance, err := dStub.WithInstance(db, &dStub.Config{})
	if err != nil {
		log.Fatal(err)
	}

	// Read migrations from /home/mattes/migrations and connect to a local postgres database.
	m, err := NewWithDatabaseInstance("file:///home/mattes/migrations", "postgres", instance)
	if err != nil {
		log.Fatal(err)
	}

	// Migrate all the way up ...
	if err := m.Up(); err != nil {
		log.Fatal(err)
	}
}

func TestNewWithSourceInstance(t *testing.T) {
	dummySource := &DummyInstance{"source"}
	sInst, err := sStub.WithInstance(dummySource, &sStub.Config{})
	if err != nil {
		t.Fatal(err)
	}

	m, err := NewWithSourceInstance(srcDrvNameStub, sInst, "stub://")
	if err != nil {
		t.Fatal(err)
	}

	if m.sourceName != srcDrvNameStub {
		t.Errorf("expected stub, got %v", m.sourceName)
	}
	if m.sourceDrv == nil {
		t.Error("expected sourceDrv not to be nil")
	}

	if m.databaseDriverName != dbDrvNameStub {
		t.Errorf("expected stub, got %v", m.databaseDriverName)
	}
	if m.databaseDrv == nil {
		t.Error("expected databaseDrv not to be nil")
	}
}

func ExampleNewWithSourceInstance() {
	di := &DummyInstance{"think any client required for a source here"}

	// Create driver instance from DummyInstance di.
	// Check each driver if it support the WithInstance function.
	// `import "github.com/golang-migrate/migrate/v4/source/stub"`
	instance, err := sStub.WithInstance(di, &sStub.Config{})
	if err != nil {
		log.Fatal(err)
	}

	// Read migrations from Stub and connect to a local postgres database.
	m, err := NewWithSourceInstance(srcDrvNameStub, instance, "postgres://mattes:secret@localhost:5432/database?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	// Migrate all the way up ...
	if err := m.Up(); err != nil {
		log.Fatal(err)
	}
}

func TestNewWithInstance(t *testing.T) {
	dummyDb := &DummyInstance{"database"}
	dbInst, err := dStub.WithInstance(dummyDb, &dStub.Config{})
	if err != nil {
		t.Fatal(err)
	}

	dummySource := &DummyInstance{"source"}
	sInst, err := sStub.WithInstance(dummySource, &sStub.Config{})
	if err != nil {
		t.Fatal(err)
	}

	m, err := NewWithInstance(srcDrvNameStub, sInst, dbDrvNameStub, dbInst)
	if err != nil {
		t.Fatal(err)
	}

	if m.sourceName != srcDrvNameStub {
		t.Errorf("expected stub, got %v", m.sourceName)
	}
	if m.sourceDrv == nil {
		t.Error("expected sourceDrv not to be nil")
	}

	if m.databaseDriverName != dbDrvNameStub {
		t.Errorf("expected stub, got %v", m.databaseDriverName)
	}
	if m.databaseDrv == nil {
		t.Error("expected databaseDrv not to be nil")
	}
}

func ExampleNewWithInstance() {
	// See NewWithDatabaseInstance and NewWithSourceInstance for an example.
}

func TestClose(t *testing.T) {
	m, _ := New("stub://", "stub://")
	sourceErr, databaseErr := m.Close()
	if sourceErr != nil {
		t.Error(sourceErr)
	}
	if databaseErr != nil {
		t.Error(databaseErr)
	}
}

func TestLock(t *testing.T) {
	m, _ := New("stub://", "stub://")
	if err := m.lock(); err != nil {
		t.Fatal(err)
	}

	if err := m.lock(); err == nil {
		t.Fatal("should be locked already")
	}
}

// newStubMigrate wires a copy of sourceStubMigrations to a fresh stub database.
func newStubMigrate(t *testing.T) (*Migrate, *sStub.Stub, *dStub.Stub) {
	t.Helper()
	m, err := New("stub://", "stub://")
	if err != nil {
		t.Fatal(err)
	}
	src := m.sourceDrv.(*sStub.Stub)
	src.Migrations = source.NewMigrations()
	for _, v := range []uint{1, 3, 4, 7} {
		src.Migrations.Append(&source.Migration{Version: v, Direction: source.Up, Identifier: fmt.Sprintf("CREATE %d", v)})
		src.Migrations.Append(&source.Migration{Version: v, Direction: source.Down, Identifier: fmt.Sprintf("DROP %d", v)})
	}
	return m, src, m.databaseDrv.(*dStub.Stub)
}

func appliedVersions(t *testing.T, db *dStub.Stub) []uint {
	t.Helper()
	vs := make([]uint, 0, len(db.Rows))
	for _, r := range db.Rows {
		vs = append(vs, r.Version)
	}
	return vs
}

func mustUp(t *testing.T, m *Migrate) {
	t.Helper()
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
}

func TestUpOneAtATime(t *testing.T) {
	m, _, db := newStubMigrate(t)

	mustUp(t, m)
	if vs := appliedVersions(t, db); !reflect.DeepEqual(vs, []uint{1}) {
		t.Fatalf("after first Up: %v", vs)
	}
	row := db.Rows[0]
	if row.Filename != "1.up.stub" || row.MD5Sum != md5hex([]byte("CREATE 1")) || row.AppliedAt.IsZero() {
		t.Fatalf("bad ledger row: %+v", row)
	}

	mustUp(t, m)
	mustUp(t, m)
	mustUp(t, m)
	if vs := appliedVersions(t, db); !reflect.DeepEqual(vs, []uint{1, 3, 4, 7}) {
		t.Fatalf("after four Ups: %v", vs)
	}
	if !db.EqualSequence([]string{"CREATE 1", "CREATE 3", "CREATE 4", "CREATE 7"}) {
		t.Fatalf("unexpected run sequence: %v", db.MigrationSequence)
	}

	if err := m.Up(); !errors.Is(err, ErrNoChange) {
		t.Fatalf("expected ErrNoChange, got %v", err)
	}
}

func TestDownOneAtATime(t *testing.T) {
	m, _, db := newStubMigrate(t)
	mustUp(t, m)
	mustUp(t, m)

	if err := m.Down(); err != nil {
		t.Fatal(err)
	}
	if vs := appliedVersions(t, db); !reflect.DeepEqual(vs, []uint{1}) {
		t.Fatalf("after Down: %v", vs)
	}
	if db.MigrationSequence[len(db.MigrationSequence)-1] != "DROP 3" {
		t.Fatalf("expected DROP 3 to run, got %v", db.MigrationSequence)
	}

	if err := m.Down(); err != nil {
		t.Fatal(err)
	}
	if err := m.Down(); !errors.Is(err, ErrNoChange) {
		t.Fatalf("expected ErrNoChange, got %v", err)
	}
}

func TestDownWithoutDownFile(t *testing.T) {
	m, src, _ := newStubMigrate(t)
	mustUp(t, m)
	src.Migrations = source.NewMigrations()
	src.Migrations.Append(&source.Migration{Version: 1, Direction: source.Up, Identifier: "CREATE 1"})

	var want ErrNoDownMigration
	if err := m.Down(); !errors.As(err, &want) || want.Version != 1 {
		t.Fatalf("expected ErrNoDownMigration{1}, got %v", err)
	}
}

func TestChecksumMismatch(t *testing.T) {
	m, src, _ := newStubMigrate(t)
	mustUp(t, m)

	// edit the applied migration locally
	src.Migrations.Up(1)
	up, _ := src.Migrations.Up(1)
	up.Identifier = "CREATE 1 -- edited"

	var want ErrChecksumMismatch
	for _, f := range []func() error{m.Up, m.Down, func() error { _, err := m.Status(); return err }} {
		err := f()
		if !errors.As(err, &want) {
			t.Fatalf("expected ErrChecksumMismatch, got %v", err)
		}
		if want.Version != 1 || want.Applied != md5hex([]byte("CREATE 1")) || want.Local != md5hex([]byte("CREATE 1 -- edited")) {
			t.Fatalf("unexpected error details: %+v", want)
		}
	}
}

func TestMissingLocal(t *testing.T) {
	m, src, _ := newStubMigrate(t)
	mustUp(t, m)
	mustUp(t, m)

	// delete version 1 locally, keep 3
	ms := source.NewMigrations()
	for _, v := range []uint{3, 4, 7} {
		up, _ := src.Migrations.Up(v)
		down, _ := src.Migrations.Down(v)
		ms.Append(up)
		ms.Append(down)
	}
	src.Migrations = ms

	var want ErrMissingLocal
	if err := m.Up(); !errors.As(err, &want) || want.Version != 1 || want.Filename != "1.up.stub" {
		t.Fatalf("expected ErrMissingLocal{1}, got %v", err)
	}
}

func TestOutOfOrder(t *testing.T) {
	m, src, _ := newStubMigrate(t)
	mustUp(t, m)
	mustUp(t, m)

	// someone adds version 2 after 3 was applied
	src.Migrations.Append(&source.Migration{Version: 2, Direction: source.Up, Identifier: "CREATE 2"})

	var want ErrOutOfOrder
	if err := m.Up(); !errors.As(err, &want) || want.Version != 2 || want.MaxApplied != 3 {
		t.Fatalf("expected ErrOutOfOrder{2,3}, got %v", err)
	}
	if err := m.Down(); !errors.As(err, &want) {
		t.Fatalf("Down must also fail on out of order, got %v", err)
	}
}

func TestStatus(t *testing.T) {
	m, _, _ := newStubMigrate(t)
	st, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Applied) != 0 || !reflect.DeepEqual(st.Pending, []uint{1, 3, 4, 7}) {
		t.Fatalf("empty ledger status: %+v", st)
	}

	mustUp(t, m)
	mustUp(t, m)
	st, err = m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Applied) != 2 || st.Applied[1].Version != 3 || !reflect.DeepEqual(st.Pending, []uint{4, 7}) {
		t.Fatalf("status after two Ups: %+v", st)
	}
}

func TestForceUnforce(t *testing.T) {
	m, _, db := newStubMigrate(t)
	mustUp(t, m)

	if err := m.Force(3); err != nil {
		t.Fatal(err)
	}
	if vs := appliedVersions(t, db); !reflect.DeepEqual(vs, []uint{1, 3}) {
		t.Fatalf("after Force(3): %v", vs)
	}
	if db.Rows[1].MD5Sum != md5hex([]byte("CREATE 3")) {
		t.Fatalf("Force must record local md5, got %+v", db.Rows[1])
	}
	if !db.EqualSequence([]string{"CREATE 1"}) {
		t.Fatalf("Force must not run SQL, sequence: %v", db.MigrationSequence)
	}
	if err := m.Force(3); err == nil {
		t.Fatal("Force on applied version must fail")
	}
	if err := m.Force(99); err == nil {
		t.Fatal("Force on unknown version must fail")
	}

	// next Up continues after the forced version
	mustUp(t, m)
	if vs := appliedVersions(t, db); !reflect.DeepEqual(vs, []uint{1, 3, 4}) {
		t.Fatalf("after Up following Force: %v", vs)
	}

	if err := m.Unforce(4); err != nil {
		t.Fatal(err)
	}
	if vs := appliedVersions(t, db); !reflect.DeepEqual(vs, []uint{1, 3}) {
		t.Fatalf("after Unforce(4): %v", vs)
	}
	if !db.EqualSequence([]string{"CREATE 1", "CREATE 4"}) {
		t.Fatalf("Unforce must not run SQL, sequence: %v", db.MigrationSequence)
	}
	if err := m.Unforce(4); err == nil {
		t.Fatal("Unforce on unapplied version must fail")
	}
}

func TestDrop(t *testing.T) {
	m, _, db := newStubMigrate(t)
	mustUp(t, m)
	if err := m.Drop(); err != nil {
		t.Fatal(err)
	}
	if len(db.Rows) != 0 || db.MigrationSequence[len(db.MigrationSequence)-1] != dStub.DROP {
		t.Fatalf("drop: rows=%v seq=%v", db.Rows, db.MigrationSequence)
	}
}

func TestUpUnlocksOnError(t *testing.T) {
	m, src, _ := newStubMigrate(t)
	mustUp(t, m)
	up, _ := src.Migrations.Up(1)
	up.Identifier = "edited"
	if err := m.Up(); err == nil {
		t.Fatal("expected error")
	}
	// lock must have been released
	if err := m.lock(); err != nil {
		t.Fatalf("expected lock to be free, got %v", err)
	}
}
