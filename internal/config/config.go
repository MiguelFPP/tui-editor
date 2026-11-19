// Package config parses flags, environment defaults and the optional JSON file.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Config is the validated runtime configuration of the TUI. ConfigPath and Keys
// come from the optional JSON file and Theme may stay empty for main to apply
// its default. TreeWidth and TreeCollapsed hold the persisted tree layout, while
// SavePath carries the resolved path where it can be written even before the
// file exists. ShowVersion, ListThemes and ListKeys ask main to print a catalog
// and exit; InitConfig writes a template and PrintConfig dumps the resolved
// values, so no repo, editor or Docker validation runs for them.
type Config struct {
	ShowVersion   bool
	ListThemes    bool
	ListKeys      bool
	InitConfig    bool
	Force         bool
	PrintConfig   bool
	Repo          string
	Editor        string
	DockerHost    string
	DebugLog      string
	ConfigPath    string
	Theme         string
	Keys          map[string][]string
	TreeWidth     int
	TreeCollapsed bool
	GitLayout     string
	GitListWidth  int
	SavePath      string
}

// Usage is the help text shown when -h or --help is requested.
const Usage = `usage: tui-editor [flags]

flags:
  --version          print the version and exit
  --list-themes      lista los temas disponibles y sale
  --list-keys        lista las acciones y sus teclas por defecto y sale
  --repo DIR         repository root, default: current directory
  --editor CMD       editor command, default: $EDITOR or vim
  --docker-host URL  Docker daemon endpoint, default: $DOCKER_HOST
  --debug-log FILE   debug log file, default: disabled
  --config FILE      archivo de configuración JSON, default: $XDG_CONFIG_HOME/tui-editor/config.json
  --theme NAME       tema de color, default: tokyonight-night
  --init-config      escribe un archivo de configuración base y sale
  --force            sobrescribe el archivo existente con --init-config
  --print-config     imprime la configuración resuelta y sale`

// ErrHelp reports that -h or --help was requested.
var ErrHelp = flag.ErrHelp

// Errors returned by Parse.
var (
	ErrRepoNotDirectory  = errors.New("--repo debe ser un directorio existente")
	ErrEditorRequired    = errors.New("--editor no puede estar vacío")
	ErrInvalidDockerHost = errors.New("--docker-host no es válido")
	ErrConfigFile        = errors.New("no se pudo leer el archivo de configuración")
	ErrConfigFormat      = errors.New("el archivo de configuración no es válido")
)

const (
	defaultEditor = "vim"

	schemeUnix  = "unix"
	schemeTCP   = "tcp"
	schemeNpipe = "npipe"
)

// Parse validates args, the argument list without the program name, together
// with the environment read through getenv. It returns the resulting
// configuration and never writes to stdout or stderr.
func Parse(args []string, getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	workDir, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("resolver el directorio de trabajo: %w", err)
	}

	flags := flag.NewFlagSet("tui-editor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}

	showVersion := flags.Bool("version", false, "muestra la versión y sale")
	listThemes := flags.Bool("list-themes", false, "lista los temas disponibles y sale")
	listKeys := flags.Bool("list-keys", false, "lista las acciones y sus teclas y sale")
	initConfig := flags.Bool("init-config", false, "escribe un archivo de configuración base y sale")
	force := flags.Bool("force", false, "sobrescribe el archivo existente con --init-config")
	printConfig := flags.Bool("print-config", false, "imprime la configuración resuelta y sale")
	repo := flags.String("repo", workDir, "directorio raíz del repositorio")
	editor := flags.String("editor", editorDefault(getenv), "comando del editor externo")
	dockerHost := flags.String("docker-host", strings.TrimSpace(getenv("DOCKER_HOST")), "endpoint del daemon Docker")
	debugLog := flags.String("debug-log", "", "archivo de log de depuración")
	configPath := flags.String("config", strings.TrimSpace(getenv("TUI_EDITOR_CONFIG")), "archivo de configuración JSON")
	theme := flags.String("theme", strings.TrimSpace(getenv("TUI_EDITOR_THEME")), "tema de color")

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if *showVersion {
		return Config{ShowVersion: true}, nil
	}
	// The catalogs are static, so neither the file nor the other flags matter.
	if *listThemes || *listKeys {
		return Config{ListThemes: *listThemes, ListKeys: *listKeys}, nil
	}
	if flags.NArg() > 0 {
		return Config{}, fmt.Errorf("argumento no esperado %q", flags.Arg(0))
	}

	// --init-config needs no existing file and wins over --print-config.
	if *initConfig {
		path, err := resolveInitPath(*configPath)
		if err != nil {
			return Config{}, err
		}

		return Config{InitConfig: true, Force: *force, ConfigPath: path}, nil
	}
	if *printConfig {
		cfg, err := loadConfig(*configPath, *theme)
		if err != nil {
			return Config{}, err
		}
		cfg.PrintConfig = true

		return cfg, nil
	}

	// The file is resolved before the repo, editor and Docker checks so the
	// dump of --print-config and the TUI share the very same loading rules.
	cfg, err := loadConfig(*configPath, *theme)
	if err != nil {
		return Config{}, err
	}

	editorCmd := strings.TrimSpace(*editor)
	if editorCmd == "" {
		return Config{}, ErrEditorRequired
	}

	repoPath, err := resolveRepoDir(*repo)
	if err != nil {
		return Config{}, err
	}

	host := strings.TrimSpace(*dockerHost)
	if err := validateDockerHost(host); err != nil {
		return Config{}, err
	}

	cfg.Repo = repoPath
	cfg.Editor = editorCmd
	cfg.DockerHost = host
	cfg.DebugLog = strings.TrimSpace(*debugLog)

	return cfg, nil
}

// loadConfig resolves the path, reads the JSON file and applies the theme
// precedence: flag, then env (already folded into the flag default), then the
// file. ConfigPath is only set when a file was actually loaded, while SavePath
// always carries the resolved path so the UI can persist the layout.
func loadConfig(configPath, theme string) (Config, error) {
	path, explicit := resolveConfigPath(configPath)
	fileCfg, loaded, err := loadFile(path, explicit)
	if err != nil {
		return Config{}, err
	}
	if fileCfg.TreeWidth < 0 {
		return Config{}, fmt.Errorf("%w: tree_width no puede ser negativo", ErrConfigFormat)
	}
	if err := validateGitLayout(fileCfg.GitLayout); err != nil {
		return Config{}, err
	}
	if fileCfg.GitListWidth < 0 {
		return Config{}, fmt.Errorf("%w: git_list_width no puede ser negativo", ErrConfigFormat)
	}

	effectiveTheme := strings.TrimSpace(theme)
	if effectiveTheme == "" {
		effectiveTheme = strings.TrimSpace(fileCfg.Theme)
	}

	cfg := Config{
		Theme:         effectiveTheme,
		Keys:          fileCfg.keyOverrides(),
		TreeWidth:     fileCfg.TreeWidth,
		TreeCollapsed: fileCfg.TreeCollapsed,
		GitLayout:     fileCfg.GitLayout,
		GitListWidth:  fileCfg.GitListWidth,
	}
	if loaded {
		cfg.ConfigPath = path
	}
	if path != "" {
		cfg.SavePath = path
	}

	return cfg, nil
}

// validateGitLayout accepts the empty default plus the two persisted names.
func validateGitLayout(layout string) error {
	switch strings.TrimSpace(layout) {
	case "", "split", "single":
		return nil
	default:
		return fmt.Errorf("%w: git_layout %q no es válido; usa \"split\" o \"single\"", ErrConfigFormat, layout)
	}
}

// resolveInitPath returns the destination of --init-config. Unlike loadConfig it
// never touches the filesystem, because the file is expected to be missing.
func resolveInitPath(fromFlag string) (string, error) {
	path, _ := resolveConfigPath(fromFlag)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%w: no se pudo resolver la ruta de destino; usa --config", ErrConfigFile)
	}

	return path, nil
}

func editorDefault(getenv func(string) string) string {
	if fromEnv := strings.TrimSpace(getenv("EDITOR")); fromEnv != "" {
		return fromEnv
	}

	return defaultEditor
}

func resolveRepoDir(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", ErrRepoNotDirectory
	}

	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolver la ruta del repo: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrRepoNotDirectory, abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrRepoNotDirectory, abs)
	}

	return abs, nil
}

// validateDockerHost only checks the shape of the endpoint: the socket itself
// is not probed here.
func validateDockerHost(raw string) error {
	if raw == "" {
		return nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidDockerHost, raw)
	}

	switch strings.ToLower(u.Scheme) {
	case schemeUnix, schemeNpipe:
		if u.Host == "" && u.Path == "" {
			return fmt.Errorf("%w: %s requiere una ruta de socket", ErrInvalidDockerHost, raw)
		}
	case schemeTCP:
		if u.Host == "" {
			return fmt.Errorf("%w: %s requiere un host", ErrInvalidDockerHost, raw)
		}
	default:
		return fmt.Errorf("%w: %s usa un esquema no soportado %q", ErrInvalidDockerHost, raw, u.Scheme)
	}

	return nil
}
