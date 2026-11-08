# Migrations

## Filename format

A migration is two files, one to move "up" to the version and one to move
"down" to the previous one:

    {version}_{title}.up.{extension}
    {version}_{title}.down.{extension}

`version` is any unsigned 64 bit integer. `title` is for humans. `extension`
is not checked; use `.sql` for SQL. Common schemes:

    1_initialize_schema.up.sql        000001_initialize_schema.up.sql   1500360784_initialize_schema.up.sql
    1_initialize_schema.down.sql      000001_initialize_schema.down.sql 1500360784_initialize_schema.down.sql

`migrate create -seq` produces the zero padded form, `migrate create` without
`-seq` produces timestamps.

## Rules the tool enforces

* **Both files must exist.** `up` fails if the version has no up file,
  `down` fails if it has no down file. An irreversible migration gets a down
  file containing only a comment.
* **Up files are immutable once applied.** The ledger stores the md5 of the up
  file body at apply time. Any later change to that file, including whitespace,
  fails every `up`, `down` and `status` until it is reverted or accepted with
  `unforce V` followed by `force V`. Down files are not checksummed and may be
  fixed after the fact.
* **Versions are applied in order.** A new file must have a version higher than
  everything already applied. If a lower version shows up (merge of an old
  branch), renumber it.
* **One step at a time.** `up` applies the lowest pending version, `down`
  reverts the highest applied one. Run them in a loop if you want more.

## Content

Files are passed to the database as-is. On Postgres the whole body runs inside
one transaction together with the ledger insert, so do not write
`BEGIN`/`COMMIT` yourself. Put a statement that cannot run in a transaction in
its own file starting with `-- migrate:no-transaction` (see README).

With `x-multi-statement=true` the body is split on `;` and executed statement
by statement, which gives better error positions. Without it the body is sent
in one round trip.

## Reversibility

Every up migration should have a down migration that restores the previous
state. Development is much easier when `down` followed by `up` is a no-op.
