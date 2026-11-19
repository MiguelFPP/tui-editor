package editor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

const (
	draftPath   domain.RelPath = "draft.txt"
	draftWidth                 = 80
	draftHeight                = 10
)

// draftSource is the content of the file the quick-edit tests open.
const draftSource = "uno\ndos\ntres\n"

var draftStamp = domain.FileStamp{Size: int64(len(draftSource)), ModTime: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)}

// draft is the file and the ports of one quick-edit test: the fake keeps the disk in memory.
type draft struct {
	files *fakes.FileService

	editors *fakes.EditorService

	disk     string
	stamp    domain.FileStamp
	resolve  error
	writeErr error
}

func newDraft() *draft {
	fx := &draft{disk: draftSource, stamp: draftStamp}

	fx.files = fakes.NewFileService()
	fx.files.OnReadFile = func(_ context.Context, path domain.RelPath) (domain.FileContent, error) {
		if path != draftPath {
			return domain.FileContent{}, fmt.Errorf("%w: %q", domain.ErrNotFound, path)
		}

		return domain.FileContent{Path: path, Text: fx.disk, Stamp: fx.stamp}, nil
	}
	fx.files.OnWriteFile = func(_ context.Context, path domain.RelPath, content string, expected domain.FileStamp) (domain.FileStamp, error) {
		if fx.writeErr != nil {
			return domain.FileStamp{}, fx.writeErr
		}
		if expected != fx.stamp {
			return domain.FileStamp{}, fmt.Errorf("%w: %q cambió en disco", domain.ErrConflict, path)
		}

		fx.disk = content
		fx.stamp = domain.FileStamp{Size: int64(len(content)), ModTime: fx.stamp.ModTime}

		return fx.stamp, nil
	}

	fx.editors = fakes.NewEditorService()
	fx.editors.OnResolve = func(_ context.Context, target domain.EditorTarget) (domain.EditorCommand, error) {
		if fx.resolve != nil {
			return domain.EditorCommand{}, fx.resolve
		}

		return domain.EditorCommand{Binary: "vim", Args: []string{target.Path.String()}}, nil
	}

	return fx
}

func (fx *draft) open(t *testing.T) Model {
	t.Helper()

	return fx.openWith(t, keymap.Default())
}

// openWith loads the draft fixture with a custom keymap.
func (fx *draft) openWith(t *testing.T, keys keymap.Registry) Model {
	t.Helper()

	m := New(fx.files, fx.editors, draftPath, keys)
	m.SetSize(draftWidth, draftHeight)

	return apply(t, m, run(t, m.Init()))
}

// run executes one command and returns its message, like the runtime does.
func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()

	if cmd == nil {
		t.Fatal("no command, want one")
	}

	return cmd()
}

// apply feeds one message back, expanding a batch but never chasing its commands.
func apply(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()

	if msg == nil {
		t.Fatal("no message, want one")
	}

	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, nested := range batch {
			if nested == nil {
				continue
			}

			m = apply(t, m, nested())
		}

		return m
	}

	next, _ := m.Update(msg)

	return next
}

func typed(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

var ctrlS = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}

func press(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	return m.Update(msg)
}

// send applies one key and drops its command.
func send(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()

	next, _ := press(t, m, msg)

	return next
}

// lastWrite returns the content and the stamp of the last WriteFile call of the fake.
func lastWrite(t *testing.T, files *fakes.FileService) (string, domain.FileStamp) {
	t.Helper()

	calls := files.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method != "WriteFile" {
			continue
		}

		content, ok := calls[i].Args[1].(string)
		if !ok {
			t.Fatalf("WriteFile content = %T, want string", calls[i].Args[1])
		}
		stamp, ok := calls[i].Args[2].(domain.FileStamp)
		if !ok {
			t.Fatalf("WriteFile stamp = %T, want domain.FileStamp", calls[i].Args[2])
		}

		return content, stamp
	}

	t.Fatal("no call to WriteFile was recorded")

	return "", domain.FileStamp{}
}

// bodyRows returns the rendered rows of the body, without the header.
func bodyRows(m Model) []string {
	rows := strings.Split(plain(m.View()), "\n")
	if len(rows) <= headerRows {
		return nil
	}

	return rows[headerRows:]
}

func TestEditEntersTheDraftAndCapturesInput(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)

	if m.CapturesInput() || m.Dirty() {
		t.Fatalf("CapturesInput()/Dirty() = %v/%v in preview, want false/false", m.CapturesInput(), m.Dirty())
	}

	m, cmd := press(t, m, typed('e'))
	if cmd == nil {
		t.Error("no command, want the focus of the draft")
	}
	if !m.editing || !m.CapturesInput() {
		t.Fatalf("editing = %v, CapturesInput() = %v, want the draft open", m.editing, m.CapturesInput())
	}
	if m.Dirty() {
		t.Error("Dirty() = true on an untouched draft, want false")
	}
	if got := m.area.Value(); got != draftSource {
		t.Errorf("draft = %q, want the file text", got)
	}

	view := visible(m.View())
	if !strings.Contains(view, "draft.txt · archivo · editando · ln 1, col 1 · 3 líneas") {
		t.Errorf("View() = %q, want the editing header with the caret and the line count", view)
	}
	if !strings.Contains(view, "uno") {
		t.Errorf("View() = %q, want the draft body", view)
	}

	m = send(t, m, typed('X'))
	if !m.Dirty() {
		t.Error("Dirty() = false after typing, want true")
	}
	if view := visible(m.View()); !strings.Contains(view, "sin guardar") {
		t.Errorf("View() = %q, want the pending-changes marker", view)
	}
}

// TestEditKeymapOverrideOpensTheDraft checks that the override replaces the default key.
func TestEditKeymapOverrideOpensTheDraft(t *testing.T) {
	fx := newDraft()
	m := fx.openWith(t, overrideKeys(t, map[string][]string{"editor_edit": {"o"}}))

	// e is no longer bound to the quick edit mode.
	m, _ = press(t, m, typed('e'))
	if m.editing || m.CapturesInput() {
		t.Fatal("e entered the draft, want the default key replaced")
	}

	m, cmd := press(t, m, typed('o'))
	if cmd == nil {
		t.Error("no command, want the focus of the draft")
	}
	if !m.editing || !m.CapturesInput() {
		t.Fatalf("editing = %v, CapturesInput() = %v, want the draft open", m.editing, m.CapturesInput())
	}
	if got := m.area.Value(); got != draftSource {
		t.Errorf("draft = %q, want the file text", got)
	}
}

// TestSaveKeymapOverride checks that the save shortcut can be reassigned.
func TestSaveKeymapOverride(t *testing.T) {
	fx := newDraft()
	m := fx.openWith(t, overrideKeys(t, map[string][]string{"editor_save": {"ctrl+w"}}))
	m = send(t, m, typed('e'))
	m = send(t, m, typed('X'))

	// ctrl+s is no longer a save key: the draft is left untouched.
	m, _ = press(t, m, ctrlS)
	if m.saving {
		t.Error("ctrl+s started a save, want the default key replaced")
	}
	if got := fx.files.CallCount("WriteFile"); got != 0 {
		t.Fatalf("WriteFile calls = %d after ctrl+s, want 0", got)
	}
	if !m.Dirty() {
		t.Error("Dirty() = false, want the draft kept")
	}

	m, cmd := press(t, m, tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("no command, want the write")
	}

	m = apply(t, m, run(t, cmd))

	if got := fx.files.CallCount("WriteFile"); got != 1 {
		t.Errorf("WriteFile calls = %d after ctrl+w, want 1", got)
	}
	if m.Dirty() || m.saving {
		t.Errorf("dirty/saving = %v/%v after the save, want false/false", m.Dirty(), m.saving)
	}
	if fx.disk != "X"+draftSource {
		t.Errorf("disk = %q, want the saved draft", fx.disk)
	}
}

func TestEditRejectsRefusedPreviews(t *testing.T) {
	t.Run("binario", func(t *testing.T) {
		m := open(t, "logo.png", draftWidth, draftHeight)

		m, _ = press(t, m, typed('e'))
		if m.editing || m.CapturesInput() {
			t.Error("a binary preview entered the draft")
		}
	})

	t.Run("truncado", func(t *testing.T) {
		m := open(t, "huge.log", draftWidth, draftHeight)

		m, _ = press(t, m, typed('e'))
		if m.editing || m.CapturesInput() {
			t.Error("a truncated preview entered the draft")
		}
	})

	t.Run("diff", func(t *testing.T) {
		diff := domain.Diff{Path: "main.go", Hunks: []domain.DiffHunk{{
			Header: "@@ -1,1 +1,1 @@",
			Lines:  []domain.DiffLine{{Kind: domain.Context, Text: "uno", OldLine: 1, NewLine: 1}},
		}}}

		m := NewDiff(diff)
		m.SetSize(draftWidth, draftHeight)

		m, _ = press(t, m, typed('e'))
		if m.editing || m.CapturesInput() {
			t.Error("a diff preview entered the draft")
		}
	})
}

func TestEditEscapeAsksBeforeDroppingTheDraft(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)
	m = send(t, m, typed('e'))
	m = send(t, m, typed('X'))

	changed := "X" + draftSource

	m, cmd := press(t, m, key(tea.KeyEscape))
	if cmd != nil {
		t.Error("asking to discard produced a command, want none")
	}
	if !m.confirmDiscard || !m.CapturesInput() {
		t.Fatalf("confirmDiscard = %v, CapturesInput() = %v, want the confirmation open", m.confirmDiscard, m.CapturesInput())
	}
	if view := visible(m.View()); !strings.Contains(view, "descartar") {
		t.Errorf("View() = %q, want the discard confirmation", view)
	}

	m = send(t, m, typed('n'))
	if m.confirmDiscard {
		t.Error("confirmDiscard = true after n, want it cancelled")
	}
	if !m.editing || !m.Dirty() {
		t.Error("n dropped the draft, want it kept")
	}
	if got := m.area.Value(); got != changed {
		t.Errorf("draft = %q, want %q", got, changed)
	}

	m = send(t, m, key(tea.KeyEscape))
	if !m.confirmDiscard {
		t.Fatal("confirmDiscard = false, want the confirmation back")
	}
	m = send(t, m, key(tea.KeyEscape))
	if m.confirmDiscard || !m.Dirty() {
		t.Error("esc did not cancel the confirmation, want the draft kept")
	}

	m = send(t, m, key(tea.KeyEscape))
	m = send(t, m, typed('y'))
	if m.editing || m.confirmDiscard || m.Dirty() {
		t.Fatalf("editing/confirmDiscard/dirty = %v/%v/%v after y, want the draft dropped", m.editing, m.confirmDiscard, m.Dirty())
	}
	if got := m.area.Value(); got != draftSource {
		t.Errorf("draft = %q, want the file text", got)
	}
	if view := visible(m.View()); !strings.Contains(view, "1 │ uno") {
		t.Errorf("View() = %q, want the preview back", view)
	}
}

func TestEditEscapeWithoutChangesReturnsToThePreview(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)
	m = send(t, m, typed('e'))

	m, cmd := press(t, m, key(tea.KeyEscape))
	if cmd != nil {
		t.Error("leaving a clean draft produced a command, want none")
	}
	if m.editing || m.confirmDiscard || m.CapturesInput() || m.Dirty() {
		t.Fatalf("editing/confirmDiscard/captures = %v/%v/%v, want the preview", m.editing, m.confirmDiscard, m.CapturesInput())
	}
	if view := visible(m.View()); !strings.Contains(view, "1 │ uno") {
		t.Errorf("View() = %q, want the preview back", view)
	}
}

func TestDirtyTracksTheDraft(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)
	m = send(t, m, typed('e'))

	if m.Dirty() {
		t.Fatal("Dirty() = true on an untouched draft, want false")
	}

	m = send(t, m, typed('X'))
	if !m.Dirty() {
		t.Fatal("Dirty() = false after typing, want true")
	}

	m = send(t, m, key(tea.KeyBackspace))
	if m.Dirty() {
		t.Error("Dirty() = true after undoing the change, want false")
	}
	if got := m.area.Value(); got != draftSource {
		t.Errorf("draft = %q, want the file text", got)
	}
}

func TestDraftBodyShowsTheHighlightedLines(t *testing.T) {
	m := open(t, "main.go", 60, goldenHeight)
	m = send(t, m, typed('e'))

	body := m.draftBody()
	if !strings.Contains(body, "\x1b[") {
		t.Fatalf("draftBody() = %q, want the syntax colours of the draft", body)
	}

	lines := strings.Split(trimFinalNewline(m.area.Value()), "\n")
	rows := strings.Split(plain(body), "\n")
	if len(rows) != m.bodyHeight() {
		t.Fatalf("draftBody() renders %d rows, want %d", len(rows), m.bodyHeight())
	}
	if len(lines) < len(rows) {
		t.Fatalf("the draft has %d lines, want at least the %d visible rows", len(lines), len(rows))
	}

	for i, row := range rows {
		want := fmt.Sprintf("%*d%s%s", gutterDigits(len(lines)), i+1, gutterSeparator, expandTabs(lines[i]))
		if row != want {
			t.Errorf("row %d = %q, want the plain draft %q", i, row, want)
		}
	}
}

func TestDraftBodyKeepsTheColourWhileTyping(t *testing.T) {
	m := open(t, "main.go", 60, 12)
	m = send(t, m, typed('e'))

	before := m.draftLines
	if len(before) == 0 || !strings.Contains(before[0], "\x1b[") {
		t.Fatalf("draftLines = %q, want the highlighted draft", before)
	}

	m = send(t, m, typed('X'))

	if len(m.draftLines) != len(before) {
		t.Fatalf("draftLines = %d lines after typing, want %d", len(m.draftLines), len(before))
	}
	// The typed line lands on the first line, so every line below keeps its colours.
	for i, line := range before[1:] {
		if m.draftLines[i+1] != line {
			t.Errorf("line %d = %q after typing, want the highlight kept %q", i+1, m.draftLines[i+1], line)
		}
	}
	if view := visible(m.View()); !strings.Contains(view, "1 │ Xpackage main") {
		t.Errorf("View() = %q, want the typed line", view)
	}
}

func TestDraftBodyWithoutALexerStaysPlain(t *testing.T) {
	m := open(t, "notes.xyz", 60, goldenHeight)
	m = send(t, m, typed('e'))

	if got := strings.Join(m.draftLines, "\n"); strings.Contains(got, "\x1b[") {
		t.Errorf("draftLines = %q, want the plain lines of a file without a lexer", got)
	}
	if view := visible(m.View()); !strings.Contains(view, "1 │ notas sin formato reconocido") {
		t.Errorf("View() = %q, want the numbered lines of the draft", view)
	}
}

func TestDraftBodyKeepsTheBodyHeight(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)

	if got, want := len(bodyRows(m)), m.bodyHeight(); got != want {
		t.Fatalf("the preview renders %d rows, want %d", got, want)
	}

	m = send(t, m, typed('e'))
	if got, want := len(bodyRows(m)), m.bodyHeight(); got != want {
		t.Fatalf("the draft renders %d rows, want %d", got, want)
	}

	// A draft shorter than the body keeps the height with blank gutter rows.
	short := open(t, "notes.xyz", 60, goldenHeight)
	short = send(t, short, typed('e'))
	if got, want := len(bodyRows(short)), short.bodyHeight(); got != want {
		t.Errorf("a short draft renders %d rows, want %d", got, want)
	}

	// The confirmation and the failure of the save come out of the same body.
	m.files = nil
	m = send(t, m, typed('X'))
	m = send(t, m, ctrlS)
	if m.saveErr == nil {
		t.Fatal("saveErr = nil without a file service, want the failure")
	}
	m = send(t, m, key(tea.KeyEscape))
	if !m.confirmDiscard {
		t.Fatal("confirmDiscard = false, want the confirmation open")
	}
	if got, want := len(bodyRows(m)), m.bodyHeight(); got != want {
		t.Errorf("the draft with its hints renders %d rows, want %d", got, want)
	}
}

func TestDraftScrollsWithTheCaret(t *testing.T) {
	m := open(t, "long.txt", 40, 8)
	m = send(t, m, typed('e'))

	rows := m.draftRows()
	if rows == 0 {
		t.Fatal("draftRows() = 0, want a window with room for the draft")
	}
	if m.draftTop != 0 {
		t.Fatalf("draftTop = %d on the opened draft, want 0", m.draftTop)
	}

	// The window follows the caret: it only moves once the caret leaves it.
	for range rows {
		m = send(t, m, key(tea.KeyDown))
	}

	if got := m.area.Line(); got != rows {
		t.Fatalf("Line() = %d after %d downs, want %d", got, rows, rows)
	}
	if m.draftTop != 1 {
		t.Fatalf("draftTop = %d with the caret below the window, want 1", m.draftTop)
	}
	if got := caretRow(m); got != rows-1 {
		t.Errorf("the caret is on row %d, want the last row %d of the window", got, rows-1)
	}
	if view := visible(m.View()); !strings.Contains(view, fmt.Sprintf("%d │ line-%d", rows+1, rows+1)) {
		t.Errorf("View() = %q, want the row of the caret", view)
	}

	// Moving back up keeps the caret on the first row of the window and rewinds it.
	for range rows {
		m = send(t, m, key(tea.KeyUp))
	}

	if got := m.area.Line(); got != 0 {
		t.Fatalf("Line() = %d after %d ups, want 0", got, rows)
	}
	if m.draftTop != 0 {
		t.Errorf("draftTop = %d with the caret on the first line, want 0", m.draftTop)
	}
	if view := visible(m.View()); !strings.Contains(view, "1 │ line-1") {
		t.Errorf("View() = %q, want the first line of the draft", view)
	}
}

func TestSaveWritesTheDraftThroughThePort(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)
	m = send(t, m, typed('e'))
	m = send(t, m, typed('X'))

	want := "X" + draftSource

	m, cmd := press(t, m, ctrlS)
	if cmd == nil {
		t.Fatal("no command, want the write")
	}

	answer := run(t, cmd)
	saved, ok := answer.(savedMsg)
	if !ok {
		t.Fatalf("the save command returned %T, want savedMsg", answer)
	}
	if saved.err != nil {
		t.Fatalf("savedMsg.err = %v, want nil", saved.err)
	}
	if saved.text != want {
		t.Errorf("savedMsg.text = %q, want %q", saved.text, want)
	}

	content, stamp := lastWrite(t, fx.files)
	if content != want {
		t.Errorf("WriteFile content = %q, want %q", content, want)
	}
	if stamp != draftStamp {
		t.Errorf("WriteFile stamp = %+v, want the stamp of the read %+v", stamp, draftStamp)
	}
	if !m.Dirty() || !m.saving {
		t.Errorf("dirty/saving = %v/%v while the write is in flight, want true/true", m.Dirty(), m.saving)
	}
	if !m.Loading() {
		t.Error("Loading() = false while saving, want true")
	}

	m, cmd = press(t, m, ctrlS)
	if cmd != nil {
		t.Error("a second ctrl+s started another write, want none")
	}

	m, cmd = m.Update(saved)

	if m.Dirty() || m.saving || m.Loading() {
		t.Errorf("dirty/saving/Loading() = %v/%v/%v after the write, want all false", m.Dirty(), m.saving, m.Loading())
	}
	if m.saveErr != nil {
		t.Errorf("saveErr = %v, want nil", m.saveErr)
	}
	if m.raw != want {
		t.Errorf("raw = %q, want the saved text", m.raw)
	}
	if m.stamp != fx.stamp || m.stamp == draftStamp {
		t.Errorf("stamp = %+v, want the confirmed one %+v", m.stamp, fx.stamp)
	}
	if len(m.draftLines) == 0 {
		t.Error("draftLines is empty after the save, want the draft still rendered")
	}
	if view := visible(m.View()); !strings.Contains(view, "1 │ Xuno") {
		t.Errorf("View() = %q, want the saved draft behind its gutter", view)
	}
	if cmd == nil {
		t.Fatal("no command, want FileChangedMsg")
	}

	announced, ok := cmd().(FileChangedMsg)
	if !ok {
		t.Fatalf("the save returned %T, want FileChangedMsg", cmd())
	}
	if announced.Path != draftPath {
		t.Errorf("FileChangedMsg.Path = %q, want %q", announced.Path, draftPath)
	}

	m = send(t, m, key(tea.KeyEscape))
	if view := visible(m.View()); !strings.Contains(view, "1 │ Xuno") {
		t.Errorf("View() = %q, want the saved preview", view)
	}
}

func TestSaveConflictKeepsTheDraft(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)
	m = send(t, m, typed('e'))
	m = send(t, m, typed('X'))

	// The file changed on disk: another process rewrote it with another size.
	fx.disk = "cambio externo\n"
	fx.stamp = domain.FileStamp{Size: int64(len(fx.disk)), ModTime: draftStamp.ModTime.Add(time.Minute)}

	m, cmd := press(t, m, ctrlS)
	if cmd == nil {
		t.Fatal("no command, want the write")
	}

	saved, ok := run(t, cmd).(savedMsg)
	if !ok {
		t.Fatal("the save command did not return savedMsg")
	}
	if !errors.Is(saved.err, domain.ErrConflict) {
		t.Fatalf("savedMsg.err = %v, want %v", saved.err, domain.ErrConflict)
	}

	m = apply(t, m, saved)

	if !errors.Is(m.saveErr, domain.ErrConflict) {
		t.Fatalf("saveErr = %v, want %v", m.saveErr, domain.ErrConflict)
	}
	if !errors.Is(m.Err(), domain.ErrConflict) {
		t.Errorf("Err() = %v, want %v", m.Err(), domain.ErrConflict)
	}
	if !m.Dirty() || m.Loading() {
		t.Errorf("dirty/Loading() = %v/%v, want the draft kept and no pending write", m.Dirty(), m.Loading())
	}
	if got := m.area.Value(); got != "X"+draftSource {
		t.Errorf("draft = %q, want the draft kept", got)
	}

	view := visible(m.View())
	if !strings.Contains(view, "cambió en disco") {
		t.Errorf("View() = %q, want the conflict description", view)
	}
	if !strings.Contains(view, "error") {
		t.Errorf("View() = %q, want the error flag in the header", view)
	}
}

func TestSaveWithoutTheFileServiceReportsAnError(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)
	m = send(t, m, typed('e'))
	m = send(t, m, typed('X'))

	m.files = nil

	m, cmd := press(t, m, ctrlS)
	if cmd != nil {
		t.Error("a missing service produced a command, want none")
	}
	if !errors.Is(m.saveErr, errNoService) {
		t.Fatalf("saveErr = %v, want %v", m.saveErr, errNoService)
	}
	if !m.Dirty() {
		t.Error("Dirty() = false, want the draft kept")
	}
	if view := visible(m.View()); !strings.Contains(view, "servicio de archivos") {
		t.Errorf("View() = %q, want the missing-service message", view)
	}
}

func TestExternalReportsAMissingEditor(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)

	fx.resolve = fmt.Errorf("%w: %q", domain.ErrEditorUnavailable, "vim")

	m, cmd := press(t, m, typed('E'))
	if cmd == nil {
		t.Fatal("no command, want the resolve")
	}

	m = apply(t, m, run(t, cmd))

	if !errors.Is(m.saveErr, domain.ErrEditorUnavailable) {
		t.Fatalf("saveErr = %v, want %v", m.saveErr, domain.ErrEditorUnavailable)
	}
	if m.external {
		t.Error("external = true, want no editor running")
	}
	if got := fx.editors.CallCount("Resolve"); got != 1 {
		t.Errorf("Resolve calls = %d, want 1", got)
	}
	if got := fx.files.CallCount("WriteFile"); got != 0 {
		t.Errorf("WriteFile calls = %d, want 0", got)
	}

	view := visible(m.View())
	if !strings.Contains(view, "editor externo no está disponible") {
		t.Errorf("View() = %q, want the missing-editor description", view)
	}
	if !strings.Contains(view, "1 │ uno") {
		t.Errorf("View() = %q, want the preview kept", view)
	}
}

func TestExternalWithoutAnEditorServiceReportsAnError(t *testing.T) {
	fx := newDraft()
	m := New(fx.files, nil, draftPath, keymap.Default())
	m.SetSize(draftWidth, draftHeight)
	m = apply(t, m, run(t, m.Init()))

	m, cmd := press(t, m, typed('E'))
	if cmd != nil {
		t.Error("a missing service produced a command, want none")
	}
	if !errors.Is(m.saveErr, errNoEditor) {
		t.Fatalf("saveErr = %v, want %v", m.saveErr, errNoEditor)
	}
	if view := visible(m.View()); !strings.Contains(view, "servicio de editor") {
		t.Errorf("View() = %q, want the missing-service message", view)
	}
}

func TestExternalDoneReloadsTheFile(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)

	m, cmd := press(t, m, typed('E'))
	if cmd == nil {
		t.Fatal("no command, want the resolve")
	}
	if !m.external {
		t.Fatal("external = false after E, want the editor being resolved")
	}

	m = apply(t, m, run(t, cmd))
	if !m.external {
		t.Fatal("external = false before the editor closed, want true")
	}

	// The external editor rewrote the file.
	fx.disk = "externo\n"
	fx.stamp = domain.FileStamp{Size: int64(len(fx.disk)), ModTime: draftStamp.ModTime.Add(time.Minute)}

	m, cmd = press(t, m, externalDoneMsg{})
	if cmd == nil {
		t.Fatal("no command, want the reload and the notice")
	}
	if m.external {
		t.Error("external = true after the editor closed, want false")
	}
	if !m.Loading() {
		t.Error("Loading() = false while reloading, want true")
	}

	m = apply(t, m, run(t, cmd))

	if m.raw != fx.disk {
		t.Errorf("raw = %q, want the file the editor left %q", m.raw, fx.disk)
	}
	if m.stamp != fx.stamp {
		t.Errorf("stamp = %+v, want %+v", m.stamp, fx.stamp)
	}
	if m.Loading() {
		t.Error("Loading() = true after the reload, want false")
	}
	if fx.files.CallCount("ReadFile") != 2 {
		t.Errorf("ReadFile calls = %d, want 2", fx.files.CallCount("ReadFile"))
	}
	if view := visible(m.View()); !strings.Contains(view, "externo") {
		t.Errorf("View() = %q, want the reloaded preview", view)
	}
}

func TestExternalDoneReportsTheFailureOfTheEditor(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)

	m, cmd := press(t, m, typed('E'))
	if cmd == nil {
		t.Fatal("no command, want the resolve")
	}
	m = apply(t, m, run(t, cmd))
	if !m.external {
		t.Fatal("external = false, want the editor resolved")
	}

	caused := errors.New("exit status 1")
	m, cmd = press(t, m, externalDoneMsg{err: caused})
	if cmd == nil {
		t.Fatal("no command, want the reload")
	}
	if !errors.Is(m.Err(), caused) {
		t.Fatalf("Err() = %v, want %v", m.Err(), caused)
	}

	m = apply(t, m, run(t, cmd))
	if m.saveErr == nil {
		t.Error("saveErr = nil after a failed editor, want the failure kept")
	}
}

// draftRow returns one rendered row of the draft window, skipping the hint rows.
func draftRow(m Model, row int) string {
	rows := strings.Split(m.draftBody(), "\n")
	index := row + m.draftHints()
	if index < 0 || index >= len(rows) {
		return ""
	}

	return rows[index]
}

// caretRow returns the row of the draft window that holds the caret, or -1.
func caretRow(m Model) int {
	rows := strings.Split(m.draftBody(), "\n")
	for i, row := range rows {
		if strings.Contains(row, cursorOn) {
			return i - m.draftHints()
		}
	}

	return -1
}

// caretCell returns the visible cell of the caret inside one rendered row, or -1.
func caretCell(row string) int {
	start := strings.Index(row, cursorOn)
	if start < 0 {
		return -1
	}

	return lipgloss.Width(plain(row[:start]))
}

// click builds a left click on one cell of the editor.
func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func TestCursorIsDrawnWithoutChangingTheText(t *testing.T) {
	m := open(t, "long.txt", 40, 6)
	m = send(t, m, typed('e'))

	body := m.draftBody()
	if !strings.Contains(body, cursorOn) || !strings.Contains(body, cursorOff) {
		t.Fatalf("draftBody() = %q, want the caret around a cell", body)
	}

	// The caret reverses a cell of the row: the visible text is the draft itself.
	lines := strings.Split(trimFinalNewline(m.area.Value()), "\n")
	rows := strings.Split(visible(m.View()), "\n")[headerRows:]
	for i, row := range rows {
		want := fmt.Sprintf("%*d%s%s", gutterDigits(len(lines)), i+1, gutterSeparator, expandTabs(lines[i]))
		if row != want {
			t.Errorf("row %d = %q, want the plain draft %q", i, row, want)
		}
	}
}

func TestCursorKeepsTheBodyHeight(t *testing.T) {
	m := open(t, "long.txt", 40, 6)
	m = send(t, m, typed('e'))
	m = send(t, m, key(tea.KeyDown))

	// The caret adds no row: the draft still fills the body exactly.
	if got, want := len(bodyRows(m)), m.bodyHeight(); got != want {
		t.Fatalf("the draft with its caret renders %d rows, want %d", got, want)
	}
	if !strings.Contains(m.draftBody(), cursorOn) {
		t.Error("draftBody() holds no caret, want the caret drawn")
	}
}

func TestCursorOnAnEmptyDraft(t *testing.T) {
	m := open(t, "empty.txt", 40, 6)
	m = send(t, m, typed('e'))

	if !m.editing {
		t.Fatal("editing = false, want the draft of an empty file")
	}
	if got := caretRow(m); got != 0 {
		t.Errorf("the caret is on row %d, want 0", got)
	}
	if got, want := caretCell(draftRow(m, 0)), m.gutterCells(); got != want {
		t.Errorf("the caret is on cell %d of the row, want %d", got, want)
	}
	if got, want := len(bodyRows(m)), m.bodyHeight(); got != want {
		t.Errorf("the empty draft renders %d rows, want %d", got, want)
	}
}

func TestCursorOnTheEmptyLineOfTheDraft(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)
	m = send(t, m, typed('e'))

	// The trailing newline of "uno\ndos\ntres\n" leaves an empty last line.
	for range m.draftLines {
		m = send(t, m, key(tea.KeyDown))
	}

	if got, want := m.area.Line(), len(m.draftLines); got != want {
		t.Fatalf("Line() = %d at the end of the draft, want %d", got, want)
	}
	if got, want := caretRow(m), len(m.draftLines)-m.draftTop; got != want {
		t.Errorf("the caret is on row %d, want %d", got, want)
	}
}

func TestCursorMovesWithTheArrowKeys(t *testing.T) {
	m := open(t, "long.txt", 40, 6)
	m = send(t, m, typed('e'))

	// The draft opens on the first line, with the caret on its first cell.
	if got := m.area.Line(); got != 0 {
		t.Fatalf("Line() = %d on the opened draft, want 0", got)
	}
	if got := m.area.Column(); got != 0 {
		t.Fatalf("Column() = %d on the opened draft, want 0", got)
	}
	if got := caretRow(m); got != 0 {
		t.Fatalf("the caret is on row %d, want 0", got)
	}
	if got, want := caretCell(draftRow(m, 0)), m.gutterCells(); got != want {
		t.Fatalf("the caret is on cell %d of the row, want %d", got, want)
	}

	m = send(t, m, key(tea.KeyRight))
	m = send(t, m, key(tea.KeyRight))
	if got := m.area.Column(); got != 2 {
		t.Fatalf("Column() = %d after two rights, want 2", got)
	}
	if got, want := caretCell(draftRow(m, 0)), m.gutterCells()+2; got != want {
		t.Errorf("the caret is on cell %d, want %d", got, want)
	}

	m = send(t, m, key(tea.KeyDown))
	if got := m.area.Line(); got != 1 {
		t.Fatalf("Line() = %d after down, want 1", got)
	}
	if got := m.area.Column(); got != 2 {
		t.Fatalf("Column() = %d after down, want the column kept", got)
	}
	if got := caretRow(m); got != 1 {
		t.Errorf("the caret is on row %d after down, want 1", got)
	}

	m = send(t, m, key(tea.KeyLeft))
	if got := m.area.Column(); got != 1 {
		t.Errorf("Column() = %d after left, want 1", got)
	}
	if got, want := caretCell(draftRow(m, 1)), m.gutterCells()+1; got != want {
		t.Errorf("the caret is on cell %d, want %d", got, want)
	}
	if got := caretRow(m); got != 1 {
		t.Errorf("the caret is on row %d after left, want 1", got)
	}
}

func TestCursorFollowsTheClick(t *testing.T) {
	m := open(t, "long.txt", 40, 6)
	m = send(t, m, typed('e'))

	m, cmd := press(t, m, click(m.gutterCells()+3, headerRows+2))
	if cmd != nil {
		t.Errorf("the click produced a command, want nil")
	}
	if got := m.area.Line(); got != 2 {
		t.Fatalf("Line() = %d after the click, want 2", got)
	}
	if got := m.area.Column(); got != 3 {
		t.Fatalf("Column() = %d after the click, want 3", got)
	}
	if m.area.HasSelection() {
		t.Error("the click left a selection, want a bare caret")
	}
	if got := caretRow(m); got != 2 {
		t.Errorf("the caret is on row %d, want 2", got)
	}
	if got, want := caretCell(draftRow(m, 2)), m.gutterCells()+3; got != want {
		t.Errorf("the caret is on cell %d, want %d", got, want)
	}
}

func TestCursorClickOnTheGutterUsesTheFirstColumn(t *testing.T) {
	m := open(t, "long.txt", 40, 6)
	m = send(t, m, typed('e'))

	for x := range m.gutterCells() {
		m, _ = press(t, m, click(x, headerRows+2))
		if got := m.area.Column(); got != 0 {
			t.Fatalf("Column() = %d after a click on cell %d of the gutter, want 0", got, x)
		}
		if got := m.area.Line(); got != 2 {
			t.Fatalf("Line() = %d after a click on the gutter, want 2", got)
		}
	}
}

func TestCursorFollowsTheClickAfterScrolling(t *testing.T) {
	m := open(t, "long.txt", 40, 6)
	m = send(t, m, typed('e'))

	for range m.draftRows() {
		m = send(t, m, key(tea.KeyDown))
	}

	// The wheel moves our window on its own; the textarea keeps its own scroll.
	top := m.draftTop
	m, _ = press(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 2, Y: 3})
	if m.draftTop != top+wheelStep {
		t.Fatalf("draftTop = %d after the wheel, want %d", m.draftTop, top+wheelStep)
	}

	m, _ = press(t, m, click(m.gutterCells()+1, headerRows+3))
	if got, want := m.area.Line(), m.draftTop+3; got != want {
		t.Errorf("Line() = %d after the click, want %d", got, want)
	}
	if got := m.area.Column(); got != 1 {
		t.Errorf("Column() = %d after the click, want 1", got)
	}
	if got := caretRow(m); got != 3 {
		t.Errorf("the caret is on row %d of the window, want 3", got)
	}
}

func TestCursorIgnoresClicksOutsideTheBody(t *testing.T) {
	m := open(t, "long.txt", 40, 6)
	m = send(t, m, typed('e'))
	m = send(t, m, key(tea.KeyDown))
	m = send(t, m, key(tea.KeyDown))

	line, column := m.area.Line(), m.area.Column()

	cases := []struct {
		name string
		x, y int
	}{
		{name: "cabecera", x: 4, y: 0},
		{name: "bajo el cuerpo", x: 4, y: headerRows + m.bodyHeight()},
		{name: "a la derecha", x: m.width, y: headerRows + 1},
		{name: "a la izquierda", x: -1, y: headerRows + 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, cmd := press(t, m, click(tc.x, tc.y))
			if cmd != nil {
				t.Errorf("the click produced a command, want nil")
			}
			if next.area.Line() != line || next.area.Column() != column {
				t.Errorf("the caret is at %d:%d after a click on %s, want %d:%d",
					next.area.Line(), next.area.Column(), tc.name, line, column)
			}
			if next.draftTop != m.draftTop {
				t.Errorf("draftTop = %d after a click on %s, want %d", next.draftTop, tc.name, m.draftTop)
			}
		})
	}
}

func TestCursorIgnoresClicksInThePreview(t *testing.T) {
	m := open(t, "long.txt", 40, 6)

	next, cmd := press(t, m, click(m.gutterCells()+1, headerRows+1))
	if cmd != nil {
		t.Errorf("the click produced a command, want nil")
	}
	if next.editing || next.draftTop != 0 {
		t.Errorf("editing/draftTop = %v/%d after a click in the preview, want false/0", next.editing, next.draftTop)
	}
}

func TestDraftWheelScrollsTheWindowWithoutMovingTheCaret(t *testing.T) {
	m := open(t, "long.txt", 40, 6)
	m = send(t, m, typed('e'))

	m, cmd := press(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 2, Y: 3})
	if cmd != nil {
		t.Errorf("the wheel produced a command, want nil")
	}
	if m.draftTop != wheelStep {
		t.Fatalf("draftTop = %d after the wheel down, want %d", m.draftTop, wheelStep)
	}
	if got := m.area.Line(); got != 0 {
		t.Errorf("Line() = %d after the wheel down, want the caret kept at 0", got)
	}
	if view := visible(m.View()); !strings.Contains(view, fmt.Sprintf("%d │ line-%d", wheelStep+1, wheelStep+1)) {
		t.Errorf("View() = %q, want the window scrolled to line %d", view, wheelStep+1)
	}

	m, _ = press(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 2, Y: 3})
	if m.draftTop != 0 {
		t.Fatalf("draftTop = %d after the wheel up, want 0", m.draftTop)
	}
	if view := visible(m.View()); !strings.Contains(view, " 1 │ line-1") {
		t.Errorf("View() = %q, want the first line of the draft back", view)
	}

	// The wheel never leaves the draft: the window stops at both ends.
	m, _ = press(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 2, Y: 3})
	if m.draftTop != 0 {
		t.Errorf("draftTop = %d after scrolling above the first line, want 0", m.draftTop)
	}
	for range 20 {
		m, _ = press(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 2, Y: 3})
	}
	if got, want := m.draftTop, len(m.draftLines)-m.draftRows(); got != want {
		t.Errorf("draftTop = %d after scrolling to the last line, want %d", got, want)
	}
	if got := m.area.Line(); got != 0 {
		t.Errorf("Line() = %d after scrolling to the last line, want the caret kept at 0", got)
	}
}

func TestDraftWheelIsIgnoredOutsideEditing(t *testing.T) {
	m := open(t, "long.txt", 40, 6)

	m = scroll(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 2, Y: 3})
	if got := m.viewport.YOffset(); got != wheelStep {
		t.Errorf("viewport.YOffset() = %d in the preview, want %d", got, wheelStep)
	}
	if m.draftTop != 0 {
		t.Errorf("draftTop = %d in the preview, want 0", m.draftTop)
	}
}

func TestDraftHeaderShowsTheCaretPosition(t *testing.T) {
	m := open(t, "long.txt", 80, 6)

	if header := plain(m.header()); strings.Contains(header, "ln ") {
		t.Fatalf("header = %q in the preview, want no caret position", header)
	}

	m = send(t, m, typed('e'))
	if header := plain(m.header()); !strings.Contains(header, "editando · ln 1, col 1") {
		t.Errorf("header = %q, want the caret right after editando", header)
	}

	m = send(t, m, key(tea.KeyDown))
	if header := plain(m.header()); !strings.Contains(header, "ln 2, col 1") {
		t.Errorf("header = %q, want the line of the caret", header)
	}

	m = send(t, m, key(tea.KeyRight))
	if header := plain(m.header()); !strings.Contains(header, "ln 2, col 2") {
		t.Errorf("header = %q, want the column of the caret", header)
	}
}
