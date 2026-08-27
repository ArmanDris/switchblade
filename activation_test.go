package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creachadair/tomledit"
)

func TestConfigureHelix(t *testing.T) {
	const themeName = "switchblade-everforest-light"
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:  "missing theme",
			input: "[editor]\nline-number = \"relative\"\n",
			contains: []string{
				`theme = "switchblade-everforest-light" # switchblade managed`,
				"[editor]\nline-number = \"relative\"",
			},
		},
		{
			name:  "string theme",
			input: "theme = \"ashen\"\n\n[editor]\nline-number = \"relative\"\n",
			contains: []string{
				`# switchblade previous theme: theme = "ashen"`,
				`theme = "switchblade-everforest-light" # switchblade managed`,
				"[editor]\nline-number = \"relative\"",
			},
		},
		{
			name:  "automatic theme table",
			input: "[theme]\ndark = \"ashen\"\nlight = \"iceberg-light\"\n\n[editor]\nmouse = false\n",
			contains: []string{
				`theme = "switchblade-everforest-light" # switchblade managed`,
				"# switchblade previous: [theme]",
				`# switchblade previous: dark = "ashen"`,
				"[editor]\nmouse = false",
			},
		},
		{
			name:  "CRLF",
			input: "theme = \"ashen\"\r\n[editor]\r\nmouse = false\r\n",
			contains: []string{
				"theme = \"switchblade-everforest-light\" # switchblade managed\r\n[editor]",
			},
		},
		{
			name:  "existing managed theme",
			input: "theme = \"switchblade-current\" # switchblade managed\n[editor]\nmouse = false\n",
			contains: []string{
				`theme = "switchblade-everforest-light" # switchblade managed`,
			},
		},
	}

	for _, test := range tests {
		output, err := configureHelix([]byte(test.input), themeName)
		if err != nil {
			t.Errorf("%s: %v", test.name, err)
			continue
		}
		if _, err := tomledit.Parse(bytes.NewReader(output)); err != nil {
			t.Errorf("%s produced invalid TOML: %v\n%s", test.name, err, output)
		}
		for _, want := range test.contains {
			if !strings.Contains(string(output), want) {
				t.Errorf("%s output does not contain %q:\n%s", test.name, want, output)
			}
		}
	}
}

func TestPreferredGhosttyConfig(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	appDirectory := filepath.Join(home, "Library", "Application Support", "com.mitchellh.ghostty")
	xdgDirectory := filepath.Join(root, "ghostty")
	writeTestFile(t, filepath.Join(xdgDirectory, "config"), "xdg legacy")

	path, err := preferredGhosttyConfig(home, root)
	if err != nil || path != filepath.Join(xdgDirectory, "config") {
		t.Fatalf("preferredGhosttyConfig() = %q, %v", path, err)
	}

	writeTestFile(t, filepath.Join(appDirectory, "config"), "app legacy")
	path, err = preferredGhosttyConfig(home, root)
	if err != nil || path != filepath.Join(appDirectory, "config") {
		t.Fatalf("preferredGhosttyConfig() = %q, %v", path, err)
	}

	writeTestFile(t, filepath.Join(appDirectory, "config.ghostty"), "app current")
	path, err = preferredGhosttyConfig(home, root)
	if err != nil || path != filepath.Join(appDirectory, "config.ghostty") {
		t.Fatalf("preferredGhosttyConfig() = %q, %v", path, err)
	}
}

func TestActivateTheme(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	helixConfig := filepath.Join(root, "helix", "config.toml")
	ghosttyConfig := filepath.Join(home, "Library", "Application Support", "com.mitchellh.ghostty", "config")
	writeTestFile(t, helixConfig, "theme = \"ashen\"\n\n[editor]\nmouse = false\n")
	writeTestFile(t, ghosttyConfig, "theme = adventure time\nfont-size = 16\n")
	if _, err := installTheme(root, themeSelections[2]); err != nil {
		t.Fatal(err)
	}
	runtime := writeTestHelixRuntime(t, "everforest_light", "solarized_dark")

	var commands []string
	dependencies := activationDependencies{
		homeDir:  func() (string, error) { return home, nil },
		lookPath: func(command string) (string, error) { return "/usr/bin/" + command, nil },
		getenv:   func(string) string { return "" },
		command: func(name string, args ...string) ([]byte, error) {
			commands = append(commands, strings.Join(append([]string{name}, args...), " "))
			if name == "hx" {
				return []byte("Runtime directories: " + runtime + "\n"), nil
			}
			if name == "pgrep" {
				return nil, nil
			}
			return nil, nil
		},
	}

	warnings, err := activateThemeWithDependencies(root, themeSelections[2], dependencies)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("activateThemeWithDependencies() = %q, %v", warnings, err)
	}
	assertLink(t, filepath.Join(root, "ghostty", "themes", "switchblade-current"), "switchblade-everforest-light")
	assertContainsFile(t, helixConfig, `theme = "switchblade-everforest-light" # switchblade managed`)
	assertContainsFile(t, helixConfig, `# switchblade previous theme: theme = "ashen"`)
	assertContainsFile(t, ghosttyConfig, "theme = adventure time")
	assertContainsFile(t, ghosttyConfig, "config-file = "+strconvQuote(filepath.Join(filepath.Dir(ghosttyConfig), "switchblade.conf")))
	assertContainsFile(t, filepath.Join(filepath.Dir(ghosttyConfig), "switchblade.conf"), "theme = switchblade-current")
	if len(commands) != 6 {
		t.Fatalf("commands = %q, want four validations and two process checks", commands)
	}
	zellijConfig := filepath.Join(home, ".config", "zellij", "config.kdl")
	assertContainsFile(t, zellijConfig, `theme "everforest-light" // switchblade managed`)
	data, err := os.ReadFile(zellijConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "theme_dark") || strings.Contains(string(data), "theme_light") {
		t.Errorf("Switchblade added automatic Zellij theme settings:\n%s", data)
	}

	if _, err := installTheme(root, themeSelections[5]); err != nil {
		t.Fatal(err)
	}
	if _, err := activateThemeWithDependencies(root, themeSelections[5], dependencies); err != nil {
		t.Fatal(err)
	}
	assertLink(t, filepath.Join(root, "ghostty", "themes", "switchblade-current"), "switchblade-selenized-bw-dark")
	assertContainsFile(t, helixConfig, `theme = "switchblade-selenized-bw-dark" # switchblade managed`)
	data, err = os.ReadFile(helixConfig)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(data, []byte("switchblade previous theme")) != 1 {
		t.Errorf("repeated activation duplicated recovery comments:\n%s", data)
	}
}

func TestConfigureZellij(t *testing.T) {
	input := []byte(`theme "gruvbox-dark"
theme_dark "gruvbox-dark"
theme_light "gruvbox-light"
keybinds {
    normal { bind "x" { Quit; } }
}

func TestConfigureZellijWarnsForConfiguredOverrides(t *testing.T) {
	for _, test := range []struct {
		name        string
		input       string
		wantWarning string
	}{
		{
			name:        "light only",
			input:       "theme_light \"gruvbox-light\"\n",
			wantWarning: "Your Zellij config contains the \x1b[3mtheme_light\x1b[0m and \x1b[3mtheme_dark\x1b[0m options. They may override your switchblade theme. Remove them to ensure the switchblade theme is always used.",
		},
		{
			name:        "dark only",
			input:       "theme_dark \"gruvbox-dark\"\n",
			wantWarning: "Your Zellij config contains the \x1b[3mtheme_light\x1b[0m and \x1b[3mtheme_dark\x1b[0m options. They may override your switchblade theme. Remove them to ensure the switchblade theme is always used.",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, warnings, err := configureZellij([]byte(test.input), "solarized-light")
			if err != nil {
				t.Fatal(err)
			}
			if len(warnings) != 1 || warnings[0] != test.wantWarning {
				t.Fatalf("warnings = %q, want %q", warnings, test.wantWarning)
			}
		})
	}
}
`)
	output, warnings, err := configureZellij(input, "solarized-light")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`// switchblade previous theme: theme "gruvbox-dark"`,
		`theme "solarized-light" // switchblade managed`,
		`theme_dark "gruvbox-dark"`,
		`theme_light "gruvbox-light"`,
		`normal { bind "x" { Quit; } }`,
	} {
		if !bytes.Contains(output, []byte(want)) {
			t.Errorf("output does not contain %q:\n%s", want, output)
		}
	}
	wantWarning := "Your Zellij config contains the \x1b[3mtheme_light\x1b[0m and \x1b[3mtheme_dark\x1b[0m options. They may override your switchblade theme. Remove them to ensure the switchblade theme is always used."
	if len(warnings) != 1 || warnings[0] != wantWarning {
		t.Fatalf("warnings = %q, want %q", warnings, wantWarning)
	}

	updated, warnings, err := configureZellij(output, "everforest-dark")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(updated, []byte("switchblade previous theme:")) != 1 {
		t.Errorf("repeated activation duplicated recovery comments:\n%s", updated)
	}
	if !bytes.Contains(updated, []byte(`theme "everforest-dark" // switchblade managed`)) {
		t.Errorf("managed theme was not updated:\n%s", updated)
	}
	if len(warnings) != 1 || warnings[0] != wantWarning {
		t.Fatalf("warnings after update = %q, want %q", warnings, wantWarning)
	}
}

func TestZellijConfigPath(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "zellij"), 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := zellijConfigPath(home, root, func(string) string { return "" })
	if err != nil || path != filepath.Join(root, "zellij", "config.kdl") {
		t.Fatalf("zellijConfigPath() = %q, %v", path, err)
	}

	explicit := filepath.Join(t.TempDir(), "config.kdl")
	path, err = zellijConfigPath(home, root, func(key string) string {
		if key == "ZELLIJ_CONFIG_FILE" {
			return explicit
		}
		return ""
	})
	if err != nil || path != explicit {
		t.Fatalf("explicit zellijConfigPath() = %q, %v", path, err)
	}
}

func TestZellijThemeName(t *testing.T) {
	for _, selection := range themeSelections {
		theme, err := zellijThemeName(selection)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(theme, "-"+selection.variant) {
			t.Errorf("unexpected Zellij mapping for %s: %q", selection.label, theme)
		}
	}
}

func TestActivationRollsBackFailedValidation(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	helixConfig := filepath.Join(root, "helix", "config.toml")
	ghosttyConfig := filepath.Join(home, "Library", "Application Support", "com.mitchellh.ghostty", "config")
	helixOriginal := []byte("theme = \"ashen\"\n")
	ghosttyOriginal := []byte("theme = adventure time\n")
	writeTestFile(t, helixConfig, string(helixOriginal))
	writeTestFile(t, ghosttyConfig, string(ghosttyOriginal))
	if _, err := installTheme(root, themeSelections[0]); err != nil {
		t.Fatal(err)
	}

	_, err := activateThemeWithDependencies(root, themeSelections[0], activationDependencies{
		homeDir:  func() (string, error) { return home, nil },
		lookPath: func(command string) (string, error) { return command, nil },
		getenv:   func(string) string { return "" },
		command: func(name string, _ ...string) ([]byte, error) {
			if name == "zellij" {
				return nil, nil
			}
			return []byte("invalid config"), errors.New("validation failed")
		},
	})
	if err == nil {
		t.Fatal("activation succeeded despite failed validation")
	}
	assertFileEquals(t, helixConfig, helixOriginal)
	assertFileEquals(t, ghosttyConfig, ghosttyOriginal)
	for _, path := range []string{
		filepath.Join(root, "ghostty", "themes", "switchblade-current"),
		filepath.Join(filepath.Dir(ghosttyConfig), "switchblade.conf"),
		filepath.Join(home, ".config", "zellij", "config.kdl"),
	} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("rollback left %s behind", path)
		}
	}
}

func TestActivationPreservesConfigSymlinks(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	targetDirectory := t.TempDir()
	helixTarget := filepath.Join(targetDirectory, "helix.toml")
	ghosttyTarget := filepath.Join(targetDirectory, "ghostty.conf")
	writeTestFile(t, helixTarget, "theme = \"default\"\n")
	writeTestFile(t, ghosttyTarget, "font-size = 16\n")

	helixConfig := filepath.Join(root, "helix", "config.toml")
	ghosttyConfig := filepath.Join(home, "Library", "Application Support", "com.mitchellh.ghostty", "config.ghostty")
	for path, target := range map[string]string{helixConfig: helixTarget, ghosttyConfig: ghosttyTarget} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := installTheme(root, themeSelections[1]); err != nil {
		t.Fatal(err)
	}
	runtime := writeTestHelixRuntime(t, "gruvbox-material")

	_, err := activateThemeWithDependencies(root, themeSelections[1], activationDependencies{
		homeDir:  func() (string, error) { return home, nil },
		lookPath: func(command string) (string, error) { return command, nil },
		getenv:   func(string) string { return "" },
		command: func(name string, _ ...string) ([]byte, error) {
			if name == "hx" {
				return []byte("Runtime directories: " + runtime + "\n"), nil
			}
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{helixConfig, ghosttyConfig} {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("config symlink %s was replaced", path)
		}
	}
	assertContainsFile(t, helixTarget, `theme = "switchblade-gruvbox-material-dark"`)
	assertContainsFile(t, ghosttyTarget, "config-file = ")
}

func TestActivationWithInstalledValidators(t *testing.T) {
	if _, err := exec.LookPath("hx"); err != nil {
		t.Skip("Helix is not installed")
	}
	if _, err := exec.LookPath("ghostty"); err != nil {
		t.Skip("Ghostty is not installed")
	}
	if _, err := exec.LookPath("zellij"); err != nil {
		t.Skip("Zellij is not installed")
	}

	home := t.TempDir()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "helix", "config.toml"), "theme = \"default\"\n")
	writeTestFile(
		t,
		filepath.Join(home, "Library", "Application Support", "com.mitchellh.ghostty", "config.ghostty"),
		"font-size = 16\n",
	)
	if _, err := installTheme(root, themeSelections[4]); err != nil {
		t.Fatal(err)
	}

	_, err := activateThemeWithDependencies(root, themeSelections[4], activationDependencies{
		homeDir:  func() (string, error) { return home, nil },
		lookPath: exec.LookPath,
		getenv:   func(string) string { return "" },
		command: func(name string, args ...string) ([]byte, error) {
			if name == "pgrep" {
				return nil, nil
			}
			command := exec.Command(name, args...)
			command.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+root)
			return command.CombinedOutput()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReloadFallbacks(t *testing.T) {
	ghosttyWarning := reloadGhostty(func(name string, args ...string) ([]byte, error) {
		switch name {
		case "pgrep":
			return []byte("42\n"), nil
		case "osascript":
			return nil, errors.New("automation denied")
		case "pkill":
			return nil, nil
		default:
			return nil, errors.New("unexpected command")
		}
	})
	if ghosttyWarning != "" {
		t.Errorf("Ghostty signal fallback returned warning %q", ghosttyWarning)
	}

	helixWarning := reloadHelix(func(name string, args ...string) ([]byte, error) {
		if name == "pgrep" {
			return []byte("42\n"), nil
		}
		return nil, errors.New("signal denied")
	})
	if !strings.Contains(helixWarning, ":config-reload") {
		t.Errorf("Helix warning lacks manual fallback: %q", helixWarning)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeTestHelixRuntime(t *testing.T, themes ...string) string {
	t.Helper()
	runtime := t.TempDir()
	for _, theme := range themes {
		writeTestFile(t, filepath.Join(runtime, "themes", theme+".toml"), `"ui.text" = "white"`)
	}
	return runtime
}

func assertLink(t *testing.T, path, target string) {
	t.Helper()
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Errorf("link %s targets %q, want %q", path, got, target)
	}
}

func assertContainsFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(want)) {
		t.Errorf("%s does not contain %q:\n%s", path, want, data)
	}
}

func assertFileEquals(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

func strconvQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
