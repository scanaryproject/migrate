package cli

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
)

const (
	defaultTimeFormat = "20060102150405"
	defaultTimezone   = "UTC"
	createUsage       = `create [-ext E] [-dir D] [-seq] [-digits N] [-format] [-tz] NAME
	   Create a set of timestamped up/down migrations titled NAME, in directory D with extension E.
	   Use -seq option to generate sequential up/down migrations with N digits.
	   Use -format option to specify a Go time format string. Note: migrations with the same time cause "duplicate migration version" error.
           Use -tz option to specify the timezone that will be used when generating non-sequential migrations (defaults: UTC).
`
	upUsage      = `up           Apply the next pending migration (exactly one)`
	downUsage    = `down         Revert the most recently applied migration (exactly one)`
	statusUsage  = `status       Print applied migrations (version, filename, md5, time) and pending versions`
	forceUsage   = `force V      Record version V as applied without running it`
	unforceUsage = `unforce V    Remove version V from the ledger without running its down migration`
	dropUsage    = `drop [-f]    Drop everything inside database
	Use -f to bypass confirmation`
)

func handleSubCmdHelp(help bool, usage string, flagSet *flag.FlagSet) {
	if help {
		fmt.Fprintln(os.Stderr, usage)
		flagSet.PrintDefaults()
		os.Exit(0)
	}
}

func newFlagSetWithHelp(name string) (*flag.FlagSet, *bool) {
	flagSet := flag.NewFlagSet(name, flag.ExitOnError)
	helpPtr := flagSet.Bool("help", false, "Print help information")
	return flagSet, helpPtr
}

// set main log
var log = &Log{}

func printUsageAndExit() {
	flag.Usage()

	// If a command is not found we exit with a status 2 to match the behavior
	// of flag.Parse() with flag.ExitOnError when parsing an invalid flag.
	os.Exit(2)
}

// parseVersionArg parses the single positional version argument of a sub command.
func parseVersionArg(fs *flag.FlagSet) uint {
	v, err := strconv.ParseUint(fs.Arg(0), 10, 64)
	if err != nil {
		log.fatal("error: can't read version argument V")
	}
	return uint(v)
}

// Main function of a cli application. It is public for backwards compatibility with `cli` package
func Main(version string) {
	helpPtr := flag.Bool("help", false, "")
	versionPtr := flag.Bool("version", false, "")
	verbosePtr := flag.Bool("verbose", false, "")
	lockTimeoutPtr := flag.Uint("lock-timeout", 15, "")
	pathPtr := flag.String("path", "", "")
	databasePtr := flag.String("database", "", "")
	sourcePtr := flag.String("source", "", "")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr,
			`Usage: migrate OPTIONS COMMAND [arg...]
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
  %s
  %s
  %s
  %s
  %s
  %s
  %s

Source drivers: `+strings.Join(source.List(), ", ")+`
Database drivers: `+strings.Join(database.List(), ", ")+"\n", createUsage, upUsage, downUsage, statusUsage, forceUsage, unforceUsage, dropUsage)
	}

	flag.Parse()

	// initialize logger
	log.verbose = *verbosePtr

	// show cli version
	if *versionPtr {
		fmt.Fprintln(os.Stderr, version)
		os.Exit(0)
	}

	// show help
	if *helpPtr {
		flag.Usage()
		os.Exit(0)
	}

	// translate -path into -source if given
	if *sourcePtr == "" && *pathPtr != "" {
		*sourcePtr = fmt.Sprintf("file://%v", *pathPtr)
	}

	// initialize migrate
	// don't catch migraterErr here and let each command decide
	// how it wants to handle the error
	migrater, migraterErr := migrate.New(*sourcePtr, *databasePtr)
	defer func() {
		if migraterErr == nil {
			if _, err := migrater.Close(); err != nil {
				log.Println(err)
			}
		}
	}()
	if migraterErr == nil {
		migrater.Log = log
		migrater.LockTimeout = time.Duration(int64(*lockTimeoutPtr)) * time.Second
	}

	startTime := time.Now()

	if len(flag.Args()) < 1 {
		printUsageAndExit()
	}
	args := flag.Args()[1:]

	// runCmd parses args for a sub command, requires a working migrater and runs f.
	// nargs is the exact number of positional arguments the command takes.
	runCmd := func(name, usage string, nargs int, f func(fs *flag.FlagSet) error) {
		fs, help := newFlagSetWithHelp(name)
		if err := fs.Parse(args); err != nil {
			log.fatalErr(err)
		}
		handleSubCmdHelp(*help, usage, fs)
		if fs.NArg() != nargs {
			log.fatal(fmt.Sprintf("error: %s takes exactly %d argument(s), got %d\n%s", name, nargs, fs.NArg(), usage))
		}
		if migraterErr != nil {
			log.fatalErr(migraterErr)
		}
		if err := f(fs); err != nil {
			log.fatalErr(err)
		}
		if log.verbose {
			log.Println("Finished after", time.Since(startTime))
		}
	}

	switch flag.Arg(0) {
	case "create":

		seq := false
		seqDigits := 6

		createFlagSet, help := newFlagSetWithHelp("create")
		extPtr := createFlagSet.String("ext", "", "File extension")
		dirPtr := createFlagSet.String("dir", "", "Directory to place file in (default: current working directory)")
		formatPtr := createFlagSet.String("format", defaultTimeFormat, `The Go time format string to use. If the string "unix" or "unixNano" is specified, then the seconds or nanoseconds since January 1, 1970 UTC respectively will be used. Caution, due to the behavior of time.Time.Format(), invalid format strings will not error`)
		timezoneName := createFlagSet.String("tz", defaultTimezone, `The timezone that will be used for generating timestamps (default: utc)`)
		createFlagSet.BoolVar(&seq, "seq", seq, "Use sequential numbers instead of timestamps (default: false)")
		createFlagSet.IntVar(&seqDigits, "digits", seqDigits, "The number of digits to use in sequences (default: 6)")

		if err := createFlagSet.Parse(args); err != nil {
			log.fatalErr(err)
		}

		handleSubCmdHelp(*help, createUsage, createFlagSet)

		if createFlagSet.NArg() == 0 {
			log.fatal("error: please specify name")
		}
		name := createFlagSet.Arg(0)

		if *extPtr == "" {
			log.fatal("error: -ext flag must be specified")
		}

		timezone, err := time.LoadLocation(*timezoneName)
		if err != nil {
			log.fatal(err)
		}

		if err := createCmd(*dirPtr, startTime.In(timezone), *formatPtr, name, *extPtr, seq, seqDigits, true); err != nil {
			log.fatalErr(err)
		}

	case "up":
		runCmd("up", upUsage, 0, func(*flag.FlagSet) error { return upCmd(migrater) })

	case "down":
		runCmd("down", downUsage, 0, func(*flag.FlagSet) error { return downCmd(migrater) })

	case "status":
		runCmd("status", statusUsage, 0, func(*flag.FlagSet) error { return statusCmd(migrater, os.Stdout) })

	case "force":
		runCmd("force", forceUsage, 1, func(fs *flag.FlagSet) error { return forceCmd(migrater, parseVersionArg(fs)) })

	case "unforce":
		runCmd("unforce", unforceUsage, 1, func(fs *flag.FlagSet) error { return unforceCmd(migrater, parseVersionArg(fs)) })

	case "drop":
		dropFlagSet, help := newFlagSetWithHelp("drop")
		forceDrop := dropFlagSet.Bool("f", false, "Force the drop command by bypassing the confirmation prompt")

		if err := dropFlagSet.Parse(args); err != nil {
			log.fatalErr(err)
		}

		handleSubCmdHelp(*help, dropUsage, dropFlagSet)

		if !*forceDrop {
			log.Println("Are you sure you want to drop the entire database schema? [y/N]")
			var response string
			_, _ = fmt.Scanln(&response)
			response = strings.ToLower(strings.TrimSpace(response))

			if response == "y" {
				log.Println("Dropping the entire database schema")
			} else {
				log.fatal("Aborted dropping the entire database schema")
			}
		}

		if migraterErr != nil {
			log.fatalErr(migraterErr)
		}

		if err := dropCmd(migrater); err != nil {
			log.fatalErr(err)
		}

		if log.verbose {
			log.Println("Finished after", time.Since(startTime))
		}

	default:
		printUsageAndExit()
	}
}
