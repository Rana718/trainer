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
trainer
```

No path needed — a native file picker opens so you can browse and select your `.exe`:

![File picker](pic/Screenshot%20From%202026-05-30%2010-52-41.png)

Then pick the game using arrow keys:

```
=== Select Game ===

  ❯ Dark Souls II                            3965675259
    Proton Experimental                      1493710
    setup.exe                                2208168421

  ↑/↓ navigate  enter select  q quit
```

Or pass the path directly to skip the file picker:

```bash
trainer path/to/trainer.exe
```

After selecting, it launches detached so your terminal is free:

```
Running:  trainer.exe
Game:     Dark Souls II (3965675259)
Proton:   /usr/share/steam/compatibilitytools.d/proton-cachyos-slr/proton

Started (PID 12345). To stop: kill 12345
```

---

### Set window scale (DPI)

```bash
trainer size
```

Useful for non-Steam Windows apps that open tiny on HiDPI screens. Sets the `LogPixels` registry key in the game's Wine prefix.

Pick scale with arrow keys:

```
=== Select Scale ===

  ❯ Low    (96 DPI  - 100%)
    Medium (120 DPI - 125%)
    High   (144 DPI - 150%)
    XHigh  (192 DPI - 200%)
    Custom (enter DPI manually)
```

Takes effect the next time you launch the app through that prefix.

---

## How it works

- Reads `~/.local/share/Steam/steamapps/compatdata/` to list all prefixes
- Resolves game names from `appmanifest_*.acf` files (Steam games) and `shortcuts.vdf` (non-Steam games)
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
- `zenity` or `kdialog` for the file picker (falls back to terminal prompt if neither is installed)
- Go 1.26+ (only needed to build)
