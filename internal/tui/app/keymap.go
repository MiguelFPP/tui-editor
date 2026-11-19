package app

import (
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

// helpLines renders the full keymap shown by the help overlay: the root scope of
// the registry, plus the keys every panel documents in its own footer.
func helpLines(keys keymap.Registry) []string {
	bindings := keys.Bindings(keymap.ScopeRoot)

	lines := make([]string, 0, len(bindings)+8)
	lines = append(lines, styles.HelpTitle.Render("tui-editor · atajos"))
	lines = append(lines, "")

	for _, binding := range bindings {
		lines = append(lines, styles.HelpKey.Render(keys.Label(binding.Action))+"  "+binding.Desc)
	}

	lines = append(lines, styles.HelpKey.Render("esc")+"  cerrar la ayuda o reenviarla al panel")
	lines = append(lines, "")
	lines = append(lines, "files: "+keys.Label(keymap.ActionEditorEdit)+" edita el archivo, "+
		keys.Label(keymap.ActionEditorSave)+" guarda y "+keys.Label(keymap.ActionEditorExternal)+" abre el editor externo")
	lines = append(lines, "git: "+keys.Label(keymap.ActionTabGit)+" abre la pestaña; dentro, "+
		keys.Label(keymap.ActionGitHelp)+" documenta cada sección y sus modales")
	lines = append(lines, "docker: "+keys.Label(keymap.ActionTabDocker)+" abre la pestaña; "+
		keys.Label(keymap.ActionDockerLogs)+" logs, "+keys.Label(keymap.ActionDockerStart)+"/"+
		keys.Label(keymap.ActionDockerStop)+"/"+keys.Label(keymap.ActionDockerRestart)+" start/stop/restart con confirmación")
	lines = append(lines, "layout: "+keys.Label(keymap.ActionToggleTree)+" minimiza o muestra el árbol, "+
		keys.Label(keymap.ActionTreeShrink)+"/"+keys.Label(keymap.ActionTreeGrow)+" lo redimensionan")

	return lines
}
