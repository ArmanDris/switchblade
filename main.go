package main

// Themes:
// gruvbox-material, https://github.com/sainnhe/gruvbox-material
// everforest, https://everforest.vercel.app/
// Selenized (Black/White versions), https://github.com/jan-warchol/selenized/blob/master/features-and-design.md

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/manifoldco/promptui"
)

type themeSelection struct {
	label        string
	theme        string
	variant      string
	ghosttyAsset string
	helixAsset   string
}

var themeSelections = []themeSelection{
	{
		label:        "Gruvbox Material Light",
		theme:        "gruvbox-material",
		variant:      "light",
		ghosttyAsset: "ghostty-readable-themes/Readable Gruvbox Material Light Soft",
		helixAsset:   "helix-readable-themes/readable_gruvbox_material_light_soft.toml",
	},
	{
		label:        "Gruvbox Material Dark",
		theme:        "gruvbox-material",
		variant:      "dark",
		ghosttyAsset: "ghostty-readable-themes/Readable Gruvbox Material Dark Soft",
		helixAsset:   "helix-readable-themes/readable_gruvbox_material_dark_soft.toml",
	},
	{
		label:        "Everforest Light",
		theme:        "everforest",
		variant:      "light",
		ghosttyAsset: "ghostty-readable-themes/Readable Everforest Light Soft",
		helixAsset:   "helix-readable-themes/readable_everforest_light_soft.toml",
	},
	{
		label:        "Everforest Dark",
		theme:        "everforest",
		variant:      "dark",
		ghosttyAsset: "ghostty-readable-themes/Readable Everforest Dark Soft",
		helixAsset:   "helix-readable-themes/readable_everforest_dark_soft.toml",
	},
	{
		label:        "Selenized BW Light",
		theme:        "selenized-bw",
		variant:      "light",
		ghosttyAsset: "ghostty-readable-themes/Readable Selenized Light",
		helixAsset:   "helix-readable-themes/readable_selenized_light.toml",
	},
	{
		label:        "Selenized BW Dark",
		theme:        "selenized-bw",
		variant:      "dark",
		ghosttyAsset: "ghostty-readable-themes/Readable Selenized Dark",
		helixAsset:   "helix-readable-themes/readable_selenized_dark.toml",
	},
}

//go:embed ghostty-readable-themes/* helix-readable-themes/*.toml
var themeAssets embed.FS

func main() {
	os.Exit(run(os.Stdout, os.Stderr, selectTheme, configRoot, activateTheme))
}

func run(
	stdout io.Writer,
	stderr io.Writer,
	chooseTheme func() (themeSelection, error),
	resolveConfigRoot func() (string, error),
	activate func(string, themeSelection) ([]string, error),
) int {
	selection, err := chooseTheme()
	if err != nil {
		if errors.Is(err, promptui.ErrInterrupt) {
			return 0
		}
		fmt.Fprintf(stderr, "select theme: %v\n", err)
		return 1
	}

	root, err := resolveConfigRoot()
	if err != nil {
		fmt.Fprintf(stderr, "resolve config directory: %v\n", err)
		return 1
	}

	_, err = installTheme(root, selection)
	if err != nil {
		fmt.Fprintf(stderr, "install theme files: %v\n", err)
		return 1
	}

	warnings, err := activate(root, selection)
	if err != nil {
		fmt.Fprintf(stderr, "activate theme: %v\n", err)
		return 1
	}
	for _, warning := range warnings {
		writeWarning(stderr, warning)
	}
	return 0
}

const (
	warningLabel  = "\x1b[48;5;214;30m ⚠ warning: \x1b[0m"
	warningIndent = "               "
	warningWidth  = 80
)

func writeWarning(stderr io.Writer, warning string) {
	words := strings.Fields(warning)
	line := ""
	firstLine := true
	writeLine := func(line string) {
		if firstLine {
			fmt.Fprintf(stderr, "%s  %s\n", warningLabel, line)
			firstLine = false
			return
		}
		fmt.Fprintf(stderr, "%s%s\n", warningIndent, line)
	}
	for _, word := range words {
		if line != "" && visibleLength(line)+1+visibleLength(word) > warningWidth-len(warningIndent) {
			writeLine(line)
			line = word
			continue
		}
		if line == "" {
			line = word
		} else {
			line += " " + word
		}
	}
	if line != "" {
		writeLine(line)
	}
}

func visibleLength(text string) int {
	length := 0
	inEscape := false
	for _, character := range text {
		switch {
		case character == '\x1b':
			inEscape = true
		case inEscape && character >= '@' && character <= '~':
			inEscape = false
		case !inEscape:
			length++
		}
	}
	return length
}

func selectTheme() (themeSelection, error) {
	labels := make([]string, len(themeSelections))
	for i, selection := range themeSelections {
		labels[i] = selection.label
	}

	prompt := promptui.Select{
		Label: "Select theme",
		Items: labels,
		Size:  len(labels),
		Stdin: quitAwareInput(os.Stdin),
		Templates: &promptui.SelectTemplates{
			Label:    "{{ . }}:",
			Active:   "\x1b[1;36m> {{ . }}\x1b[0m",
			Inactive: "  {{ . }}",
			Selected: "> {{ . }}",
		},
	}

	index, _, err := prompt.Run()
	if err != nil {
		return themeSelection{}, err
	}
	return themeSelections[index], nil
}

func configRoot() (string, error) {
	if root := os.Getenv("XDG_CONFIG_HOME"); root != "" {
		if !filepath.IsAbs(root) {
			return "", fmt.Errorf("XDG_CONFIG_HOME must be absolute")
		}
		return filepath.Clean(root), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}

type themeDestination struct {
	directory string
	path      string
	data      []byte
	upgrade   bool
}

func installTheme(root string, selection themeSelection) ([]string, error) {
	filename := "switchblade-" + selection.theme + "-" + selection.variant
	destinations := []themeDestination{
		{
			directory: filepath.Join(root, "ghostty", "themes"),
			path:      filepath.Join(root, "ghostty", "themes", filename),
		},
		{
			directory: filepath.Join(root, "helix", "themes"),
			path:      filepath.Join(root, "helix", "themes", filename+".toml"),
		},
	}

	assetPaths := []string{selection.ghosttyAsset, selection.helixAsset}
	var pending []themeDestination
	for i := range destinations {
		data, err := themeAssets.ReadFile(assetPaths[i])
		if err != nil {
			return nil, fmt.Errorf("read bundled theme %s: %w", assetPaths[i], err)
		}
		destinations[i].data = data

		info, err := os.Lstat(destinations[i].path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			pending = append(pending, destinations[i])
		case err != nil:
			return nil, fmt.Errorf("inspect %s: %w", destinations[i].path, err)
		case !info.Mode().IsRegular():
			return nil, fmt.Errorf("%s is not a regular file", destinations[i].path)
		default:
			existing, err := os.ReadFile(destinations[i].path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", destinations[i].path, err)
			}
			if isLegacyTheme(existing, data) {
				destinations[i].upgrade = true
				pending = append(pending, destinations[i])
			} else if !bytes.Equal(existing, data) {
				return nil, fmt.Errorf("%s already exists with different contents", destinations[i].path)
			}
		}
	}

	var created []string
	var installedDirectories []string
	for _, destination := range pending {
		if err := os.MkdirAll(destination.directory, 0o700); err != nil {
			removeFiles(created)
			return nil, fmt.Errorf("create %s: %w", destination.directory, err)
		}
		if destination.upgrade {
			if err := writeFileAtomic(destination.path, destination.data, 0o644); err != nil {
				removeFiles(created)
				return nil, fmt.Errorf("upgrade %s: %w", destination.path, err)
			}
			installedDirectories = append(installedDirectories, destination.directory)
			continue
		}
		if err := writeFileExclusive(destination.path, destination.data); err != nil {
			removeFiles(created)
			return nil, fmt.Errorf("write %s: %w", destination.path, err)
		}
		created = append(created, destination.path)
		installedDirectories = append(installedDirectories, destination.directory)
	}

	return installedDirectories, nil
}

func isLegacyTheme(existing, replacement []byte) bool {
	for _, oldInheritance := range [][]byte{
		[]byte(`inherits = "gruvbox_material_dark_soft"`),
		[]byte(`inherits = "gruvbox_material_light_soft"`),
	} {
		if !bytes.Contains(existing, oldInheritance) {
			continue
		}
		upgraded := bytes.Replace(existing, oldInheritance, []byte(`inherits = "gruvbox-material"`), 1)
		if bytes.Equal(upgraded, replacement) {
			return true
		}
	}
	return false
}

func writeFileExclusive(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".switchblade-theme-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Link(temporaryPath, path)
}

func removeFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}
