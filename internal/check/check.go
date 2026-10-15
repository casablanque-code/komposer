// Package check implements `komposer check`: a non-interactive run of the
// same validation the editor performs, for scripts and CI.
package check

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/casablanque-code/komposer/pkg/composer"
)

// Exit codes of `komposer check`.
const (
	ExitOK    = 0 // nothing to report (warnings alone don't fail, unless --strict)
	ExitFail  = 1 // the file has problems
	ExitUsage = 2 // bad arguments, or the file couldn't be read
)

// defaultFiles are the names Docker Compose itself looks for, in its order.
var defaultFiles = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
}

const usage = `Usage: komposer check [flags] [file]

Validate a Compose file without starting the editor. Three layers run:

  Errors      komposer's own semantic checks (missing image, bad ports,
              unknown depends_on targets, ...)
  Warnings    security and reliability advisories (published ports, Docker
              socket mounts, hardcoded secrets, mutable image tags, ...)
  Compose specification
              the official Compose Specification JSON schema

If no file is given, the first of compose.yaml, compose.yml,
docker-compose.yaml, docker-compose.yml found in the current directory
is checked.

Flags:
  --strict           also fail (exit 1) when there are warnings
  --format <fmt>     output format: text (default) or json
  -h, --help         show this help

Exit codes:
  0  no errors and no specification issues (and, with --strict, no warnings)
  1  problems found
  2  bad arguments, or the file could not be read

Examples:
  komposer check
  komposer check --strict compose.prod.yaml
  komposer check --format json | jq '.warnings[].message'
`

// Finding is one error or warning from komposer's own checks.
type Finding struct {
	Service string `json:"service,omitempty"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (f Finding) String() string {
	if f.Service != "" {
		return fmt.Sprintf("service '%s': %s: %s", f.Service, f.Field, f.Message)
	}
	return fmt.Sprintf("%s: %s", f.Field, f.Message)
}

// SpecReport is the result of validating against the official schema.
type SpecReport struct {
	Valid  bool     `json:"valid"`
	Issues []string `json:"issues"`
}

// Report is everything `komposer check` found in one file.
type Report struct {
	File     string     `json:"file"`
	Strict   bool       `json:"strict"`
	Passed   bool       `json:"passed"`
	Errors   []Finding  `json:"errors"`
	Warnings []Finding  `json:"warnings"`
	Spec     SpecReport `json:"spec"`
}

// Analyze runs all validation layers on the file at path. The error is
// only for problems reading the file; anything wrong with its content is
// reported in the Report.
func Analyze(path string, strict bool) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}

	rep := Report{
		File:     path,
		Strict:   strict,
		Errors:   []Finding{},
		Warnings: []Finding{},
		Spec:     SpecReport{Valid: true, Issues: []string{}},
	}

	spec, err := composer.ValidateYAMLAgainstSpec(data)
	if err != nil {
		// Not parseable as YAML at all: nothing else can run.
		rep.Spec = SpecReport{Valid: false, Issues: []string{err.Error()}}
		rep.Passed = false
		return rep, nil
	}
	rep.Spec.Valid = spec.Valid
	if len(spec.Issues) > 0 {
		rep.Spec.Issues = spec.Issues
	}

	cfg, err := composer.ImportYAML(path)
	if err != nil {
		rep.Errors = append(rep.Errors, Finding{Field: "import", Message: err.Error()})
	} else {
		res := cfg.Validate()
		for _, e := range res.Errors {
			rep.Errors = append(rep.Errors, Finding{Service: e.Service, Field: e.Field, Message: e.Message})
		}
		for _, w := range res.Warnings {
			rep.Warnings = append(rep.Warnings, Finding{Service: w.Service, Field: w.Field, Message: w.Message})
		}
	}

	rep.Passed = len(rep.Errors) == 0 && rep.Spec.Valid && !(strict && len(rep.Warnings) > 0)
	return rep, nil
}

// Styler colors the text output. Each func takes plain text and returns
// it styled; PlainStyler leaves everything as is.
type Styler struct {
	Error, Warn, OK, Dim, Bold func(string) string
}

// PlainStyler returns a Styler that adds no styling.
func PlainStyler() Styler {
	id := func(s string) string { return s }
	return Styler{Error: id, Warn: id, OK: id, Dim: id, Bold: id}
}

func (s Styler) withDefaults() Styler {
	id := func(s string) string { return s }
	for _, f := range []*func(string) string{&s.Error, &s.Warn, &s.OK, &s.Dim, &s.Bold} {
		if *f == nil {
			*f = id
		}
	}
	return s
}

type options struct {
	file   string
	strict bool
	format string
	help   bool
}

func parseArgs(args []string) (options, error) {
	opts := options{format: "text"}
	positional := func(a string) error {
		if opts.file != "" {
			return fmt.Errorf("expected a single file, got %q and %q", opts.file, a)
		}
		opts.file = a
		return nil
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			opts.help = true
		case a == "--strict":
			opts.strict = true
		case a == "--format":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--format needs a value (text or json)")
			}
			i++
			opts.format = args[i]
		case strings.HasPrefix(a, "--format="):
			opts.format = strings.TrimPrefix(a, "--format=")
		case a == "--":
			for _, rest := range args[i+1:] {
				if err := positional(rest); err != nil {
					return opts, err
				}
			}
			i = len(args)
		case strings.HasPrefix(a, "-") && a != "-":
			return opts, fmt.Errorf("unknown flag %q", a)
		default:
			if err := positional(a); err != nil {
				return opts, err
			}
		}
	}
	if opts.format != "text" && opts.format != "json" {
		return opts, fmt.Errorf("unknown format %q (use text or json)", opts.format)
	}
	return opts, nil
}

func findDefaultFile() (string, error) {
	for _, name := range defaultFiles {
		if st, err := os.Stat(name); err == nil && !st.IsDir() {
			return name, nil
		}
	}
	return "", fmt.Errorf("no compose file found in the current directory (looked for %s)",
		strings.Join(defaultFiles, ", "))
}

// Run executes `komposer check` with the arguments after "check" and
// returns the process exit code.
func Run(args []string, stdout, stderr io.Writer, st Styler) int {
	st = st.withDefaults()

	fail := func(err error) int {
		fmt.Fprintf(stderr, "komposer check: %v\n\nRun 'komposer check --help' for usage.\n", err)
		return ExitUsage
	}

	opts, err := parseArgs(args)
	if err != nil {
		return fail(err)
	}
	if opts.help {
		fmt.Fprint(stdout, usage)
		return ExitOK
	}

	path := opts.file
	if path == "" {
		if path, err = findDefaultFile(); err != nil {
			return fail(err)
		}
	}

	rep, err := Analyze(path, opts.strict)
	if err != nil {
		return fail(fmt.Errorf("cannot read %s: %w", path, err))
	}

	if opts.format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return fail(err)
		}
	} else {
		render(stdout, rep, st)
	}

	if rep.Passed {
		return ExitOK
	}
	return ExitFail
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func render(w io.Writer, rep Report, st Styler) {
	fmt.Fprintf(w, "%s %s\n\n", st.Bold("komposer check"), rep.File)

	if len(rep.Errors) == 0 {
		fmt.Fprintln(w, st.OK("✓ Errors: none"))
	} else {
		fmt.Fprintln(w, st.Error(fmt.Sprintf("✗ Errors (%d)", len(rep.Errors))))
		for _, f := range rep.Errors {
			fmt.Fprintf(w, "    %s\n", f)
		}
	}

	if len(rep.Warnings) == 0 {
		fmt.Fprintln(w, st.OK("✓ Warnings: none"))
	} else {
		fmt.Fprintln(w, st.Warn(fmt.Sprintf("⚠ Warnings (%d)", len(rep.Warnings))))
		for _, f := range rep.Warnings {
			fmt.Fprintf(w, "    %s\n", f)
		}
	}

	if rep.Spec.Valid {
		fmt.Fprintln(w, st.OK("✓ Compose specification: valid"))
	} else {
		fmt.Fprintln(w, st.Error(fmt.Sprintf("✗ Compose specification (%d): does not conform", len(rep.Spec.Issues))))
		for _, issue := range rep.Spec.Issues {
			fmt.Fprintf(w, "    %s\n", issue)
		}
	}

	fmt.Fprintln(w)
	if rep.Passed {
		line := "PASS"
		if n := len(rep.Warnings); n > 0 {
			line += " — " + plural(n, "warning") + " (use --strict to fail on warnings)"
		}
		fmt.Fprintln(w, st.OK(line))
		return
	}
	var parts []string
	if n := len(rep.Errors); n > 0 {
		parts = append(parts, plural(n, "error"))
	}
	if !rep.Spec.Valid {
		parts = append(parts, plural(len(rep.Spec.Issues), "specification issue"))
	}
	if n := len(rep.Warnings); n > 0 && rep.Strict {
		parts = append(parts, plural(n, "warning")+" (--strict)")
	}
	fmt.Fprintln(w, st.Error("FAIL — "+strings.Join(parts, ", ")))
}
