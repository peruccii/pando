package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

var terminalFontFallbacks = []string{
	"JetBrains Mono",
	"SF Mono",
	"Menlo",
	"Monaco",
	"Fira Code",
	"Cascadia Code",
	"Consolas",
	"Courier New",
}

// GetAvailableFonts retorna famílias de fontes instaladas no sistema,
// priorizando as fontes mais comuns para terminal.
func GetAvailableFonts() []string {
	families, err := listInstalledFontFamilies()
	if err != nil || len(families) == 0 {
		return append([]string(nil), terminalFontFallbacks...)
	}

	return prioritizeTerminalFamilies(families)
}

func listInstalledFontFamilies() ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		return listFontFamiliesFromSystemProfiler()
	default:
		return nil, fmt.Errorf("font listing not implemented for %s", runtime.GOOS)
	}
}

func listFontFamiliesFromSystemProfiler() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "system_profiler", "SPFontsDataType", "-json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("system_profiler failed: %w", err)
	}

	return parseSystemProfilerFontFamilies(output)
}

func parseSystemProfilerFontFamilies(raw []byte) ([]string, error) {
	type typeface struct {
		Family  string `json:"family"`
		Enabled string `json:"enabled"`
	}

	type fontEntry struct {
		Enabled   string     `json:"enabled"`
		Typefaces []typeface `json:"typefaces"`
	}

	var payload struct {
		Fonts []fontEntry `json:"SPFontsDataType"`
	}

	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid system_profiler payload: %w", err)
	}

	familyByKey := make(map[string]string)
	for _, entry := range payload.Fonts {
		if !isSystemFontEnabled(entry.Enabled) {
			continue
		}

		for _, face := range entry.Typefaces {
			if !isSystemFontEnabled(face.Enabled) {
				continue
			}

			family := strings.TrimSpace(face.Family)
			if !isSupportedTerminalFamily(family) {
				continue
			}

			key := strings.ToLower(family)
			if _, exists := familyByKey[key]; !exists {
				familyByKey[key] = family
			}
		}
	}

	if len(familyByKey) == 0 {
		return nil, fmt.Errorf("no font families found")
	}

	families := make([]string, 0, len(familyByKey))
	for _, family := range familyByKey {
		families = append(families, family)
	}

	sort.Slice(families, func(i, j int) bool {
		return strings.ToLower(families[i]) < strings.ToLower(families[j])
	})

	return families, nil
}

func isSystemFontEnabled(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "" || normalized == "yes" || normalized == "enabled" || normalized == "true"
}

func isSupportedTerminalFamily(family string) bool {
	cleaned := strings.TrimSpace(family)
	if cleaned == "" {
		return false
	}

	// Famílias iniciadas com "." são fontes internas/ocultas do macOS
	// e não funcionam bem no xterm/browser.
	if strings.HasPrefix(cleaned, ".") {
		return false
	}

	return true
}

func prioritizeTerminalFamilies(families []string) []string {
	byKey := make(map[string]string, len(families))
	for _, family := range families {
		trimmed := strings.TrimSpace(family)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := byKey[key]; !exists {
			byKey[key] = trimmed
		}
	}

	ordered := make([]string, 0, len(byKey))
	seen := make(map[string]bool, len(byKey))

	for _, preferred := range terminalFontFallbacks {
		key := strings.ToLower(preferred)
		if actual, ok := byKey[key]; ok {
			ordered = append(ordered, actual)
			seen[key] = true
		}
	}

	remaining := make([]string, 0, len(byKey))
	for key, family := range byKey {
		if seen[key] {
			continue
		}
		remaining = append(remaining, family)
	}

	sort.Slice(remaining, func(i, j int) bool {
		return strings.ToLower(remaining[i]) < strings.ToLower(remaining[j])
	})

	ordered = append(ordered, remaining...)
	return ordered
}
