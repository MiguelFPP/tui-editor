// Package config parses the command-line flags and environment defaults used
// by the composition root.
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

// Config is the validated runtime configuration of the TUI.
type Config struct {
	Repo       string
	Editor     string
	DockerHost string
	DebugLog   string
}

// Usage is the help text shown when -h or --help is requested.
const Usage = `usage: tui-editor [flags]

flags:
  --repo DIR         repository root, default: current directory
  --editor CMD       editor command, default: $EDITOR or vim
  --docker-host URL  Docker daemon endpoint, default: $DOCKER_HOST
  --debug-log FILE   debug log file, default: disabled`

// ErrHelp reports that -h or --help was requested.
var ErrHelp = flag.ErrHelp

// Errors returned by Parse.
var (
	ErrRepoNotDirectory  = errors.New("--repo debe ser un directorio existente")
	ErrEditorRequired    = errors.New("--editor no puede estar vacío")
	ErrInvalidDockerHost = errors.New("--docker-host no es válido")
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

	repo := flags.String("repo", workDir, "directorio raíz del repositorio")
	editor := flags.String("editor", editorDefault(getenv), "comando del editor externo")
	dockerHost := flags.String("docker-host", strings.TrimSpace(getenv("DOCKER_HOST")), "endpoint del daemon Docker")
	debugLog := flags.String("debug-log", "", "archivo de log de depuración")

	if err := flags.Parse(args); err != nil {
		return Config{}, err
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

	return Config{
		Repo:       repoPath,
		Editor:     editorCmd,
		DockerHost: host,
		DebugLog:   strings.TrimSpace(*debugLog),
	}, nil
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
