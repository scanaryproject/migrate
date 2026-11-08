# pgx

This package is for [pgx/v5](https://pkg.go.dev/github.com/jackc/pgx/v5). A backend for the older [pgx/v4](https://pkg.go.dev/github.com/jackc/pgx/v4). is [also available](..).

`pgx5://user:password@host:port/dbname?query`

| URL Query  | WithInstance Config | Description |
|------------|---------------------|-------------|
| `x-migrations-table` | `MigrationsTable` | Name of the migrations table |
| `x-migrations-table-quoted` | `MigrationsTableQuoted` | By default, migrate quotes the migration table for SQL injection safety reasons. This option disable quoting and naively checks that you have quoted the migration table name. e.g. `"my_schema"."schema_migrations"` |
| `x-statement-timeout` | `StatementTimeout` | Abort any statement that takes more than the specified number of milliseconds |
| `x-multi-statement` | `MultiStatementEnabled` | Enable multi-statement execution (default: false) |
| `x-multi-statement-max-size` | `MultiStatementMaxSize` | Maximum size of single statement in bytes (default: 10MB) |
| `dbname` | `DatabaseName` | The name of the database to connect to |
| `search_path` | | This variable specifies the order in which schemas are searched when an object is referenced by a simple name with no schema specified. |
| `user` | | The user to sign in as |
| `password` | | The user's password |
| `host` | | The host to connect to. Values that start with / are for unix domain sockets. (default is localhost) |
| `port` | | The port to bind to. (default is 5432) |
| `fallback_application_name` | | An application_name to fall back to if one isn't provided. |
| `connect_timeout` | | Maximum wait for connection, in seconds. Zero or not specified means wait indefinitely. |
| `sslcert` | | Cert file location. The file must contain PEM encoded data. |
| `sslkey` | | Key file location. The file must contain PEM encoded data. |
| `sslrootcert` | | The location of the root certificate file. The file must contain PEM encoded data. |
| `sslmode` | | Whether or not to use SSL (disable\|require\|verify-ca\|verify-full) |


## Ledger table

Created on first connect (needs CREATE on the schema; later connects only need
SELECT/INSERT/DELETE):

```sql
CREATE TABLE schema_migrations (
  version      bigint      NOT NULL PRIMARY KEY,
  filename     text        NOT NULL,
  md5sum       char(32)    NOT NULL,
  time_applied timestamptz NOT NULL DEFAULT now()
);
```

A table with the legacy golang-migrate shape `(version, dirty)` under the same
name is rejected with `legacy migrations table ... found`. Drop it or use
`x-migrations-table`.

## Transactions

`Apply` runs the migration body and the ledger `INSERT` in one transaction,
`Revert` runs the down body and the ledger `DELETE` in one transaction. A
failing statement rolls back everything. Do not write `BEGIN`/`COMMIT` in
migration files.

A body whose first line is `-- migrate:no-transaction` runs outside a
transaction (for `CREATE INDEX CONCURRENTLY` and friends); the ledger row is
written afterwards in its own statement.

## Multi-statement mode

Without `x-multi-statement`, the body is sent to Postgres in one `Exec`. With
it, the body is split on `;` and each statement is executed on its own, which
gives precise error positions and lets a `no-transaction` file hold several
statements that each must run outside a transaction.
