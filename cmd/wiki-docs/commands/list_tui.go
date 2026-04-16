package commands

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"sort"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// VERSION and VALIDITY Clarification:
// ----------------------------------
// VERSION (VER):  The specic version of the doc file itself (from its source YAML).
//                 Shows '-' if unversioned or if YAML is stripped (local repo files).
// VALIDITY (VALID): The software versions this doc is approved for.
//                   Highlighted GREEN if matching current 'VERSION' file.

var (
	colorHeader          = lipgloss.Color("#2596be") // Deep Blue
	colorSelected        = lipgloss.Color("#5733FF") // Vibrant Purple
	colorText            = lipgloss.Color("#FAFAFA") // Off-white
	colorDim             = lipgloss.Color("#666666") // Grey
	colorStatusObsoleted = lipgloss.Color("#FF5F00") // Orange-Red
	colorStatusOutdated  = lipgloss.Color("#AF8700") // Dark Gold
	colorSuccess         = lipgloss.Color("#46B946") // Safe Green
	colorWarning         = lipgloss.Color("#EBCB8B") // Warm Yellow
	colorError           = lipgloss.Color("#BF616A") // Soft Red

	baseStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#585858")).
			Padding(0)

	headerStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorHeader).
			Bold(true).
			Padding(0, 1)

	// Column Styles (Fixed visual widths)
	styleColStat  = lipgloss.NewStyle().Width(4).Align(lipgloss.Center)
	styleColYAML  = lipgloss.NewStyle().Width(4).Align(lipgloss.Center)
	styleColVer   = lipgloss.NewStyle().Width(6).Align(lipgloss.Center)
	styleColValid = lipgloss.NewStyle().Width(8).Align(lipgloss.Center)

	styleFilterBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#2E3440")).
			Foreground(colorText).
			Padding(0, 1)

	styleHelpBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#4C566A")).
			Foreground(lipgloss.Color("#A3BE8C")).
			Bold(true).
			Padding(0, 1)

	paneStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0)
)

// listModel represents the state of the interactive Wiki-Docs dashboard.
// It manages the list viewport, the info (metadata) viewport, and handles
// user interactions for filtering, navigation, and triggering deployment actions.
type listModel struct {
	// Viewports and View State
	viewport     viewport.Model
	infoViewport viewport.Model
	activeView   int // 0 = List View, 1 = Info Pane
	infoVisible  bool
	cursor       int
	quitting     bool
	err          error

	// Terminal Dimensions
	terminalWidth  int
	terminalHeight int

	// Data Collections
	allItems     []FileItem
	visibleItems []FileItem
	config       Config

	// Filter Toggles
	showSame      bool
	showChanged   bool
	showLocal     bool
	showObsoleted bool
	showOutdated  bool

	// Contextual Metadata
	repoVersion string
}

func (m *listModel) ApplyFilterAndSort() {
	var filtered []FileItem
	for _, item := range m.allItems {
		switch item.Status {
		case StatusCurrent:
			if m.showSame {
				filtered = append(filtered, item)
			}
		case StatusModified:
			if m.showChanged {
				filtered = append(filtered, item)
			}
		case StatusUntracked:
			if m.showLocal {
				filtered = append(filtered, item)
			}
		case StatusObsoleted:
			if m.showObsoleted {
				filtered = append(filtered, item)
			}
		case StatusOutdated:
			if m.showOutdated {
				filtered = append(filtered, item)
			}
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		priority := map[FileStatus]int{
			StatusModified: 0, StatusUntracked: 1, StatusOutdated: 2, StatusObsoleted: 3, StatusCurrent: 4,
		}
		pi := priority[filtered[i].Status]
		pj := priority[filtered[j].Status]
		if pi != pj {
			return pi < pj
		}
		return filtered[i].RelPath < filtered[j].RelPath
	})
	m.visibleItems = filtered
	if m.cursor >= len(m.visibleItems) {
		m.cursor = len(m.visibleItems) - 1
	}
	if m.cursor < 0 && len(m.visibleItems) > 0 {
		m.cursor = 0
	}
}

func (m *listModel) renderRows() string {
	var lines []string

	// contentWidth should strictly match viewport interior
	// Columns: STAT(4), YAML(4), VER(6), VALID(8) = 22
	fixedW := 22
	contentWidth := m.viewport.Width - fixedW
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Headers
	headerRow := lipgloss.JoinHorizontal(lipgloss.Top,
		styleColStat.Underline(true).Render("STAT"),
		styleColYAML.Underline(true).Render("YAML"),
		styleColVer.Underline(true).Render("VER"),
		styleColValid.Underline(true).Render("VALID"),
		lipgloss.NewStyle().Padding(0, 1).Width(contentWidth).Underline(true).Render("REPO PATH"),
	)
	lines = append(lines, headerRow)

	for i, item := range m.visibleItems {
		statusIcon := "✅"
		if item.IsIgnored {
			statusIcon = "🤫"
		} else {
			switch item.Status {
			case StatusModified:
				statusIcon = "📝"
			case StatusUntracked:
				statusIcon = "🆕"
			case StatusObsoleted: // Renamed from StatusOrphan
				statusIcon = "🗑️"
			case StatusOutdated: // Renamed from StatusLegacy
				statusIcon = "⚠️"
			}
		}

		yamlIcon := "❌"
		if item.HasValidYAML {
			yamlIcon = "✅"
		}

		verStr := strings.TrimSpace(item.Version)
		if verStr == "" || strings.EqualFold(verStr, "None") {
			verStr = "-"
		}

		appStr := strings.TrimSpace(item.Approved)
		if appStr == "" || strings.EqualFold(appStr, "None") {
			appStr = "-"
		}

		isComp := false
		if m.repoVersion != "" {
			if item.Approved == "*" {
				isComp = true
			} else {
				parts := strings.Split(item.Approved, ",")
				for _, p := range parts {
					if strings.Contains(strings.TrimSpace(p), m.repoVersion) {
						isComp = true
						break
					}
				}
			}
		}

		pathStr := item.RelPath
		if len(pathStr) > contentWidth-2 && contentWidth > 5 {
			pathStr = "..." + pathStr[len(pathStr)-contentWidth+5:]
		}

		// Row Styling
		rowStyle := lipgloss.NewStyle()
		if i == m.cursor {
			rowStyle = rowStyle.Background(colorSelected).Foreground(colorText).Bold(true)
		}

		sStat := styleColStat.Inherit(rowStyle)
		sYAML := styleColYAML.Inherit(rowStyle)
		if !item.HasValidYAML && i != m.cursor {
			sYAML = sYAML.Foreground(colorError)
		}

		sVer := styleColVer.Inherit(rowStyle)

		sValid := styleColValid.Inherit(rowStyle)
		if isComp && i != m.cursor {
			sValid = sValid.Foreground(colorSuccess)
		}

		sPath := lipgloss.NewStyle().Padding(0, 1).Width(contentWidth).Inherit(rowStyle)

		rowContent := lipgloss.JoinHorizontal(lipgloss.Top,
			sStat.Render(statusIcon),
			sYAML.Render(yamlIcon),
			sVer.Render(verStr),
			sValid.Render(appStr),
			sPath.Render(pathStr),
		)
		lines = append(lines, rowContent)
	}
	return strings.Join(lines, "\n")
}

func (m listModel) Init() tea.Cmd { return nil }

func (m *listModel) recalculateLayout(w, h int) {
	m.terminalWidth = w
	m.terminalHeight = h

	chromeH := 3 // Header(1) + Filters(1) + Help(1)
	availableH := h - chromeH
	if availableH < 0 {
		availableH = 0
	}

	listH := availableH
	infoH := 0

	if m.infoVisible {
		listH = int(float64(availableH) * 0.5)
		if listH < 6 {
			listH = 6
		}
		infoH = availableH - listH
	}

	// m.viewport.Width is the content width (interior)
	m.viewport.Width = w - 2
	if m.viewport.Width < 1 {
		m.viewport.Width = 1
	}
	m.viewport.Height = listH - 2
	if m.viewport.Height < 1 {
		m.viewport.Height = 1
	}

	m.infoViewport.Width = w - 2
	if m.infoViewport.Width < 1 {
		m.infoViewport.Width = 1
	}
	m.infoViewport.Height = infoH - 2
	if m.infoViewport.Height < 1 {
		m.infoViewport.Height = 1
	}

	m.updateViewports()
}

func (m *listModel) updateViewports() {
	m.viewport.SetContent(m.renderRows())
	m.updateInfoContent()

	rowOffset := 1
	cursorY := m.cursor + rowOffset
	if cursorY < m.viewport.YOffset {
		m.viewport.YOffset = cursorY
	} else if cursorY >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.YOffset = cursorY - m.viewport.Height + 1
	}
}

func (m *listModel) updateInfoContent() {
	var info string
	if m.cursor >= 0 && m.cursor < len(m.visibleItems) {
		item := m.visibleItems[m.cursor]

		if item.InfoLayout != "" {
			// Template-driven layout: Merges Meta, TemplateAttrs, and logic fields for execution.
			tmpl, err := template.New("info").Parse(item.InfoLayout)
			if err == nil {
				// Prepare data map for backward compatibility with custom templates
				data := make(map[string]interface{})

				// 1. Load Template-derived attributes (Shadowed)
				for k, v := range item.TemplateAttrs {
					data[k] = v
				}
				// 2. Overwrite with explicit Local metadata (Staged)
				for k, v := range item.Meta {
					data[k] = v
				}

				// 3. Inject Helper Fields
				data["RelPath"] = item.RelPath
				data["WikiPath"] = item.WikiPath
				data["Status"] = item.Status
				data["Version"] = item.Version
				data["Approved"] = item.Approved

				var buf bytes.Buffer
				if err := tmpl.Execute(&buf, data); err == nil {
					info = buf.String()
				}
			}
		} else {
			// Sophisticated Metadata Dashboard (Wiki Authority View)
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf(" %s %s\n", lipgloss.NewStyle().Foreground(colorHeader).Bold(true).Render("WIKI SOURCE:"), item.WikiPath))
			sb.WriteString(fmt.Sprintf(" %s %s\n", lipgloss.NewStyle().Foreground(colorHeader).Bold(true).Render("LOCAL STAGED:"), item.RelPath))
			sb.WriteString("\n")

			sb.WriteString(fmt.Sprintf(" %-20s %s\n",
				lipgloss.NewStyle().Foreground(colorHeader).Render("DEPLOY STATUS:"),
				item.Status))
			sb.WriteString("\n")

			// Table simulation for metadata (The "Source Status")
			sb.WriteString(lipgloss.NewStyle().Foreground(colorHeader).Bold(true).Underline(true).Render(" WIKI METADATA ") + "\n")

			keys := make([]string, 0, len(item.Meta))
			for k := range item.Meta {
				keys = append(keys, k)
			}

			// Priority Sorting
			sort.Slice(keys, func(i, j int) bool {
				priority := map[string]int{"title": 0, "version": 1, "approved_versions": 2}
				pi, oki := priority[keys[i]]
				pj, okj := priority[keys[j]]
				if !oki {
					pi = 99
				}
				if !okj {
					pj = 99
				}
				if pi != pj {
					return pi < pj
				}
				return keys[i] < keys[j]
			})

			for _, k := range keys {
				val := item.Meta[k]
				valStr := fmt.Sprintf("%v", val)
				labelStyle := lipgloss.NewStyle().Foreground(colorHeader).Width(20)
				valStyle := lipgloss.NewStyle()

				if strings.Contains(valStr, "(inherited)") {
					valStyle = valStyle.Foreground(colorStatusOutdated).Italic(true)
				} else if k == "version" || k == "approved_versions" {
					valStyle = valStyle.Foreground(lipgloss.Color("#00FF00")).Bold(true)
				}

				sb.WriteString(fmt.Sprintf(" %-20s %s\n", labelStyle.Render(strings.ToUpper(k)+":"), valStyle.Render(valStr)))
			}

			info = sb.String()
		}
	}
	m.infoViewport.SetContent(info)
}

// Update handles incoming messages (Window resize, Key presses) and updates the model state.
func (m *listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.recalculateLayout(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "tab":
			if m.infoVisible {
				m.activeView = 1 - m.activeView
			}
			return m, nil
		case "i":
			m.infoVisible = !m.infoVisible
			m.recalculateLayout(m.terminalWidth, m.terminalHeight)
			return m, nil
		case "up", "k":
			if m.activeView == 1 {
				m.infoViewport.ScrollUp(1)
				return m, nil
			}
			if m.cursor > 0 {
				m.cursor--
				m.updateViewports()
			}
		case "down", "j":
			if m.activeView == 1 {
				m.infoViewport.ScrollDown(1)
				return m, nil
			}
			if m.cursor < len(m.visibleItems)-1 {
				m.cursor++
				m.updateViewports()
			}
		case "home":
			if m.activeView == 0 {
				m.cursor = 0
				m.updateViewports()
			}
		case "end":
			if m.activeView == 0 {
				m.cursor = len(m.visibleItems) - 1
				m.updateViewports()
			}
		case "pgup", "pageup":
			if m.activeView == 0 {
				m.cursor -= m.viewport.Height
				if m.cursor < 0 {
					m.cursor = 0
				}
				m.updateViewports()
			} else {
				m.infoViewport.HalfViewUp()
			}
		case "pgdown", "pagedown", "pgdn":
			if m.activeView == 0 {
				m.cursor += m.viewport.Height
				if m.cursor >= len(m.visibleItems) {
					m.cursor = len(m.visibleItems) - 1
				}
				m.updateViewports()
			} else {
				m.infoViewport.HalfViewDown()
			}
		case "1", "2", "3", "4", "5":
			switch msg.String() {
			case "1":
				m.showSame = !m.showSame
			case "2":
				m.showChanged = !m.showChanged
			case "3":
				m.showLocal = !m.showLocal
			case "4":
				m.showObsoleted = !m.showObsoleted
			case "5":
				m.showOutdated = !m.showOutdated
			}
			m.ApplyFilterAndSort()
			m.updateViewports()
		case "e", "p", "u", "a":
			if m.cursor >= 0 && m.cursor < len(m.visibleItems) {
				item := m.visibleItems[m.cursor]
				var c *exec.Cmd
				switch msg.String() {
				case "e":
					editor := os.Getenv("EDITOR")
					if editor == "" {
						editor = "code"
					}
					c = exec.Command(editor, item.LocalPath)
				case "p":
					c = exec.Command(os.Args[0], "push", item.RelPath)
				case "u":
					c = exec.Command(os.Args[0], "pull", item.RelPath)
				case "a":
					c = exec.Command(os.Args[0], "add", item.RelPath)
				}
				if c != nil {
					return m, tea.ExecProcess(c, func(err error) tea.Msg { return nil })
				}
			}
		}
	}
	return m, cmd
}

// View renders the entire TUI based on the current model state.
// It ensures absolute boundary math is applied for terminal stability.
func (m *listModel) View() string {
	if m.terminalWidth == 0 {
		return "Initializing..."
	}

	rv := ""
	if m.repoVersion != "" {
		rv = fmt.Sprintf(" [REPO v%s] ", m.repoVersion)
	}
	header := headerStyle.Width(m.terminalWidth).Render(" WIKI-DOCS DASHBOARD " + rv)

	lStyle := baseStyle.Width(m.terminalWidth)
	if m.activeView == 0 {
		lStyle = lStyle.BorderForeground(colorSelected)
	}
	listView := lStyle.Render(m.viewport.View())

	infoView := ""
	if m.infoVisible {
		iStyle := paneStyle.Width(m.terminalWidth)
		if m.activeView == 1 {
			iStyle = iStyle.BorderForeground(colorSelected)
		}
		infoView = iStyle.Render(m.infoViewport.View())
	}

	filters := styleFilterBar.Width(m.terminalWidth).Render(fmt.Sprintf(
		" Filters: [1] Current:%v [2] Modified:%v [3] Local:%v [4] Obsoleted:%v [5] Outdated:%v ",
		m.showSame, m.showChanged, m.showLocal, m.showObsoleted, m.showOutdated,
	))

	help := styleHelpBar.Width(m.terminalWidth).Render(
		" Tab: Focus • i: Toggle • ↑/↓: Nav • e: Edit • p: Push • u: Pull • a: Add • q: Quit ",
	)

	// Build the segments
	mainPanes := lipgloss.JoinHorizontal(lipgloss.Top, listView, infoView)
	if !m.infoVisible {
		mainPanes = listView
	}

	// Main content stacking (Absolute Layout)
	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		filters,
		mainPanes,
		help,
	)

	// Ensure the entire content is wrapped at terminalWidth
	return lipgloss.NewStyle().Width(m.terminalWidth).Render(content)
}

// runListTUI initializes and starts the Bubble Tea program for the Dashboard.
func runListTUI(items []FileItem, cfg Config) error {
	versionBytes, _ := os.ReadFile("VERSION")
	repoVer := strings.TrimSpace(string(versionBytes))

	m := listModel{
		allItems:      items,
		config:        cfg,
		showSame:      true,
		showChanged:   true,
		showLocal:     true,
		showObsoleted: true,
		showOutdated:  true,
		repoVersion:   repoVer,
		infoVisible:   true,
	}
	m.ApplyFilterAndSort()

	// Initial size for layout math
	m.terminalWidth = 80
	m.terminalHeight = 24
	m.recalculateLayout(80, 24)

	if _, err := tea.NewProgram(&m, tea.WithAltScreen()).Run(); err != nil {
		return err
	}
	return nil
}
