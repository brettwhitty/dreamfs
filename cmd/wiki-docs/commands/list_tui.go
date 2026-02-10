package commands

import (
	"fmt"
	"os"
	"os/exec"

	"sort"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Premium color palette (HSL-tailored)
	colorHeader    = lipgloss.Color("#2596be") // Deep Blue
	colorSelected  = lipgloss.Color("#5733FF") // Vibrant Purple
	colorText      = lipgloss.Color("#FAFAFA") // Off-white
	colorDim       = lipgloss.Color("#666666") // Grey
	colorSuccess   = lipgloss.Color("#46B946") // Safe Green
	colorWarning   = lipgloss.Color("#EBCB8B") // Warm Yellow
	colorError     = lipgloss.Color("#BF616A") // Soft Red
	colorUntracked = lipgloss.Color("#A3BE8C") // Sage Green (New)

	baseStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1)

	headerStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorHeader).
			Bold(true).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Italic(true).
			MarginTop(1)

	paneStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			Width(116)
)

type listModel struct {
	table        table.Model
	allItems     []FileItem
	visibleItems []FileItem
	config       Config
	showSame     bool
	showChanged  bool
	showLocal    bool
	showOrphan   bool
	showLegacy   bool
}

func (m *listModel) ApplyFilterAndSort() {
	var filtered []FileItem
	for _, item := range m.allItems {
		switch item.Status {
		case "Same":
			if m.showSame {
				filtered = append(filtered, item)
			}
		case "Changed":
			if m.showChanged {
				filtered = append(filtered, item)
			}
		case "Untracked":
			if m.showLocal {
				filtered = append(filtered, item)
			}
		case "Orphan":
			if m.showOrphan {
				filtered = append(filtered, item)
			}
		case "Legacy":
			if m.showLegacy {
				filtered = append(filtered, item)
			}
		}
	}

	// Sort: Current > Modified > Local > Orphan > Legacy
	sort.Slice(filtered, func(i, j int) bool {
		priority := map[string]int{
			"Same":      0,
			"Changed":   1,
			"Untracked": 2,
			"Orphan":    3,
			"Legacy":    4,
		}
		pi := priority[filtered[i].Status]
		pj := priority[filtered[j].Status]
		if pi != pj {
			return pi < pj
		}
		return filtered[i].RelPath < filtered[j].RelPath
	})

	m.visibleItems = filtered
	var rows []table.Row
	for _, item := range filtered {
		statusIcon := "❔"
		statusLabel := "Local"
		switch item.Status {
		case "Same":
			statusIcon = "✅"
			statusLabel = "Current"
		case "Changed":
			statusIcon = "📝"
			statusLabel = "Modified"
		case "Untracked":
			statusIcon = "🆕"
			statusLabel = "Local"
		case "Orphan":
			statusIcon = "⚠️"
			statusLabel = "Orphan"
		case "Legacy":
			statusIcon = "💾"
			statusLabel = "Legacy"
		}

		yamlIcon := "✅"
		if !item.HasValidYAML {
			yamlIcon = "❌"
		}

		rows = append(rows, table.Row{
			fmt.Sprintf("%s %s", statusIcon, statusLabel),
			yamlIcon,
			item.Version,
			item.Approved,
			item.RelPath,
		})
	}
	m.table.SetRows(rows)
}

func (m listModel) Init() tea.Cmd { return nil }

func (m *listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "e":
			// Edit selected file
			idx := m.table.Cursor()
			if idx >= 0 && idx < len(m.visibleItems) {
				item := m.visibleItems[idx]
				if item.LocalPath != "" {
					editor := os.Getenv("EDITOR")
					if editor == "" {
						editor = "code" // Fallback
					}
					c := exec.Command(editor, item.LocalPath)
					return m, tea.ExecProcess(c, func(err error) tea.Msg {
						return nil
					})
				}
			}
		case "p":
			// Push selected file
			idx := m.table.Cursor()
			if idx >= 0 && idx < len(m.visibleItems) {
				item := m.visibleItems[idx]
				c := exec.Command(os.Args[0], "push", item.RelPath)
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return nil
				})
			}
		case "u":
			// Pull (Update) selected file
			idx := m.table.Cursor()
			if idx >= 0 && idx < len(m.visibleItems) {
				item := m.visibleItems[idx]
				c := exec.Command(os.Args[0], "pull", item.RelPath)
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return nil
				})
			}
		case "a":
			// Add (Publish) selected file
			idx := m.table.Cursor()
			if idx >= 0 && idx < len(m.visibleItems) {
				item := m.visibleItems[idx]
				c := exec.Command(os.Args[0], "add", item.RelPath)
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return nil
				})
			}
		case "r":
			// Refresh logic...
		case "1":
			m.showSame = !m.showSame
			m.ApplyFilterAndSort()
		case "2":
			m.showChanged = !m.showChanged
			m.ApplyFilterAndSort()
		case "3":
			m.showLocal = !m.showLocal
			m.ApplyFilterAndSort()
		case "4":
			m.showOrphan = !m.showOrphan
			m.ApplyFilterAndSort()
		case "5":
			m.showLegacy = !m.showLegacy
			m.ApplyFilterAndSort()
		}
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m listModel) View() string {
	title := headerStyle.Render(" WIKI-DOCS DASHBOARD ") + "\n\n"

	// Info Pane logic
	idx := m.table.Cursor()
	var info string
	if idx >= 0 && idx < len(m.visibleItems) {
		item := m.visibleItems[idx]
		wikiName := item.WikiPath
		if wikiName == "" {
			wikiName = lipgloss.NewStyle().Foreground(colorDim).Render("(Not Published)")
		}

		info = fmt.Sprintf(
			" %s %s\n %s %s\n %s %s",
			lipgloss.NewStyle().Foreground(colorHeader).Bold(true).Render("WIKI PAGE:"), wikiName,
			lipgloss.NewStyle().Foreground(colorHeader).Bold(true).Render("LOCAL FILE:"), item.RelPath,
			lipgloss.NewStyle().Foreground(colorHeader).Bold(true).Render("APPROVED:"), item.Approved,
		)
	}

	filterInfo := fmt.Sprintf(
		" Filters: [1] Current:%v [2] Modified:%v [3] Local:%v [4] Orphan:%v [5] Legacy:%v ",
		m.showSame, m.showChanged, m.showLocal, m.showOrphan, m.showLegacy,
	)

	help := footerStyle.Render(" ↑/↓: Navigate • e: Edit • p: Push • u: Pull • a: Add • q: Quit ")
	return title +
		baseStyle.Render(m.table.View()) + "\n" +
		paneStyle.Render(info) + "\n" +
		lipgloss.NewStyle().Foreground(colorDim).Render(filterInfo) + "\n" +
		help + "\n"
}

func runListTUI(items []FileItem, cfg Config) error {
	columns := []table.Column{
		{Title: "STATUS", Width: 15},
		{Title: "YAML", Width: 6},
		{Title: "VERSION", Width: 10},
		{Title: "APPROVED", Width: 20},
		{Title: "RELEASE PATH", Width: 60},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(18),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true).
		Foreground(colorHeader)
	s.Selected = s.Selected.
		Foreground(colorText).
		Background(colorSelected).
		Bold(true)
	t.SetStyles(s)

	m := listModel{
		table:       t,
		allItems:    items,
		config:      cfg,
		showSame:    true,
		showChanged: true,
		showLocal:   true,
		showOrphan:  true,
		showLegacy:  true,
	}
	m.ApplyFilterAndSort()

	if _, err := tea.NewProgram(&m).Run(); err != nil {
		return err
	}

	return nil
}
