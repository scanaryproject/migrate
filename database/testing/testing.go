// Package testing has the database tests.
// All database drivers must pass the Test function.
// This lives in it's own package so it stays a test dependency.
package testing

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
)

// Test runs tests against database implementations.
func Test(t *testing.T, d database.Driver, migration []byte) {
	if migration == nil {
		t.Fatal("test must provide migration reader")
	}

	TestEmptyLedger(t, d) // test first
	TestLockAndUnlock(t, d)
	TestApplyRevert(t, d, migration)
	// Drop breaks the driver, so test it last.
	TestDrop(t, d)
}

func TestEmptyLedger(t *testing.T, d database.Driver) {
	rows, err := d.Applied()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("Applied: expected empty ledger, got %v", rows)
	}
}

func TestLockAndUnlock(t *testing.T, d database.Driver) {
	// add a timeout, in case there is a deadlock
	done := make(chan struct{})
	errs := make(chan error)

	go func() {
		timeout := time.After(15 * time.Second)
		for {
			select {
			case <-done:
				return
			case <-timeout:
				errs <- fmt.Errorf("timeout after 15 seconds, looks like a deadlock in Lock/UnLock\n%#v", d)
				return
			}
		}
	}()

	// run the locking test ...
	go func() {
		if err := d.Lock(); err != nil {
			errs <- err
			return
		}

		// try to acquire lock again
		if err := d.Lock(); err == nil {
			errs <- errors.New("lock: expected err not to be nil")
			return
		}

		// unlock
		if err := d.Unlock(); err != nil {
			errs <- err
			return
		}

		// try to lock
		if err := d.Lock(); err != nil {
			errs <- err
			return
		}
		if err := d.Unlock(); err != nil {
			errs <- err
			return
		}
		// notify everyone
		close(done)
	}()

	// wait for done or any error
	for {
		select {
		case <-done:
			return
		case err := <-errs:
			t.Fatal(err)
		}
	}
}

// TestApplyRevert applies a real migration, a record-only one, then reverts both.
func TestApplyRevert(t *testing.T, d database.Driver, migration []byte) {
	m1 := database.AppliedMigration{Version: 1, Filename: "1_test.up.sql", MD5Sum: "0123456789abcdef0123456789abcdef"}
	if err := d.Apply(m1, migration); err != nil {
		t.Fatal(err)
	}
	m2 := database.AppliedMigration{Version: 2, Filename: "2_forced.up.sql", MD5Sum: "fedcba9876543210fedcba9876543210"}
	if err := d.Apply(m2, nil); err != nil {
		t.Fatal(err)
	}

	rows, err := d.Applied()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("Applied: expected 2 rows, got %d: %v", len(rows), rows)
	}
	for i, want := range []database.AppliedMigration{m1, m2} {
		got := rows[i]
		if got.Version != want.Version || got.Filename != want.Filename || got.MD5Sum != want.MD5Sum {
			t.Errorf("row %d: got %+v want %+v", i, got, want)
		}
		if got.AppliedAt.IsZero() {
			t.Errorf("row %d: AppliedAt not set", i)
		}
	}

	if err := d.Apply(m2, nil); err == nil {
		t.Error("Apply: expected error on duplicate version")
	}

	if err := d.Revert(2, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Revert(1, []byte("")); err != nil {
		t.Fatal(err)
	}
	rows, err = d.Applied()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("Applied: expected empty ledger after revert, got %v", rows)
	}
}

func TestDrop(t *testing.T, d database.Driver) {
	if err := d.Drop(); err != nil {
		t.Fatal(err)
	}
}
