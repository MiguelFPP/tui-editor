package keymap

import (
	"slices"
	"strings"
	"testing"
)

// expectedActions is the catalog in display order; it also fixes the size.
var expectedActions = []Action{
	ActionQuit, ActionHelp, ActionConfig, ActionTabFiles, ActionTabGit, ActionTabDocker, ActionFocusNext, ActionFocusPrev,

	ActionTreeUp, ActionTreeDown, ActionTreePageUp, ActionTreePageDown, ActionTreeHome, ActionTreeEnd,
	ActionTreeExpand, ActionTreeCollapse, ActionTreeActivate, ActionTreeFilter, ActionTreeClearFilter,

	ActionEditorEdit, ActionEditorExternal,

	ActionEditorSave, ActionEditorCancel,

	ActionEditorDiscardYes, ActionEditorDiscardNo,

	ActionGitSectionNext, ActionGitSectionPrev, ActionGitUp, ActionGitDown, ActionGitActivate, ActionGitDiff,
	ActionGitStage, ActionGitUnstage, ActionGitCommit, ActionGitNewBranch, ActionGitDeleteBranch, ActionGitMerge,
	ActionGitStash, ActionGitResolve, ActionGitResolveTheirs, ActionGitAbortMerge, ActionGitFetch, ActionGitPull,
	ActionGitPush, ActionGitHunkPrev, ActionGitHunkNext, ActionGitHunkToggle, ActionGitRefresh, ActionGitHelp,
	ActionGitBack,

	ActionGitCommitSave, ActionGitCommitCancel,

	ActionGitPromptConfirm, ActionGitPromptCancel,

	ActionGitConfirmYes, ActionGitConfirmNo,

	ActionDockerUp, ActionDockerDown, ActionDockerPageUp, ActionDockerPageDown, ActionDockerHome, ActionDockerEnd,
	ActionDockerStats, ActionDockerLogs, ActionDockerStart, ActionDockerStop, ActionDockerRestart, ActionDockerRefresh,
	ActionDockerScopeToggle,

	ActionDockerConfirmYes, ActionDockerConfirmNo,

	ActionDockerLogsClose, ActionDockerLogsReload, ActionDockerLogsUp, ActionDockerLogsDown, ActionDockerLogsPageUp,
	ActionDockerLogsPageDown, ActionDockerLogsHome, ActionDockerLogsEnd,
}

// equalBindings compares two binding lists field by field; Binding is not
// comparable because it carries slices.
func equalBindings(got, want []Binding) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		a, b := got[i], want[i]
		if a.Action != b.Action || a.Scope != b.Scope || a.Desc != b.Desc || a.Configurable != b.Configurable {
			return false
		}
		if !slices.Equal(a.Keys, b.Keys) || !slices.Equal(a.FixedKeys, b.FixedKeys) {
			return false
		}
	}

	return true
}

// TestDefaultCatalogHasTheExpectedActions also proves the static catalog has no
// conflicts: Default panics through mustRegistry when two bindings clash.
func TestDefaultCatalogHasTheExpectedActions(t *testing.T) {
	registry := Default()

	got := make([]Action, 0, len(expectedActions))
	for _, binding := range registry.All() {
		got = append(got, binding.Action)
	}

	if !slices.Equal(got, expectedActions) {
		t.Fatalf("acciones de Default() = %d %v, want %d %v", len(got), got, len(expectedActions), expectedActions)
	}
}

// TestDockerLogsKeysFollowTheScrollDirection pins the logs bindings to the physical direction: k/up
// scroll up (and load the older page at the top) while j/down scroll down.
func TestDockerLogsKeysFollowTheScrollDirection(t *testing.T) {
	registry := Default()

	descs := make(map[Action]string)
	for _, binding := range registry.Bindings(ScopeDockerLogs) {
		descs[binding.Action] = binding.Desc
	}

	cases := []struct {
		name   string
		action Action
		keys   []string
		desc   string
	}{
		{name: "subir", action: ActionDockerLogsUp, keys: []string{"k", "up"}, desc: "subir"},
		{name: "bajar", action: ActionDockerLogsDown, keys: []string{"j", "down"}, desc: "bajar"},
	}

	for _, tc := range cases {
		if got := registry.Keys(tc.action); !slices.Equal(got, tc.keys) {
			t.Errorf("%s: Keys(%q) = %v, want %v", tc.name, tc.action, got, tc.keys)
		}
		if got := descs[tc.action]; got != tc.desc {
			t.Errorf("%s: Desc(%q) = %q, want %q", tc.name, tc.action, got, tc.desc)
		}
	}

	matches := []struct {
		key  string
		want Action
	}{
		{key: "k", want: ActionDockerLogsUp},
		{key: "up", want: ActionDockerLogsUp},
		{key: "j", want: ActionDockerLogsDown},
		{key: "down", want: ActionDockerLogsDown},
	}

	for _, tc := range matches {
		if action, ok := registry.Match(ScopeDockerLogs, tc.key); !ok || action != tc.want {
			t.Errorf("Match(docker_logs, %q) = (%q, %v), want (%q, true)", tc.key, action, ok, tc.want)
		}
	}
}

func TestApplyWithNoOverridesKeepsTheDefaults(t *testing.T) {
	registry := Default()

	updated, err := registry.Apply(nil)
	if err != nil {
		t.Fatalf("Apply(nil) = %v, want nil error", err)
	}

	if !equalBindings(updated.All(), registry.All()) {
		t.Errorf("Apply(nil).All() = %v, want the defaults", updated.All())
	}

	empty, err := registry.Apply(map[string][]string{})
	if err != nil {
		t.Fatalf("Apply(map vacío) = %v, want nil error", err)
	}

	if !slices.Equal(empty.Keys(ActionTreeUp), registry.Keys(ActionTreeUp)) {
		t.Errorf("Apply(map vacío) Keys = %v, want %v", empty.Keys(ActionTreeUp), registry.Keys(ActionTreeUp))
	}
}

func TestMatchResolvesInsideEachScope(t *testing.T) {
	registry := Default()

	cases := []struct {
		name  string
		scope Scope
		key   string
		want  Action
	}{
		{name: "root q", scope: ScopeRoot, key: "q", want: ActionQuit},
		{name: "root tab", scope: ScopeRoot, key: "tab", want: ActionFocusNext},
		{name: "tree enter", scope: ScopeTree, key: "enter", want: ActionTreeActivate},
		{name: "tree space", scope: ScopeTree, key: "space", want: ActionTreeActivate},
		{name: "tree espacio crudo", scope: ScopeTree, key: " ", want: ActionTreeActivate},
		{name: "tree k", scope: ScopeTree, key: "k", want: ActionTreeUp},
		{name: "git enter", scope: ScopeGit, key: "enter", want: ActionGitActivate},
		{name: "git tab", scope: ScopeGit, key: "tab", want: ActionGitSectionNext},
		{name: "docker enter", scope: ScopeDocker, key: "enter", want: ActionDockerStats},
		{name: "docker l", scope: ScopeDocker, key: "l", want: ActionDockerLogs},
		{name: "root help", scope: ScopeRoot, key: "?", want: ActionHelp},
		{name: "root config", scope: ScopeRoot, key: ",", want: ActionConfig},
		{name: "git help", scope: ScopeGit, key: "?", want: ActionGitHelp},
		{name: "tree clear filter", scope: ScopeTree, key: "esc", want: ActionTreeClearFilter},
		{name: "git back", scope: ScopeGit, key: "esc", want: ActionGitBack},
		{name: "editor cancel", scope: ScopeEditorDraft, key: "esc", want: ActionEditorCancel},
		{name: "docker logs close", scope: ScopeDockerLogs, key: "esc", want: ActionDockerLogsClose},
		{name: "git hunk toggle", scope: ScopeGit, key: "space", want: ActionGitHunkToggle},
		{name: "editor external mayúscula", scope: ScopeEditor, key: "E", want: ActionEditorExternal},
	}

	for _, tc := range cases {
		action, ok := registry.Match(tc.scope, tc.key)
		if !ok || action != tc.want {
			t.Errorf("%s: Match(%q, %q) = (%q, %v), want (%q, true)", tc.name, tc.scope, tc.key, action, ok, tc.want)
		}
	}
}

func TestMatchIsIsolatedPerScope(t *testing.T) {
	registry := Default()

	cases := []struct {
		scope Scope
		key   string
	}{
		{scope: ScopeRoot, key: "enter"},
		{scope: ScopeRoot, key: "space"},
		{scope: ScopeRoot, key: "esc"},
		{scope: ScopeTree, key: "?"},
		{scope: ScopeTree, key: "q"},
		{scope: ScopeEditor, key: "enter"},
		{scope: ScopeGit, key: "e"},
		{scope: ScopeDockerLogs, key: "?"},
		{scope: ScopeDocker, key: "q"},
	}

	for _, tc := range cases {
		if action, ok := registry.Match(tc.scope, tc.key); ok {
			t.Errorf("Match(%q, %q) = %q, want no action", tc.scope, tc.key, action)
		}
	}
}

func TestMatchUnknownScopeOrKeyMisses(t *testing.T) {
	registry := Default()

	if action, ok := registry.Match(Scope("nope"), "q"); ok {
		t.Errorf("Match(scope desconocido) = %q, want no action", action)
	}
	if action, ok := registry.Match(ScopeRoot, ""); ok {
		t.Errorf("Match(root, tecla vacía) = %q, want no action", action)
	}
	if action, ok := registry.Match(ScopeRoot, "zzz"); ok {
		t.Errorf("Match(root, zzz) = %q, want no action", action)
	}
}

func TestQuitAlwaysAnswersControlC(t *testing.T) {
	registry, err := Default().Apply(map[string][]string{"quit": {"x"}})
	if err != nil {
		t.Fatalf("Apply(quit: x) = %v, want nil error", err)
	}

	if action, ok := registry.Match(ScopeRoot, "ctrl+c"); !ok || action != ActionQuit {
		t.Errorf("Match(root, ctrl+c) = (%q, %v), want (%q, true)", action, ok, ActionQuit)
	}
	if action, ok := registry.Match(ScopeRoot, "x"); !ok || action != ActionQuit {
		t.Errorf("Match(root, x) = (%q, %v), want (%q, true)", action, ok, ActionQuit)
	}
	if action, ok := registry.Match(ScopeRoot, "q"); ok {
		t.Errorf("Match(root, q) = %q, want no action after the override", action)
	}
	if got, want := registry.Keys(ActionQuit), []string{"x", "ctrl+c"}; !slices.Equal(got, want) {
		t.Errorf("Keys(quit) = %v, want %v", got, want)
	}
}

func TestApplyValidOverrideReplacesKeys(t *testing.T) {
	registry, err := Default().Apply(map[string][]string{"tree_up": {"w"}})
	if err != nil {
		t.Fatalf("Apply(tree_up: w) = %v, want nil error", err)
	}

	if action, ok := registry.Match(ScopeTree, "w"); !ok || action != ActionTreeUp {
		t.Errorf("Match(tree, w) = (%q, %v), want (%q, true)", action, ok, ActionTreeUp)
	}
	if action, ok := registry.Match(ScopeTree, "up"); ok {
		t.Errorf("Match(tree, up) = %q, want no action after the override", action)
	}
	if action, ok := registry.Match(ScopeTree, "k"); ok {
		t.Errorf("Match(tree, k) = %q, want no action after the override", action)
	}
	if got, want := registry.Keys(ActionTreeUp), []string{"w"}; !slices.Equal(got, want) {
		t.Errorf("Keys(tree_up) = %v, want %v", got, want)
	}
}

func TestApplyRejectsUnknownAction(t *testing.T) {
	_, err := Default().Apply(map[string][]string{"nope": {"g"}})
	if err == nil {
		t.Fatal("Apply(nope) = nil error, want an unknown action error")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error = %q, want it to mention the action", err)
	}
}

func TestApplyRejectsNonConfigurableAction(t *testing.T) {
	_, err := Default().Apply(map[string][]string{"editor_cancel": {"g"}})
	if err == nil {
		t.Fatal("Apply(editor_cancel) = nil error, want a non-configurable error")
	}
	if !strings.Contains(err.Error(), "no es configurable") {
		t.Errorf("error = %q, want it to mention non-configurable", err)
	}
}

func TestApplyRejectsEmptyKeyList(t *testing.T) {
	cases := []struct {
		name string
		keys []string
	}{
		{name: "vacía", keys: []string{}},
		{name: "nil", keys: nil},
	}

	for _, tc := range cases {
		if _, err := Default().Apply(map[string][]string{"tree_up": tc.keys}); err == nil {
			t.Errorf("Apply(tree_up: %s) = nil error, want an empty list error", tc.name)
		}
	}
}

func TestApplyRejectsInvalidKey(t *testing.T) {
	keys := []string{"", "   ", "foo", "ctrl+", "ctrl+ab", "shift+x", "ctrl+tab", "meta+x", "f13"}

	for _, key := range keys {
		if _, err := Default().Apply(map[string][]string{"tree_up": {key}}); err == nil {
			t.Errorf("Apply(tree_up: %q) = nil error, want an invalid key error", key)
		}
	}
}

func TestApplyRejectsReservedKeys(t *testing.T) {
	keys := []string{"ctrl+c", "esc", "enter"}

	for _, key := range keys {
		_, err := Default().Apply(map[string][]string{"tree_up": {key}})
		if err == nil {
			t.Errorf("Apply(tree_up: %q) = nil error, want a reserved key error", key)
			continue
		}
		if !strings.Contains(err.Error(), "reservada") {
			t.Errorf("Apply(tree_up: %q) error = %q, want it to mention the reservation", key, err)
		}
	}
}

func TestApplyRejectsConflictInsideScope(t *testing.T) {
	_, err := Default().Apply(map[string][]string{"tree_up": {"down"}})
	if err == nil {
		t.Fatal("Apply(tree_up: down) = nil error, want a conflict error")
	}

	for _, part := range []string{"tree", "down", "tree_up", "tree_down"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error = %q, want it to mention %q", err, part)
		}
	}
}

func TestApplyAllowsTheSameKeyInDifferentScopes(t *testing.T) {
	registry := Default()

	// "enter" is shared by several scopes and "d" lives in git, not in tree.
	scopes := []Scope{ScopeTree, ScopeGit, ScopeDocker, ScopeGitConfirm, ScopeDockerConfirm, ScopeGitPrompt}
	for _, scope := range scopes {
		if _, ok := registry.Match(scope, "enter"); !ok {
			t.Errorf("Match(%q, enter) not found", scope)
		}
	}

	if _, err := Default().Apply(map[string][]string{"tree_up": {"d"}}); err != nil {
		t.Errorf("Apply(tree_up: d) = %v, want nil error", err)
	}
}

func TestApplyDeduplicatesPreservingOrder(t *testing.T) {
	registry, err := Default().Apply(map[string][]string{"tree_up": {"up", "w", "up", "w"}})
	if err != nil {
		t.Fatalf("Apply(tree_up) = %v, want nil error", err)
	}

	if got, want := registry.Keys(ActionTreeUp), []string{"up", "w"}; !slices.Equal(got, want) {
		t.Errorf("Keys(tree_up) = %v, want %v", got, want)
	}
}

func TestApplyDoesNotMutateTheDefaultNorTheReceiver(t *testing.T) {
	registry := Default()
	before := registry.Keys(ActionTreeUp)

	updated, err := registry.Apply(map[string][]string{"tree_up": {"w"}})
	if err != nil {
		t.Fatalf("Apply = %v, want nil error", err)
	}

	if got := registry.Keys(ActionTreeUp); !slices.Equal(got, before) {
		t.Errorf("el receptor cambió: Keys(tree_up) = %v, want %v", got, before)
	}
	if action, ok := registry.Match(ScopeTree, "up"); !ok || action != ActionTreeUp {
		t.Errorf("el receptor cambió: Match(tree, up) = (%q, %v), want (%q, true)", action, ok, ActionTreeUp)
	}
	if action, ok := updated.Match(ScopeTree, "w"); !ok || action != ActionTreeUp {
		t.Errorf("el registro nuevo no aplicó el override: Match(tree, w) = (%q, %v)", action, ok)
	}
	if got, want := Default().Keys(ActionTreeUp), []string{"up", "k"}; !slices.Equal(got, want) {
		t.Errorf("el default cambió: Keys(tree_up) = %v, want %v", got, want)
	}
}

func TestKeysReturnsACopy(t *testing.T) {
	registry := Default()

	keys := registry.Keys(ActionTreeUp)
	if len(keys) == 0 {
		t.Fatal("Keys(tree_up) está vacío")
	}

	keys[0] = "mutada"
	if got := registry.Keys(ActionTreeUp)[0]; got == "mutada" {
		t.Errorf("Keys compartió el slice interno: %v", registry.Keys(ActionTreeUp))
	}
}

func TestLabelJoinsTheEffectiveKeys(t *testing.T) {
	registry := Default()

	cases := []struct {
		action Action
		want   string
	}{
		{action: ActionQuit, want: "q / ctrl+c"},
		{action: ActionTreeUp, want: "up / k"},
		{action: ActionGitHunkToggle, want: "space"},
		{action: Action("nope"), want: ""},
	}

	for _, tc := range cases {
		if got := registry.Label(tc.action); got != tc.want {
			t.Errorf("Label(%q) = %q, want %q", tc.action, got, tc.want)
		}
	}
}

func TestBindingsFilterInCatalogOrderAndCopy(t *testing.T) {
	registry := Default()

	bindings := registry.Bindings(ScopeTree)
	got := make([]Action, 0, len(bindings))
	for _, binding := range bindings {
		got = append(got, binding.Action)
	}

	want := []Action{
		ActionTreeUp, ActionTreeDown, ActionTreePageUp, ActionTreePageDown, ActionTreeHome, ActionTreeEnd,
		ActionTreeExpand, ActionTreeCollapse, ActionTreeActivate, ActionTreeFilter, ActionTreeClearFilter,
	}
	if !slices.Equal(got, want) {
		t.Errorf("Bindings(tree) = %v, want %v", got, want)
	}

	bindings[0].Keys[0] = "mutada"
	if got := registry.Bindings(ScopeTree)[0].Keys[0]; got == "mutada" {
		t.Errorf("Bindings compartió el slice interno: %q", got)
	}

	if empty := registry.Bindings(Scope("nope")); len(empty) != 0 {
		t.Errorf("Bindings(scope desconocido) = %v, want empty", empty)
	}
}

func TestAllAndConfigurableReturnCopiesAndCounts(t *testing.T) {
	registry := Default()

	all := registry.All()
	if len(all) != len(expectedActions) {
		t.Errorf("All() = %d bindings, want %d", len(all), len(expectedActions))
	}
	if all[0].Action != expectedActions[0] {
		t.Errorf("All() empieza en %q, want %q", all[0].Action, expectedActions[0])
	}
	if last := all[len(all)-1].Action; last != ActionDockerLogsEnd {
		t.Errorf("All() termina en %q, want %q", last, ActionDockerLogsEnd)
	}

	all[0].Keys[0] = "mutada"
	if got := registry.All()[0].Keys[0]; got == "mutada" {
		t.Errorf("All compartió el slice interno: %q", got)
	}

	configurable := registry.Configurable()
	if len(configurable) != 60 {
		t.Errorf("Configurable() = %d bindings, want 60", len(configurable))
	}
	for _, binding := range configurable {
		if !binding.Configurable {
			t.Errorf("Configurable() devolvió %q, que no es configurable", binding.Action)
		}
	}

	configurable[0].Keys[0] = "mutada"
	if got := registry.Configurable()[0].Keys[0]; got == "mutada" {
		t.Errorf("Configurable compartió el slice interno: %q", got)
	}
}
