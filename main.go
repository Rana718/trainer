package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Rana718/trainer/internal/steam"
	"github.com/Rana718/trainer/internal/ui"
)

func pickGame() steam.Game {
	games := steam.ListGames()
	items := make([]string, len(games))
	for i, g := range games {
		items[i] = fmt.Sprintf("%-40s  \033[2m%s\033[0m", g.Name, g.AppID)
	}
	idx := ui.Select("=== Select Game ===", items)
	if idx < 0 {
		os.Exit(0)
	}
	return games[idx]
}

func resolveProton(appID string) string {
	toolName := steam.CompatToolName(appID)
	if toolName == "" {
		fmt.Fprintln(os.Stderr, "No compat tool found for", appID)
		os.Exit(1)
	}
	p := steam.FindProton(toolName)
	if p == "" {
		fmt.Fprintf(os.Stderr, "Proton not found for tool: %s\n", toolName)
		os.Exit(1)
	}
	return p
}

func prompt(msg string) string {
	fmt.Print(msg)
	sc := bufio.NewScanner(os.Stdin)
	sc.Scan()
	return strings.TrimSpace(sc.Text())
}

// pickFile opens a native file dialog and returns the selected path.
// Falls back to terminal prompt if no dialog tool is available.
func pickFile() string {
	for _, tool := range [][]string{
		{"zenity", "--file-selection", "--title=Select Trainer .exe", "--file-filter=Windows Executable | *.exe"},
		{"kdialog", "--getopenfilename", ".", "*.exe"},
	} {
		if _, err := exec.LookPath(tool[0]); err == nil {
			out, err := exec.Command(tool[0], tool[1:]...).Output()
			if err == nil {
				return strings.TrimSpace(string(out))
			}
			return "" // user cancelled
		}
	}
	return prompt("Trainer .exe path: ")
}

func cmdSize() {
	selected := pickGame()
	protonPath := resolveProton(selected.AppID)

	scaleItems := []string{
		"Low    (96 DPI  - 100%)",
		"Medium (120 DPI - 125%)",
		"High   (144 DPI - 150%)",
		"XHigh  (192 DPI - 200%)",
		"Custom (enter DPI manually)",
	}
	idx := ui.Select("=== Select Scale ===", scaleItems)
	if idx < 0 {
		os.Exit(0)
	}

	dpiList := []int{96, 120, 144, 192}
	dpi := 0
	if idx < 4 {
		dpi = dpiList[idx]
	} else {
		fmt.Sscanf(prompt("Enter DPI value: "), "%d", &dpi)
		if dpi < 96 || dpi > 480 {
			fmt.Fprintln(os.Stderr, "Invalid DPI (96-480)")
			os.Exit(1)
		}
	}

	wineBin := filepath.Join(filepath.Dir(protonPath), "files/bin/wine")
	pfx := filepath.Join(steam.Dir, "steamapps/compatdata", selected.AppID, "pfx")

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
	fmt.Printf("Scale set to %d DPI for %s\n", dpi, selected.Name)
}

func cmdIdeaClean() {
	home := os.Getenv("HOME")
	base := filepath.Join(home, ".config/JetBrains")
	entries, err := os.ReadDir(base)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot read JetBrains config:", err)
		os.Exit(1)
	}

	found := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(base, e.Name())

		// Remove idea.key
		if err := os.Remove(filepath.Join(dir, "idea.key")); err == nil {
			fmt.Println("Deleted: idea.key in", e.Name())
			found = true
		}

		// Reset trial entries in other.xml
		otherXML := filepath.Join(dir, "options/other.xml")
		if resetTrialInOtherXML(otherXML) {
			fmt.Println("Reset trial in:", e.Name()+"/options/other.xml")
			found = true
		}

		// Remove eval directory
		evalDir := filepath.Join(dir, "eval")
		if err := os.RemoveAll(evalDir); err == nil {
			if _, serr := os.Stat(evalDir); serr != nil {
				// dir existed and was removed
			}
			found = true
		}
	}

	// Reset device IDs in Java prefs
	prefsFile := filepath.Join(home, ".java/.userPrefs/jetbrains/prefs.xml")
	if err := os.Remove(prefsFile); err == nil {
		fmt.Println("Deleted: ~/.java/.userPrefs/jetbrains/prefs.xml")
		found = true
	}

	if !found {
		fmt.Println("Nothing to clean.")
	} else {
		fmt.Println("\nDone. Trial should be reset on next launch.")
	}
}

func resetTrialInOtherXML(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	content := string(data)
	// Find the JSON-like properties section and parse/modify trial keys
	// The file is XML but contains JSON inside a <property> value
	// We need to find "keyToString" map and reset trial entries

	type OtherXML struct {
		KeyToString     map[string]interface{} `json:"keyToString"`
		KeyToStringList map[string]interface{} `json:"keyToStringList"`
	}

	// Extract JSON content between <component> tags with properties
	startMarker := `<component name="PropertiesComponent">`
	endMarker := `</component>`
	startIdx := strings.Index(content, startMarker)
	if startIdx < 0 {
		return false
	}
	startIdx += len(startMarker)
	endIdx := strings.Index(content[startIdx:], endMarker)
	if endIdx < 0 {
		return false
	}
	jsonStr := strings.TrimSpace(content[startIdx : startIdx+endIdx])

	var props OtherXML
	if err := json.Unmarshal([]byte(jsonStr), &props); err != nil {
		return false
	}

	if props.KeyToString == nil {
		return false
	}

	modified := false
	// Reset expiration to 30 days from now
	newExpiry := fmt.Sprintf("%d", time.Now().AddDate(0, 0, 30).UnixMilli())
	if _, ok := props.KeyToString["trial.state.free.trial.expiration.date"]; ok {
		props.KeyToString["trial.state.free.trial.expiration.date"] = newExpiry
		modified = true
	}
	if _, ok := props.KeyToString["trial.state.last.availability.check"]; ok {
		props.KeyToString["trial.state.last.availability.check"] = fmt.Sprintf("%d", time.Now().UnixMilli())
		modified = true
	}
	if _, ok := props.KeyToString["trial.state.last.state"]; ok {
		props.KeyToString["trial.state.last.state"] = "ACTIVE"
		modified = true
	}

	// Remove shown versions list so banner shows again fresh
	if props.KeyToStringList != nil {
		delete(props.KeyToStringList, "trial.active.editor.tab.shown.versions")
	}

	if !modified {
		return false
	}

	newJSON, err := json.MarshalIndent(props, "  ", "  ")
	if err != nil {
		return false
	}

	newContent := content[:startIdx] + "\n  " + string(newJSON) + "\n  " + content[startIdx+endIdx:]
	return os.WriteFile(path, []byte(newContent), 0644) == nil
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "size" {
		cmdSize()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "idea-clean" {
		cmdIdeaClean()
		return
	}

	trainerExe := ""
	if len(os.Args) > 1 {
		trainerExe = os.Args[1]
	}
	if trainerExe == "" {
		fmt.Println("Select your .exe file...")
		trainerExe = pickFile()
	}
	if _, err := os.Stat(trainerExe); err != nil {
		fmt.Fprintln(os.Stderr, "File not found:", trainerExe)
		os.Exit(1)
	}

	selected := pickGame()
	protonPath := resolveProton(selected.AppID)

	fmt.Printf("\nRunning:  %s\n", filepath.Base(trainerExe))
	fmt.Printf("Game:     %s (%s)\n", selected.Name, selected.AppID)
	fmt.Printf("Proton:   %s\n\n", protonPath)

	cmd := exec.Command(protonPath, "run", trainerExe)
	cmd.Env = append(os.Environ(),
		"STEAM_COMPAT_DATA_PATH="+filepath.Join(steam.Dir, "steamapps/compatdata", selected.AppID),
		"STEAM_COMPAT_CLIENT_INSTALL_PATH="+steam.Dir,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to start:", err)
		os.Exit(1)
	}
	fmt.Printf("Started (PID %d). To stop: kill %d\n", cmd.Process.Pid, cmd.Process.Pid)
}
