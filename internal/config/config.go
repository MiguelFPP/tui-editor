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
// come from the optional JSON file; Theme may stay empty for main to apply its
// default. ListThemes and ListKeys ask main to print a catalog and exit, so no
// other validation runs for them.
type Config struct {
	ShowVersion bool
	ListThemes  bool
	ListKeys    bool
	Repo        string
	Editor      string
	DockerHost  string
	DebugLog    string
	ConfigPath  string
	Theme       string
	Keys        map[string][]string
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
  --theme NAME       tema de color, default: tokyonight-night`

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

	path, explicit := resolveConfigPath(*configPath)
	fileCfg, loaded, err := loadFile(path, explicit)
	if err != nil {
		return Config{}, err
	}

	// Theme precedence: flag, then env (already folded into the flag default),
	// then the file.
	effectiveTheme := strings.TrimSpace(*theme)
	if effectiveTheme == "" {
		effectiveTheme = strings.TrimSpace(fileCfg.Theme)
	}

	cfg := Config{
		Repo:       repoPath,
		Editor:     editorCmd,
		DockerHost: host,
		DebugLog:   strings.TrimSpace(*debugLog),
		Theme:      effectiveTheme,
		Keys:       fileCfg.keyOverrides(),
	}
	if loaded {
		cfg.ConfigPath = path
	}

	return cfg, nil
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
