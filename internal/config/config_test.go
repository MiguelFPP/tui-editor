package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestMain isolates the XDG default path so a real host config file can never
// leak into the tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tui-editor-config-test")
	if err == nil {
		_ = os.Setenv("XDG_CONFIG_HOME", dir)
	}

	code := m.Run()

	if err == nil {
		_ = os.RemoveAll(dir)
	}

	os.Exit(code)
}

func noEnv(string) string {
	return ""
}

// withDefaultConfigDir points XDG_CONFIG_HOME at an empty temp dir and returns
// it, skipping when os.UserConfigDir ignores the variable.
func withDefaultConfigDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	got, err := os.UserConfigDir()
	if err != nil || got != dir {
		t.Skipf("os.UserConfigDir() no respeta XDG_CONFIG_HOME: got %q, err %v", got, err)
	}

	return dir
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

func TestParseVersionSkipsValidation(t *testing.T) {
	args := []string{
		"--version",
		"--repo", filepath.Join(t.TempDir(), "nope"),
		"--editor", "   ",
		"--docker-host", "http://127.0.0.1:2375",
	}

	cfg, err := Parse(args, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if !cfg.ShowVersion {
		t.Error("ShowVersion = false, want true")
	}
	if cfg.Repo != "" || cfg.Editor != "" || cfg.DockerHost != "" {
		t.Errorf("Parse() = %+v, want an empty config besides ShowVersion", cfg)
	}
}

func TestParseVersionDefaultsOff(t *testing.T) {
	cfg, err := Parse(nil, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.ShowVersion {
		t.Error("ShowVersion = true, want false")
	}
}

func TestParseConfigDefaultMissing(t *testing.T) {
	withDefaultConfigDir(t)

	cfg, err := Parse(nil, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.ConfigPath != "" {
		t.Errorf("ConfigPath = %q, want empty", cfg.ConfigPath)
	}
	if cfg.Theme != "" {
		t.Errorf("Theme = %q, want empty", cfg.Theme)
	}
	if cfg.Keys != nil {
		t.Errorf("Keys = %v, want nil", cfg.Keys)
	}
}

func TestParseConfigDefaultLoaded(t *testing.T) {
	dir := withDefaultConfigDir(t)
	path := filepath.Join(dir, "tui-editor", "config.json")
	writeConfigFile(t, path, `{"theme":"xdg-theme","keys":{"quit":"q"}}`)

	cfg, err := Parse(nil, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.ConfigPath != path {
		t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, path)
	}
	if cfg.Theme != "xdg-theme" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "xdg-theme")
	}
	if got := cfg.Keys["quit"]; !slices.Equal(got, []string{"q"}) {
		t.Errorf("Keys[quit] = %v, want [q]", got)
	}
}

func TestParseConfigExplicitMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")

	t.Run("flag", func(t *testing.T) {
		_, err := Parse([]string{"--config", missing}, noEnv)
		if !errors.Is(err, ErrConfigFile) {
			t.Fatalf("Parse() error = %v, want %v", err, ErrConfigFile)
		}
	})

	t.Run("env", func(t *testing.T) {
		getenv := func(key string) string {
			if key == "TUI_EDITOR_CONFIG" {
				return missing
			}

			return ""
		}

		_, err := Parse(nil, getenv)
		if !errors.Is(err, ErrConfigFile) {
			t.Fatalf("Parse() error = %v, want %v", err, ErrConfigFile)
		}
	})
}

func TestParseConfigPathPrecedence(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env.json")
	flagPath := filepath.Join(dir, "flag.json")
	writeConfigFile(t, envPath, `{"theme":"env-theme"}`)
	writeConfigFile(t, flagPath, `{"theme":"flag-theme"}`)

	getenv := func(key string) string {
		if key == "TUI_EDITOR_CONFIG" {
			return envPath
		}

		return ""
	}

	t.Run("env over default", func(t *testing.T) {
		cfg, err := Parse(nil, getenv)
		if err != nil {
			t.Fatalf("Parse() error = %v, want nil", err)
		}
		if cfg.ConfigPath != envPath {
			t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, envPath)
		}
		if cfg.Theme != "env-theme" {
			t.Errorf("Theme = %q, want %q", cfg.Theme, "env-theme")
		}
	})

	t.Run("flag over env", func(t *testing.T) {
		cfg, err := Parse([]string{"--config", flagPath}, getenv)
		if err != nil {
			t.Fatalf("Parse() error = %v, want nil", err)
		}
		if cfg.ConfigPath != flagPath {
			t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, flagPath)
		}
		if cfg.Theme != "flag-theme" {
			t.Errorf("Theme = %q, want %q", cfg.Theme, "flag-theme")
		}
	})
}

func TestParseConfigThemePrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"theme":"from-file"}`)

	getenv := func(key string) string {
		if key == "TUI_EDITOR_THEME" {
			return "from-env"
		}

		return ""
	}

	t.Run("file", func(t *testing.T) {
		cfg, err := Parse([]string{"--config", path}, noEnv)
		if err != nil {
			t.Fatalf("Parse() error = %v, want nil", err)
		}
		if cfg.Theme != "from-file" {
			t.Errorf("Theme = %q, want %q", cfg.Theme, "from-file")
		}
	})

	t.Run("env over file", func(t *testing.T) {
		cfg, err := Parse([]string{"--config", path}, getenv)
		if err != nil {
			t.Fatalf("Parse() error = %v, want nil", err)
		}
		if cfg.Theme != "from-env" {
			t.Errorf("Theme = %q, want %q", cfg.Theme, "from-env")
		}
	})

	t.Run("flag over env", func(t *testing.T) {
		cfg, err := Parse([]string{"--config", path, "--theme", "from-flag"}, getenv)
		if err != nil {
			t.Fatalf("Parse() error = %v, want nil", err)
		}
		if cfg.Theme != "from-flag" {
			t.Errorf("Theme = %q, want %q", cfg.Theme, "from-flag")
		}
	})
}

func TestParseConfigFileErrors(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name string
		body string
		want error
	}{
		{name: "invalid json", body: `{"theme":`, want: ErrConfigFormat},
		{name: "unknown field", body: `{"nope":true}`, want: ErrConfigFormat},
		{name: "extra data", body: `{"theme":"a"}{"theme":"b"}`, want: ErrConfigFormat},
		{name: "not an object", body: `[]`, want: ErrConfigFormat},
		{name: "null key value", body: `{"keys":{"quit":null}}`, want: ErrConfigFormat},
		{name: "oversized", body: strings.Repeat(" ", maxConfigBytes+1), want: ErrConfigFile},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".json")
			writeConfigFile(t, path, tc.body)

			_, err := Parse([]string{"--config", path}, noEnv)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Parse() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestParseConfigKeysPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"keys":{"quit":"q","copy":["ctrl+c","y"],"empty":[]}}`)

	cfg, err := Parse([]string{"--config", path}, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if got := cfg.Keys["quit"]; !slices.Equal(got, []string{"q"}) {
		t.Errorf("Keys[quit] = %v, want [q]", got)
	}
	if got := cfg.Keys["copy"]; !slices.Equal(got, []string{"ctrl+c", "y"}) {
		t.Errorf("Keys[copy] = %v, want [ctrl+c y]", got)
	}

	empty, ok := cfg.Keys["empty"]
	if !ok {
		t.Fatal("Keys[empty] is missing, want an empty list")
	}
	if empty == nil {
		t.Error("Keys[empty] = nil, want an empty non-nil list")
	}
	if len(empty) != 0 {
		t.Errorf("Keys[empty] = %v, want empty", empty)
	}
}

func TestParseVersionSkipsConfigLoad(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")

	cfg, err := Parse([]string{"--version", "--config", missing, "--theme", "ignored"}, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if !cfg.ShowVersion {
		t.Error("ShowVersion = false, want true")
	}
	if cfg.ConfigPath != "" || cfg.Theme != "" || cfg.Keys != nil {
		t.Errorf("Parse() = %+v, want no config file data", cfg)
	}
}
