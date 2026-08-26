# Readable, low-glare Ghostty themes

Six Ghostty themes: three dark and three light. They favor warm or muted backgrounds, moderate default-text contrast, and restrained selections instead of pure black/white or inverted selection blocks.

## The three palette families

| Family | Why it made the cut | Dark contrast | Light contrast |
| --- | --- | ---: | ---: |
| Everforest Soft | Warm green-gray palette explicitly designed to be soft and comfortable. These files use Everforest's actual `bg0` Soft backgrounds rather than its darker/lighter `bg_dim` colors. | 6.65:1 | 4.66:1 |
| Gruvbox Material Soft | A less harsh, more internally consistent Gruvbox derivative. These files combine its Material accents with its official Soft backgrounds and subdued selection colors. | 7.26:1 | 6.67:1 |
| Selenized | A less-common Solarized redesign tuned in CIE Lab for moderately low contrast, balanced accent lightness, and better readability. These files follow the current upstream Kitty terminal port. | 6.07:1 | 5.37:1 |

The ratios above are WCAG-style sRGB contrast measurements for the default foreground against the background. They are a useful sanity check, not a complete measure of terminal readability. Font weight, display gamma, ambient light, and individual CLI applications' use of ANSI colors still matter.

Sources:

- Ghostty theme documentation: https://ghostty.org/docs/features/theme
- Everforest palette: https://github.com/sainnhe/everforest/blob/master/palette.md
- Gruvbox Material: https://github.com/sainnhe/gruvbox-material
- Selenized design notes: https://github.com/jan-warchol/selenized/blob/master/features-and-design.md

## Install

Copy the six extensionless theme files into Ghostty's user theme directory:

```sh
mkdir -p ~/.config/ghostty/themes
cp "Readable "* ~/.config/ghostty/themes/
```

Choose one automatic light/dark pair in `~/.config/ghostty/config`:

```ini
# Everforest
theme = dark:Readable Everforest Dark Soft,light:Readable Everforest Light Soft

# Gruvbox Material (use instead of the line above)
theme = dark:Readable Gruvbox Material Dark Soft,light:Readable Gruvbox Material Light Soft

# Selenized (use instead of the lines above)
theme = dark:Readable Selenized Dark,light:Readable Selenized Light
```

Only keep one `theme = ...` line active. Reload Ghostty's configuration afterward. `ghostty +list-themes` opens the interactive theme preview when run in a terminal.

To validate an individual downloaded file before installing it:

```sh
ghostty +validate-config --config-file "/absolute/path/to/Readable Selenized Dark"
```

## What a Ghostty theme needs

A theme is a normal Ghostty configuration file. A practical color-only theme defines:

- `palette = 0=#RRGGBB` through `palette = 15=#RRGGBB`
- `background` and `foreground`
- `cursor-color` and `cursor-text`
- `selection-background` and `selection-foreground`

Named theme files are discovered first in `$XDG_CONFIG_HOME/ghostty/themes` (normally `~/.config/ghostty/themes`) and then in Ghostty's bundled resources. A theme can instead be referenced by absolute path. Named lookup is case-sensitive on case-sensitive filesystems. A theme may technically contain other Ghostty settings, so review third-party files before installing them.

## Comparison with Ghostty's built-ins

Checked 2026-08-26 against the theme bundle pinned by Ghostty `main` (`ghostty-themes-release-20260810-152212-0173c3c.tgz`). Ghostty sources its bundled themes from iTerm2-Color-Schemes and updates the dependency regularly.

There are no exact matches, but every generated theme has an obvious close built-in:

| Generated theme | Nearest built-in | Exact color fields | Important difference |
| --- | --- | ---: | --- |
| Readable Everforest Dark Soft | `Everforest Dark Soft` | 11/22 | Built-in uses `#293136` (`bg_dim`) instead of upstream Soft `bg0` `#333c43`, and mixes in brighter light-side ANSI colors. |
| Readable Everforest Light Soft | `Everforest Light Soft` | 10/22 | Built-in uses `#e5dfc5` (`bg_dim`) instead of upstream Soft `bg0` `#f3ead3`; this port also darkens white/gray terminal entries for legibility. |
| Readable Gruvbox Material Dark Soft | `Gruvbox Material Dark` | 17/22 | Built-in background is Medium `#282828`; this file uses Soft `#32302f` and a muted selection instead of full inversion. |
| Readable Gruvbox Material Light Soft | `Gruvbox Material Light` | 17/22 | Built-in background is Medium `#fbf1c7`; this file uses Soft `#f2e5bc` and a muted selection instead of full inversion. |
| Readable Selenized Dark | `Selenized Dark` | 17/22 | Same base colors and accents; current upstream Kitty mappings differ for ANSI 0/7/8 and selection. |
| Readable Selenized Light | `Selenized Light` | 17/22 | Same base colors and accents; current upstream Kitty mappings differ for ANSI 0/7/8 and selection. |

“Exact color fields” compares the 16 ANSI entries plus background, foreground, cursor color/text, and selection background/foreground. If you prefer zero custom files, the six nearest built-ins above are reasonable substitutes. The custom files mainly preserve the intended Soft backgrounds and gentler selection behavior.
