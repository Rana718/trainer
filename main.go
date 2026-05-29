package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

var steamDir = filepath.Join(os.Getenv("HOME"), ".local/share/Steam")

// reads a text VDF and returns flat key->value pairs
func parseVDF(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	re := regexp.MustCompile(`"([^"]+)"\s+"([^"]+)"`)
	result := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := re.FindStringSubmatch(sc.Text())
		if len(m) == 3 {
			result[strings.ToLower(m[1])] = m[2]
		}
	}
	return result
}

// shortcuts.vdf is binary — walk it manually to get appID -> exe name for non-Steam games
func parseShortcuts() map[string]string {
	result := map[string]string{}
	userdata := filepath.Join(steamDir, "userdata")
	entries, _ := os.ReadDir(userdata)
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(userdata, e.Name(), "config/shortcuts.vdf"))
		if err != nil {
			continue
		}
		i := 0
		for i < len(data) {
			pos := bytes.Index(data[i:], []byte("\x02appid\x00"))
			if pos == -1 {
				break
			}
			pos += i
			if pos+11 > len(data) {
				break
			}
			appID := fmt.Sprintf("%d", binary.LittleEndian.Uint32(data[pos+7:pos+11]))
			namePos := bytes.Index(data[pos:], []byte("\x01AppName\x00"))
			if namePos != -1 {
				namePos += pos + 9
				nameEnd := bytes.IndexByte(data[namePos:], 0x00)
				if nameEnd != -1 {
					result[appID] = string(data[namePos : namePos+nameEnd])
				}
			}
			i = pos + 1
		}
	}
	return result
}

// looks up game name from appmanifest ACF across all Steam library folders
func gameName(appID string) string {
	libs := []string{filepath.Join(steamDir, "steamapps")}
	vdf := parseVDF(filepath.Join(steamDir, "steamapps/libraryfolders.vdf"))
	for k, v := range vdf {
		if regexp.MustCompile(`^\d+$`).MatchString(k) {
			libs = append(libs, filepath.Join(v, "steamapps"))
		}
	}
	for _, lib := range libs {
		m := parseVDF(filepath.Join(lib, "appmanifest_"+appID+".acf"))
		if n := m["name"]; n != "" {
			return n
		}
	}
	return ""
}

// reads which Proton tool Steam assigned to a given appID from config.vdf
func compatToolName(appID string) string {
	data, err := os.ReadFile(filepath.Join(steamDir, "config/config.vdf"))
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`(?s)"` + regexp.QuoteMeta(appID) + `"\s*\{[^}]*"name"\s+"([^"]+)"`)
	m := re.FindSubmatch(data)
	if len(m) == 2 {
		return string(m[1])
	}
	return ""
}

// finds the proton binary by matching the tool name against compatibilitytool.vdf
// checks steamapps/common, user compatibilitytools.d, and system-wide /usr/share/steam
func findProton(toolName string) string {
	searchDirs := []string{
		filepath.Join(steamDir, "steamapps/common"),
		filepath.Join(os.Getenv("HOME"), ".local/share/Steam/compatibilitytools.d"),
		"/usr/share/steam/compatibilitytools.d",
	}
	for _, base := range searchDirs {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(base, e.Name())
			cvdf := filepath.Join(dir, "compatibilitytool.vdf")
			if data, err := os.ReadFile(cvdf); err == nil {
				re := regexp.MustCompile(`"([^"]+)"\s*(?://[^\n]*)?\s*\{`)
				for _, m := range re.FindAllSubmatch(data, -1) {
					if strings.EqualFold(string(m[1]), toolName) {
						p := filepath.Join(dir, "proton")
						if _, err := os.Stat(p); err == nil {
							return p
						}
					}
				}
			}
			// fuzzy match by directory name
			normalized := strings.ToLower(strings.ReplaceAll(e.Name(), " ", "_"))
			normalizedTool := strings.ToLower(strings.ReplaceAll(toolName, "-", "_"))
			if strings.Contains(normalized, normalizedTool) || strings.Contains(normalizedTool, strings.ToLower(e.Name())) {
				p := filepath.Join(dir, "proton")
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
	}
	return ""
}

// last resort: match by stripping "proton_" prefix and comparing loosely
// handles cases like proton_experimental -> "Proton - Experimental"
func findProtonFallback(toolName string) string {
	base := filepath.Join(steamDir, "steamapps/common")
	entries, _ := os.ReadDir(base)
	tl := strings.ToLower(strings.ReplaceAll(toolName, "_", " "))
	for _, e := range entries {
		el := strings.ToLower(e.Name())
		el2 := strings.TrimPrefix(strings.TrimPrefix(el, "proton - "), "proton ")
		tl2 := strings.TrimPrefix(tl, "proton ")
		if strings.Contains(el, tl) || strings.Contains(el2, tl2) {
			p := filepath.Join(base, e.Name(), "proton")
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

type game struct {
	appID string
	name  string
}

func listGames() []game {
	compatdata := filepath.Join(steamDir, "steamapps/compatdata")
	entries, err := os.ReadDir(compatdata)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot read compatdata:", err)
		os.Exit(1)
	}
	shortcuts := parseShortcuts()
	var games []game
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if id == "0" {
			continue
		}
		name := gameName(id)
		if name == "" {
			name = shortcuts[id]
		}
		if name == "" {
			name = "AppID: " + id
		}
		games = append(games, game{appID: id, name: name})
	}
	return games
}

func prompt(msg string) string {
	fmt.Print(msg)
	sc := bufio.NewScanner(os.Stdin)
	sc.Scan()
	return strings.TrimSpace(sc.Text())
}

func pickGame() game {
	games := listGames()
	fmt.Println("=== Installed Games (compatdata) ===")
	for i, g := range games {
		fmt.Printf("  [%d] %s (AppID: %s)\n", i+1, g.name, g.appID)
	}
	fmt.Println()
	choice := prompt("Select game number: ")
	idx := 0
	fmt.Sscanf(choice, "%d", &idx)
	if idx < 1 || idx > len(games) {
		fmt.Fprintln(os.Stderr, "Invalid selection")
		os.Exit(1)
	}
	return games[idx-1]
}

func resolveProton(appID string) string {
	toolName := compatToolName(appID)
	if toolName == "" {
		fmt.Fprintln(os.Stderr, "No compat tool found for", appID)
		os.Exit(1)
	}
	p := findProton(toolName)
	if p == "" {
		p = findProtonFallback(toolName)
	}
	if p == "" {
		fmt.Fprintf(os.Stderr, "Proton not found for tool: %s\n", toolName)
		os.Exit(1)
	}
	return p
}

// trainer size — sets Wine DPI (window scale) for a prefix without launching anything
func cmdSize() {
	selected := pickGame()
	protonPath := resolveProton(selected.appID)

	fmt.Println("\nScale options:")
	fmt.Println("  [1] Low    (96 DPI  - 100%)")
	fmt.Println("  [2] Medium (120 DPI - 125%)")
	fmt.Println("  [3] High   (144 DPI - 150%)")
	fmt.Println("  [4] XHigh  (192 DPI - 200%)")
	fmt.Println("  [5] Custom (enter DPI manually)")
	fmt.Println()

	scaleIdx := 0
	fmt.Sscanf(prompt("Select scale: "), "%d", &scaleIdx)

	dpiMap := map[int]int{1: 96, 2: 120, 3: 144, 4: 192}
	dpi, ok := dpiMap[scaleIdx]
	if !ok {
		if scaleIdx != 5 {
			fmt.Fprintln(os.Stderr, "Invalid selection")
			os.Exit(1)
		}
		fmt.Sscanf(prompt("Enter DPI value: "), "%d", &dpi)
		if dpi < 96 || dpi > 480 {
			fmt.Fprintln(os.Stderr, "Invalid DPI (96-480)")
			os.Exit(1)
		}
	}

	wineBin := filepath.Join(filepath.Dir(protonPath), "files/bin/wine")
	pfx := filepath.Join(steamDir, "steamapps/compatdata", selected.appID, "pfx")

	cmd := exec.Command(wineBin, "reg", "add",
		`HKCU\Control Panel\Desktop`,
		"/v", "LogPixels", "/t", "REG_DWORD",
		"/d", fmt.Sprintf("%d", dpi), "/f",
	)
	cmd.Env = append(os.Environ(), "WINEPREFIX="+pfx)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Scale set to %d DPI for %s\n", dpi, selected.name)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "size" {
		cmdSize()
		return
	}

	trainerExe := ""
	if len(os.Args) > 1 {
		trainerExe = os.Args[1]
	}
	if trainerExe == "" {
		trainerExe = prompt("Trainer .exe path: ")
	}
	if _, err := os.Stat(trainerExe); err != nil {
		fmt.Fprintln(os.Stderr, "File not found:", trainerExe)
		os.Exit(1)
	}

	selected := pickGame()
	protonPath := resolveProton(selected.appID)

	fmt.Printf("\nRunning:  %s\n", filepath.Base(trainerExe))
	fmt.Printf("Game:     %s (%s)\n", selected.name, selected.appID)
	fmt.Printf("Proton:   %s\n\n", protonPath)

	cmd := exec.Command(protonPath, "run", trainerExe)
	cmd.Env = append(os.Environ(),
		"STEAM_COMPAT_DATA_PATH="+filepath.Join(steamDir, "steamapps/compatdata", selected.appID),
		"STEAM_COMPAT_CLIENT_INSTALL_PATH="+steamDir,
	)
	// new session so the process keeps running after trainer exits
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to start:", err)
		os.Exit(1)
	}
	fmt.Printf("Started (PID %d). To stop: kill %d\n", cmd.Process.Pid, cmd.Process.Pid)
}
