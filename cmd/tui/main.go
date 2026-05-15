package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var titleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#00BFFF")).
	Padding(1, 2).
	Align(lipgloss.Center)

type model struct{}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	s := titleStyle.Render("Senpay")
	s += "\n\n"
	s += lipgloss.NewStyle().Align(lipgloss.Center).Render("Indonesian E-Wallet System")
	s += "\n\n"
	s += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Press q or Ctrl+C to quit")

	return s
}

func main() {
	p := tea.NewProgram(model{}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
