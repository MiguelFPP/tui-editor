package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestParseListFlagsSkipValidation(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")

	cases := []struct {
		name  string
		flag  string
		check func(Config) bool
	}{
		{name: "list-themes", flag: "--list-themes", check: func(cfg Config) bool { return cfg.ListThemes }},
		{name: "list-keys", flag: "--list-keys", check: func(cfg Config) bool { return cfg.ListKeys }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				tc.flag,
				"--repo", filepath.Join(t.TempDir(), "nope"),
				"--editor", "   ",
				"--docker-host", "http://127.0.0.1:2375",
				"--config", missing,
				"extra",
			}

			cfg, err := Parse(args, noEnv)
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if !tc.check(cfg) {
				t.Errorf("Parse() = %+v, want %s activo", cfg, tc.flag)
			}
			if cfg.ListThemes == cfg.ListKeys {
				t.Errorf("Parse() = %+v, want exactly one list flag", cfg)
			}
			if cfg.ShowVersion || cfg.Repo != "" || cfg.Editor != "" || cfg.DockerHost != "" ||
				cfg.DebugLog != "" || cfg.ConfigPath != "" || cfg.Theme != "" || cfg.Keys != nil {
				t.Errorf("Parse() = %+v, want only the list flag", cfg)
			}
		})
	}
}

func TestParseListFlagsDefaultOff(t *testing.T) {
	cfg, err := Parse(nil, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.ListThemes || cfg.ListKeys {
		t.Errorf("Parse() = %+v, want both list flags off", cfg)
	}
}

func TestUsageListsCatalogs(t *testing.T) {
	for _, want := range []string{"--list-themes", "--list-keys"} {
		if !strings.Contains(Usage, want) {
			t.Errorf("Usage no menciona %q", want)
		}
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

func TestParseConfigTreeLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"tree_width":42,"tree_collapsed":true}`)

	cfg, err := Parse([]string{"--config", path}, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.TreeWidth != 42 {
		t.Errorf("TreeWidth = %d, want 42", cfg.TreeWidth)
	}
	if !cfg.TreeCollapsed {
		t.Error("TreeCollapsed = false, want true")
	}
	// Both paths are set for a loaded file: ConfigPath marks the load and
	// SavePath is where a layout change lands.
	if cfg.ConfigPath != path {
		t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, path)
	}
	if cfg.SavePath != path {
		t.Errorf("SavePath = %q, want %q", cfg.SavePath, path)
	}
}

func TestParseConfigGitLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"git_layout":"single"}`)

	cfg, err := Parse([]string{"--config", path}, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.GitLayout != "single" {
		t.Errorf("GitLayout = %q, want %q", cfg.GitLayout, "single")
	}
}

func TestParseConfigGitLayoutRejectsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"git_layout":"triple"}`)

	_, err := Parse([]string{"--config", path}, noEnv)
	if !errors.Is(err, ErrConfigFormat) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrConfigFormat)
	}
}

func TestParseSavePathWithoutFile(t *testing.T) {
	dir := withDefaultConfigDir(t)
	want := filepath.Join(dir, "tui-editor", "config.json")

	cfg, err := Parse(nil, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.SavePath != want {
		t.Errorf("SavePath = %q, want %q", cfg.SavePath, want)
	}
	if cfg.ConfigPath != "" {
		t.Errorf("ConfigPath = %q, want empty for an unloaded file", cfg.ConfigPath)
	}
	if cfg.TreeWidth != 0 || cfg.TreeCollapsed {
		t.Errorf("Parse() = %+v, want zero tree layout", cfg)
	}
}

func TestParseConfigTreeWidthNegative(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"tree_width":-1}`)

	_, err := Parse([]string{"--config", path}, noEnv)
	if !errors.Is(err, ErrConfigFormat) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrConfigFormat)
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

func TestParseInitConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")

	args := []string{
		"--init-config",
		"--force",
		"--config", path,
		"--repo", filepath.Join(t.TempDir(), "nope"),
		"--editor", "   ",
		"--docker-host", "http://127.0.0.1:2375",
	}

	cfg, err := Parse(args, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if !cfg.InitConfig {
		t.Error("InitConfig = false, want true")
	}
	if !cfg.Force {
		t.Error("Force = false, want true")
	}
	if cfg.ConfigPath != path {
		t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, path)
	}
	if cfg.ShowVersion || cfg.ListThemes || cfg.ListKeys || cfg.PrintConfig {
		t.Errorf("Parse() = %+v, want only the init flags", cfg)
	}
	if cfg.Repo != "" || cfg.Editor != "" || cfg.DockerHost != "" || cfg.DebugLog != "" ||
		cfg.Theme != "" || cfg.Keys != nil {
		t.Errorf("Parse() = %+v, want no other field set", cfg)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("Parse() touched %s (err = %v), want no I/O", path, statErr)
	}
}

func TestParseInitConfigDefaultPath(t *testing.T) {
	dir := withDefaultConfigDir(t)

	cfg, err := Parse([]string{"--init-config"}, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	want := filepath.Join(dir, "tui-editor", "config.json")
	if cfg.ConfigPath != want {
		t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, want)
	}
	if !cfg.InitConfig {
		t.Error("InitConfig = false, want true")
	}
	if cfg.Force {
		t.Error("Force = true, want false")
	}
}

func TestParseInitConfigUnresolvablePath(t *testing.T) {
	const relative = "relative-xdg-dir"

	t.Setenv("XDG_CONFIG_HOME", relative)

	// Hosts where the variable is ignored never reach the resolution error, so
	// the case is skipped instead of failing.
	if dir, err := os.UserConfigDir(); err == nil {
		t.Skipf("os.UserConfigDir() = %q, want an error for %q", dir, relative)
	}

	if _, err := Parse([]string{"--init-config"}, noEnv); !errors.Is(err, ErrConfigFile) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrConfigFile)
	}
}

func TestParsePrintConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"theme":"dracula","keys":{"quit":["q","c"],"docker_logs":"o"}}`)

	args := []string{
		"--print-config",
		"--config", path,
		"--repo", filepath.Join(t.TempDir(), "nope"),
		"--editor", "   ",
		"--docker-host", "http://127.0.0.1:2375",
	}

	cfg, err := Parse(args, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if !cfg.PrintConfig {
		t.Error("PrintConfig = false, want true")
	}
	if cfg.ConfigPath != path {
		t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, path)
	}
	if cfg.Theme != "dracula" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "dracula")
	}
	if got := cfg.Keys["quit"]; !slices.Equal(got, []string{"q", "c"}) {
		t.Errorf("Keys[quit] = %v, want [q c]", got)
	}
	if got := cfg.Keys["docker_logs"]; !slices.Equal(got, []string{"o"}) {
		t.Errorf("Keys[docker_logs] = %v, want [o]", got)
	}
	if cfg.ShowVersion || cfg.ListThemes || cfg.ListKeys || cfg.InitConfig || cfg.Force {
		t.Errorf("Parse() = %+v, want only the print flag", cfg)
	}
	if cfg.Repo != "" || cfg.Editor != "" || cfg.DockerHost != "" || cfg.DebugLog != "" {
		t.Errorf("Parse() = %+v, want no repo, editor or Docker data", cfg)
	}
}

func TestParsePrintConfigThemeFlagWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"theme":"dracula"}`)

	cfg, err := Parse([]string{"--print-config", "--config", path, "--theme", "gruvbox-dark"}, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.Theme != "gruvbox-dark" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "gruvbox-dark")
	}
}

func TestParsePrintConfigMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")

	_, err := Parse([]string{"--print-config", "--config", missing}, noEnv)
	if !errors.Is(err, ErrConfigFile) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrConfigFile)
	}
}

func TestParsePrintConfigInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"theme":`)

	_, err := Parse([]string{"--print-config", "--config", path}, noEnv)
	if !errors.Is(err, ErrConfigFormat) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrConfigFormat)
	}
}

func TestParseEarlyOutputPrecedence(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"theme":"dracula"}`)

	cases := []struct {
		name string
		args []string
		want func(Config) bool
		off  func(Config) bool
	}{
		{
			name: "init wins over print",
			args: []string{"--init-config", "--print-config", "--config", path},
			want: func(cfg Config) bool { return cfg.InitConfig },
			off:  func(cfg Config) bool { return cfg.PrintConfig },
		},
		{
			name: "version wins over init",
			args: []string{"--version", "--init-config", "--config", path},
			want: func(cfg Config) bool { return cfg.ShowVersion },
			off:  func(cfg Config) bool { return cfg.InitConfig || cfg.PrintConfig },
		},
		{
			name: "list wins over print",
			args: []string{"--list-themes", "--print-config", "--config", missing},
			want: func(cfg Config) bool { return cfg.ListThemes },
			off:  func(cfg Config) bool { return cfg.PrintConfig || cfg.InitConfig },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse(tc.args, noEnv)
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if !tc.want(cfg) {
				t.Errorf("Parse() = %+v, want the winning early output", cfg)
			}
			if tc.off(cfg) {
				t.Errorf("Parse() = %+v, want the losing flags off", cfg)
			}
		})
	}
}

func TestUsageIncludesInitAndPrint(t *testing.T) {
	for _, want := range []string{"--init-config", "--force", "--print-config"} {
		if !strings.Contains(Usage, want) {
			t.Errorf("Usage no menciona %q", want)
		}
	}
}

func TestWriteDefault(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deeper")
	path := filepath.Join(dir, "config.json")

	if err := WriteDefault(path, false); err != nil {
		t.Fatalf("WriteDefault() error = %v, want nil", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("os.Stat(%s) error = %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("os.Stat(%s).IsDir() = false, want true", dir)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(body) != BaseFile {
		t.Errorf("contenido = %q, want %q", body, BaseFile)
	}
	if !bytes.HasSuffix(body, []byte("\n")) {
		t.Errorf("contenido = %q, want a trailing newline", body)
	}
	if _, err := decodeFile(body, path); err != nil {
		t.Errorf("decodeFile() error = %v, want nil", err)
	}

	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	// Windows only tracks the read-only attribute, so the mode is not asserted.
	if perm := info.Mode().Perm(); perm != 0o600 && runtime.GOOS != "windows" {
		t.Errorf("permisos = %v, want 0600", perm)
	}

	// A second run without --force leaves the existing file untouched.
	if err := WriteDefault(path, false); !errors.Is(err, ErrConfigExists) {
		t.Fatalf("WriteDefault() error = %v, want %v", err, ErrConfigExists)
	}
	if got, err := os.ReadFile(path); err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	} else if string(got) != BaseFile {
		t.Errorf("contenido tras el segundo intento = %q, want %q", got, BaseFile)
	}

	if err := os.WriteFile(path, []byte(`{"theme":"dracula"}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := WriteDefault(path, true); err != nil {
		t.Fatalf("WriteDefault(force) error = %v, want nil", err)
	}
	if got, err := os.ReadFile(path); err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	} else if string(got) != BaseFile {
		t.Errorf("contenido tras --force = %q, want %q", got, BaseFile)
	}

	// The atomic write must not leave temporary files behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}

		t.Errorf("directorio = %v, want solo config.json", names)
	}
}

func TestWriteDefaultEmptyPath(t *testing.T) {
	if err := WriteDefault("   ", false); !errors.Is(err, ErrConfigFile) {
		t.Fatalf("WriteDefault() error = %v, want %v", err, ErrConfigFile)
	}
}

func TestWriteDefaultDirectoryTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config.json")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "child"), []byte("x"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// A non-empty directory can neither be replaced nor removed, so the forced
	// write fails and must leave the target and no temporary file behind.
	err := WriteDefault(target, true)
	if !errors.Is(err, ErrConfigFile) {
		t.Fatalf("WriteDefault() error = %v, want %v", err, ErrConfigFile)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("os.Stat(%s) error = %v", target, err)
	}
	if !info.IsDir() {
		t.Errorf("os.Stat(%s).IsDir() = false, want true", target)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("os.ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Errorf("directorio = %v, want solo config.json", entries)
	}
}

func TestSaveLayoutCreatesFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deeper")
	path := filepath.Join(dir, "config.json")

	if err := SaveLayout(path, 42, true); err != nil {
		t.Fatalf("SaveLayout() error = %v, want nil", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("os.Stat(%s) error = %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("os.Stat(%s).IsDir() = false, want true", dir)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if !bytes.HasSuffix(body, []byte("\n")) {
		t.Errorf("contenido = %q, want a trailing newline", body)
	}

	// A brand new file only carries the layout: theme and keys stay omitted.
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := fields["theme"]; ok {
		t.Errorf("contenido = %q, want no theme key", body)
	}
	if _, ok := fields["keys"]; ok {
		t.Errorf("contenido = %q, want no keys key", body)
	}
	if got := fields["tree_width"]; got != float64(42) {
		t.Errorf("tree_width = %v, want 42", got)
	}
	if got := fields["tree_collapsed"]; got != true {
		t.Errorf("tree_collapsed = %v, want true", got)
	}

	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	// Windows only tracks the read-only attribute, so the mode is not asserted.
	if perm := info.Mode().Perm(); perm != 0o600 && runtime.GOOS != "windows" {
		t.Errorf("permisos = %v, want 0600", perm)
	}

	cfg, err := Parse([]string{"--config", path}, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.TreeWidth != 42 || !cfg.TreeCollapsed {
		t.Errorf("Parse() = %+v, want tree_width 42 and collapsed", cfg)
	}
}

func TestSaveGitLayoutPreservesEveryOtherField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"theme":"dracula","tree_width":10,"tree_collapsed":true,"keys":{"quit":["q","c"]}}`)

	if err := SaveGitLayout(path, "single"); err != nil {
		t.Fatalf("SaveGitLayout() error = %v, want nil", err)
	}

	cfg, err := Parse([]string{"--config", path}, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.GitLayout != "single" {
		t.Errorf("GitLayout = %q, want %q", cfg.GitLayout, "single")
	}
	if cfg.Theme != "dracula" || cfg.TreeWidth != 10 || !cfg.TreeCollapsed {
		t.Errorf("Parse() = %+v, want the rest of the file preserved", cfg)
	}
	if !slices.Equal(cfg.Keys["quit"], []string{"q", "c"}) {
		t.Errorf("Keys[quit] = %v, want [q c]", cfg.Keys["quit"])
	}
}

func TestSaveGitLayoutRejectsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	if err := SaveGitLayout(path, "triple"); !errors.Is(err, ErrConfigFormat) {
		t.Fatalf("SaveGitLayout() error = %v, want %v", err, ErrConfigFormat)
	}
}

func TestSaveGitLayoutDoesNotOverwriteAnInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, `{"theme": 42}`)

	if err := SaveGitLayout(path, "single"); err == nil {
		t.Fatal("SaveGitLayout() error = nil, want an invalid file error")
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(body) != `{"theme": 42}` {
		t.Errorf("archivo = %q, want the original content", body)
	}
}

func TestSaveLayoutPreservesThemeAndKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeConfigFile(t, path, `{"theme":"dracula","tree_width":10,"keys":{"quit":["q","c"]}}`)

	if err := SaveLayout(path, 30, false); err != nil {
		t.Fatalf("SaveLayout() error = %v, want nil", err)
	}

	// The rewritten file must parse again with the theme and keys intact.
	cfg, err := Parse([]string{"--config", path}, noEnv)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.Theme != "dracula" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "dracula")
	}
	if cfg.TreeWidth != 30 {
		t.Errorf("TreeWidth = %d, want 30", cfg.TreeWidth)
	}
	if cfg.TreeCollapsed {
		t.Error("TreeCollapsed = true, want false")
	}
	if got := cfg.Keys["quit"]; !slices.Equal(got, []string{"q", "c"}) {
		t.Errorf("Keys[quit] = %v, want [q c]", got)
	}

	// The atomic write must not leave temporary files behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Errorf("directorio = %v, want solo config.json", entries)
	}
}

func TestSaveLayoutInvalidFileNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"theme":`
	writeConfigFile(t, path, body)

	if err := SaveLayout(path, 30, true); !errors.Is(err, ErrConfigFormat) {
		t.Fatalf("SaveLayout() error = %v, want %v", err, ErrConfigFormat)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(got) != body {
		t.Errorf("contenido = %q, want %q", got, body)
	}
}

func TestSaveLayoutRejectsInvalidArgs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	t.Run("negative width", func(t *testing.T) {
		if err := SaveLayout(path, -1, false); !errors.Is(err, ErrConfigFormat) {
			t.Fatalf("SaveLayout() error = %v, want %v", err, ErrConfigFormat)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("SaveLayout() touched %s (err = %v), want no I/O", path, err)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		if err := SaveLayout("   ", 10, false); !errors.Is(err, ErrConfigFile) {
			t.Fatalf("SaveLayout() error = %v, want %v", err, ErrConfigFile)
		}
	})
}
