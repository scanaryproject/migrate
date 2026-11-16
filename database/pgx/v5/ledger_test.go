package pgx

import (
	"errors"
	"strings"
	"testing"

	"github.com/dhui/dktest"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/dktesting"
)

// openDriver opens a fresh driver against the container, failing the test on error.
func openDriver(t *testing.T, c dktest.ContainerInfo, options ...string) database.Driver {
	t.Helper()
	ip, port, err := c.FirstPort()
	if err != nil {
		t.Fatal(err)
	}
	p := &Postgres{}
	d, err := p.Open(pgConnectionString(ip, port, options...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	return d
}

func tableExists(t *testing.T, d database.Driver, table string) bool {
	t.Helper()
	p := d.(*Postgres)
	var n int
	if err := p.conn.QueryRowContext(t.Context(), `SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1`, table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// TestApplyTransactional: a failing statement rolls back both the migration body and the ledger row.
func TestApplyTransactional(t *testing.T) {
	dktesting.ParallelTest(t, specs, func(t *testing.T, c dktest.ContainerInfo) {
		d := openDriver(t, c)
		body := []byte("CREATE TABLE tx_ok (id int); CREATE TABLEE tx_broken (id int);")
		if err := d.Apply(row(1), body); err == nil {
			t.Fatal("expected error from broken SQL")
		}
		if tableExists(t, d, "tx_ok") {
			t.Error("tx_ok should have been rolled back")
		}
		if vs := appliedVersions(t, d); len(vs) != 0 {
			t.Error("ledger should be empty after rollback, got", vs)
		}

		// success path records the row
		if err := d.Apply(row(1), []byte("CREATE TABLE tx_ok (id int)")); err != nil {
			t.Fatal(err)
		}
		if !tableExists(t, d, "tx_ok") || len(appliedVersions(t, d)) != 1 {
			t.Fatal("expected tx_ok and one ledger row")
		}

		// failing revert keeps the row
		if err := d.Revert(1, []byte("DROP TABLE does_not_exist")); err == nil {
			t.Fatal("expected error from broken down SQL")
		}
		if len(appliedVersions(t, d)) != 1 {
			t.Fatal("ledger row must survive a failed revert")
		}
		if err := d.Revert(1, []byte("DROP TABLE tx_ok")); err != nil {
			t.Fatal(err)
		}
		if tableExists(t, d, "tx_ok") || len(appliedVersions(t, d)) != 0 {
			t.Fatal("expected tx_ok dropped and empty ledger")
		}
	})
}

// TestApplyNoTransactionMarker: the marker allows statements that cannot run inside a transaction.
func TestApplyNoTransactionMarker(t *testing.T) {
	dktesting.ParallelTest(t, specs, func(t *testing.T, c dktest.ContainerInfo) {
		d := openDriver(t, c, "x-multi-statement=true")
		if err := execSQL(d, "CREATE TABLE notx (foo text)"); err != nil {
			t.Fatal(err)
		}
		body := []byte(database.NoTransactionMarker + "\nCREATE INDEX CONCURRENTLY idx_notx ON notx (foo);")
		if err := d.Apply(row(1), body); err != nil {
			t.Fatal(err)
		}
		if vs := appliedVersions(t, d); len(vs) != 1 {
			t.Fatal("expected one ledger row, got", vs)
		}
	})
}

// TestLegacyTable: an old (version, dirty) table is rejected instead of silently reused.
func TestLegacyTable(t *testing.T) {
	dktesting.ParallelTest(t, specs, func(t *testing.T, c dktest.ContainerInfo) {
		d := openDriver(t, c)
		if err := execSQL(d, "CREATE TABLE legacy_migrations (version bigint not null primary key, dirty boolean not null)"); err != nil {
			t.Fatal(err)
		}
		ip, port, err := c.FirstPort()
		if err != nil {
			t.Fatal(err)
		}
		p := &Postgres{}
		d2, err := p.Open(pgConnectionString(ip, port, "x-migrations-table=legacy_migrations"))
		if d2 != nil {
			defer func() { _ = d2.Close() }()
		}
		if !errors.Is(err, ErrLegacyTable) || !strings.Contains(err.Error(), "legacy_migrations") {
			t.Fatalf("expected ErrLegacyTable mentioning the table, got %v", err)
		}
	})
}
