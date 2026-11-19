package gitpanel

import (
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

// TestSplitBodyDrawsTwoFrames checks that each column is framed on its own.
func TestSplitBodyDrawsTwoFrames(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(120, 20)

	if !m.split {
		t.Fatal("120 cells must split")
	}

	body := m.body()
	top := plain(body[0])
	bottom := plain(body[len(body)-1])

	// The top row of both frames starts with the rounded corner, and the left frame closes before
	// the right one starts.
	if !strings.HasPrefix(strings.TrimRight(top, " "), "╭") {
		t.Errorf("top row = %q, want a rounded corner", top)
	}
	if !strings.HasPrefix(strings.TrimRight(bottom, " "), "╰") {
		t.Errorf("bottom row = %q, want the closing corner", bottom)
	}

	corner := strings.Count(top, "╭")
	if corner != 2 {
		t.Errorf("top row = %q, want two frames, got %d corners", top, corner)
	}

	// Every row of the body keeps the panel width.
	for index, line := range body {
		if got := len([]rune(plain(line))); got != m.width {
			t.Errorf("row %d width = %d, want %d", index, got, m.width)
		}
	}
}

// TestSplitBodyFramesUseTheBorderToken checks the frame honours the active palette.
func TestSplitBodyFramesUseTheBorderToken(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(120, 20)

	rendered := m.body()[0]
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("frame row = %q, want the border style applied", rendered)
	}
	if plain(rendered) == rendered {
		t.Errorf("frame row = %q, want ANSI escapes", rendered)
	}
}

// TestListColumnKeepsItsHeader checks the left column names itself and counts its rows.
func TestListColumnKeepsItsHeader(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(120, 20)

	body := plain(strings.Join(m.body(), "\n"))
	if !strings.Contains(body, "Cambios · 5") {
		t.Errorf("body = %q, want the list header with its count", body)
	}

	// The header sits on the first inner row, one cell inside the frame.
	header := plain(m.body()[1])
	if !strings.HasPrefix(header, "│Cambios · 5") {
		t.Errorf("header row = %q, want it inside the frame", header)
	}
}

// TestListClickLandsOnTheRowUnderTheCursorAfterFraming guards the new offsets.
func TestListClickLandsOnTheRowUnderTheCursorAfterFraming(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(120, 20)

	if !m.split {
		t.Fatal("120 cells must split")
	}

	// The header row of the list belongs to no row: the click is ignored.
	x := m.listRect.Min.X + 1
	header := clickAt(t, m, x, m.listRect.Min.Y-listHeaderRows)
	if got := header.cursors[Changes]; got != 0 {
		t.Errorf("cursors[Changes] = %d after clicking the header, want 0", got)
	}

	// The second visible row of the list is index 1.
	next := clickAt(t, header, x, header.listRect.Min.Y+1)
	if got := next.cursors[Changes]; got != 1 {
		t.Errorf("cursors[Changes] = %d, want 1", got)
	}
}

// TestSingleModeStillHasNoInnerFrames keeps the pre-frame render below the threshold.
func TestSingleModeStillHasNoInnerFrames(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(80, 16)

	if m.split {
		t.Fatal("80 cells must stay in one column")
	}

	body := strings.Join(m.body(), "\n")
	if strings.Contains(plain(body), "╭") {
		t.Errorf("body = %q, want no inner frames in single mode", plain(body))
	}
	if got := m.listRect.Dx(); got != m.width {
		t.Errorf("listRect width = %d, want the whole panel", got)
	}
}

// TestFramesFallBackWithoutRoomAvoidsAPanicOnTinyPanels.
func TestFramesFallBackWithoutRoom(t *testing.T) {
	for _, size := range []struct{ width, height int }{{100, 4}, {100, 3}, {60, 6}} {
		m := open(t, gitFake(t))
		m.SetSize(size.width, size.height)

		body := m.body()
		if got, want := len(body), m.bodyHeight(); got != want {
			t.Errorf("%dx%d: body rows = %d, want %d", size.width, size.height, got, want)
		}
	}
}

// TestFrameColumnFitsContent isolates the frame helper.
func TestFrameColumnFitsContent(t *testing.T) {
	rows := []string{"uno", "dos", "tres"}
	framed := frameColumn(rows, 12, 5)

	if len(framed) != 5 {
		t.Fatalf("frameColumn() = %d rows, want 5", len(framed))
	}
	for index, line := range framed {
		if got := len([]rune(plain(line))); got != 12 {
			t.Errorf("row %d width = %d, want 12", index, got)
		}
	}

	first, last := []rune(plain(framed[0])), []rune(plain(framed[4]))
	if first[0] != '╭' || last[0] != '╰' {
		t.Errorf("frame = %q, want rounded corners", plain(strings.Join(framed, "\n")))
	}

	// Without room for a frame the content is clipped and padded instead.
	tiny := frameColumn(rows, 2, 2)
	if len(tiny) != 2 {
		t.Fatalf("frameColumn() = %d rows, want 2", len(tiny))
	}
	if strings.Contains(plain(strings.Join(tiny, "")), "╭") {
		t.Error("a 2x2 area must not draw a frame")
	}
}

// TestBorderTokenIsTheFrame keeps the helper tied to the shared palette.
func TestBorderTokenIsTheFrame(t *testing.T) {
	if styles.Border.GetBorderStyle().Top == "" {
		t.Fatal("styles.Border has no top border")
	}
}
