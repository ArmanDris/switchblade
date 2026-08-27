package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/manifoldco/promptui"
)

func TestRunAcceptsThemeSelections(t *testing.T) {
	for _, selection := range themeSelections {
		root := t.TempDir()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := run(
			&stdout,
			&stderr,
			func() (themeSelection, error) { return selection, nil },
			func() (string, error) { return root, nil },
			noOpActivator,
		)
		if exitCode != 0 || stderr.Len() != 0 {
			t.Errorf("selection %q was rejected", selection.label)
		}

		filename := "switchblade-" + selection.theme + "-" + selection.variant
		assertThemeInstalled(t, filepath.Join(root, "ghostty", "themes", filename), selection.ghosttyAsset)
		assertThemeInstalled(t, filepath.Join(root, "helix", "themes", filename+".toml"), selection.helixAsset)
	}
}

func TestRunChecksThemeFiles(t *testing.T) {
	tests := []struct {
		ghosttyExists bool
		helixExists   bool
		wantOutput    bool
	}{
		{true, true, false},
		{false, false, true},
		{false, true, true},
		{true, false, true},
	}

	for _, test := range tests {
		root := t.TempDir()
		ghosttyDirectory := filepath.Join(root, "ghostty", "themes")
		helixDirectory := filepath.Join(root, "helix", "themes")
		if test.ghosttyExists {
			writeTestTheme(
				t,
				filepath.Join(ghosttyDirectory, "switchblade-everforest-dark"),
				"ghostty-readable-themes/Readable Everforest Dark Soft",
			)
		}
		if test.helixExists {
			writeTestTheme(
				t,
				filepath.Join(helixDirectory, "switchblade-everforest-dark.toml"),
				"helix-readable-themes/readable_everforest_dark_soft.toml",
			)
		}

		installed, err := installTheme(root, themeSelections[3])
		if err != nil {
			t.Errorf("existence check failed: %v", err)
		}
		if (len(installed) > 0) != test.wantOutput {
			t.Errorf("unexpected installed directories %q", installed)
		}
		if !test.ghosttyExists && !containsString(installed, ghosttyDirectory) {
			t.Errorf("missing Ghostty directory in %q", installed)
		}
		if !test.helixExists && !containsString(installed, helixDirectory) {
			t.Errorf("missing Helix directory in %q", installed)
		}
		if test.ghosttyExists && containsString(installed, ghosttyDirectory) {
			t.Errorf("existing Ghostty theme reported missing")
		}
		if test.helixExists && containsString(installed, helixDirectory) {
			t.Errorf("existing Helix theme reported missing")
		}
	}
}

func TestInstallThemeUpgradesBrokenGruvboxInheritance(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "helix", "themes", "switchblade-gruvbox-material-dark.toml")
	current, err := themeAssets.ReadFile(themeSelections[1].helixAsset)
	if err != nil {
		t.Fatal(err)
	}
	legacy := bytes.Replace(current, []byte(`inherits = "gruvbox-material"`), []byte(`inherits = "gruvbox_material_dark_soft"`), 1)
	writeTestFile(t, path, string(legacy))

	if _, err := installTheme(root, themeSelections[1]); err != nil {
		t.Fatal(err)
	}
	assertThemeInstalled(t, path, themeSelections[1].helixAsset)
}

func TestRunReportsErrors(t *testing.T) {
	root := t.TempDir()
	themePath := filepath.Join(root, "ghostty", "themes", "switchblade-everforest-light")
	if err := os.MkdirAll(themePath, 0o700); err != nil {
		t.Fatal(err)
	}
	differentRoot := t.TempDir()
	writeTestTheme(
		t,
		filepath.Join(differentRoot, "ghostty", "themes", "switchblade-everforest-light"),
		"helix-readable-themes/readable_selenized_dark.toml",
	)

	for _, test := range []struct {
		selectTheme func() (themeSelection, error)
		configRoot  func() (string, error)
	}{
		{func() (themeSelection, error) { return themeSelection{}, errors.New("cancelled") }, func() (string, error) { return root, nil }},
		{func() (themeSelection, error) { return themeSelection{theme: "everforest", variant: "light"}, nil }, func() (string, error) { return "", errors.New("bad config") }},
		{func() (themeSelection, error) { return themeSelections[2], nil }, func() (string, error) { return root, nil }},
		{func() (themeSelection, error) { return themeSelections[2], nil }, func() (string, error) { return differentRoot, nil }},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if run(&stdout, &stderr, test.selectTheme, test.configRoot, noOpActivator) != 1 {
			t.Error("expected command to fail")
		}
	}
}

func TestRunCancelIsSuccessful(t *testing.T) {
	activated := false
	exitCode := run(
		io.Discard,
		io.Discard,
		func() (themeSelection, error) { return themeSelection{}, promptui.ErrInterrupt },
		func() (string, error) { return t.TempDir(), nil },
		func(string, themeSelection) ([]string, error) {
			activated = true
			return nil, nil
		},
	)
	if exitCode != 0 || activated {
		t.Fatalf("cancel returned %d and activated=%t", exitCode, activated)
	}
}

func TestRunStylesWarnings(t *testing.T) {
	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	warning := "Your Zellij config contains the \x1b[3mtheme_light\x1b[0m and \x1b[3mtheme_dark\x1b[0m options. They may override your switchblade theme. Remove them to ensure the switchblade theme is always used."
	exitCode := run(
		&stdout,
		&stderr,
		func() (themeSelection, error) { return themeSelections[2], nil },
		func() (string, error) { return root, nil },
		func(string, themeSelection) ([]string, error) { return []string{warning}, nil },
	)
	if exitCode != 0 {
		t.Fatalf("run() = %d, want 0", exitCode)
	}
	if stdout.Len() != 0 {
		t.Errorf("success output = %q, want no output", stdout.String())
	}
	want := "\x1b[48;5;214;30m ⚠ warning: \x1b[0m  Your Zellij config contains the \x1b[3mtheme_light\x1b[0m and\n" +
		"               \x1b[3mtheme_dark\x1b[0m options. They may override your switchblade theme.\n" +
		"               Remove them to ensure the switchblade theme is always used.\n"
	if got := stderr.String(); got != want {
		t.Errorf("warning output = %q, want %q", got, want)
	}
}

func TestConfigRoot(t *testing.T) {
	absolute := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", absolute)
	if root, err := configRoot(); err != nil || root != absolute {
		t.Fatalf("configRoot() = %q, %v", root, err)
	}

	t.Setenv("XDG_CONFIG_HOME", "relative")
	if _, err := configRoot(); err == nil {
		t.Fatal("configRoot() accepted a relative XDG_CONFIG_HOME")
	}
}

func writeTestTheme(t *testing.T, path, assetPath string) {
	t.Helper()
	data, err := themeAssets.ReadFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertThemeInstalled(t *testing.T, path, assetPath string) {
	t.Helper()
	want, err := themeAssets.ReadFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("installed theme %s does not match %s", path, assetPath)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("installed theme mode = %o, want 644", info.Mode().Perm())
	}
}

func noOpActivator(string, themeSelection) ([]string, error) { return nil, nil }

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
