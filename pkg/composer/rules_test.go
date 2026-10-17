package composer

import (
	"os"
	"regexp"
	"testing"
)

var ruleIDPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func TestRuleRegistryIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range rules {
		if !ruleIDPattern.MatchString(r.ID) {
			t.Errorf("rule ID %q is not kebab-case", r.ID)
		}
		if seen[r.ID] {
			t.Errorf("duplicate rule ID %q", r.ID)
		}
		seen[r.ID] = true
		if r.Severity != SeverityError && r.Severity != SeverityWarning {
			t.Errorf("%s: bad severity %q", r.ID, r.Severity)
		}
		if r.Title == "" || r.Why == "" || r.Fix == "" {
			t.Errorf("%s: Title, Why and Fix must all be filled in", r.ID)
		}
	}
}

// sourceRuleUses finds every rule ID passed as the first argument of
// result.Add / result.AddWarning in the package's source files.
func sourceRuleUses(t *testing.T) (errors, warnings map[string]bool) {
	t.Helper()
	errors, warnings = map[string]bool{}, map[string]bool{}
	call := regexp.MustCompile(`result\.(Add|AddWarning)\(\s*"([^"]+)"`)
	for _, file := range []string{"validate.go", "security.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range call.FindAllSubmatch(src, -1) {
			if string(m[1]) == "Add" {
				errors[string(m[2])] = true
			} else {
				warnings[string(m[2])] = true
			}
		}
	}
	return errors, warnings
}

// The registry and the code must agree: every ID the checks emit is
// registered with the matching severity, and no registered rule is dead.
func TestRuleRegistryMatchesSource(t *testing.T) {
	errs, warns := sourceRuleUses(t)
	for id := range errs {
		if r, ok := LookupRule(id); !ok || r.Severity != SeverityError {
			t.Errorf("result.Add uses %q, which is not a registered error rule", id)
		}
	}
	for id := range warns {
		if r, ok := LookupRule(id); !ok || r.Severity != SeverityWarning {
			t.Errorf("result.AddWarning uses %q, which is not a registered warning rule", id)
		}
	}
	for _, r := range rules {
		if !errs[r.ID] && !warns[r.ID] {
			t.Errorf("rule %q is registered but nothing emits it", r.ID)
		}
	}
}

func TestEveryFindingCarriesARegisteredRule(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx
    ports: ["8080:80"]
    privileged: true
    depends_on: [ghost]
`)
	r := cfg.Validate()
	if len(r.Errors) == 0 || len(r.Warnings) == 0 {
		t.Fatalf("test document should produce both errors and warnings: %+v", r)
	}
	for _, f := range append(append([]ValidationError{}, r.Errors...), r.Warnings...) {
		if _, ok := LookupRule(f.Rule); !ok {
			t.Errorf("finding %q has unregistered rule %q", f.Message, f.Rule)
		}
	}
}

func TestRulesSortedErrorsFirst(t *testing.T) {
	list := Rules()
	seenWarning := false
	for _, r := range list {
		if r.Severity == SeverityWarning {
			seenWarning = true
		} else if seenWarning {
			t.Fatalf("error rule %q listed after a warning", r.ID)
		}
	}
}
