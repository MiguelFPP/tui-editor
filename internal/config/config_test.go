package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func noEnv(string) string {
	return ""
}

func TestParseDefaults(t *testing.T) {
	workDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}

	cfg, err := Parse(nil, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if cfg.Repo != workDir {
		t.Errorf("Repo = %q, want %q", cfg.Repo, workDir)
	}
	if cfg.Editor != "vim" {
		t.Errorf("Editor = %q, want %q", cfg.Editor, "vim")
	}
	if cfg.DockerHost != "" {
		t.Errorf("DockerHost = %q, want empty", cfg.DockerHost)
	}
	if cfg.DebugLog != "" {
		t.Errorf("DebugLog = %q, want empty", cfg.DebugLog)
	}
}

func TestParseEnvironmentDefaults(t *testing.T) {
	getenv := func(key string) string {
		switch key {
		case "EDITOR":
			return "nano"
		case "DOCKER_HOST":
			return "unix:///var/run/docker.sock"
		default:
			return ""
		}
	}

	cfg, err := Parse(nil, getenv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if cfg.Editor != "nano" {
		t.Errorf("Editor = %q, want %q", cfg.Editor, "nano")
	}
	if cfg.DockerHost != "unix:///var/run/docker.sock" {
		t.Errorf("DockerHost = %q, want %q", cfg.DockerHost, "unix:///var/run/docker.sock")
	}
}

func TestParseNilGetenv(t *testing.T) {
	cfg, err := Parse(nil, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if cfg.Editor != "vim" {
		t.Errorf("Editor = %q, want %q", cfg.Editor, "vim")
	}
}

func TestParseFlagsOverrideEnvironment(t *testing.T) {
	getenv := func(key string) string {
		switch key {
		case "EDITOR":
			return "nano"
		case "DOCKER_HOST":
			return "unix:///var/run/docker.sock"
		default:
			return ""
		}
	}

	args := []string{
		"--repo", t.TempDir(),
		"--editor", "vim",
		"--docker-host", "tcp://127.0.0.1:2375",
		"--debug-log", "/var/log/tui-editor.log",
	}

	cfg, err := Parse(args, getenv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if cfg.Editor != "vim" {
		t.Errorf("Editor = %q, want %q", cfg.Editor, "vim")
	}
	if cfg.DockerHost != "tcp://127.0.0.1:2375" {
		t.Errorf("DockerHost = %q, want %q", cfg.DockerHost, "tcp://127.0.0.1:2375")
	}
	if cfg.DebugLog != "/var/log/tui-editor.log" {
		t.Errorf("DebugLog = %q, want %q", cfg.DebugLog, "/var/log/tui-editor.log")
	}
}

func TestParseRepoRelativePath(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}

	t.Chdir(root)

	cfg, err := Parse([]string{"--repo", "repo"}, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.Repo != repo {
		t.Errorf("Repo = %q, want %q", cfg.Repo, repo)
	}
}

func TestParseRepoErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cases := []struct {
		name string
		repo string
	}{
		{name: "missing", repo: filepath.Join(t.TempDir(), "nope")},
		{name: "file", repo: file},
		{name: "empty", repo: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]string{"--repo", tc.repo}, noEnv)
			if !errors.Is(err, ErrRepoNotDirectory) {
				t.Fatalf("Parse() error = %v, want %v", err, ErrRepoNotDirectory)
			}
		})
	}
}

func TestParseDockerHostAccepted(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "unix", value: "unix:///var/run/docker.sock"},
		{name: "tcp", value: "tcp://127.0.0.1:2375"},
		{name: "npipe", value: "npipe:////./pipe/docker_engine"},
		{name: "empty", value: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse([]string{"--docker-host", tc.value}, noEnv)
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if cfg.DockerHost != tc.value {
				t.Errorf("DockerHost = %q, want %q", cfg.DockerHost, tc.value)
			}
		})
	}
}

func TestParseDockerHostRejected(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "unsupported scheme", value: "http://127.0.0.1:2375"},
		{name: "missing scheme", value: "127.0.0.1:2375"},
		{name: "tcp without host", value: "tcp://"},
		{name: "unix without path", value: "unix://"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]string{"--docker-host", tc.value}, noEnv)
			if !errors.Is(err, ErrInvalidDockerHost) {
				t.Fatalf("Parse() error = %v, want %v", err, ErrInvalidDockerHost)
			}
		})
	}
}

func TestParseEditorRequired(t *testing.T) {
	_, err := Parse([]string{"--editor", "   "}, noEnv)
	if !errors.Is(err, ErrEditorRequired) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrEditorRequired)
	}
}

func TestParseUnknownFlag(t *testing.T) {
	if _, err := Parse([]string{"--nope"}, noEnv); err == nil {
		t.Fatal("Parse() error = nil, want an error")
	}
}

func TestParseUnexpectedArgument(t *testing.T) {
	if _, err := Parse([]string{"extra"}, noEnv); err == nil {
		t.Fatal("Parse() error = nil, want an error")
	}
}

func TestParseHelp(t *testing.T) {
	if _, err := Parse([]string{"--help"}, noEnv); !errors.Is(err, ErrHelp) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrHelp)
	}
}
