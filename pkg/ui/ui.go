package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
)

// Gnomatix Branding
const (
	Author  = "(c) 2024-6 Brett Whitty, GNOMATIX"
	License = "Enterprise License: BSL (Business Source License). Contact support@gnomatix.com for details."
)

// Palette defines a set of colors for the UI
type Palette struct {
	Name      string
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color
	Success   lipgloss.Color
	Error     lipgloss.Color
	Muted     lipgloss.Color
}

var (
	// GnomatixTheme is the default high-tech aesthetic
	GnomatixTheme = Palette{
		Name:      "Gnomatix",
		Primary:   lipgloss.Color("#00FFFF"), // Cyan
		Secondary: lipgloss.Color("#FF00FF"), // Magenta
		Accent:    lipgloss.Color("#7D00FF"), // Vortex Purple
		Success:   lipgloss.Color("#00FF00"),
		Error:     lipgloss.Color("#FF0000"),
		Muted:     lipgloss.Color("#3C3C3C"),
	}

	// Authentic Wes Anderson Themes from karthik/wesanderson
	BottleRocket1Theme = Palette{Name: "BottleRocket1", Primary: lipgloss.Color("#A42820"), Secondary: lipgloss.Color("#5F5647"), Accent: lipgloss.Color("#9B110E"), Success: lipgloss.Color("#3F5151"), Error: lipgloss.Color("#4E2A1E"), Muted: lipgloss.Color("#550307")}
	BottleRocket2Theme = Palette{Name: "BottleRocket2", Primary: lipgloss.Color("#FAD510"), Secondary: lipgloss.Color("#CB2314"), Accent: lipgloss.Color("#273046"), Success: lipgloss.Color("#354823"), Error: lipgloss.Color("#1E1E1E"), Muted: lipgloss.Color("#273046")}
	RushmoreTheme      = Palette{Name: "Rushmore1", Primary: lipgloss.Color("#E1BD6D"), Secondary: lipgloss.Color("#EABE94"), Accent: lipgloss.Color("#0B775E"), Success: lipgloss.Color("#35274A"), Error: lipgloss.Color("#F2300F"), Muted: lipgloss.Color("#EABE94")}
	Royal1Theme        = Palette{Name: "Royal1", Primary: lipgloss.Color("#899DA4"), Secondary: lipgloss.Color("#C93312"), Accent: lipgloss.Color("#FAEFD1"), Success: lipgloss.Color("#DC863B"), Error: lipgloss.Color("#C93312"), Muted: lipgloss.Color("#899DA4")}
	Royal2Theme        = Palette{Name: "Royal2", Primary: lipgloss.Color("#9A8822"), Secondary: lipgloss.Color("#F5CDB4"), Accent: lipgloss.Color("#F8AFA8"), Success: lipgloss.Color("#FDDDA0"), Error: lipgloss.Color("#74A089"), Muted: lipgloss.Color("#9A8822")}
	ZissouTheme        = Palette{Name: "Zissou1", Primary: lipgloss.Color("#3B9AB2"), Secondary: lipgloss.Color("#78B7C5"), Accent: lipgloss.Color("#EBCC2A"), Success: lipgloss.Color("#E1AF00"), Error: lipgloss.Color("#F21A00"), Muted: lipgloss.Color("#3B9AB2")}
	Darjeeling1Theme   = Palette{Name: "Darjeeling1", Primary: lipgloss.Color("#FF0000"), Secondary: lipgloss.Color("#00A08A"), Accent: lipgloss.Color("#F2AD00"), Success: lipgloss.Color("#F98400"), Error: lipgloss.Color("#5BBCD6"), Muted: lipgloss.Color("#F2AD00")}
	Darjeeling2Theme   = Palette{Name: "Darjeeling2", Primary: lipgloss.Color("#ECCBAE"), Secondary: lipgloss.Color("#046C9A"), Accent: lipgloss.Color("#D69C4E"), Success: lipgloss.Color("#ABDDDE"), Error: lipgloss.Color("#000000"), Muted: lipgloss.Color("#D69C4E")}
	ChevalierTheme     = Palette{Name: "Chevalier1", Primary: lipgloss.Color("#446455"), Secondary: lipgloss.Color("#FDD262"), Accent: lipgloss.Color("#D3DDDC"), Success: lipgloss.Color("#C7B19C"), Error: lipgloss.Color("#446455"), Muted: lipgloss.Color("#D3DDDC")}
	FantasticFoxTheme  = Palette{Name: "FantasticFox1", Primary: lipgloss.Color("#DD8D29"), Secondary: lipgloss.Color("#E2D200"), Accent: lipgloss.Color("#46ACC8"), Success: lipgloss.Color("#E58601"), Error: lipgloss.Color("#B40F20"), Muted: lipgloss.Color("#DD8D29")}
	Moonrise1Theme     = Palette{Name: "Moonrise1", Primary: lipgloss.Color("#F3DF6C"), Secondary: lipgloss.Color("#CEAB07"), Accent: lipgloss.Color("#D5D5D3"), Success: lipgloss.Color("#24281A"), Error: lipgloss.Color("#24281A"), Muted: lipgloss.Color("#D5D5D3")}
	Moonrise2Theme     = Palette{Name: "Moonrise2", Primary: lipgloss.Color("#798E87"), Secondary: lipgloss.Color("#C27D38"), Accent: lipgloss.Color("#CCC591"), Success: lipgloss.Color("#29211F"), Error: lipgloss.Color("#29211F"), Muted: lipgloss.Color("#CCC591")}
	Moonrise3Theme     = Palette{Name: "Moonrise3", Primary: lipgloss.Color("#85D4E3"), Secondary: lipgloss.Color("#F4B5BD"), Accent: lipgloss.Color("#9C964A"), Success: lipgloss.Color("#CDC08C"), Error: lipgloss.Color("#FAD77B"), Muted: lipgloss.Color("#CDC08C")}
	CavalcantiTheme    = Palette{Name: "Cavalcanti1", Primary: lipgloss.Color("#D8B70A"), Secondary: lipgloss.Color("#02401B"), Accent: lipgloss.Color("#A2A475"), Success: lipgloss.Color("#81A88D"), Error: lipgloss.Color("#972D15"), Muted: lipgloss.Color("#A2A475")}
	GrandBudapest1Theme = Palette{Name: "GrandBudapest1", Primary: lipgloss.Color("#F1BB7B"), Secondary: lipgloss.Color("#FD6467"), Accent: lipgloss.Color("#5B1A18"), Success: lipgloss.Color("#D67236"), Error: lipgloss.Color("#5B1A18"), Muted: lipgloss.Color("#F1BB7B")}
	GrandBudapest2Theme = Palette{Name: "GrandBudapest2", Primary: lipgloss.Color("#E6A0C4"), Secondary: lipgloss.Color("#C6CDF7"), Accent: lipgloss.Color("#D8A499"), Success: lipgloss.Color("#7294D4"), Error: lipgloss.Color("#E6A0C4"), Muted: lipgloss.Color("#D8A499")}
	IsleofDogs1Theme   = Palette{Name: "IsleofDogs1", Primary: lipgloss.Color("#9986A5"), Secondary: lipgloss.Color("#79402E"), Accent: lipgloss.Color("#CCBA72"), Success: lipgloss.Color("#0F0D0E"), Error: lipgloss.Color("#D9D0D3"), Muted: lipgloss.Color("#8D8680")}
	IsleofDogs2Theme   = Palette{Name: "IsleofDogs2", Primary: lipgloss.Color("#EAD3BF"), Secondary: lipgloss.Color("#AA9486"), Accent: lipgloss.Color("#B6854D"), Success: lipgloss.Color("#39312F"), Error: lipgloss.Color("#1C1718"), Muted: lipgloss.Color("#AA9486")}
	FrenchDispatchTheme = Palette{Name: "FrenchDispatch", Primary: lipgloss.Color("#90D4CC"), Secondary: lipgloss.Color("#BD3027"), Accent: lipgloss.Color("#B0AFA2"), Success: lipgloss.Color("#7FC0C6"), Error: lipgloss.Color("#BD3027"), Muted: lipgloss.Color("#B0AFA2")}
	AsteroidCity1Theme  = Palette{Name: "AsteroidCity1", Primary: lipgloss.Color("#0A9F9D"), Secondary: lipgloss.Color("#CEB175"), Accent: lipgloss.Color("#E54E21"), Success: lipgloss.Color("#6C8645"), Error: lipgloss.Color("#C18748"), Muted: lipgloss.Color("#CEB175")}
)

var CurrentTheme = GnomatixTheme

// SetTheme allows changing the current UI theme by name
func SetTheme(name string) {
	switch strings.ToLower(name) {
	case "gnomatix":
		CurrentTheme = GnomatixTheme
	case "bottlerocket1":
		CurrentTheme = BottleRocket1Theme
	case "bottlerocket2":
		CurrentTheme = BottleRocket2Theme
	case "rushmore":
		CurrentTheme = RushmoreTheme
	case "royal1":
		CurrentTheme = Royal1Theme
	case "royal2":
		CurrentTheme = Royal2Theme
	case "zissou":
		CurrentTheme = ZissouTheme
	case "darjeeling1":
		CurrentTheme = Darjeeling1Theme
	case "darjeeling2":
		CurrentTheme = Darjeeling2Theme
	case "chevalier":
		CurrentTheme = ChevalierTheme
	case "fantasticfox":
		CurrentTheme = FantasticFoxTheme
	case "moonrise1":
		CurrentTheme = Moonrise1Theme
	case "moonrise2":
		CurrentTheme = Moonrise2Theme
	case "moonrise3":
		CurrentTheme = Moonrise3Theme
	case "cavalcanti":
		CurrentTheme = CavalcantiTheme
	case "grandbudapest1", "budapest":
		CurrentTheme = GrandBudapest1Theme
	case "grandbudapest2":
		CurrentTheme = GrandBudapest2Theme
	case "isleofdogs1":
		CurrentTheme = IsleofDogs1Theme
	case "isleofdogs2":
		CurrentTheme = IsleofDogs2Theme
	case "frenchdispatch":
		CurrentTheme = FrenchDispatchTheme
	case "asteroidcity1":
		CurrentTheme = AsteroidCity1Theme
	}
}

// Shared Styles
var (
	StyleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)

	StyleKey = lipgloss.NewStyle().Bold(true)

	// Muted text
	StyleMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))

	// Value text
	StyleValue = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
)

// Dynamic Style Helpers

func SuccessStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Success).Bold(true)
}

func ErrorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Error).Bold(true)
}

func WarningStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Accent).Bold(true)
}

func InfoStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Primary)
}

func PrimaryStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Primary).Bold(true)
}

func SpinnerStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Primary).Bold(true)
}

// ThemedProgressBar returns a progress.Model styled with the current theme.
func ThemedProgressBar() progress.Model {
	return progress.New(progress.WithGradient(string(CurrentTheme.Primary), string(CurrentTheme.Secondary)))
}

// RenderHeader renders a themed header for DreamFS
func RenderHeader(version, build string) string {
	title := " DreamFS " + version + " (" + build + ") "
	header := StyleHeader.Background(CurrentTheme.Accent).Render(title)
	author := StyleMuted.Render(Author)
	return fmt.Sprintf("\n%s %s\n\n", header, author)
}

// RenderCard renders content within a themed border
func RenderCard(title string, rows [][]string) string {
	var content strings.Builder
	if title != "" {
		content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(CurrentTheme.Secondary).Render(title) + "\n\n")
	}

	for _, row := range rows {
		if len(row) == 2 {
			k := StyleKey.Foreground(CurrentTheme.Primary).Render(row[0] + ":")
			v := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Render(row[1])
			content.WriteString(fmt.Sprintf("%-20s %s\n", k, v))
		}
	}

	content.WriteString("\n" + StyleMuted.Render(License) + "\n")

	return StyleBox.BorderForeground(CurrentTheme.Accent).Render(content.String()) + "\n"
}

// RenderJSON pretty-prints JSON with syntax highlighting
func RenderJSON(v interface{}) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	str := string(data)
	
	// Basic syntax highlighting for the "WOW" factor
	str = strings.ReplaceAll(str, "{", lipgloss.NewStyle().Foreground(CurrentTheme.Accent).Render("{"))
	str = strings.ReplaceAll(str, "}", lipgloss.NewStyle().Foreground(CurrentTheme.Accent).Render("}"))
	str = strings.ReplaceAll(str, "\":", lipgloss.NewStyle().Foreground(CurrentTheme.Primary).Render("\":"))
	
	return StyleBox.BorderForeground(CurrentTheme.Accent).Render(str) + "\n"
}

// PrintLogo returns the ASCII art logo with vortex gradient
func PrintLogo() string {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Accent).Render(`
    ____                              _______
   / __ \_________  ____ _____ ___  / ____/ ____
  / / / / ___/ __ \/ __ '/ __ '__ \/ /_  / ___/
 / /_/ / /  / /_/ / /_/ / / / / / / __/ (__  )
/_____/_/   \____/\__,_/_/ /_/ /_/_/   /____/
	`)
}
