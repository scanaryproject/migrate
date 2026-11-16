// Package testing has the database tests.
// All database drivers must pass the Test function.
// This lives in it's own package so it stays a test dependency.
package testing

import (
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

// TestMigrate runs integration-tests between the Migrate layer and database implementations.
func TestMigrate(t *testing.T, m *migrate.Migrate) {
	TestMigrateUp(t, m)
	TestMigrateDown(t, m)
	TestMigrateDrop(t, m)
}

// TestMigrateUp steps up one migration at a time until nothing is pending.
func TestMigrateUp(t *testing.T, m *migrate.Migrate) {
	t.Log("UP")
	for {
		err := m.Up()
		if errors.Is(err, migrate.ErrNoChange) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestMigrateDown steps down one migration at a time until the ledger is empty.
func TestMigrateDown(t *testing.T, m *migrate.Migrate) {
	t.Log("DOWN")
	for {
		err := m.Down()
		if errors.Is(err, migrate.ErrNoChange) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Regression test for preventing a regression for #164 https://github.com/golang-migrate/migrate/pull/173
// Tests the dropping mechanism through the Migrate logic to check for double-locking.
func TestMigrateDrop(t *testing.T, m *migrate.Migrate) {
	if err := m.Drop(); err != nil {
		t.Fatal(err)
	}
}
