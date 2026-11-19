package styles

import (
	"slices"
	"testing"

	chromastyles "github.com/alecthomas/chroma/v2/styles"
)

// wantNames pins the catalog order exposed by the package.
var wantNames = []string{"tokyonight-night", "catppuccin-mocha", "dracula", "gruvbox-dark"}

func TestThemesCatalog(t *testing.T) {
	themes := Themes()
	if len(themes) != len(wantNames) {
		t.Fatalf("len(Themes()) = %d, want %d", len(themes), len(wantNames))
	}

	seen := make(map[string]bool, len(themes))
	for _, theme := range themes {
		if seen[theme.Name] {
			t.Errorf("theme %q is duplicated in the catalog", theme.Name)
		}
		seen[theme.Name] = true

		fields := []struct {
			name  string
			value string
		}{
			{"Name", theme.Name},
			{"Description", theme.Description},
			{"ChromaStyle", theme.ChromaStyle},
			{"Accent", theme.Accent},
			{"Muted", theme.Muted},
			{"Border", theme.Border},
			{"Green", theme.Green},
			{"Red", theme.Red},
		}
		for _, field := range fields {
			if field.value == "" {
				t.Errorf("theme %q has an empty %s", theme.Name, field.name)
			}
		}

		if theme.Accent == theme.Border {
			t.Errorf("theme %q repeats Accent and Border as %q, want two distinct colours", theme.Name, theme.Accent)
		}
		if chromastyles.Get(theme.ChromaStyle) == nil {
			t.Errorf("theme %q maps to the unknown Chroma style %q", theme.Name, theme.ChromaStyle)
		}
	}
}

func TestThemesReturnsACopy(t *testing.T) {
	themes := Themes()
	if len(themes) == 0 {
		t.Fatal("Themes() = nil, want the catalog")
	}

	themes[0].Name = "mutated"

	if again := Themes(); again[0].Name == "mutated" {
		t.Error("Themes() shares its backing array, want a copy per call")
	}
}

func TestDefaultThemeIsTokyoNight(t *testing.T) {
	def := DefaultTheme()
	if def.Name != wantNames[0] {
		t.Errorf("DefaultTheme().Name = %q, want %q", def.Name, wantNames[0])
	}
	if first := Themes()[0]; first != def {
		t.Errorf("Themes()[0] = %+v, want the default %+v first", first, def)
	}
}

func TestThemeByName(t *testing.T) {
	theme, ok := ThemeByName("catppuccin-mocha")
	if !ok {
		t.Fatal("ThemeByName(catppuccin-mocha) not found, want it in the catalog")
	}
	if theme.Name != "catppuccin-mocha" {
		t.Errorf("ThemeByName(catppuccin-mocha).Name = %q", theme.Name)
	}

	if trimmed, ok := ThemeByName("  dracula  "); !ok || trimmed.Name != "dracula" {
		t.Errorf("ThemeByName(\"  dracula  \") = %q (%v), want dracula", trimmed.Name, ok)
	}

	if notFound, ok := ThemeByName("nope"); ok {
		t.Errorf("ThemeByName(nope) = %+v, want not found", notFound)
	}
	if notFound, ok := ThemeByName(""); ok {
		t.Errorf("ThemeByName(\"\") = %+v, want not found", notFound)
	}
}

func TestThemeNamesFollowTheCatalogOrder(t *testing.T) {
	if got := ThemeNames(); !slices.Equal(got, wantNames) {
		t.Errorf("ThemeNames() = %v, want %v", got, wantNames)
	}
}

func TestApplyRebuildsTheTokens(t *testing.T) {
	Apply(DefaultTheme()) // a known starting point, whatever ran before

	beforeBorder := Border.Render("x")
	beforeTitle := Title.Render("x")
	beforeAdded := Added.Render("x")

	theme, ok := ThemeByName("dracula")
	if !ok {
		t.Fatal("ThemeByName(dracula) not found, want it in the catalog")
	}
	Apply(theme)

	if got := Border.Render("x"); got == beforeBorder {
		t.Errorf("Border.Render() = %q, want it changed by Apply", got)
	}
	if got := Title.Render("x"); got == beforeTitle {
		t.Errorf("Title.Render() = %q, want it changed by Apply", got)
	}
	if got := Added.Render("x"); got == beforeAdded {
		t.Errorf("Added.Render() = %q, want it changed by Apply", got)
	}
	if got := CurrentTheme(); got.Name != theme.Name {
		t.Errorf("CurrentTheme().Name = %q, want %q after Apply", got.Name, theme.Name)
	}

	Apply(DefaultTheme())

	if got := Border.Render("x"); got != beforeBorder {
		t.Errorf("Border.Render() = %q, want the default %q restored", got, beforeBorder)
	}
	if got := Title.Render("x"); got != beforeTitle {
		t.Errorf("Title.Render() = %q, want the default %q restored", got, beforeTitle)
	}
	if got := Added.Render("x"); got != beforeAdded {
		t.Errorf("Added.Render() = %q, want the default %q restored", got, beforeAdded)
	}
}

func TestCurrentThemeFollowsApply(t *testing.T) {
	t.Cleanup(func() { Apply(DefaultTheme()) })

	theme, ok := ThemeByName("gruvbox-dark")
	if !ok {
		t.Fatal("ThemeByName(gruvbox-dark) not found, want it in the catalog")
	}
	Apply(theme)

	if got := CurrentTheme(); got != theme {
		t.Errorf("CurrentTheme() = %+v, want the applied %+v", got, theme)
	}
}
