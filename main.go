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
)

var steamDir = filepath.Join(os.Getenv("HOME"), ".local/share/Steam")

// Parse a VDF file and return key->value pairs (flat, case-insensitive keys)
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

// Parse binary shortcuts.vdf and return appID -> AppName map for non-Steam games
func parseShortcuts() map[string]string {
	result := map[string]string{}
	userdata := filepath.Join(steamDir, "userdata")
	entries, _ := os.ReadDir(userdata)
	for _, e := range entries {
		path := filepath.Join(userdata, e.Name(), "config/shortcuts.vdf")
		data, err := os.ReadFile(path)
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
			appidRaw := binary.LittleEndian.Uint32(data[pos+7 : pos+11])
			appID := fmt.Sprintf("%d", uint32(appidRaw))
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

// Get game name from appmanifest ACF files across all library folders
func gameName(appID string) string {
	libs := []string{filepath.Join(steamDir, "steamapps")}
	vdf := parseVDF(filepath.Join(steamDir, "steamapps/libraryfolders.vdf"))
	for k, v := range vdf {
		if regexp.MustCompile(`^\d+$`).MatchString(k) {
			libs = append(libs, filepath.Join(v, "steamapps"))
		}
	}
	for _, lib := range libs {
		acf := filepath.Join(lib, "appmanifest_"+appID+".acf")
		m := parseVDF(acf)
		if n := m["name"]; n != "" {
			return n
		}
	}
	return ""
}

// Read per-game compat tool name from config.vdf CompatToolMapping
func compatToolName(appID string) string {
	data, err := os.ReadFile(filepath.Join(steamDir, "config/config.vdf"))
	if err != nil {
		return ""
	}
	// Find the appID block inside CompatToolMapping and grab "name"
	re := regexp.MustCompile(`(?s)"` + regexp.QuoteMeta(appID) + `"\s*\{[^}]*"name"\s+"([^"]+)"`)
	m := re.FindSubmatch(data)
	if len(m) == 2 {
		return string(m[1])
	}
	return ""
}

// Resolve compat tool name -> proton executable path
// Searches: steamapps/common, compatibilitytools.d (user + system)
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

			// Match by internal tool name in compatibilitytool.vdf
			cvdf := filepath.Join(dir, "compatibilitytool.vdf")
			if data, err := os.ReadFile(cvdf); err == nil {
				// internal key is the quoted name before the block
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

			// Also match by directory name (e.g. "Proton - Experimental" for proton_experimental)
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

// Special case: map Steam internal names like "proton_experimental" to dir names
func findProtonFallback(toolName string) string {
	// proton_experimental -> "Proton - Experimental"
	// proton_9 -> "Proton 9.0"  etc.
	base := filepath.Join(steamDir, "steamapps/common")
	entries, _ := os.ReadDir(base)
	tl := strings.ToLower(strings.ReplaceAll(toolName, "_", " "))
	for _, e := range entries {
		el := strings.ToLower(e.Name())
		// strip "proton - " or "proton " prefix for comparison
		el2 := strings.TrimPrefix(el, "proton - ")
		el2 = strings.TrimPrefix(el2, "proton ")
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

func main() {
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
	selected := games[idx-1]

	toolName := compatToolName(selected.appID)
	if toolName == "" {
		fmt.Fprintln(os.Stderr, "No compat tool found for", selected.appID)
		os.Exit(1)
	}

	protonPath := findProton(toolName)
	if protonPath == "" {
		protonPath = findProtonFallback(toolName)
	}
	if protonPath == "" {
		fmt.Fprintf(os.Stderr, "Proton not found for tool: %s\n", toolName)
		os.Exit(1)
	}

	fmt.Printf("\nRunning:  %s\n", filepath.Base(trainerExe))
	fmt.Printf("Game:     %s (%s)\n", selected.name, selected.appID)
	fmt.Printf("Proton:   %s\n", protonPath)
	fmt.Println()

	cmd := exec.Command(protonPath, "run", trainerExe)
	cmd.Env = append(os.Environ(),
		"STEAM_COMPAT_DATA_PATH="+filepath.Join(steamDir, "steamapps/compatdata", selected.appID),
		"STEAM_COMPAT_CLIENT_INSTALL_PATH="+steamDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to start:", err)
		os.Exit(1)
	}

	fmt.Printf("Started (PID %d). Press Enter to stop, or Ctrl+C to detach.\n", cmd.Process.Pid)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	input := make(chan struct{}, 1)
	go func() {
		bufio.NewReader(os.Stdin).ReadString('\n')
		input <- struct{}{}
	}()

	select {
	case err := <-done:
		if err != nil {
			fmt.Fprintln(os.Stderr, "Process exited with error:", err)
		} else {
			fmt.Println("Process finished.")
		}
	case <-input:
		fmt.Println("Stopping...")
		cmd.Process.Kill()
	}
}
