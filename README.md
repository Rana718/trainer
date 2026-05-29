# trainer

A small Linux CLI tool to run Windows `.exe` trainers (or any Windows app) through the same Proton version a Steam game is already using — no manual prefix setup, no digging through Steam folders.

> **Linux only.** Requires Steam with Proton.

---

## Why

When you run a game trainer on Linux, it needs to use the exact same Wine prefix and Proton version as the game, otherwise it either crashes or can't attach to the game process. Doing this manually means finding the right compatdata folder, figuring out which Proton build Steam picked, and constructing the right environment variables every time.

`trainer` does all of that automatically.

---

## Install

```bash
go install github.com/Rana718/trainer
```

---

## Commands

### Run a trainer

```bash
trainer <path/to/trainer.exe>
```

Shows your installed games (by name, not AppID), you pick one, and it launches the `.exe` through that game's Proton prefix — detached, so your terminal is free immediately.

```
=== Installed Games (compatdata) ===
  [1] Proton Experimental (AppID: 1493710)
  [2] setup.exe (AppID: 2208168421)
  [3] DS2.exe (AppID: 3965675259)

Select game number: 3

Running:  trainer.exe
Game:     DS2.exe (3965675259)
Proton:   /usr/share/steam/compatibilitytools.d/proton-cachyos-slr/proton

Started (PID 12345). To stop: kill 12345
```

To stop it later: `kill <PID>` or just close the trainer window.

---

### Set window scale (DPI)

```bash
trainer size
```

Useful for non-Steam Windows apps that open tiny on HiDPI screens. Sets the `LogPixels` registry key in the game's Wine prefix — no window opens, just writes the config.

```
Scale options:
  [1] Low    (96 DPI  - 100%)
  [2] Medium (120 DPI - 125%)
  [3] High   (144 DPI - 150%)
  [4] XHigh  (192 DPI - 200%)
  [5] Custom (enter DPI manually)
```

Takes effect the next time you launch the app through that prefix.

---

## How it works

- Reads `~/.local/share/Steam/steamapps/compatdata/` to list all prefixes
- Resolves game names from `appmanifest_*.acf` files (Steam games) and `shortcuts.vdf` (non-Steam games added to Steam)
- Reads `~/.local/share/Steam/config/config.vdf` to find which Proton tool Steam assigned to each game
- Searches for the Proton binary in:
  - `~/.local/share/Steam/steamapps/common/` (official Proton builds)
  - `~/.local/share/Steam/compatibilitytools.d/` (GE-Proton, etc.)
  - `/usr/share/steam/compatibilitytools.d/` (distro-installed, e.g. CachyOS Proton)

Works with any Proton variant: Proton Experimental, GE-Proton, CachyOS Proton, or anything else Steam knows about.

---

## Requirements

- Linux
- Steam installed (native or via package manager)
- At least one game with a Proton prefix set up
- Go 1.26+ (only needed to build)
