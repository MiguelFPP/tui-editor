package app

// binding is one documented root keymap entry, rendered in the help overlay.
type binding struct {
	keys string
	desc string
}

// bindings are the root-level shortcuts; panels add their own documentation.
var bindings = []binding{
	{keys: "1 / 2", desc: "cambiar de pestaña (Files, Git)"},
	{keys: "tab", desc: "alternar el foco entre el árbol y el editor"},
	{keys: "shift+tab", desc: "alternar el foco en sentido inverso"},
	{keys: "esc", desc: "cerrar la ayuda o reenviarla al panel"},
	{keys: "?", desc: "mostrar u ocultar esta ayuda"},
	{keys: "q / ctrl+c", desc: "salir"},
}

// helpLines renders the full keymap shown by the help overlay.
func helpLines() []string {
	lines := make([]string, 0, len(bindings)+4)
	lines = append(lines, styles.helpTitle.Render("tui-editor · atajos"))
	lines = append(lines, "")

	for _, b := range bindings {
		lines = append(lines, styles.helpKey.Render(b.keys)+"  "+b.desc)
	}

	lines = append(lines, "")
	lines = append(lines, "git: tab cambia de sección, enter carga el diff, d alterna staged, r refresca")
	lines = append(lines, "git: s/u preparan o quitan archivos y hunks, c abre el commit, ctrl+s lo confirma")

	return lines
}
