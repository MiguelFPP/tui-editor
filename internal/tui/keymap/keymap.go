// Package keymap centralizes the keyboard catalog of the TUI: the default
// bindings per scope, the canonical key parser, the validation of user
// overrides, the conflicts inside a scope and the reserved keys.
//
// Production code only depends on the standard library: the parser understands
// the canonical strings produced by Bubble Tea's KeyPressMsg.
package keymap

import (
	"fmt"
	"sort"
	"strings"
)

// Scope identifies a keyboard context: a screen, a panel or a modal.
type Scope string

// Scopes of the application.
const (
	ScopeRoot          Scope = "root"
	ScopeTree          Scope = "tree"
	ScopeEditor        Scope = "editor"
	ScopeEditorDraft   Scope = "editor_draft"
	ScopeEditorDiscard Scope = "editor_discard"
	ScopeGit           Scope = "git"
	ScopeGitCommit     Scope = "git_commit"
	ScopeGitPrompt     Scope = "git_prompt"
	ScopeGitConfirm    Scope = "git_confirm"
	ScopeDocker        Scope = "docker"
	ScopeDockerConfirm Scope = "docker_confirm"
	ScopeDockerLogs    Scope = "docker_logs"
)

// Action is the stable identifier of one user-facing command.
type Action string

// Actions of the root scope.
const (
	ActionQuit      Action = "quit"
	ActionHelp      Action = "help"
	ActionTabFiles  Action = "tab_files"
	ActionTabGit    Action = "tab_git"
	ActionTabDocker Action = "tab_docker"
	ActionFocusNext Action = "focus_next"
	ActionFocusPrev Action = "focus_prev"
)

// Actions of the file tree scope.
const (
	ActionTreeUp          Action = "tree_up"
	ActionTreeDown        Action = "tree_down"
	ActionTreePageUp      Action = "tree_page_up"
	ActionTreePageDown    Action = "tree_page_down"
	ActionTreeHome        Action = "tree_home"
	ActionTreeEnd         Action = "tree_end"
	ActionTreeExpand      Action = "tree_expand"
	ActionTreeCollapse    Action = "tree_collapse"
	ActionTreeActivate    Action = "tree_activate"
	ActionTreeFilter      Action = "tree_filter"
	ActionTreeClearFilter Action = "tree_clear_filter"
)

// Actions of the editor scopes.
const (
	ActionEditorEdit       Action = "editor_edit"
	ActionEditorExternal   Action = "editor_external"
	ActionEditorSave       Action = "editor_save"
	ActionEditorCancel     Action = "editor_cancel"
	ActionEditorDiscardYes Action = "editor_discard_yes"
	ActionEditorDiscardNo  Action = "editor_discard_no"
)

// Actions of the Git scopes.
const (
	ActionGitSectionNext   Action = "git_section_next"
	ActionGitSectionPrev   Action = "git_section_prev"
	ActionGitUp            Action = "git_up"
	ActionGitDown          Action = "git_down"
	ActionGitActivate      Action = "git_activate"
	ActionGitDiff          Action = "git_diff"
	ActionGitStage         Action = "git_stage"
	ActionGitUnstage       Action = "git_unstage"
	ActionGitCommit        Action = "git_commit"
	ActionGitNewBranch     Action = "git_new_branch"
	ActionGitDeleteBranch  Action = "git_delete_branch"
	ActionGitMerge         Action = "git_merge"
	ActionGitStash         Action = "git_stash"
	ActionGitResolve       Action = "git_resolve"
	ActionGitResolveTheirs Action = "git_resolve_theirs"
	ActionGitAbortMerge    Action = "git_abort_merge"
	ActionGitFetch         Action = "git_fetch"
	ActionGitPull          Action = "git_pull"
	ActionGitPush          Action = "git_push"
	ActionGitHunkPrev      Action = "git_hunk_prev"
	ActionGitHunkNext      Action = "git_hunk_next"
	ActionGitHunkToggle    Action = "git_hunk_toggle"
	ActionGitRefresh       Action = "git_refresh"
	ActionGitHelp          Action = "git_help"
	ActionGitBack          Action = "git_back"
)

// Actions of the Git modal scopes.
const (
	ActionGitCommitSave    Action = "git_commit_save"
	ActionGitCommitCancel  Action = "git_commit_cancel"
	ActionGitPromptConfirm Action = "git_prompt_confirm"
	ActionGitPromptCancel  Action = "git_prompt_cancel"
	ActionGitConfirmYes    Action = "git_confirm_yes"
	ActionGitConfirmNo     Action = "git_confirm_no"
)

// Actions of the Docker scopes.
const (
	ActionDockerUp          Action = "docker_up"
	ActionDockerDown        Action = "docker_down"
	ActionDockerPageUp      Action = "docker_page_up"
	ActionDockerPageDown    Action = "docker_page_down"
	ActionDockerHome        Action = "docker_home"
	ActionDockerEnd         Action = "docker_end"
	ActionDockerStats       Action = "docker_stats"
	ActionDockerLogs        Action = "docker_logs"
	ActionDockerStart       Action = "docker_start"
	ActionDockerStop        Action = "docker_stop"
	ActionDockerRestart     Action = "docker_restart"
	ActionDockerRefresh     Action = "docker_refresh"
	ActionDockerScopeToggle Action = "docker_scope_toggle"
	ActionDockerConfirmYes  Action = "docker_confirm_yes"
	ActionDockerConfirmNo   Action = "docker_confirm_no"
)

// Actions of the Docker logs scope.
const (
	ActionDockerLogsClose    Action = "docker_logs_close"
	ActionDockerLogsReload   Action = "docker_logs_reload"
	ActionDockerLogsUp       Action = "docker_logs_up"
	ActionDockerLogsDown     Action = "docker_logs_down"
	ActionDockerLogsPageUp   Action = "docker_logs_page_up"
	ActionDockerLogsPageDown Action = "docker_logs_page_down"
	ActionDockerLogsHome     Action = "docker_logs_home"
	ActionDockerLogsEnd      Action = "docker_logs_end"
)

// Binding is one action inside a scope together with its keys and description.
//
// Keys are the configurable keys; FixedKeys are always active and cannot be
// overridden. Configurable reports whether the user may replace Keys.
type Binding struct {
	Action       Action
	Scope        Scope
	Keys         []string
	FixedKeys    []string
	Desc         string
	Configurable bool
}

// Registry is an immutable catalog indexed by action and by scope. The zero
// value is not usable: build it with Default or Apply.
type Registry struct {
	bindings []Binding
	actions  map[Action]int
	scopes   map[Scope]map[string]Action
}

// Default returns the built-in registry. It is the only source of defaults and
// descriptions, and it never shares slices with the returned value.
func Default() Registry {
	return mustRegistry(cloneBindings(catalog))
}

// Apply returns a new registry with overrides applied on top of r. It never
// mutates r nor the defaults, so registries can be chained. Overrides are
// validated in action order to keep the reported error deterministic.
func (r Registry) Apply(overrides map[string][]string) (Registry, error) {
	bindings := cloneBindings(r.bindings)

	for _, name := range sortedActions(overrides) {
		action := Action(name)

		index, ok := r.actions[action]
		if !ok {
			return Registry{}, fmt.Errorf("keymap: acción desconocida %q", name)
		}

		binding := &bindings[index]
		if !binding.Configurable {
			return Registry{}, fmt.Errorf("keymap: la acción %q no es configurable", name)
		}

		keys, err := normalizeKeys(action, overrides[name])
		if err != nil {
			return Registry{}, err
		}

		binding.Keys = keys
	}

	return newRegistry(bindings)
}

// Match canonicalizes key and resolves it inside scope.
func (r Registry) Match(scope Scope, key string) (Action, bool) {
	canonical := CanonicalKey(key)
	if canonical == "" {
		return "", false
	}

	keys, ok := r.scopes[scope]
	if !ok {
		return "", false
	}

	action, ok := keys[canonical]

	return action, ok
}

// Keys returns the effective keys of action (configurable plus fixed) in order.
// The result is always a fresh slice: callers never see the internal one.
func (r Registry) Keys(action Action) []string {
	index, ok := r.actions[action]
	if !ok {
		return nil
	}

	return effectiveKeys(r.bindings[index])
}

// Label joins the effective keys of action with " / ".
func (r Registry) Label(action Action) string {
	return strings.Join(r.Keys(action), " / ")
}

// Bindings returns the bindings of scope in catalog order, with copied slices.
func (r Registry) Bindings(scope Scope) []Binding {
	bindings := make([]Binding, 0, len(r.bindings))
	for _, binding := range r.bindings {
		if binding.Scope == scope {
			bindings = append(bindings, cloneBinding(binding))
		}
	}

	return bindings
}

// Configurable returns only the bindings the user may override, in catalog order.
func (r Registry) Configurable() []Binding {
	bindings := make([]Binding, 0, len(r.bindings))
	for _, binding := range r.bindings {
		if binding.Configurable {
			bindings = append(bindings, cloneBinding(binding))
		}
	}

	return bindings
}

// All returns every binding in catalog order, with copied slices.
func (r Registry) All() []Binding {
	bindings := make([]Binding, 0, len(r.bindings))
	for _, binding := range r.bindings {
		bindings = append(bindings, cloneBinding(binding))
	}

	return bindings
}

// reservedKeys cannot be used by user overrides; the defaults may still use them.
var reservedKeys = map[string]struct{}{
	"ctrl+c": {},
	"esc":    {},
	"enter":  {},
}

// catalog is the single source of the default bindings, in display order.
var catalog = []Binding{
	// root
	{Action: ActionQuit, Scope: ScopeRoot, Keys: []string{"q"}, FixedKeys: []string{"ctrl+c"}, Desc: "salir de la aplicación", Configurable: true},
	{Action: ActionHelp, Scope: ScopeRoot, Keys: []string{"?"}, Desc: "abrir la ayuda", Configurable: true},
	{Action: ActionTabFiles, Scope: ScopeRoot, Keys: []string{"1"}, Desc: "pestaña de archivos", Configurable: true},
	{Action: ActionTabGit, Scope: ScopeRoot, Keys: []string{"2"}, Desc: "pestaña de Git", Configurable: true},
	{Action: ActionTabDocker, Scope: ScopeRoot, Keys: []string{"3"}, Desc: "pestaña de Docker", Configurable: true},
	{Action: ActionFocusNext, Scope: ScopeRoot, Keys: []string{"tab"}, Desc: "siguiente panel", Configurable: true},
	{Action: ActionFocusPrev, Scope: ScopeRoot, Keys: []string{"shift+tab"}, Desc: "panel anterior", Configurable: true},

	// tree
	{Action: ActionTreeUp, Scope: ScopeTree, Keys: []string{"up", "k"}, Desc: "subir", Configurable: true},
	{Action: ActionTreeDown, Scope: ScopeTree, Keys: []string{"down", "j"}, Desc: "bajar", Configurable: true},
	{Action: ActionTreePageUp, Scope: ScopeTree, Keys: []string{"pgup"}, Desc: "página arriba", Configurable: true},
	{Action: ActionTreePageDown, Scope: ScopeTree, Keys: []string{"pgdown"}, Desc: "página abajo", Configurable: true},
	{Action: ActionTreeHome, Scope: ScopeTree, Keys: []string{"home"}, Desc: "inicio del árbol", Configurable: true},
	{Action: ActionTreeEnd, Scope: ScopeTree, Keys: []string{"end"}, Desc: "fin del árbol", Configurable: true},
	{Action: ActionTreeExpand, Scope: ScopeTree, Keys: []string{"right", "l"}, Desc: "expandir carpeta", Configurable: true},
	{Action: ActionTreeCollapse, Scope: ScopeTree, Keys: []string{"left", "h"}, Desc: "colapsar carpeta", Configurable: true},
	{Action: ActionTreeActivate, Scope: ScopeTree, Keys: []string{"enter", "space"}, Desc: "abrir archivo", Configurable: true},
	{Action: ActionTreeFilter, Scope: ScopeTree, Keys: []string{"/"}, Desc: "filtrar", Configurable: true},
	{Action: ActionTreeClearFilter, Scope: ScopeTree, Keys: []string{"esc"}, Desc: "limpiar filtro", Configurable: true},

	// editor
	{Action: ActionEditorEdit, Scope: ScopeEditor, Keys: []string{"e"}, Desc: "editar archivo", Configurable: true},
	{Action: ActionEditorExternal, Scope: ScopeEditor, Keys: []string{"E", "ctrl+e"}, Desc: "editar en editor externo", Configurable: true},

	// editor draft
	{Action: ActionEditorSave, Scope: ScopeEditorDraft, Keys: []string{"ctrl+s"}, Desc: "guardar borrador", Configurable: true},
	{Action: ActionEditorCancel, Scope: ScopeEditorDraft, Keys: []string{"esc"}, Desc: "cancelar borrador", Configurable: false},

	// editor discard
	{Action: ActionEditorDiscardYes, Scope: ScopeEditorDiscard, Keys: []string{"y"}, Desc: "descartar cambios", Configurable: false},
	{Action: ActionEditorDiscardNo, Scope: ScopeEditorDiscard, Keys: []string{"n", "esc"}, Desc: "conservar cambios", Configurable: false},

	// git
	{Action: ActionGitSectionNext, Scope: ScopeGit, Keys: []string{"tab"}, Desc: "siguiente sección", Configurable: true},
	{Action: ActionGitSectionPrev, Scope: ScopeGit, Keys: []string{"shift+tab"}, Desc: "sección anterior", Configurable: true},
	{Action: ActionGitUp, Scope: ScopeGit, Keys: []string{"j", "down"}, Desc: "subir", Configurable: true},
	{Action: ActionGitDown, Scope: ScopeGit, Keys: []string{"k", "up"}, Desc: "bajar", Configurable: true},
	{Action: ActionGitActivate, Scope: ScopeGit, Keys: []string{"enter"}, Desc: "abrir selección", Configurable: true},
	{Action: ActionGitDiff, Scope: ScopeGit, Keys: []string{"d"}, Desc: "ver diff", Configurable: true},
	{Action: ActionGitStage, Scope: ScopeGit, Keys: []string{"s"}, Desc: "preparar cambio", Configurable: true},
	{Action: ActionGitUnstage, Scope: ScopeGit, Keys: []string{"u"}, Desc: "quitar de preparados", Configurable: true},
	{Action: ActionGitCommit, Scope: ScopeGit, Keys: []string{"c"}, Desc: "escribir commit", Configurable: true},
	{Action: ActionGitNewBranch, Scope: ScopeGit, Keys: []string{"n"}, Desc: "nueva rama", Configurable: true},
	{Action: ActionGitDeleteBranch, Scope: ScopeGit, Keys: []string{"x"}, Desc: "borrar rama", Configurable: true},
	{Action: ActionGitMerge, Scope: ScopeGit, Keys: []string{"m"}, Desc: "fusionar rama", Configurable: true},
	{Action: ActionGitStash, Scope: ScopeGit, Keys: []string{"z"}, Desc: "guardar en stash", Configurable: true},
	{Action: ActionGitResolve, Scope: ScopeGit, Keys: []string{"o"}, Desc: "resolver conflicto", Configurable: true},
	{Action: ActionGitResolveTheirs, Scope: ScopeGit, Keys: []string{"t"}, Desc: "usar versión entrante", Configurable: true},
	{Action: ActionGitAbortMerge, Scope: ScopeGit, Keys: []string{"A"}, Desc: "abortar fusión", Configurable: true},
	{Action: ActionGitFetch, Scope: ScopeGit, Keys: []string{"f"}, Desc: "traer cambios", Configurable: true},
	{Action: ActionGitPull, Scope: ScopeGit, Keys: []string{"p"}, Desc: "hacer pull", Configurable: true},
	{Action: ActionGitPush, Scope: ScopeGit, Keys: []string{"P"}, Desc: "hacer push", Configurable: true},
	{Action: ActionGitHunkPrev, Scope: ScopeGit, Keys: []string{"["}, Desc: "hunk anterior", Configurable: true},
	{Action: ActionGitHunkNext, Scope: ScopeGit, Keys: []string{"]"}, Desc: "hunk siguiente", Configurable: true},
	{Action: ActionGitHunkToggle, Scope: ScopeGit, Keys: []string{"space"}, Desc: "marcar hunk", Configurable: true},
	{Action: ActionGitRefresh, Scope: ScopeGit, Keys: []string{"r"}, Desc: "refrescar", Configurable: true},
	{Action: ActionGitHelp, Scope: ScopeGit, Keys: []string{"?"}, Desc: "ayuda de Git", Configurable: true},
	{Action: ActionGitBack, Scope: ScopeGit, Keys: []string{"esc"}, Desc: "cerrar panel Git", Configurable: true},

	// git commit
	{Action: ActionGitCommitSave, Scope: ScopeGitCommit, Keys: []string{"ctrl+s"}, Desc: "guardar commit", Configurable: false},
	{Action: ActionGitCommitCancel, Scope: ScopeGitCommit, Keys: []string{"esc"}, Desc: "cancelar commit", Configurable: false},

	// git prompt
	{Action: ActionGitPromptConfirm, Scope: ScopeGitPrompt, Keys: []string{"enter"}, Desc: "confirmar entrada", Configurable: false},
	{Action: ActionGitPromptCancel, Scope: ScopeGitPrompt, Keys: []string{"esc"}, Desc: "cancelar entrada", Configurable: false},

	// git confirm
	{Action: ActionGitConfirmYes, Scope: ScopeGitConfirm, Keys: []string{"y", "enter"}, Desc: "confirmar", Configurable: false},
	{Action: ActionGitConfirmNo, Scope: ScopeGitConfirm, Keys: []string{"n", "esc"}, Desc: "cancelar", Configurable: false},

	// docker
	{Action: ActionDockerUp, Scope: ScopeDocker, Keys: []string{"j", "down"}, Desc: "subir", Configurable: true},
	{Action: ActionDockerDown, Scope: ScopeDocker, Keys: []string{"k", "up"}, Desc: "bajar", Configurable: true},
	{Action: ActionDockerPageUp, Scope: ScopeDocker, Keys: []string{"pgup"}, Desc: "página arriba", Configurable: true},
	{Action: ActionDockerPageDown, Scope: ScopeDocker, Keys: []string{"pgdown"}, Desc: "página abajo", Configurable: true},
	{Action: ActionDockerHome, Scope: ScopeDocker, Keys: []string{"home"}, Desc: "inicio de la lista", Configurable: true},
	{Action: ActionDockerEnd, Scope: ScopeDocker, Keys: []string{"end"}, Desc: "fin de la lista", Configurable: true},
	{Action: ActionDockerStats, Scope: ScopeDocker, Keys: []string{"enter"}, Desc: "ver estadísticas", Configurable: true},
	{Action: ActionDockerLogs, Scope: ScopeDocker, Keys: []string{"l"}, Desc: "ver logs", Configurable: true},
	{Action: ActionDockerStart, Scope: ScopeDocker, Keys: []string{"s"}, Desc: "arrancar contenedor", Configurable: true},
	{Action: ActionDockerStop, Scope: ScopeDocker, Keys: []string{"x"}, Desc: "detener contenedor", Configurable: true},
	{Action: ActionDockerRestart, Scope: ScopeDocker, Keys: []string{"R"}, Desc: "reiniciar contenedor", Configurable: true},
	{Action: ActionDockerRefresh, Scope: ScopeDocker, Keys: []string{"r"}, Desc: "refrescar", Configurable: true},
	{Action: ActionDockerScopeToggle, Scope: ScopeDocker, Keys: []string{"a"}, Desc: "alternar alcance", Configurable: true},

	// docker confirm
	{Action: ActionDockerConfirmYes, Scope: ScopeDockerConfirm, Keys: []string{"y", "enter"}, Desc: "confirmar", Configurable: false},
	{Action: ActionDockerConfirmNo, Scope: ScopeDockerConfirm, Keys: []string{"n", "esc"}, Desc: "cancelar", Configurable: false},

	// docker logs
	{Action: ActionDockerLogsClose, Scope: ScopeDockerLogs, Keys: []string{"esc"}, Desc: "cerrar logs", Configurable: false},
	{Action: ActionDockerLogsReload, Scope: ScopeDockerLogs, Keys: []string{"r"}, Desc: "recargar logs", Configurable: false},
	{Action: ActionDockerLogsUp, Scope: ScopeDockerLogs, Keys: []string{"j", "down"}, Desc: "subir", Configurable: false},
	{Action: ActionDockerLogsDown, Scope: ScopeDockerLogs, Keys: []string{"k", "up"}, Desc: "bajar", Configurable: false},
	{Action: ActionDockerLogsPageUp, Scope: ScopeDockerLogs, Keys: []string{"pgup"}, Desc: "página arriba", Configurable: false},
	{Action: ActionDockerLogsPageDown, Scope: ScopeDockerLogs, Keys: []string{"pgdown"}, Desc: "página abajo", Configurable: false},
	{Action: ActionDockerLogsHome, Scope: ScopeDockerLogs, Keys: []string{"home"}, Desc: "inicio de los logs", Configurable: false},
	{Action: ActionDockerLogsEnd, Scope: ScopeDockerLogs, Keys: []string{"end"}, Desc: "fin de los logs", Configurable: false},
}

// newRegistry indexes bindings by action and by scope, rejecting duplicates.
func newRegistry(bindings []Binding) (Registry, error) {
	registry := Registry{
		bindings: bindings,
		actions:  make(map[Action]int, len(bindings)),
	}

	for index, binding := range bindings {
		if _, ok := registry.actions[binding.Action]; ok {
			return Registry{}, fmt.Errorf("keymap: acción duplicada %q", binding.Action)
		}

		registry.actions[binding.Action] = index
	}

	scopes, err := indexScopes(bindings)
	if err != nil {
		return Registry{}, err
	}

	registry.scopes = scopes

	return registry, nil
}

// mustRegistry builds a registry and panics only when the static catalog is
// inconsistent, which is a programming error.
func mustRegistry(bindings []Binding) Registry {
	registry, err := newRegistry(bindings)
	if err != nil {
		panic("keymap: catálogo de defaults inválido: " + err.Error())
	}

	return registry
}

// indexScopes maps every effective key of every scope to its action and rejects
// two actions sharing a key inside the same scope.
func indexScopes(bindings []Binding) (map[Scope]map[string]Action, error) {
	scopes := make(map[Scope]map[string]Action, len(bindings))

	for _, binding := range bindings {
		keys := scopes[binding.Scope]
		if keys == nil {
			keys = make(map[string]Action, len(binding.Keys)+len(binding.FixedKeys))
			scopes[binding.Scope] = keys
		}

		for _, key := range effectiveKeys(binding) {
			if previous, ok := keys[key]; ok {
				return nil, fmt.Errorf(
					"keymap: conflicto en el scope %q: la tecla %q está asignada a %q y a %q",
					binding.Scope, key, previous, binding.Action,
				)
			}

			keys[key] = binding.Action
		}
	}

	return scopes, nil
}

// normalizeKeys validates one override list and returns the canonical, unique
// keys in order.
func normalizeKeys(action Action, raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("keymap: la acción %q necesita al menos una tecla", action)
	}

	keys := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))

	for _, item := range raw {
		key, err := ParseKey(item)
		if err != nil {
			return nil, fmt.Errorf("keymap: la acción %q: tecla inválida %q", action, item)
		}

		if _, reserved := reservedKeys[key]; reserved {
			return nil, fmt.Errorf("keymap: la acción %q: la tecla %q está reservada", action, key)
		}

		if _, duplicated := seen[key]; duplicated {
			continue
		}

		seen[key] = struct{}{}
		keys = append(keys, key)
	}

	return keys, nil
}

// effectiveKeys returns the configurable keys followed by the fixed ones. The
// result is always a fresh slice.
func effectiveKeys(binding Binding) []string {
	keys := make([]string, 0, len(binding.Keys)+len(binding.FixedKeys))
	keys = append(keys, binding.Keys...)
	keys = append(keys, binding.FixedKeys...)

	return keys
}

// sortedActions returns the override keys in deterministic order.
func sortedActions(overrides map[string][]string) []string {
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func cloneBindings(bindings []Binding) []Binding {
	if bindings == nil {
		return nil
	}

	cloned := make([]Binding, len(bindings))
	for index, binding := range bindings {
		cloned[index] = cloneBinding(binding)
	}

	return cloned
}

func cloneBinding(binding Binding) Binding {
	binding.Keys = cloneStrings(binding.Keys)
	binding.FixedKeys = cloneStrings(binding.FixedKeys)

	return binding
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]string, len(values))
	copy(cloned, values)

	return cloned
}
