package editor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
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

	m := New(fx.files, fx.editors, draftPath)
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
	if !strings.Contains(view, "draft.txt · archivo · editando · 3 líneas") {
		t.Errorf("View() = %q, want the editing header with the line count", view)
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

	changed := draftSource + "X"

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

func TestSaveWritesTheDraftThroughThePort(t *testing.T) {
	fx := newDraft()
	m := fx.open(t)
	m = send(t, m, typed('e'))
	m = send(t, m, typed('X'))

	want := draftSource + "X"

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
	if view := visible(m.View()); !strings.Contains(view, "4 │ X") {
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
	if got := m.area.Value(); got != draftSource+"X" {
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
	m := New(fx.files, nil, draftPath)
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
