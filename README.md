# migrate

Database migrations CLI for PostgreSQL, Liquibase-style. Fork of
[golang-migrate/migrate](https://github.com/golang-migrate/migrate) with a
different model:

* **Ledger, not a version pointer.** The database keeps one row per applied
  migration: `version`, `filename`, `md5sum`, `time_applied`.
* **One step per invocation.** `up` applies exactly the next pending migration,
  `down` reverts exactly the last applied one. No `up N`, no `goto`.
* **Checksums are enforced.** Before every `up`, `down` or `status` the md5 of
  every applied migration's local up file is compared with the ledger. A
  mismatch fails the command.
* **Order is enforced.** A local migration whose version is below the highest
  applied one but which is not itself applied is an error. So is an applied
  version whose local file is gone.
* **Atomic apply.** On Postgres the migration body and the ledger insert run in
  one transaction. A failing migration leaves no trace.

## Install

```bash
go install -tags 'postgres pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

or `make build` in this repo (produces `./migrate`), or grab a binary from the
GitHub releases of this fork.

## Usage

```bash
export DB='postgres://user:pass@localhost:5432/dbname?sslmode=disable'

migrate create -ext sql -dir db/migrations -seq create_users   # 000001_create_users.{up,down}.sql
migrate -path db/migrations -database "$DB" status             # ledger + pending versions
migrate -path db/migrations -database "$DB" up                 # apply next pending migration
migrate -path db/migrations -database "$DB" down               # revert last applied migration
migrate -path db/migrations -database "$DB" force 3            # record version 3 as applied, run nothing
migrate -path db/migrations -database "$DB" unforce 3          # forget version 3, run nothing
migrate -path db/migrations -database "$DB" drop -f            # drop every table in the schema
```

`status` output:

```
VERSION  FILENAME                 MD5                               APPLIED
1        000001_create_users.up.sql  64323e06e4ac48a19d5522589efb95f7  2026-11-08T15:02:46Z
2        -                        -                                 pending
```

Database drivers: `postgres://` (lib/pq) and `pgx5://` (pgx v5). See
[database/postgres](database/postgres) for URL options. Source drivers:
`file://` and Go `io/fs`.

## Ledger table

Created on first connect as `schema_migrations` (override with
`x-migrations-table`):

```sql
CREATE TABLE schema_migrations (
  version      bigint      NOT NULL PRIMARY KEY,
  filename     text        NOT NULL,
  md5sum       char(32)    NOT NULL,
  time_applied timestamptz NOT NULL DEFAULT now()
);
```

An existing table in the old golang-migrate shape `(version, dirty)` is
rejected with an error. Drop it, or point `x-migrations-table` elsewhere.

## Transactions

Each migration body is wrapped in `BEGIN ... COMMIT` together with its ledger
row. Do not put `BEGIN`/`COMMIT` in migration files yourself.

Statements that cannot run inside a transaction (`CREATE INDEX CONCURRENTLY`,
`ALTER TYPE ... ADD VALUE` on older Postgres, `VACUUM`) go in their own
migration file whose first line is:

```sql
-- migrate:no-transaction
CREATE INDEX CONCURRENTLY users_email_idx ON users (email);
```

The body then runs without a transaction and the ledger row is inserted
afterwards. If the body fails there is no row; fix the database by hand and
rerun `up`.

## Recovery

| Situation | What to do |
|-----------|------------|
| Migration failed (transactional) | Nothing was recorded. Fix the SQL, rerun `up`. |
| Migration failed (`no-transaction`) | Partially applied, nothing recorded. Clean up by hand, rerun `up`, or `force V` if it is effectively done. |
| `checksum mismatch` | Someone edited an applied up file. Revert the edit, or accept it with `unforce V` then `force V` (re-records with the new md5). |
| `is not applied but version N already is` | A lower version appeared after higher ones were applied. Renumber it above the max applied version, or `force V` if it was applied by other means. |
| `has no local up migration` | An applied file was deleted locally. Restore it or `unforce V`. |

`force` and `unforce` do not validate the ledger, so they can repair any of the
states above. Everything else refuses to touch the database until `status`
passes.

## Development

```bash
go test ./... -short             # unit tests, no Docker
go test ./database/...           # spins up Postgres 14..18 via Docker
golangci-lint run
```

See [MIGRATIONS.md](MIGRATIONS.md) for file naming rules and
[CONTRIBUTING.md](CONTRIBUTING.md).
