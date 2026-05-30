package steam

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var Dir = filepath.Join(os.Getenv("HOME"), ".local/share/Steam")

type Game struct {
	AppID string
	Name  string
}

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

func parseShortcuts() map[string]string {
	result := map[string]string{}
	entries, _ := os.ReadDir(filepath.Join(Dir, "userdata"))
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(Dir, "userdata", e.Name(), "config/shortcuts.vdf"))
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

func gameName(appID string) string {
	libs := []string{filepath.Join(Dir, "steamapps")}
	vdf := parseVDF(filepath.Join(Dir, "steamapps/libraryfolders.vdf"))
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

func CompatToolName(appID string) string {
	data, err := os.ReadFile(filepath.Join(Dir, "config/config.vdf"))
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

func FindProton(toolName string) string {
	searchDirs := []string{
		filepath.Join(Dir, "steamapps/common"),
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
			if data, err := os.ReadFile(filepath.Join(dir, "compatibilitytool.vdf")); err == nil {
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
	// fallback: loose match in steamapps/common
	base := filepath.Join(Dir, "steamapps/common")
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

func ListGames() []Game {
	compatdata := filepath.Join(Dir, "steamapps/compatdata")
	entries, err := os.ReadDir(compatdata)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot read compatdata:", err)
		os.Exit(1)
	}
	shortcuts := parseShortcuts()
	var games []Game
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
		games = append(games, Game{AppID: id, Name: name})
	}
	return games
}
