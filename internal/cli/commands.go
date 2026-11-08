package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

var (
	errInvalidSequenceWidth     = errors.New("digits must be positive")
	errIncompatibleSeqAndFormat = errors.New("the seq and format options are mutually exclusive")
	errInvalidTimeFormat        = errors.New("time format may not be empty")
)

func nextSeqVersion(matches []string, seqDigits int) (string, error) {
	if seqDigits <= 0 {
		return "", errInvalidSequenceWidth
	}

	nextSeq := uint64(1)

	if len(matches) > 0 {
		filename := matches[len(matches)-1]
		matchSeqStr := filepath.Base(filename)
		idx := strings.Index(matchSeqStr, "_")

		if idx < 1 { // Using 1 instead of 0 since there should be at least 1 digit
			return "", fmt.Errorf("malformed migration filename: %s", filename)
		}

		var err error
		matchSeqStr = matchSeqStr[0:idx]
		nextSeq, err = strconv.ParseUint(matchSeqStr, 10, 64)

		if err != nil {
			return "", err
		}

		nextSeq++
	}

	version := fmt.Sprintf("%0[2]*[1]d", nextSeq, seqDigits)

	if len(version) > seqDigits {
		return "", fmt.Errorf("next sequence number %s too large, at most %d digits are allowed", version, seqDigits)
	}

	return version, nil
}

func timeVersion(startTime time.Time, format string) (version string, err error) {
	switch format {
	case "":
		err = errInvalidTimeFormat
	case "unix":
		version = strconv.FormatInt(startTime.Unix(), 10)
	case "unixNano":
		version = strconv.FormatInt(startTime.UnixNano(), 10)
	default:
		version = startTime.Format(format)
	}

	return
}

// createCmd (meant to be called via a CLI command) creates a new migration
func createCmd(dir string, startTime time.Time, format string, name string, ext string, seq bool, seqDigits int, print bool) error {
	if seq && format != defaultTimeFormat {
		return errIncompatibleSeqAndFormat
	}

	var version string
	var err error

	dir = filepath.Clean(dir)
	ext = "." + strings.TrimPrefix(ext, ".")

	if seq {
		matches, err := filepath.Glob(filepath.Join(dir, "*"+ext))

		if err != nil {
			return err
		}

		version, err = nextSeqVersion(matches, seqDigits)

		if err != nil {
			return err
		}
	} else {
		version, err = timeVersion(startTime, format)

		if err != nil {
			return err
		}
	}

	versionGlob := filepath.Join(dir, version+"_*"+ext)
	matches, err := filepath.Glob(versionGlob)

	if err != nil {
		return err
	}

	if len(matches) > 0 {
		return fmt.Errorf("duplicate migration version: %s", version)
	}

	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	for _, direction := range []string{"up", "down"} {
		basename := fmt.Sprintf("%s_%s.%s%s", version, name, direction, ext)
		filename := filepath.Join(dir, basename)

		if err = createFile(filename); err != nil {
			return err
		}

		if print {
			absPath, _ := filepath.Abs(filename)
			log.Println(absPath)
		}
	}

	return nil
}

func createFile(filename string) error {
	// create exclusive (fails if file already exists)
	// os.Create() specifies 0666 as the FileMode, so we're doing the same
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)

	if err != nil {
		return err
	}

	return f.Close()
}

func upCmd(m *migrate.Migrate) error {
	return noChangeIsInfo(m.Up())
}

func downCmd(m *migrate.Migrate) error {
	return noChangeIsInfo(m.Down())
}

// noChangeIsInfo turns migrate.ErrNoChange into a log line instead of a failure.
func noChangeIsInfo(err error) error {
	if errors.Is(err, migrate.ErrNoChange) {
		log.Println(err)
		return nil
	}
	return err
}

func dropCmd(m *migrate.Migrate) error {
	return m.Drop()
}

func forceCmd(m *migrate.Migrate, v uint) error {
	return m.Force(v)
}

func unforceCmd(m *migrate.Migrate, v uint) error {
	return m.Unforce(v)
}

// statusCmd prints the ledger and pending versions as a table.
func statusCmd(m *migrate.Migrate, w io.Writer) error {
	st, err := m.Status()
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "VERSION\tFILENAME\tMD5\tAPPLIED")
	for _, a := range st.Applied {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", a.Version, a.Filename, a.MD5Sum, a.AppliedAt.UTC().Format(time.RFC3339))
	}
	for _, v := range st.Pending {
		_, _ = fmt.Fprintf(tw, "%d\t-\t-\tpending\n", v)
	}
	return tw.Flush()
}
