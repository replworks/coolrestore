// Package config loads the explicitly selected environment inputs used for
// S3-compatible archive acquisition.
package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

var supportedEnvironmentKeys = map[string]struct{}{
	"AWS_ACCESS_KEY_ID":       {},
	"AWS_SECRET_ACCESS_KEY":   {},
	"AWS_REGION":              {},
	"AWS_ENDPOINT_URL":        {},
	"AWS_S3_FORCE_PATH_STYLE": {},
}

// LoadEnvFile reads a simple KEY=VALUE file and applies its supported S3
// variables to the current process environment. It does not evaluate shell
// syntax. The file is parsed completely before any environment value is set.
func LoadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %q: %w", path, err)
	}
	defer file.Close()

	values, err := parseEnvFile(file)
	if err != nil {
		return fmt.Errorf("parsing %q: %w", path, err)
	}
	for key, value := range values {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("setting %s: %w", key, err)
		}
	}
	return nil
}

func parseEnvFile(reader io.Reader) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(reader)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || !isEnvironmentKey(key) {
			return nil, fmt.Errorf("line %d must contain a supported KEY=VALUE assignment", lineNumber)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("line %d contains duplicate key %s", lineNumber, key)
		}
		value = strings.TrimSpace(value)
		if strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("line %d contains an invalid value", lineNumber)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	return values, nil
}

func isEnvironmentKey(key string) bool {
	if _, ok := supportedEnvironmentKeys[key]; !ok {
		return false
	}
	return true
}
