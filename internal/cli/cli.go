// Package cli validates one restore invocation without accessing local or
// remote resources.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Invocation contains the normalized, structurally valid input for one
// restore operation. Resource and target inspection belongs to later stages.
type Invocation struct {
	Source       string
	Latest       bool
	Target       string
	Mode         string
	Confirm      bool
	Staging      string
	SkipChecksum bool
	EnvFile      string
}

// DiagnoseInvocation contains the normalized input for a read-only source
// diagnosis.
type DiagnoseInvocation struct {
	Source  string
	Latest  bool
	EnvFile string
}

// ListInvocation contains the normalized input for a read-only S3 listing.
type ListInvocation struct {
	Source  string
	EnvFile string
}

// Parse parses and validates command-line input. It deliberately performs no
// filesystem or network access, so invalid requests fail before any resource
// can be touched.
func Parse(args []string) (Invocation, error) {
	var invocation Invocation
	flags := newRestoreFlagSet(os.Stderr, &invocation)

	if err := flags.Parse(args); err != nil {
		return Invocation{}, err
	}
	if flags.NArg() != 0 {
		return Invocation{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}

	if err := validate(invocation); err != nil {
		return Invocation{}, err
	}
	return invocation, nil
}

// ParseDiagnose parses the arguments for the read-only diagnose subcommand.
func ParseDiagnose(args []string) (DiagnoseInvocation, error) {
	var invocation DiagnoseInvocation
	flags := newDiagnoseFlagSet(os.Stderr, &invocation)

	if err := flags.Parse(args); err != nil {
		return DiagnoseInvocation{}, err
	}
	if flags.NArg() != 0 {
		return DiagnoseInvocation{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	if invocation.Source == "" {
		return DiagnoseInvocation{}, errors.New("--source is required")
	}
	if invocation.Latest {
		if err := validateS3Prefix(invocation.Source); err != nil {
			return DiagnoseInvocation{}, fmt.Errorf("--latest requires an S3 prefix source ending in /: %w", err)
		}
	} else if isS3Prefix(invocation.Source) {
		return DiagnoseInvocation{}, errors.New("an S3 prefix source ending in / requires --latest")
	} else if err := validateSource(invocation.Source); err != nil {
		return DiagnoseInvocation{}, err
	}
	return invocation, nil
}

// ParseList parses the arguments for the read-only list subcommand.
func ParseList(args []string) (ListInvocation, error) {
	var invocation ListInvocation
	flags := newListFlagSet(os.Stderr, &invocation)
	if err := flags.Parse(args); err != nil {
		return ListInvocation{}, err
	}
	if flags.NArg() != 0 {
		return ListInvocation{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	if invocation.Source == "" {
		return ListInvocation{}, errors.New("--source is required")
	}
	if err := validateS3Prefix(invocation.Source); err != nil {
		return ListInvocation{}, err
	}
	return invocation, nil
}

// PrintUsage writes the root command's usage and all supported restore flags.
func PrintUsage(w io.Writer) {
	var invocation Invocation
	flags := newRestoreFlagSet(w, &invocation)
	_, _ = fmt.Fprintln(w, "Usage: coolrestore [flags]")
	flags.PrintDefaults()
	_, _ = fmt.Fprintln(w, "       coolrestore diagnose --source SOURCE [--latest] [--env-file PATH]")
	_, _ = fmt.Fprintln(w, "       coolrestore list --source PREFIX [--env-file PATH]")
	_, _ = fmt.Fprintln(w, "       coolrestore --version")
}

// PrintDiagnoseUsage writes the diagnose subcommand's usage and flags.
func PrintDiagnoseUsage(w io.Writer) {
	var invocation DiagnoseInvocation
	flags := newDiagnoseFlagSet(w, &invocation)
	_, _ = fmt.Fprintln(w, "Usage: coolrestore diagnose --source SOURCE [--latest] [--env-file PATH]")
	flags.PrintDefaults()
}

// PrintListUsage writes the list subcommand's usage and flags.
func PrintListUsage(w io.Writer) {
	var invocation ListInvocation
	flags := newListFlagSet(w, &invocation)
	_, _ = fmt.Fprintln(w, "Usage: coolrestore list --source PREFIX [--env-file PATH]")
	flags.PrintDefaults()
}

func newRestoreFlagSet(output io.Writer, invocation *Invocation) *flag.FlagSet {
	flags := flag.NewFlagSet("coolrestore", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&invocation.Source, "source", "", "local archive path or s3://bucket/object-key")
	flags.BoolVar(&invocation.Latest, "latest", false, "select the newest .tar.gz object below an S3 prefix source")
	flags.StringVar(&invocation.Target, "target", "", "absolute restore target directory")
	flags.StringVar(&invocation.Mode, "mode", "merge", "restore mode: merge or replace")
	flags.BoolVar(&invocation.Confirm, "confirm", false, "authorize target changes")
	flags.StringVar(&invocation.Staging, "staging", "", "staging base directory")
	flags.BoolVar(&invocation.SkipChecksum, "skip-checksum", false, "skip size-based integrity verification")
	flags.StringVar(&invocation.EnvFile, "env-file", "", "explicit S3 environment file")
	return flags
}

func newListFlagSet(output io.Writer, invocation *ListInvocation) *flag.FlagSet {
	flags := flag.NewFlagSet("coolrestore list", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&invocation.Source, "source", "", "S3 prefix to list; must end with /")
	flags.StringVar(&invocation.EnvFile, "env-file", "", "explicit S3 environment file")
	return flags
}

func newDiagnoseFlagSet(output io.Writer, invocation *DiagnoseInvocation) *flag.FlagSet {
	flags := flag.NewFlagSet("coolrestore diagnose", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&invocation.Source, "source", "", "local archive path or s3://bucket/object-key")
	flags.BoolVar(&invocation.Latest, "latest", false, "select the newest .tar.gz object below an S3 prefix source")
	flags.StringVar(&invocation.EnvFile, "env-file", "", "explicit S3 environment file")
	return flags
}

func validate(invocation Invocation) error {
	if invocation.Source == "" {
		return errors.New("--source is required")
	}
	if invocation.Latest {
		if err := validateS3Prefix(invocation.Source); err != nil {
			return fmt.Errorf("--latest requires an S3 prefix source ending in /: %w", err)
		}
	} else if isS3Prefix(invocation.Source) {
		return errors.New("an S3 prefix source ending in / requires --latest")
	} else if err := validateSource(invocation.Source); err != nil {
		return err
	}

	if invocation.Target == "" {
		return errors.New("--target is required")
	}
	if !filepath.IsAbs(invocation.Target) {
		return fmt.Errorf("--target must be an absolute path: %q", invocation.Target)
	}
	if isProtectedTarget(invocation.Target) {
		return fmt.Errorf("--target is a protected system directory: %q", invocation.Target)
	}

	switch invocation.Mode {
	case "merge":
	case "replace":
		if !invocation.Confirm {
			return errors.New("--mode=replace requires --confirm")
		}
	default:
		return fmt.Errorf("--mode must be merge or replace, got %q", invocation.Mode)
	}

	if invocation.Staging != "" && !filepath.IsAbs(invocation.Staging) {
		return fmt.Errorf("--staging must be an absolute path: %q", invocation.Staging)
	}
	return nil
}

func validateSource(source string) error {
	if strings.HasPrefix(source, "s3://") {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Scheme != "s3" || parsed.Host == "" || parsed.Path == "" || parsed.Path == "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("--source must be an S3 URI in the form s3://bucket/object-key: %q", source)
		}
		return nil
	}
	if !filepath.IsAbs(source) {
		return fmt.Errorf("--source must be an absolute local path or an S3 URI: %q", source)
	}
	return nil
}

func validateS3Prefix(prefix string) error {
	if !strings.HasPrefix(prefix, "s3://") {
		return fmt.Errorf("S3 prefix must be an S3 URI: %q", prefix)
	}
	parsed, err := url.Parse(prefix)
	if err != nil || parsed.Scheme != "s3" || parsed.Host == "" || parsed.Path == "" || parsed.Path == "/" || !strings.HasSuffix(parsed.Path, "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("S3 prefix must be in the form s3://bucket/object-prefix/: %q", prefix)
	}
	return nil
}

func isS3Prefix(source string) bool {
	return strings.HasPrefix(source, "s3://") && strings.HasSuffix(source, "/")
}

func isProtectedTarget(target string) bool {
	return isProtectedTargetForOS(target, runtime.GOOS)
}

func isProtectedTargetForOS(target, goos string) bool {
	clean := filepath.Clean(target)
	if clean == string(filepath.Separator) {
		return true
	}

	protected := protectedTargets(goos)
	for _, path := range protected {
		if clean == filepath.Clean(path) {
			return true
		}
	}
	return false
}

func protectedTargets(goos string) []string {
	switch goos {
	case "windows":
		return []string{`C:\`, `C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)`, `C:\ProgramData`, `C:\Users`, `C:\Recovery`, `C:\System Volume Information`}
	case "darwin":
		return []string{"/System", "/Library", "/Applications", "/Users", "/Volumes"}
	default:
		return []string{"/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib64", "/media", "/mnt", "/opt", "/proc", "/root", "/run", "/sbin", "/srv", "/sys", "/usr", "/var"}
	}
}
