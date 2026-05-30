package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	title  string
	items  []string
	cursor int
	chosen int
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = m.cursor
			return m, tea.Quit
		case "ctrl+c", "q":
			m.chosen = -1
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	s := m.title + "\n\n"
	for i, item := range m.items {
		if i == m.cursor {
			s += fmt.Sprintf("  \033[1;36m❯ %s\033[0m\n", item)
		} else {
			s += fmt.Sprintf("    %s\n", item)
		}
	}
	s += "\n  ↑/↓ navigate  enter select  q quit\n"
	return s
}

// Select shows an interactive arrow-key menu and returns the chosen index, or -1 if cancelled.
func Select(title string, items []string) int {
	m, err := tea.NewProgram(model{title: title, items: items, chosen: -1}).Run()
	if err != nil {
		return -1
	}
	return m.(model).chosen
}
