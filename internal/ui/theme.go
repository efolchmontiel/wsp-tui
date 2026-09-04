package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// ThemeName is a selectable palette id (persisted in config.toml).
type ThemeName string

const (
	ThemeDark   ThemeName = "dark"
	ThemeLight  ThemeName = "light"
	ThemeOcean  ThemeName = "ocean"
	ThemeForest ThemeName = "forest"
)

var themeOrder = []ThemeName{ThemeDark, ThemeLight, ThemeOcean, ThemeForest}

type theme struct {
	name                                                   ThemeName
	title, statusOK, statusWait, statusErr, muted, help    lipgloss.Style
	sidebarSel, sidebar, header, mine, theirs, accent, box lipgloss.Style
	readTick                                               lipgloss.Style
	callIncoming, callMissed                               lipgloss.Style
}

func themeByName(name string) theme {
	switch ThemeName(name) {
	case ThemeLight:
		return lightTheme()
	case ThemeOcean:
		return oceanTheme()
	case ThemeForest:
		return forestTheme()
	default:
		return darkTheme()
	}
}

func nextTheme(cur ThemeName) ThemeName {
	for i, n := range themeOrder {
		if n == cur {
			return themeOrder[(i+1)%len(themeOrder)]
		}
	}
	return ThemeDark
}

func darkTheme() theme {
	return theme{
		name:       ThemeDark,
		title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")),
		statusOK:   lipgloss.NewStyle().Foreground(lipgloss.Color("82")),
		statusWait: lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		statusErr:  lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		muted:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		help:       lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		sidebarSel: lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("86")).Bold(true),
		sidebar:    lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		header:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("159")),
		mine:       lipgloss.NewStyle().Foreground(lipgloss.Color("121")),
		theirs:     lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		accent:     lipgloss.NewStyle().Foreground(lipgloss.Color("86")),
		box:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 2),
		readTick:     lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		callIncoming: lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("178")).Padding(0, 1),
		callMissed:   lipgloss.NewStyle().Foreground(lipgloss.Color("52")).Background(lipgloss.Color("174")).Padding(0, 1),
	}
}

func lightTheme() theme {
	return theme{
		name:         ThemeLight,
		title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("22")),
		statusOK:     lipgloss.NewStyle().Foreground(lipgloss.Color("28")),
		statusWait:   lipgloss.NewStyle().Foreground(lipgloss.Color("130")),
		statusErr:    lipgloss.NewStyle().Foreground(lipgloss.Color("160")),
		muted:        lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		help:         lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		sidebarSel:   lipgloss.NewStyle().Background(lipgloss.Color("254")).Foreground(lipgloss.Color("22")).Bold(true),
		sidebar:      lipgloss.NewStyle().Foreground(lipgloss.Color("235")),
		header:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("24")),
		mine:         lipgloss.NewStyle().Foreground(lipgloss.Color("22")),
		theirs:       lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
		accent:       lipgloss.NewStyle().Foreground(lipgloss.Color("30")),
		box:          lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("24")).Padding(1, 2),
		readTick:     lipgloss.NewStyle().Foreground(lipgloss.Color("27")),
		callIncoming: lipgloss.NewStyle().Foreground(lipgloss.Color("94")).Background(lipgloss.Color("229")).Padding(0, 1),
		callMissed:   lipgloss.NewStyle().Foreground(lipgloss.Color("52")).Background(lipgloss.Color("217")).Padding(0, 1),
	}
}

func oceanTheme() theme {
	return theme{
		name:         ThemeOcean,
		title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
		statusOK:     lipgloss.NewStyle().Foreground(lipgloss.Color("43")),
		statusWait:   lipgloss.NewStyle().Foreground(lipgloss.Color("179")),
		statusErr:    lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		muted:        lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		help:         lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		sidebarSel:   lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("51")).Bold(true),
		sidebar:      lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		header:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45")),
		mine:         lipgloss.NewStyle().Foreground(lipgloss.Color("80")),
		theirs:       lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		accent:       lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		box:          lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 2),
		readTick:     lipgloss.NewStyle().Foreground(lipgloss.Color("45")),
		callIncoming: lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("178")).Padding(0, 1),
		callMissed:   lipgloss.NewStyle().Foreground(lipgloss.Color("52")).Background(lipgloss.Color("174")).Padding(0, 1),
	}
}

func forestTheme() theme {
	return theme{
		name:         ThemeForest,
		title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("71")),
		statusOK:     lipgloss.NewStyle().Foreground(lipgloss.Color("70")),
		statusWait:   lipgloss.NewStyle().Foreground(lipgloss.Color("178")),
		statusErr:    lipgloss.NewStyle().Foreground(lipgloss.Color("167")),
		muted:        lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		help:         lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		sidebarSel:   lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("150")).Bold(true),
		sidebar:      lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		header:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("108")),
		mine:         lipgloss.NewStyle().Foreground(lipgloss.Color("114")),
		theirs:       lipgloss.NewStyle().Foreground(lipgloss.Color("251")),
		accent:       lipgloss.NewStyle().Foreground(lipgloss.Color("71")),
		box:          lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("71")).Padding(1, 2),
		readTick:     lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		callIncoming: lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("178")).Padding(0, 1),
		callMissed:   lipgloss.NewStyle().Foreground(lipgloss.Color("52")).Background(lipgloss.Color("174")).Padding(0, 1),
	}
}

func defaultTheme() theme { return darkTheme() }
