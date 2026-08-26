# Readable low-glare themes for Helix

Helix ports of the same six palettes in the companion Ghostty package:

- Everforest Dark Soft
- Everforest Light Soft
- Gruvbox Material Dark Soft
- Gruvbox Material Light Soft
- Selenized Dark
- Selenized Light

The selectable theme names are the filenames without `.toml`, for example `readable_selenized_dark`.

## Install

Copy all six files into Helix's user theme directory:

```sh
mkdir -p ~/.config/helix/themes
cp readable_*.toml ~/.config/helix/themes/
```

Preview a theme in a running Helix session:

```text
:theme readable_selenized_dark
```

To select one permanently, put this at the top of `~/.config/helix/config.toml`:

```toml
theme = "readable_selenized_dark"
```

Current Helix also supports automatic light/dark selection when the terminal reports its appearance preference:

```toml
[theme]
dark = "readable_selenized_dark"
light = "readable_selenized_light"
fallback = "readable_selenized_dark"
```

Replace the two Selenized names with the Everforest or Gruvbox Material pair if preferred.

## Format and implementation

Helix themes are TOML files stored in `~/.config/helix/themes`. Styles map semantic scopes such as `keyword`, `function`, `ui.selection`, and `diagnostic.error` to named colors. The `[palette]` table is last because all keys after that header belong to the palette.

These ports use Helix's documented `inherits` feature:

| Theme pair | Built-in semantic base | What this package changes |
| --- | --- | --- |
| Everforest Soft | `everforest_dark` / `everforest_light` | Replaces the built-in Medium background series with official Soft colors and adds gentler selections and accent cursors. |
| Gruvbox Material Soft | Matching built-in Soft themes | Provides matching `readable_*` aliases and the warm cursor behavior used in the Ghostty package. |
| Selenized | `solarized_dark` / `solarized_light` | Replaces Solarized with Selenized's perceptually balanced palette and supplies a complete low-glare UI mapping. |

The syntax scope mappings therefore track Helix as its Tree-sitter captures evolve, while the chosen palettes and UI styling stay local to these files.

## Validation

Checked against Helix `main` on 2026-08-26:

- all six files parse as TOML;
- every inherited theme exists;
- inheritance merges without cycles;
- every foreground, background, underline, and scalar color reference resolves to a palette entry;
- the merged themes cover 83–99 semantic and interface scopes.

Helix already bundles `gruvbox_material_dark_soft` and `gruvbox_material_light_soft`. It bundles Everforest Dark/Light only with Medium backgrounds, and it does not currently bundle Selenized.

## Sources

- Helix theme documentation: https://docs.helix-editor.com/master/themes.html
- Helix built-in themes: https://github.com/helix-editor/helix/tree/master/runtime/themes
- Everforest palette: https://github.com/sainnhe/everforest/blob/master/palette.md
- Gruvbox Material: https://github.com/sainnhe/gruvbox-material
- Selenized design: https://github.com/jan-warchol/selenized/blob/master/features-and-design.md
