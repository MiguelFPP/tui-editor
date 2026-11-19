package gitpanel

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// TestSplitBodyKeepsExactRowsAndWidth checks the tiling at several widths.
func TestSplitBodyKeepsExactRowsAndWidth(t *testing.T) {
	cases := []struct{ width, height int }{
		{100, 20},
		{120, 40},
		{200, 16},
	}

	for _, tc := range cases {
		m := open(t, gitFake(t))
		m.SetSize(tc.width, tc.height)

		if !m.split {
			t.Fatalf("%dx%d: the panel must split", tc.width, tc.height)
		}

		lines := strings.Split(m.View(), "\n")
		body := m.body()

		if got, want := len(body), m.bodyHeight(); got != want {
			t.Errorf("%dx%d: body rows = %d, want %d", tc.width, tc.height, got, want)
		}
		for i, line := range body {
			if width := len([]rune(plain(line))); width != tc.width {
				t.Errorf("%dx%d: row %d width = %d, want %d", tc.width, tc.height, i, width, tc.width)
			}
		}
		if len(lines) != m.bodyHeight()+headerRows+footerRows {
			t.Errorf("%dx%d: View rows = %d, want %d",
				tc.width, tc.height, len(lines), m.bodyHeight()+headerRows+footerRows)
		}
	}
}

// TestSplitShowsListAndPreview composes both columns from one loaded diff.
func TestSplitShowsListAndPreview(t *testing.T) {
	m := open(t, twoHunkFake(t))
	m.SetSize(120, 20)

	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	body := plain(strings.Join(m.body(), "\n"))
	if !strings.Contains(body, "Cambios") && !strings.Contains(body, "normal.txt") {
		t.Errorf("left column missing the change list: %q", body)
	}

	m = goTo(t, m, Diff)
	body = plain(strings.Join(m.body(), "\n"))
	if !strings.Contains(body, "normal.txt · diff") {
		t.Errorf("right column missing the preview header: %q", body)
	}
	if !strings.Contains(body, "@@ -1,1 +1,1 @@") {
		t.Errorf("right column missing the first hunk: %q", body)
	}
}

// TestSplitColumnsScrollIndependently checks that one offset never moves the other.
func TestSplitColumnsScrollIndependently(t *testing.T) {
	m := open(t, twoHunkFake(t))
	m.SetSize(120, 8)

	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))
	m = goTo(t, m, Diff)

	if !m.split {
		t.Fatal("the panel must split")
	}

	before := m.scrollList
	m.scrollPreviewBy(2)

	if m.scrollList != before {
		t.Errorf("scrollList = %d, want %d untouched by the preview", m.scrollList, before)
	}
	if m.scrollPreview != 2 {
		t.Errorf("scrollPreview = %d, want 2", m.scrollPreview)
	}

	m.scrollPreviewBy(-5)
	if m.scrollPreview != 0 {
		t.Errorf("scrollPreview = %d, want it clamped at 0", m.scrollPreview)
	}

	m.scrollListBy(1)
	if m.scrollPreview != 0 {
		t.Errorf("scrollPreview = %d, want 0 after scrolling the list", m.scrollPreview)
	}
}

// TestSplitEmptyDiffKeepsTheList checks that the preview placeholder never blanks the left column.
func TestSplitEmptyDiffKeepsTheList(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(120, 20)

	body := plain(strings.Join(m.body(), "\n"))
	if !strings.Contains(body, "normal.txt") {
		t.Errorf("left column missing the change list: %q", body)
	}
	if !strings.Contains(body, "sin diff") {
		t.Errorf("right column missing the placeholder: %q", body)
	}
}

// TestSingleModeRenderIsUnchanged keeps the pre-existing layout below the threshold.
func TestSingleModeRenderIsUnchanged(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(80, 16)

	if m.split {
		t.Fatal("80 cells must stay in one column")
	}
	m.SetSize(80, 16)

	body := m.body()
	for i, line := range body {
		if width := len([]rune(plain(line))); width > 80 {
			t.Errorf("row %d width = %d, want at most 80", i, width)
		}
	}
	if strings.Contains(plain(strings.Join(body, "")), "sin diff") && m.section == Changes {
		t.Error("the single column must not show the preview placeholder in Changes")
	}
}

// TestSplitDropsWhenNarrowing checks the graceful fallback while a diff is loaded.
func TestSplitDropsWhenNarrowing(t *testing.T) {
	m := open(t, twoHunkFake(t))
	m.SetSize(120, 20)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	m.SetSize(80, 16)

	if m.split {
		t.Fatal("narrowing below the threshold must collapse the columns")
	}
	if m.previewW != 0 || m.previewRect.Dx() != 0 {
		t.Errorf("previewW, previewRect = %d, %d, want 0, 0", m.previewW, m.previewRect.Dx())
	}
	if got := len(m.body()); got != m.bodyHeight() {
		t.Errorf("body rows = %d, want %d", got, m.bodyHeight())
	}
}

// TestSplitFailureReplacesTheWholeBody keeps the error banner readable across both columns.
func TestSplitFailureReplacesTheWholeBody(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(120, 20)
	m.errs.status = domain.ErrNotRepository

	body := m.body()
	if len(body) != m.bodyHeight() {
		t.Fatalf("body rows = %d, want %d", len(body), m.bodyHeight())
	}
	text := plain(strings.Join(body, "\n"))
	if !strings.Contains(text, "no es un repositorio") {
		t.Fatalf("body = %q, want the failure line", text)
	}
}

// TestNarrowWidthsKeepTheLayoutIntact crosses the threshold in one-cell steps.
func TestNarrowWidthsKeepTheLayoutIntact(t *testing.T) {
	for _, width := range []int{40, 60, 99, 100, 101, 120} {
		for _, height := range []int{6, 10, 20, 40} {
			m := open(t, gitFake(t))
			m.SetSize(width, height)

			body := m.body()
			if got, want := len(body), m.bodyHeight(); got != want {
				t.Errorf("%dx%d: body rows = %d, want %d", width, height, got, want)
			}
			for index, line := range body {
				got := len([]rune(plain(line)))
				if m.split && got != width {
					t.Fatalf("%dx%d: row %d width = %d, want %d", width, height, index, got, width)
				}
				if got > width {
					t.Fatalf("%dx%d: row %d width = %d, want at most %d", width, height, index, got, width)
				}
			}
			if m.split != (width >= gitSplitMinWidth) {
				t.Errorf("%dx%d: split = %v, want %v", width, height, m.split, width >= gitSplitMinWidth)
			}
			if got := len(strings.Split(m.View(), "\n")); got != m.bodyHeight()+headerRows+footerRows {
				t.Errorf("%dx%d: View rows = %d, want %d",
					width, height, got, m.bodyHeight()+headerRows+footerRows)
			}
		}
	}
}

// TestHelpDocumentsTheLayoutToggle keeps the key discoverable.
func TestHelpDocumentsTheLayoutToggle(t *testing.T) {
	m := openWith(t, gitFake(t), overrideKeys(t, map[string][]string{"git_layout_toggle": {"g"}}))
	m.SetSize(120, 40)

	m = press(t, m, key('?'))
	view := plain(m.View())
	if !strings.Contains(view, "g") || !strings.Contains(view, "alternar una o dos columnas") {
		t.Errorf("View() = %q, want the toggle documented with its effective key", view)
	}

	m = press(t, m, key('?'))
	footer := m.footerText()
	if !strings.Contains(footer, "g: columnas") {
		t.Errorf("footer = %q, want the toggle hint", footer)
	}
}
