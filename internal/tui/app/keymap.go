package app

// binding is one documented root keymap entry, rendered in the help overlay.
type binding struct {
	keys string
	desc string
}

// bindings are the root-level shortcuts; panels add their own documentation.
var bindings = []binding{
	{keys: "1 / 2 / 3", desc: "cambiar de pestaña (Files, Git, Docker)"},
	{keys: "tab", desc: "alternar el foco entre el árbol y el editor"},
	{keys: "shift+tab", desc: "alternar el foco en sentido inverso"},
	{keys: "esc", desc: "cerrar la ayuda o reenviarla al panel"},
	{keys: "?", desc: "mostrar u ocultar esta ayuda"},
	{keys: "q / ctrl+c", desc: "salir"},
}

// helpLines renders the full keymap shown by the help overlay.
func helpLines() []string {
	lines := make([]string, 0, len(bindings)+5)
	lines = append(lines, styles.helpTitle.Render("tui-editor · atajos"))
	lines = append(lines, "")

	for _, b := range bindings {
		lines = append(lines, styles.helpKey.Render(b.keys)+"  "+b.desc)
	}

	lines = append(lines, "")
	lines = append(lines, "files: e edita el archivo, ctrl+s guarda y E abre el editor externo")
	lines = append(lines, "git: 2 abre la pestaña; dentro, ? documenta cada sección y sus modales")
	lines = append(lines, "docker: 3 abre la pestaña; l logs, s/x/R start/stop/restart con confirmación")

	return lines
}
