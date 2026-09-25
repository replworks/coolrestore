// Package cli validates one restore invocation without accessing local or
// remote resources.
package cli

import (
	"errors"
	"flag"
	"fmt"
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
	Target       string
	Mode         string
	Confirm      bool
	Staging      string
	SkipChecksum bool
}

// Parse parses and validates command-line input. It deliberately performs no
// filesystem or network access, so invalid requests fail before any resource
// can be touched.
func Parse(args []string) (Invocation, error) {
	flags := flag.NewFlagSet("coolrestore", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	var invocation Invocation
	flags.StringVar(&invocation.Source, "source", "", "local archive path or s3://bucket/object-key")
	flags.StringVar(&invocation.Target, "target", "", "absolute restore target directory")
	flags.StringVar(&invocation.Mode, "mode", "merge", "restore mode: merge or replace")
	flags.BoolVar(&invocation.Confirm, "confirm", false, "authorize target changes")
	flags.StringVar(&invocation.Staging, "staging", "", "staging base directory")
	flags.BoolVar(&invocation.SkipChecksum, "skip-checksum", false, "skip size-based integrity verification")

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

func validate(invocation Invocation) error {
	if invocation.Source == "" {
		return errors.New("--source is required")
	}
	if err := validateSource(invocation.Source); err != nil {
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
		return []string{`C:\\`, `C:\\Windows`, `C:\\Program Files`, `C:\\Program Files (x86)`, `C:\\ProgramData`, `C:\\Users`, `C:\\Recovery`, `C:\\System Volume Information`}
	case "darwin":
		return []string{"/System", "/Library", "/Applications", "/Users", "/Volumes"}
	default:
		return []string{"/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib64", "/media", "/mnt", "/opt", "/proc", "/root", "/run", "/sbin", "/srv", "/sys", "/usr", "/var"}
	}
}
