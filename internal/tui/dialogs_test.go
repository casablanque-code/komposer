package tui

import (
	"testing"

	"github.com/casablanque-code/komposer/pkg/composer"
	tea "github.com/charmbracelet/bubbletea"
)

// These cover the pure, easily-testable pieces of the validation
// dialog's scroll/scrollbar math — the part of this sprint's several
// rounds of scroll bugs (and my own regression in between them) that
// went untested the whole time, verified only by isolated builds and
// manual reasoning. internal/tui had zero test files before this.

func TestValidationScrollWindow_ClampsToStartAndEnd(t *testing.T) {
	m := Model{height: 30}

	// Scrolling below 0 clamps to 0.
	scroll, _ := m.validationScrollWindow(-5, 100)
	if scroll != 0 {
		t.Errorf("scroll = %d, want 0 (clamped from -5)", scroll)
	}

	// Scrolling past the end clamps to the last full window.
	scroll, budget := m.validationScrollWindow(1000, 100)
	if scroll+budget != 100 {
		t.Errorf("scroll+budget = %d, want 100 (the report's full length)", scroll+budget)
	}
	if scroll < 0 {
		t.Errorf("scroll = %d, want >= 0", scroll)
	}
}

func TestValidationScrollWindow_NeverExceedsReportLength(t *testing.T) {
	m := Model{height: 30}
	// A short report shouldn't get a budget larger than it actually is
	// — otherwise the window would try to show lines past the end.
	_, budget := m.validationScrollWindow(0, 3)
	if budget > 3 {
		t.Errorf("budget = %d, want <= 3 for a 3-line report", budget)
	}
}

func TestValidationScrollWindow_AccountsForBannerAndLegend(t *testing.T) {
	// This is the exact bug from earlier in the sprint: bodyBudget
	// computed without knowing about the optional banner/legend chrome
	// overflowed the terminal, so the window claimed more room than
	// was actually on screen. A window that accounts for them should
	// report a strictly smaller (or at least not larger) budget than
	// one that doesn't, for the same terminal height and report.
	base := Model{height: 30}
	_, plainBudget := base.validationScrollWindow(0, 200)

	withBanner := Model{height: 30, validationDialog: validationDialog{
		actionMessage: "Moved FOO to .env",
		warnings:      []string{"one warning, so the legend line shows too"},
	}}
	_, chromeBudget := withBanner.validationScrollWindow(0, 200)

	if chromeBudget >= plainBudget {
		t.Errorf("budget with banner+legend = %d, want strictly less than plain budget %d",
			chromeBudget, plainBudget)
	}
}

func TestValidationScrollWindow_NeverGoesBelowOne(t *testing.T) {
	// A tiny terminal shouldn't produce a zero or negative budget,
	// which would make the report unrenderable (and would break the
	// maxScroll math below, which divides conceptually by budget).
	m := Model{height: 1}
	_, budget := m.validationScrollWindow(0, 50)
	if budget < 1 {
		t.Errorf("budget = %d, want >= 1 even for a tiny terminal", budget)
	}
}

func TestRenderValidationScrollbar_FullBarWhenEverythingFits(t *testing.T) {
	bars := renderValidationScrollbar(0, 10, 6)
	for i, b := range bars {
		if b != scrollbarThumbChar {
			t.Fatalf("bars[%d] = %q, want a solid thumb (%q) when the whole report fits", i, b, scrollbarThumbChar)
		}
	}
}

func TestRenderValidationScrollbar_ThumbAtTopWhenScrolledToStart(t *testing.T) {
	bars := renderValidationScrollbar(0, 10, 40)
	if bars[0] != scrollbarThumbChar {
		t.Fatalf("bars[0] = %q, want the thumb (%q) at scroll=0", bars[0], scrollbarThumbChar)
	}
	if bars[len(bars)-1] == scrollbarThumbChar {
		t.Fatalf("bars[last] = thumb, want track — the thumb shouldn't span the whole bar when there's more to scroll")
	}
}

func TestRenderValidationScrollbar_ThumbAtBottomWhenScrolledToEnd(t *testing.T) {
	// maxScroll = totalLines - visibleRows = 40 - 10 = 30
	bars := renderValidationScrollbar(30, 10, 40)
	if bars[len(bars)-1] != scrollbarThumbChar {
		t.Fatalf("bars[last] = %q, want the thumb (%q) at maxScroll", bars[len(bars)-1], scrollbarThumbChar)
	}
	if bars[0] == scrollbarThumbChar {
		t.Fatalf("bars[0] = thumb, want track — the thumb shouldn't span the whole bar when scrolled to the end")
	}
}

func TestRenderValidationScrollbar_NoPanicOnZeroVisibleRows(t *testing.T) {
	bars := renderValidationScrollbar(0, 0, 40)
	if len(bars) != 0 {
		t.Fatalf("len(bars) = %d, want 0 for zero visible rows", len(bars))
	}
}

func TestValidationDialogContentWidth_CapsToTerminalWidth(t *testing.T) {
	if got := validationDialogContentWidth(40); got > 40 {
		t.Errorf("validationDialogContentWidth(40) = %d, want <= 40", got)
	}
}

func TestValidationDialogContentWidth_PrefersDoubleTheNormalDialogWidth(t *testing.T) {
	// On a wide-enough terminal, the validation dialog should claim
	// meaningfully more width than dialogContentWidth's default —
	// that's the whole point of it having its own width function (see
	// its doc comment) rather than reusing dialogContentWidth.
	if got := validationDialogContentWidth(300); got <= dialogContentWidth(300) {
		t.Errorf("validationDialogContentWidth(300) = %d, want > dialogContentWidth(300) = %d",
			got, dialogContentWidth(300))
	}
}

func TestUpdateValidation_DownMovesToNextWarning(t *testing.T) {
	m := Model{
		height: 40,
		validationDialog: validationDialog{
			warnings:        []string{"first", "second", "third"},
			secretRefs:      []*composer.HardcodedSecret{nil, nil, nil},
			selectedWarning: 0,
		},
	}
	next, _ := m.updateValidation(tea.KeyMsg{Type: tea.KeyDown})
	got := next.(Model)
	if got.validationDialog.selectedWarning != 1 {
		t.Fatalf("selectedWarning = %d, want 1", got.validationDialog.selectedWarning)
	}
}

func TestUpdateValidation_DownStopsAtLastWarningThenScrolls(t *testing.T) {
	m := Model{
		// A small height, not 40: with a tall enough terminal the
		// whole (tiny) report already fits on screen and there's
		// nothing below the last warning to scroll to at all — scroll
		// staying 0 in that case is correct, not a bug, and would make
		// this test pass for the wrong reason. Shrinking the window
		// forces there to be real content below the last warning (the
		// Compose Spec section) that only a fallback to plain
		// scrolling can reach.
		height: 10,
		validationDialog: validationDialog{
			warnings:        []string{"first", "second"},
			secretRefs:      []*composer.HardcodedSecret{nil, nil},
			selectedWarning: 1, // already the last one
			scroll:          0,
		},
	}
	next, _ := m.updateValidation(tea.KeyMsg{Type: tea.KeyDown})
	got := next.(Model)
	if got.validationDialog.selectedWarning != 1 {
		t.Fatalf("selectedWarning = %d, want it to stay at 1 (the last warning)", got.validationDialog.selectedWarning)
	}
	// This is the "Down couldn't reach past the last warning" bug from
	// earlier in the sprint: once there's no further warning to select,
	// Down has to fall through to a plain scroll instead of doing
	// nothing, or content below the warnings (e.g. the Compose Spec
	// section) becomes permanently unreachable.
	if got.validationDialog.scroll == 0 {
		t.Fatalf("scroll stayed at 0 — Down should fall through to plain scrolling once past the last warning")
	}
}

func TestUpdateValidation_UpStopsAtFirstWarning(t *testing.T) {
	m := Model{
		height: 40,
		validationDialog: validationDialog{
			warnings:        []string{"first", "second"},
			secretRefs:      []*composer.HardcodedSecret{nil, nil},
			selectedWarning: 0,
		},
	}
	next, _ := m.updateValidation(tea.KeyMsg{Type: tea.KeyUp})
	got := next.(Model)
	if got.validationDialog.selectedWarning != 0 {
		t.Fatalf("selectedWarning = %d, want it to stay at 0", got.validationDialog.selectedWarning)
	}
}

func TestUpdateValidation_EnterOnFixableWarningOpensPicker(t *testing.T) {
	ref := &composer.HardcodedSecret{Service: "db", Key: "POSTGRES_PASSWORD"}
	m := Model{
		height: 40,
		validationDialog: validationDialog{
			warnings:        []string{"hardcoded secret"},
			secretRefs:      []*composer.HardcodedSecret{ref},
			selectedWarning: 0,
		},
	}
	next, _ := m.updateValidation(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if got.currentMode != modeSecretStrategy {
		t.Fatalf("currentMode = %v, want modeSecretStrategy", got.currentMode)
	}
	if got.secretStrategy.service != "db" || got.secretStrategy.key != "POSTGRES_PASSWORD" {
		t.Fatalf("secretStrategy = %+v, want service=db key=POSTGRES_PASSWORD", got.secretStrategy)
	}
}

func TestUpdateValidation_EnterOnNonFixableWarningDoesNothing(t *testing.T) {
	m := Model{
		height:      40,
		currentMode: modeValidation,
		validationDialog: validationDialog{
			warnings:        []string{"port exposed"},
			secretRefs:      []*composer.HardcodedSecret{nil},
			selectedWarning: 0,
		},
	}
	next, _ := m.updateValidation(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	// Per explicit design (see updateValidation's own comment): Enter
	// on a non-fixable warning must not close the dialog either —
	// Esc is the only way out.
	if got.currentMode != modeValidation {
		t.Fatalf("currentMode = %v, want it to stay modeValidation (Enter on a non-fixable warning must be a no-op)", got.currentMode)
	}
}

func TestUpdateValidation_EscClosesDialog(t *testing.T) {
	m := Model{height: 40, currentMode: modeValidation}
	next, _ := m.updateValidation(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(Model)
	if got.currentMode != modeNormal {
		t.Fatalf("currentMode = %v, want modeNormal after Esc", got.currentMode)
	}
}
