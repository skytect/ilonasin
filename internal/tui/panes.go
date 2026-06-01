package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const maxDashboardPanes = 4

const (
	apiPaneSummary = iota
	apiPaneTokens
	apiPaneGuidance
)

const (
	providersPaneInstances = iota
	providersPaneCredentials
	providersPaneOAuth
)

const (
	usagePaneMetrics = iota
	usagePaneSubscriptions
	usagePaneHealth
)

const (
	logsPaneRequests = iota
	logsPaneFallbacks
	logsPanePruning
)

type dashboardPane struct {
	id      int
	title   string
	content func(width int) string
}

type panePlacement struct {
	pane   dashboardPane
	x      int
	y      int
	width  int
	height int
}

func (m Model) activeTabPanes() []dashboardPane {
	return m.tabPanes(m.validActiveTab())
}

func (m Model) tabPanes(tab tuiTab) []dashboardPane {
	switch tab {
	case tabAPI:
		return m.apiPanes()
	case tabProviders:
		return m.providerPanes()
	case tabUsage:
		return m.usagePanes()
	case tabLogs:
		return m.logPanes()
	default:
		return m.apiPanes()
	}
}

func (m Model) renderDashboard() string {
	width := m.viewWidth()
	height := m.viewportHeight()
	panes := m.activeTabPanes()
	if len(panes) == 0 {
		return strings.Repeat("\n", maxInt(0, height-1))
	}
	focus := m.validPaneFocus(m.validActiveTab())
	if width < 92 || len(panes) == 1 {
		return m.renderPaneColumn(width, height, panes, focus)
	}
	gap := 2
	leftWidth := (width - gap) / 2
	rightWidth := width - gap - leftWidth
	leftCount := (len(panes) + 1) / 2
	left := m.renderPaneColumn(leftWidth, height, panes[:leftCount], focus)
	right := ""
	if leftCount < len(panes) {
		right = m.renderPaneColumn(rightWidth, height, panes[leftCount:], focus)
	}
	if right == "" {
		return left
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
}

func (m Model) renderPaneColumn(width, height int, panes []dashboardPane, focus int) string {
	if len(panes) == 0 {
		return ""
	}
	gap := 1
	available := height - gap*(len(panes)-1)
	if available < len(panes)*3 {
		available = len(panes) * 3
	}
	base := available / len(panes)
	extra := available % len(panes)
	rendered := make([]string, 0, len(panes)*2-1)
	for i, pane := range panes {
		paneHeight := base
		if i < extra {
			paneHeight++
		}
		if paneHeight < 3 {
			paneHeight = 3
		}
		rendered = append(rendered, m.renderPane(panePlacement{pane: pane, width: width, height: paneHeight}, pane.id == focus))
		if i < len(panes)-1 {
			rendered = append(rendered, strings.Repeat("\n", gap))
		}
	}
	column := strings.Join(rendered, "")
	columnLines := splitBodyLines(column)
	for len(columnLines) < height {
		columnLines = append(columnLines, "")
	}
	if len(columnLines) > height {
		columnLines = columnLines[:height]
	}
	for i := range columnLines {
		columnLines[i] = clipPlainLine(columnLines[i], width)
	}
	return strings.Join(columnLines, "\n")
}

func (m Model) renderPane(placement panePlacement, focused bool) string {
	width := placement.width
	height := placement.height
	if width < 12 {
		width = 12
	}
	if height < 3 {
		height = 3
	}
	style := paneStyle
	if focused {
		style = focusedPaneStyle
	}
	innerWidth := width - style.GetHorizontalFrameSize()
	innerHeight := height - style.GetVerticalFrameSize()
	if innerWidth < 4 {
		innerWidth = 4
	}
	if innerHeight < 1 {
		innerHeight = 1
	}
	title := placement.pane.title
	if focused {
		title = ">" + title
	}
	title = clipPlainLine(title, innerWidth)
	contentLines := splitBodyLines(m.paneContentForWidth(placement.pane, innerWidth))
	maxOffset := maxInt(0, len(contentLines)-maxInt(1, innerHeight-1))
	offset := m.paneOffset(m.validActiveTab(), placement.pane.id)
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	body := make([]string, 0, innerHeight)
	body = append(body, paneTitleStyle.Render(title))
	visibleContentHeight := innerHeight - 1
	for i := 0; i < visibleContentHeight; i++ {
		line := ""
		index := offset + i
		if index < len(contentLines) {
			line = contentLines[index]
		}
		if i == visibleContentHeight-1 && maxOffset > 0 {
			line = paneScrollMarker(line, offset, maxOffset, innerWidth)
		}
		body = append(body, clipPlainLine(line, innerWidth))
	}
	for len(body) < innerHeight {
		body = append(body, "")
	}
	return style.Width(innerWidth).Height(innerHeight).Render(strings.Join(body, "\n"))
}

func paneScrollMarker(line string, offset, maxOffset, width int) string {
	marker := fmt.Sprintf(" %d/%d", offset, maxOffset)
	lineWidth := ansi.StringWidth(line)
	markerWidth := ansi.StringWidth(marker)
	if markerWidth >= width {
		return ansi.Truncate(marker, width, "")
	}
	if lineWidth+markerWidth > width {
		line = ansi.Truncate(line, width-markerWidth, "")
		lineWidth = ansi.StringWidth(line)
	}
	return line + strings.Repeat(" ", width-lineWidth-markerWidth) + mutedStyle.Render(marker)
}

func (m Model) validPaneFocus(tab tuiTab) int {
	panes := m.tabPanes(tab)
	if len(panes) == 0 {
		return 0
	}
	focus := m.paneFocus[tab]
	for _, pane := range panes {
		if pane.id == focus {
			return focus
		}
	}
	return panes[0].id
}

func (m *Model) focusNextPane() {
	m.focusPaneDelta(1)
}

func (m *Model) focusPreviousPane() {
	m.focusPaneDelta(-1)
}

func (m *Model) focusPaneDelta(delta int) {
	tab := m.validActiveTab()
	panes := m.tabPanes(tab)
	if len(panes) == 0 {
		return
	}
	current := m.validPaneFocus(tab)
	index := 0
	for i, pane := range panes {
		if pane.id == current {
			index = i
			break
		}
	}
	index = (index + delta + len(panes)) % len(panes)
	m.paneFocus[tab] = panes[index].id
}

func (m Model) paneOffset(tab tuiTab, paneID int) int {
	if paneID < 0 || paneID >= maxDashboardPanes {
		return 0
	}
	return m.paneScrollOffsets[tab][paneID]
}

func (m *Model) scrollFocusedPane(delta int) {
	tab := m.validActiveTab()
	paneID := m.validPaneFocus(tab)
	m.setPaneScroll(tab, paneID, m.paneOffset(tab, paneID)+delta)
}

func (m *Model) setFocusedPaneScroll(offset int) {
	tab := m.validActiveTab()
	m.setPaneScroll(tab, m.validPaneFocus(tab), offset)
}

func (m *Model) setPaneScroll(tab tuiTab, paneID, offset int) {
	if paneID < 0 || paneID >= maxDashboardPanes {
		return
	}
	maxOffset := m.paneScrollMax(tab, paneID)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	m.paneScrollOffsets[tab][paneID] = offset
}

func (m Model) focusedPaneScrollMax() int {
	tab := m.validActiveTab()
	return m.paneScrollMax(tab, m.validPaneFocus(tab))
}

func (m Model) paneScrollMax(tab tuiTab, paneID int) int {
	height := m.paneContentHeight(tab, paneID)
	contentLines := splitBodyLines(m.paneContent(tab, paneID))
	return maxInt(0, len(contentLines)-height)
}

func (m Model) paneContentHeight(tab tuiTab, paneID int) int {
	placements := panePlacementsForScroll(m.viewWidth(), m.viewportHeight(), m.tabPanes(tab))
	for _, placement := range placements {
		if placement.pane.id == paneID {
			style := paneStyle
			innerHeight := placement.height - style.GetVerticalFrameSize() - 1
			if innerHeight < 1 {
				return 1
			}
			return innerHeight
		}
	}
	return 1
}

func panePlacementsForScroll(width, height int, panes []dashboardPane) []panePlacement {
	if width < 92 || len(panes) == 1 {
		return paneColumnPlacementsForScroll(width, height, panes)
	}
	gap := 2
	leftWidth := (width - gap) / 2
	rightWidth := width - gap - leftWidth
	leftCount := (len(panes) + 1) / 2
	placements := paneColumnPlacementsForScroll(leftWidth, height, panes[:leftCount])
	if leftCount < len(panes) {
		placements = append(placements, paneColumnPlacementsForScroll(rightWidth, height, panes[leftCount:])...)
	}
	return placements
}

func paneColumnPlacementsForScroll(width, height int, panes []dashboardPane) []panePlacement {
	if len(panes) == 0 {
		return nil
	}
	gap := 1
	available := height - gap*(len(panes)-1)
	if available < len(panes)*3 {
		available = len(panes) * 3
	}
	base := available / len(panes)
	extra := available % len(panes)
	placements := make([]panePlacement, 0, len(panes))
	for i, pane := range panes {
		paneHeight := base
		if i < extra {
			paneHeight++
		}
		if paneHeight < 3 {
			paneHeight = 3
		}
		placements = append(placements, panePlacement{pane: pane, width: width, height: paneHeight})
	}
	return placements
}

func (m Model) paneContent(tab tuiTab, paneID int) string {
	for _, pane := range m.tabPanes(tab) {
		if pane.id == paneID {
			return m.paneContentForWidth(pane, m.paneBodyWidth())
		}
	}
	return ""
}

func (m Model) paneContentForWidth(pane dashboardPane, width int) string {
	if pane.content == nil {
		return ""
	}
	return pane.content(width)
}

func (m *Model) clampPaneState() {
	if m.activeTab < 0 || m.activeTab >= tuiTabCount {
		m.activeTab = tabAPI
	}
	for _, tab := range tuiTabs {
		validFocus := m.validPaneFocus(tab.id)
		m.paneFocus[tab.id] = validFocus
		for paneID := 0; paneID < maxDashboardPanes; paneID++ {
			maxOffset := m.paneScrollMax(tab.id, paneID)
			if m.paneScrollOffsets[tab.id][paneID] > maxOffset {
				m.paneScrollOffsets[tab.id][paneID] = maxOffset
			}
			if m.paneScrollOffsets[tab.id][paneID] < 0 {
				m.paneScrollOffsets[tab.id][paneID] = 0
			}
		}
	}
}
