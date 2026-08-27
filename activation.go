package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/creachadair/tomledit"
)

const (
	currentThemeName       = "switchblade-current"
	ghosttyManagedFilename = "switchblade.conf"
	ghosttyManagedMarker   = "# Switchblade-managed theme override. Remove this directive to restore the previous theme."
)

type activationDependencies struct {
	homeDir  func() (string, error)
	lookPath func(string) (string, error)
	command  func(string, ...string) ([]byte, error)
	getenv   func(string) string
}

func activateTheme(root string, selection themeSelection) ([]string, error) {
	return activateThemeWithDependencies(root, selection, activationDependencies{
		homeDir:  os.UserHomeDir,
		lookPath: exec.LookPath,
		getenv:   os.Getenv,
		command: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		},
	})
}

func activateThemeWithDependencies(
	root string,
	selection themeSelection,
	dependencies activationDependencies,
) ([]string, error) {
	for _, command := range []string{"hx", "ghostty", "zellij"} {
		if _, err := dependencies.lookPath(command); err != nil {
			return nil, fmt.Errorf("find %s: %w", command, err)
		}
	}

	home, err := dependencies.homeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	filename := "switchblade-" + selection.theme + "-" + selection.variant
	zellijTheme, zellijDarkTheme, zellijLightTheme, err := zellijThemeNames(selection)
	if err != nil {
		return nil, err
	}

	helixConfig := filepath.Join(root, "helix", "config.toml")
	ghosttyConfig, err := preferredGhosttyConfig(home, root)
	if err != nil {
		return nil, err
	}
	ghosttyManagedConfig := filepath.Join(filepath.Dir(ghosttyConfig), ghosttyManagedFilename)
	zellijConfig, err := zellijConfigPath(home, root, dependencyGetenv(dependencies))
	if err != nil {
		return nil, err
	}

	helixConfigTarget, err := writableConfigTarget(helixConfig)
	if err != nil {
		return nil, fmt.Errorf("resolve Helix config: %w", err)
	}
	ghosttyConfigTarget, err := writableConfigTarget(ghosttyConfig)
	if err != nil {
		return nil, fmt.Errorf("resolve Ghostty config: %w", err)
	}
	zellijConfigTarget, err := writableConfigTarget(zellijConfig)
	if err != nil {
		return nil, fmt.Errorf("resolve Zellij config: %w", err)
	}

	helixOriginal, helixMode, err := readOptionalRegularFile(helixConfigTarget, 0o600)
	if err != nil {
		return nil, fmt.Errorf("read Helix config: %w", err)
	}
	helixCandidate, err := configureHelix(helixOriginal, filename)
	if err != nil {
		return nil, fmt.Errorf("edit Helix config: %w", err)
	}

	ghosttyOriginal, ghosttyMode, err := readOptionalRegularFile(ghosttyConfigTarget, 0o600)
	if err != nil {
		return nil, fmt.Errorf("read Ghostty config: %w", err)
	}
	ghosttyCandidate, err := configureGhostty(ghosttyOriginal, ghosttyManagedConfig)
	if err != nil {
		return nil, fmt.Errorf("edit Ghostty config: %w", err)
	}

	managedContents := []byte("# Managed by Switchblade. Remove the config-file directive from your Ghostty config to disable.\n" +
		"theme = " + currentThemeName + "\n")
	managedInfo, managedStatErr := os.Lstat(ghosttyManagedConfig)
	managedOriginal, managedMode, err := readOptionalRegularFile(ghosttyManagedConfig, 0o600)
	if err != nil {
		return nil, fmt.Errorf("read Ghostty managed config: %w", err)
	}
	if managedStatErr != nil && !errors.Is(managedStatErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("inspect Ghostty managed config: %w", managedStatErr)
	}
	if managedInfo != nil && !bytes.Equal(managedOriginal, managedContents) {
		return nil, fmt.Errorf("%s already exists with unexpected contents", ghosttyManagedConfig)
	}

	zellijOriginal, zellijMode, err := readOptionalRegularFile(zellijConfigTarget, 0o600)
	if err != nil {
		return nil, fmt.Errorf("read Zellij config: %w", err)
	}
	zellijCandidate, err := configureZellij(zellijOriginal, zellijTheme, zellijDarkTheme, zellijLightTheme)
	if err != nil {
		return nil, fmt.Errorf("edit Zellij config: %w", err)
	}
	if err := validateZellijConfig(zellijCandidate, filepath.Dir(zellijConfig), dependencies.command); err != nil {
		return nil, err
	}

	helixTheme := filepath.Join(root, "helix", "themes", filename+".toml")
	ghosttyAlias := filepath.Join(root, "ghostty", "themes", currentThemeName)
	if err := validateInstalledTheme(helixTheme); err != nil {
		return nil, err
	}
	if err := validateInstalledTheme(filepath.Join(filepath.Dir(ghosttyAlias), filename)); err != nil {
		return nil, err
	}
	if err := validateManagedAlias(ghosttyAlias, ""); err != nil {
		return nil, err
	}

	transaction := newFileTransaction()
	commit := func(err error) ([]string, error) {
		if rollbackErr := transaction.rollback(); rollbackErr != nil {
			return nil, errors.Join(err, fmt.Errorf("rollback activation: %w", rollbackErr))
		}
		return nil, err
	}

	if err := transaction.replaceSymlink(ghosttyAlias, filename); err != nil {
		return commit(fmt.Errorf("update Ghostty theme alias: %w", err))
	}
	if err := transaction.replaceFile(ghosttyManagedConfig, managedContents, managedMode); err != nil {
		return commit(fmt.Errorf("write Ghostty managed config: %w", err))
	}
	if err := transaction.replaceFile(helixConfigTarget, helixCandidate, helixMode); err != nil {
		return commit(fmt.Errorf("write Helix config: %w", err))
	}
	if err := transaction.replaceFile(ghosttyConfigTarget, ghosttyCandidate, ghosttyMode); err != nil {
		return commit(fmt.Errorf("write Ghostty config: %w", err))
	}

	helixHealth, err := dependencies.command("hx", "--config", helixConfig, "--health", "all")
	if err != nil {
		return commit(commandError("validate Helix config", helixHealth, err))
	}
	if err := validateHelixTheme(helixTheme, root, helixHealth); err != nil {
		return commit(fmt.Errorf("validate Helix theme: %w", err))
	}
	if output, err := dependencies.command("ghostty", "+validate-config", "--config-file="+ghosttyConfig); err != nil {
		return commit(commandError("validate Ghostty config", output, err))
	}
	if output, err := dependencies.command("ghostty", "+validate-config"); err != nil {
		return commit(commandError("validate effective Ghostty config", output, err))
	}
	if err := transaction.replaceFile(zellijConfigTarget, zellijCandidate, zellijMode); err != nil {
		return commit(fmt.Errorf("write Zellij config: %w", err))
	}

	var warnings []string
	if warning := reloadHelix(dependencies.command); warning != "" {
		warnings = append(warnings, warning)
	}
	if warning := reloadGhostty(dependencies.command); warning != "" {
		warnings = append(warnings, warning)
	}
	return warnings, nil
}

func dependencyGetenv(dependencies activationDependencies) func(string) string {
	if dependencies.getenv != nil {
		return dependencies.getenv
	}
	return os.Getenv
}

func preferredGhosttyConfig(home, root string) (string, error) {
	appSupport := filepath.Join(home, "Library", "Application Support", "com.mitchellh.ghostty")
	paths := []string{
		filepath.Join(appSupport, "config.ghostty"),
		filepath.Join(appSupport, "config"),
		filepath.Join(root, "ghostty", "config.ghostty"),
		filepath.Join(root, "ghostty", "config"),
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return "", fmt.Errorf("inspect Ghostty config %s: %w", path, err)
		case !info.Mode().IsRegular():
			return "", fmt.Errorf("Ghostty config %s is not a regular file", path)
		case info.Size() > 0:
			return path, nil
		}
	}
	return paths[0], nil
}

func zellijConfigPath(home, root string, getenv func(string) string) (string, error) {
	if path := getenv("ZELLIJ_CONFIG_FILE"); path != "" {
		if !filepath.IsAbs(path) {
			return "", fmt.Errorf("ZELLIJ_CONFIG_FILE must be absolute")
		}
		return filepath.Clean(path), nil
	}
	if directory := getenv("ZELLIJ_CONFIG_DIR"); directory != "" {
		if !filepath.IsAbs(directory) {
			return "", fmt.Errorf("ZELLIJ_CONFIG_DIR must be absolute")
		}
		return filepath.Join(filepath.Clean(directory), "config.kdl"), nil
	}

	for _, directory := range []string{
		filepath.Join(home, ".config", "zellij"),
		filepath.Join(root, "zellij"),
		"/etc/zellij",
	} {
		info, err := os.Stat(directory)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return "", fmt.Errorf("inspect Zellij config directory %s: %w", directory, err)
		case info.IsDir():
			return filepath.Join(directory, "config.kdl"), nil
		default:
			return "", fmt.Errorf("Zellij config directory %s is not a directory", directory)
		}
	}
	return filepath.Join(home, ".config", "zellij", "config.kdl"), nil
}

func zellijThemeNames(selection themeSelection) (theme, dark, light string, err error) {
	switch selection.theme {
	case "gruvbox-material":
		return "gruvbox-" + selection.variant, "gruvbox-dark", "gruvbox-light", nil
	case "everforest":
		return "everforest-" + selection.variant, "everforest-dark", "everforest-light", nil
	case "selenized-bw":
		return "solarized-" + selection.variant, "solarized-dark", "solarized-light", nil
	default:
		return "", "", "", fmt.Errorf("no Zellij theme mapping for %s", selection.theme)
	}
}

var zellijThemeLine = regexp.MustCompile(`^(\s*)(theme|theme_dark|theme_light)\s+"(?:[^"\\]|\\.)*"\s*;?\s*(?://.*)?(?:\r?\n)?$`)

func configureZellij(original []byte, theme, darkTheme, lightTheme string) ([]byte, error) {
	newline := "\n"
	if bytes.Contains(original, []byte("\r\n")) {
		newline = "\r\n"
	}
	desired := map[string]string{
		"theme":       theme,
		"theme_dark":  darkTheme,
		"theme_light": lightTheme,
	}
	lines := strings.SplitAfter(string(original), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	depth := 0
	found := make(map[string]int)
	for index, line := range lines {
		if depth == 0 {
			trimmed := strings.TrimSpace(line)
			for _, key := range []string{"theme", "theme_dark", "theme_light"} {
				if !strings.HasPrefix(trimmed, key) || (len(trimmed) > len(key) && trimmed[len(key)] != ' ' && trimmed[len(key)] != '\t' && trimmed[len(key)] != '"') {
					continue
				}
				matches := zellijThemeLine.FindStringSubmatch(line)
				if matches == nil || matches[2] != key {
					return nil, fmt.Errorf("unsupported top-level Zellij %s declaration", key)
				}
				if _, duplicate := found[key]; duplicate {
					return nil, fmt.Errorf("multiple top-level Zellij %s declarations", key)
				}
				found[key] = index
			}
		}
		nextDepth, err := advanceKDLDepth(line, depth)
		if err != nil {
			return nil, err
		}
		depth = nextDepth
	}
	if depth != 0 {
		return nil, fmt.Errorf("unterminated Zellij KDL block")
	}

	var additions strings.Builder
	for _, key := range []string{"theme", "theme_dark", "theme_light"} {
		managed := key + " " + strconv.Quote(desired[key]) + " // switchblade managed" + newline
		index, exists := found[key]
		if !exists {
			additions.WriteString(managed)
			continue
		}
		if strings.Contains(lines[index], "// switchblade managed") {
			lines[index] = managed
			continue
		}
		lines[index] = "// switchblade previous " + key + ": " + strings.TrimSpace(lines[index]) + newline + managed
	}
	if additions.Len() > 0 {
		lines[0] = additions.String() + lines[0]
	}
	return []byte(strings.Join(lines, "")), nil
}

func advanceKDLDepth(line string, depth int) (int, error) {
	inString := false
	escaped := false
	for index := 0; index < len(line); index++ {
		character := line[index]
		if inString {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		if character == '"' {
			inString = true
			continue
		}
		if character == '/' && index+1 < len(line) && line[index+1] == '/' {
			break
		}
		switch character {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return 0, fmt.Errorf("unexpected closing brace in Zellij config")
			}
		}
	}
	if inString {
		return 0, fmt.Errorf("unsupported multiline string in Zellij config")
	}
	return depth, nil
}

func validateZellijConfig(candidate []byte, configDirectory string, command func(string, ...string) ([]byte, error)) error {
	temporary, err := os.CreateTemp("", "switchblade-zellij-*.kdl")
	if err != nil {
		return fmt.Errorf("create temporary Zellij config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(candidate); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary Zellij config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Zellij config: %w", err)
	}
	output, err := command("zellij", "--config", temporaryPath, "--config-dir", configDirectory, "setup", "--check")
	if err != nil {
		return commandError("validate Zellij config", output, err)
	}
	return nil
}

func writableConfigTarget(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return filepath.Clean(target), nil
}

func configureHelix(original []byte, themeName string) ([]byte, error) {
	document, err := tomledit.Parse(bytes.NewReader(original))
	if err != nil {
		return nil, err
	}
	theme := document.First("theme")
	managed := `theme = "` + themeName + `" # switchblade managed`
	newline := "\n"
	if bytes.Contains(original, []byte("\r\n")) {
		newline = "\r\n"
	}

	if theme == nil {
		return append([]byte(managed+newline), original...), nil
	}
	if theme.IsMapping() {
		if !theme.Section.IsGlobal() {
			return nil, fmt.Errorf("theme is not a top-level setting")
		}
		start, end, line, ok := findGlobalThemeLine(original)
		if !ok {
			return nil, fmt.Errorf("could not locate the top-level theme setting")
		}
		if strings.Contains(line, "# switchblade managed") {
			return replaceRange(original, start, end, []byte(managed+newline)), nil
		}
		if theme.Value.String() == `"`+themeName+`"` {
			return original, nil
		}
		replacement := "# switchblade previous theme: " + strings.TrimSpace(line) + newline + managed + newline
		return replaceRange(original, start, end, []byte(replacement)), nil
	}
	if theme.IsSection() {
		start, end, block, ok := findThemeSection(original)
		if !ok {
			return nil, fmt.Errorf("could not locate the [theme] table")
		}
		var backup strings.Builder
		backup.WriteString(managed)
		backup.WriteString(newline)
		backup.WriteString("# switchblade previous theme configuration:")
		backup.WriteString(newline)
		for _, line := range strings.Split(strings.TrimSuffix(strings.ReplaceAll(block, "\r\n", "\n"), "\n"), "\n") {
			backup.WriteString("# switchblade previous: ")
			backup.WriteString(line)
			backup.WriteString(newline)
		}
		candidate := replaceRange(original, start, end, nil)
		return append([]byte(backup.String()), candidate...), nil
	}
	return nil, fmt.Errorf("unsupported theme configuration")
}

func validateHelixTheme(themePath, root string, healthOutput []byte) error {
	data, err := os.ReadFile(themePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", themePath, err)
	}
	document, err := tomledit.Parse(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse %s: %w", themePath, err)
	}
	inherits := document.First("inherits")
	if inherits == nil {
		return nil
	}
	if !inherits.IsMapping() || !inherits.Section.IsGlobal() {
		return fmt.Errorf("%s has an invalid inherits setting", themePath)
	}
	parent, err := strconv.Unquote(inherits.Value.String())
	if err != nil || parent == "" {
		return fmt.Errorf("%s has an invalid inherits value", themePath)
	}

	themeDirectories := []string{filepath.Join(root, "helix", "themes")}
	const runtimeMarker = "Runtime directories:"
	for _, line := range strings.Split(string(healthOutput), "\n") {
		line = stripANSI(line)
		marker := strings.Index(line, runtimeMarker)
		if marker < 0 {
			continue
		}
		for _, directory := range strings.Split(strings.TrimSpace(line[marker+len(runtimeMarker):]), ";") {
			if directory = strings.TrimSpace(directory); directory != "" {
				themeDirectories = append(themeDirectories, filepath.Join(directory, "themes"))
			}
		}
		break
	}

	for _, directory := range themeDirectories {
		candidate := filepath.Join(directory, parent+".toml")
		info, err := os.Stat(candidate)
		switch {
		case err == nil && info.Mode().IsRegular():
			return nil
		case err == nil:
			return fmt.Errorf("inherited Helix theme %s is not a regular file", candidate)
		case errors.Is(err, fs.ErrNotExist):
			continue
		default:
			return fmt.Errorf("inspect inherited Helix theme %s: %w", candidate, err)
		}
	}
	return fmt.Errorf("%s inherits from missing theme %q", filepath.Base(themePath), parent)
}

func stripANSI(value string) string {
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\x1b' || index+1 >= len(value) || value[index+1] != '[' {
			result.WriteByte(value[index])
			continue
		}
		index += 2
		for index < len(value) && (value[index] < '@' || value[index] > '~') {
			index++
		}
	}
	return result.String()
}

func findGlobalThemeLine(data []byte) (int, int, string, bool) {
	for start := 0; start < len(data); {
		end := bytes.IndexByte(data[start:], '\n')
		if end < 0 {
			end = len(data)
		} else {
			end += start + 1
		}
		line := strings.TrimSuffix(string(data[start:end]), "\n")
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			return 0, 0, "", false
		}
		if strings.HasPrefix(trimmed, "theme") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "theme"))
			if strings.HasPrefix(rest, "=") {
				return start, end, line, true
			}
		}
		start = end
	}
	return 0, 0, "", false
}

func findThemeSection(data []byte) (int, int, string, bool) {
	sectionStart := -1
	sectionEnd := len(data)
	for start := 0; start < len(data); {
		end := bytes.IndexByte(data[start:], '\n')
		if end < 0 {
			end = len(data)
		} else {
			end += start + 1
		}
		trimmed := strings.TrimSpace(strings.TrimSuffix(string(data[start:end]), "\n"))
		if strings.HasPrefix(trimmed, "[") {
			if sectionStart >= 0 {
				sectionEnd = start
				break
			}
			withoutComment := strings.TrimSpace(strings.SplitN(trimmed, "#", 2)[0])
			if withoutComment == "[theme]" {
				sectionStart = start
			}
		}
		start = end
	}
	if sectionStart < 0 {
		return 0, 0, "", false
	}
	return sectionStart, sectionEnd, string(data[sectionStart:sectionEnd]), true
}

func replaceRange(data []byte, start, end int, replacement []byte) []byte {
	result := make([]byte, 0, len(data)-end+start+len(replacement))
	result = append(result, data[:start]...)
	result = append(result, replacement...)
	result = append(result, data[end:]...)
	return result
}

func configureGhostty(original []byte, managedPath string) ([]byte, error) {
	directive := "config-file = " + strconv.Quote(managedPath)
	if bytes.Contains(original, []byte(directive)) {
		return original, nil
	}
	if bytes.Contains(original, []byte(ghosttyManagedMarker)) {
		return nil, fmt.Errorf("found the Switchblade marker without its expected config-file directive")
	}
	candidate := append([]byte(nil), original...)
	if len(candidate) > 0 && candidate[len(candidate)-1] != '\n' {
		candidate = append(candidate, '\n')
	}
	if len(candidate) > 0 {
		candidate = append(candidate, '\n')
	}
	candidate = append(candidate, []byte(ghosttyManagedMarker+"\n")...)
	candidate = append(candidate, []byte(directive+"\n")...)
	return candidate, nil
}

func readOptionalRegularFile(path string, defaultMode fs.FileMode) ([]byte, fs.FileMode, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, defaultMode, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%s is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	return data, info.Mode().Perm(), err
}

func validateManagedAlias(path, extension string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect theme alias %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("theme alias %s exists and is not a symlink", path)
	}
	target, err := os.Readlink(path)
	if err != nil {
		return fmt.Errorf("read theme alias %s: %w", path, err)
	}
	base := filepath.Base(target)
	if target != base || !isSwitchbladeThemeFilename(base, extension) {
		return fmt.Errorf("theme alias %s points to unmanaged target %s", path, target)
	}
	return nil
}

func isSwitchbladeThemeFilename(filename, extension string) bool {
	for _, selection := range themeSelections {
		if filename == "switchblade-"+selection.theme+"-"+selection.variant+extension {
			return true
		}
	}
	return false
}

func validateInstalledTheme(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect installed theme %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("installed theme %s is not a regular file", path)
	}
	return nil
}

type pathSnapshot struct {
	path       string
	mode       fs.FileMode
	data       []byte
	linkTarget string
	missing    bool
}

type fileTransaction struct {
	snapshots []pathSnapshot
}

func newFileTransaction() *fileTransaction { return new(fileTransaction) }

func (t *fileTransaction) capture(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.snapshots = append(t.snapshots, pathSnapshot{path: path, missing: true})
		return nil
	}
	if err != nil {
		return err
	}
	snapshot := pathSnapshot{path: path, mode: info.Mode().Perm()}
	if info.Mode()&os.ModeSymlink != 0 {
		snapshot.linkTarget, err = os.Readlink(path)
	} else if info.Mode().IsRegular() {
		snapshot.data, err = os.ReadFile(path)
	} else {
		return fmt.Errorf("%s is not a regular file or symlink", path)
	}
	if err != nil {
		return err
	}
	t.snapshots = append(t.snapshots, snapshot)
	return nil
}

func (t *fileTransaction) replaceFile(path string, data []byte, mode fs.FileMode) error {
	existing, _, err := readOptionalRegularFile(path, mode)
	if err != nil {
		return err
	}
	if bytes.Equal(existing, data) && len(existing) > 0 {
		return nil
	}
	if err := t.capture(path); err != nil {
		return err
	}
	return writeFileAtomic(path, data, mode)
}

func (t *fileTransaction) replaceSymlink(path, target string) error {
	if current, err := os.Readlink(path); err == nil && current == target {
		return nil
	}
	if err := t.capture(path); err != nil {
		return err
	}
	return writeSymlinkAtomic(path, target)
}

func (t *fileTransaction) rollback() error {
	var rollbackErrors []error
	for i := len(t.snapshots) - 1; i >= 0; i-- {
		snapshot := t.snapshots[i]
		var err error
		switch {
		case snapshot.missing:
			err = os.Remove(snapshot.path)
			if errors.Is(err, fs.ErrNotExist) {
				err = nil
			}
		case snapshot.linkTarget != "":
			err = writeSymlinkAtomic(snapshot.path, snapshot.linkTarget)
		default:
			err = writeFileAtomic(snapshot.path, snapshot.data, snapshot.mode)
		}
		if err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore %s: %w", snapshot.path, err))
		}
	}
	return errors.Join(rollbackErrors...)
}

func writeFileAtomic(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".switchblade-config-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
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
	return os.Rename(temporaryPath, path)
}

func writeSymlinkAtomic(path, target string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".switchblade-link-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		os.Remove(temporaryPath)
		return err
	}
	if err := os.Remove(temporaryPath); err != nil {
		return err
	}
	defer os.Remove(temporaryPath)
	if err := os.Symlink(target, temporaryPath); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func commandError(action string, output []byte, err error) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, message)
}

func processRunning(command func(string, ...string) ([]byte, error), name string) (bool, error) {
	output, err := command("pgrep", "-x", name)
	if err == nil {
		return len(bytes.TrimSpace(output)) > 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func reloadHelix(command func(string, ...string) ([]byte, error)) string {
	running, err := processRunning(command, "hx")
	if err != nil {
		return fmt.Sprintf("could not detect running Helix processes: %v; use :config-reload", err)
	}
	if !running {
		return ""
	}
	if output, err := command("pkill", "-USR1", "-x", "hx"); err != nil {
		return fmt.Sprintf("could not reload Helix: %v (%s); use :config-reload", err, strings.TrimSpace(string(output)))
	}
	return ""
}

func reloadGhostty(command func(string, ...string) ([]byte, error)) string {
	running, err := processRunning(command, "ghostty")
	if err != nil {
		return fmt.Sprintf("could not detect running Ghostty processes: %v; press Cmd+Shift+,", err)
	}
	if !running {
		return ""
	}
	script := `tell application "Ghostty"
if (count of terminals) is 0 then error "no Ghostty terminals"
repeat with currentTerminal in terminals
perform action "reload_config" on currentTerminal
end repeat
end tell`
	if _, err := command("osascript", "-e", script); err == nil {
		return ""
	}
	if output, err := command("pkill", "-USR2", "-x", "ghostty"); err != nil {
		return fmt.Sprintf("could not reload Ghostty: %v (%s); press Cmd+Shift+,", err, strings.TrimSpace(string(output)))
	}
	return ""
}
