package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// semverRe validates the version argument to -set.
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// site is one file that pins the client version. pattern must contain exactly
// one capturing group around the version token; everything else is preserved
// verbatim on rewrite.
type site struct {
	path    string
	pattern *regexp.Regexp
}

// sites lists every location that must agree with config.go's const. config.go
// is listed first and treated as the source of truth by -check.
func sites() []site {
	return []site{
		{
			path:    "config.go",
			pattern: regexp.MustCompile(`(?m)^const defaultClientVersion = "(\d+\.\d+\.\d+)"`),
		},
		{
			path:    "config.example.yaml",
			pattern: regexp.MustCompile(`(?m)^\s*client_version:\s*"(\d+\.\d+\.\d+)"`),
		},
		{
			path:    filepath.Join("charts", "ai-proxy", "values.yaml"),
			pattern: regexp.MustCompile(`(?m)^\s*clientVersion:\s*"(\d+\.\d+\.\d+)"`),
		},
		{
			path:    filepath.Join("charts", "ai-proxy", "README.md"),
			pattern: regexp.MustCompile("(?m)^\\| config.clientVersion \\| string \\| `\"(\\d+\\.\\d+\\.\\d+)\"`"),
		},
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("client-version", flag.ContinueOnError)
	set := fs.String("set", "", "new version to write everywhere (e.g. 1.4.8)")
	check := fs.Bool("check", false, "verify all locations agree with config.go; exit non-zero on drift")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*set == "") == (!*check) {
		return fmt.Errorf("provide exactly one of -set <version> or -check")
	}

	root, err := repoRoot()
	if err != nil {
		return err
	}

	if *check {
		return runCheck(root, stdout)
	}

	if !semverRe.MatchString(*set) {
		return fmt.Errorf("invalid version %q: must be semver like 1.4.8", *set)
	}
	return runSet(root, *set, stdout)
}

func runCheck(root string, stdout io.Writer) error {
	var want string
	var mismatches []string
	for i, s := range sites() {
		got, err := readVersion(root, s)
		if err != nil {
			return err
		}
		if i == 0 {
			want = got // config.go is the source of truth
			continue
		}
		if got != want {
			mismatches = append(mismatches, fmt.Sprintf("  %s has %q, want %q", s.path, got, want))
		}
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("client version drift (source of truth config.go = %q):\n%s\nrun: go run ./cmd/client-version -set %s", want, join(mismatches), want)
	}
	fmt.Fprintf(stdout, "client version consistent: %s\n", want)
	return nil
}

func runSet(root, version string, stdout io.Writer) error {
	for _, s := range sites() {
		changed, old, err := writeVersion(root, s, version)
		if err != nil {
			return err
		}
		switch {
		case !changed:
			fmt.Fprintf(stdout, "  %s already %s\n", s.path, version)
		default:
			fmt.Fprintf(stdout, "  %s %s -> %s\n", s.path, old, version)
		}
	}
	fmt.Fprintf(stdout, "client version set to %s\n", version)
	return nil
}

func readVersion(root string, s site) (string, error) {
	full := filepath.Join(root, s.path)
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", s.path, err)
	}
	m := s.pattern.FindSubmatch(data)
	if m == nil {
		return "", fmt.Errorf("%s: no client-version line matched the expected pattern", s.path)
	}
	return string(m[1]), nil
}

// writeVersion rewrites only the captured version token, leaving the rest of the
// line (and the file's line endings) untouched. Returns whether it changed.
func writeVersion(root string, s site, version string) (changed bool, old string, err error) {
	full := filepath.Join(root, s.path)
	data, err := os.ReadFile(full)
	if err != nil {
		return false, "", fmt.Errorf("read %s: %w", s.path, err)
	}
	loc := s.pattern.FindSubmatchIndex(data)
	if loc == nil {
		return false, "", fmt.Errorf("%s: no client-version line matched the expected pattern", s.path)
	}
	// loc[2]:loc[3] is the capturing group (the version token).
	old = string(data[loc[2]:loc[3]])
	if old == version {
		return false, old, nil
	}
	out := make([]byte, 0, len(data)+len(version)-len(old))
	out = append(out, data[:loc[2]]...)
	out = append(out, version...)
	out = append(out, data[loc[3]:]...)
	if err := os.WriteFile(full, out, 0o644); err != nil {
		return false, "", fmt.Errorf("write %s: %w", s.path, err)
	}
	return true, old, nil
}

// repoRoot walks up from the executable's working directory to find go.mod so
// the tool works regardless of where it is invoked from.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repo root (no go.mod found)")
		}
		dir = parent
	}
}

func join(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
