# switchblade

Swaps your `ghostty`, `zellij`, and `helix` theme all at once.

Supported themes are:
  - Gruvbox Material Light
  - Gruvbox Material Dark
  - Everforest Light
  - Everforest Dark
  - Selenized BW Light
  - Selenized BW Dark

## Danger

This program writes to your config files. It is developped to always backup
any configs before writing, but you never know...

Developed for Mac OS, will probably work on Linux too. Attempts to respect
each program's config directory based on ENV variables then default location.

## Installation

```sh
go build
./switchblade
```
