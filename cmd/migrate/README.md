# migrate CLI

## Installation

### Pre-built binary

Download from this fork's GitHub releases and put `migrate` on your `PATH`.

### With Go toolchain

```bash
$ go install -tags 'postgres pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

The build tags select database drivers: `postgres` (lib/pq) and/or `pgx5`
(pgx v5). The `file` source is always included.

## Usage

```bash
$ migrate -help
Usage: migrate OPTIONS COMMAND [arg...]
       migrate [ -version | -help ]

Options:
  -source          Location of the migrations (driver://url)
  -path            Shorthand for -source=file://path
  -database        Run migrations against this database (driver://url)
  -lock-timeout N  Allow N seconds to acquire database lock (default 15)
  -verbose         Print verbose logging
  -version         Print version
  -help            Print usage

Commands:
  create [-ext E] [-dir D] [-seq] [-digits N] [-format] [-tz] NAME
           Create a set of timestamped up/down migrations titled NAME, in directory D with extension E.
           Use -seq option to generate sequential up/down migrations with N digits.
           Use -format option to specify a Go time format string. Note: migrations with the same time cause "duplicate migration version" error.
           Use -tz option to specify the timezone that will be used when generating non-sequential migrations (defaults: UTC).

  up           Apply the next pending migration (exactly one)
  down         Revert the most recently applied migration (exactly one)
  status       Print applied migrations (version, filename, md5, time) and pending versions
  force V      Record version V as applied without running it
  unforce V    Remove version V from the ledger without running its down migration
  drop [-f]    Drop everything inside database
        Use -f to bypass confirmation
```

Apply the next pending migration:

```bash
$ migrate -source file://path/to/migrations -database postgres://localhost:5432/database up
```

Apply everything that is pending:

```bash
$ until ! migrate -path path/to/migrations -database "$DB" up 2>&1 | grep -v '^no change$'; do :; done
```

Exit code is 0 for `up`/`down` when there was nothing to do (`no change` is
printed), 1 on any error including checksum or ordering violations.

## Reading CLI arguments from somewhere else

### ENV variables

```bash
$ migrate -database "$MY_MIGRATE_DATABASE"
```

### JSON files

Check out https://stedolan.github.io/jq/

```bash
$ migrate -database "$(cat config.json | jq -r '.database')"
```

### YAML files

```bash
$ migrate -database "$(cat config/database.yml | ruby -ryaml -e "print YAML.load(STDIN.read)['database']")"
$ migrate -database "$(cat config/database.yml | python -c 'import yaml,sys;print yaml.safe_load(sys.stdin)["database"]')"
```
