package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const maxDashboardPanes = 4
const paneColumnGap = 2
const paneRowGap = 1
const minPaneColumnWidth = 52

const (
	apiPaneSummary = iota
	apiPaneTokens
)

const (
	providersPaneInstances = iota
	providersPaneCredentials
	providersPaneOAuth
	providersPaneFallback
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
	y      int
	width  int
	height int
	limit  int
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
	columns := paneLayout(width, height, panes)
	rendered := make([]string, 0, len(columns)*2-1)
	for i, column := range columns {
		rendered = append(rendered, m.renderPaneColumn(column, focus))
		if i < len(columns)-1 {
			rendered = append(rendered, strings.Repeat(" ", paneColumnGap))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (m Model) renderPaneColumn(placements []panePlacement, focus int) string {
	if len(placements) == 0 {
		return ""
	}
	width := placements[0].width
	height := placements[0].limit
	rendered := make([]string, 0, len(placements)*2-1)
	for i, placement := range placements {
		rendered = append(rendered, m.renderPane(placement, placement.pane.id == focus))
		if i < len(placements)-1 {
			rendered = append(rendered, strings.Repeat("\n", paneRowGap))
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
	contentLines := paneWrappedContentLines(m.paneContentForWidth(placement.pane, innerWidth), innerWidth)
	visibleContentHeight := innerHeight - 1
	showScrollMarker := innerHeight >= 2 && len(contentLines) > visibleContentHeight
	if showScrollMarker {
		visibleContentHeight--
	}
	if visibleContentHeight < 0 {
		visibleContentHeight = 0
	}
	maxOffset := maxInt(0, len(contentLines)-maxInt(1, visibleContentHeight))
	offset := m.paneOffset(m.validActiveTab(), placement.pane.id)
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	body := make([]string, 0, innerHeight)
	body = append(body, paneTitleStyle.Render(title))
	for i := 0; i < visibleContentHeight; i++ {
		line := ""
		index := offset + i
		if index < len(contentLines) {
			line = contentLines[index]
		}
		body = append(body, fitPaneBodyLine(line, innerWidth))
	}
	if showScrollMarker {
		body = append(body, paneScrollMarkerLine(offset, maxOffset, innerWidth))
	}
	for len(body) < innerHeight {
		body = append(body, "")
	}
	return style.Width(innerWidth).Height(innerHeight).Render(strings.Join(body, "\n"))
}

func paneScrollMarkerLine(offset, maxOffset, width int) string {
	marker := fmt.Sprintf(" %d/%d", offset, maxOffset)
	markerWidth := ansi.StringWidth(marker)
	if markerWidth >= width {
		return ansi.Truncate(marker, width, "")
	}
	return strings.Repeat(" ", width-markerWidth) + mutedStyle.Render(marker)
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
	placement, ok := m.panePlacement(tab, paneID)
	if !ok {
		return 0
	}
	contentLines := paneWrappedContentLines(m.paneContentForWidth(placement.pane, paneInnerWidth(placement)), paneInnerWidth(placement))
	return paneScrollMaxForLineCount(placement, len(contentLines))
}

func (m Model) paneContentHeight(tab tuiTab, paneID int) int {
	placement, ok := m.panePlacement(tab, paneID)
	if !ok {
		return 1
	}
	return paneVisibleContentHeight(placement)
}

func panePlacementsForScroll(width, height int, panes []dashboardPane) []panePlacement {
	columns := paneLayout(width, height, panes)
	placements := make([]panePlacement, 0, len(panes))
	for _, column := range columns {
		placements = append(placements, column...)
	}
	return placements
}

func paneLayout(width, height int, panes []dashboardPane) [][]panePlacement {
	if len(panes) == 0 {
		return nil
	}
	columnCount := paneColumnCount(width, len(panes))
	columnWidths := paneColumnWidths(width, columnCount)
	columns := make([][]panePlacement, 0, columnCount)
	paneIndex := 0
	for columnIndex := 0; columnIndex < columnCount; columnIndex++ {
		count := panesInColumn(len(panes), columnCount, columnIndex)
		columnPanes := panes[paneIndex : paneIndex+count]
		column := paneColumnPlacements(columnWidths[columnIndex], height, columnPanes)
		columns = append(columns, column)
		paneIndex += count
	}
	return columns
}

func paneColumnCount(width, paneCount int) int {
	if paneCount <= 1 {
		return maxInt(1, paneCount)
	}
	maxColumns := (width + paneColumnGap) / (minPaneColumnWidth + paneColumnGap)
	if maxColumns < 1 {
		maxColumns = 1
	}
	if maxColumns > maxDashboardPanes {
		maxColumns = maxDashboardPanes
	}
	if maxColumns > paneCount {
		maxColumns = paneCount
	}
	return maxColumns
}

func paneColumnWidths(width, columnCount int) []int {
	if columnCount < 1 {
		return nil
	}
	totalGap := paneColumnGap * (columnCount - 1)
	available := width - totalGap
	if available < columnCount {
		available = columnCount
	}
	base := available / columnCount
	extra := available % columnCount
	widths := make([]int, columnCount)
	for i := 0; i < columnCount; i++ {
		widths[i] = base
		if i < extra {
			widths[i]++
		}
	}
	return widths
}

func panesInColumn(paneCount, columnCount, columnIndex int) int {
	if columnCount <= 0 {
		return 0
	}
	base := paneCount / columnCount
	extra := paneCount % columnCount
	count := base
	if columnIndex < extra {
		count++
	}
	return count
}

func paneColumnPlacements(width, height int, panes []dashboardPane) []panePlacement {
	if len(panes) == 0 {
		return nil
	}
	available := height - paneRowGap*(len(panes)-1)
	if available < len(panes)*3 {
		available = len(panes) * 3
	}
	base := available / len(panes)
	extra := available % len(panes)
	placements := make([]panePlacement, 0, len(panes))
	y := 0
	for i, pane := range panes {
		paneHeight := base
		if i < extra {
			paneHeight++
		}
		if paneHeight < 3 {
			paneHeight = 3
		}
		placements = append(placements, panePlacement{pane: pane, y: y, width: width, height: paneHeight, limit: height})
		y += paneHeight + paneRowGap
	}
	return placements
}

func (m Model) panePlacement(tab tuiTab, paneID int) (panePlacement, bool) {
	placements := panePlacementsForScroll(m.viewWidth(), m.viewportHeight(), m.tabPanes(tab))
	for _, placement := range placements {
		if placement.pane.id == paneID {
			return placement, true
		}
	}
	return panePlacement{}, false
}

func paneInnerWidth(placement panePlacement) int {
	innerWidth := placement.width - paneStyle.GetHorizontalFrameSize()
	if innerWidth < 4 {
		return 4
	}
	return innerWidth
}

func paneWrappedContentLines(body string, width int) []string {
	lines := splitBodyLines(body)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrapStyledLine(line, width)...)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func fitPaneBodyLine(line string, width int) string {
	if width <= 0 || ansi.StringWidth(line) <= width {
		return line
	}
	wrapped := wrapStyledLine(line, width)
	if len(wrapped) == 0 {
		return ""
	}
	return wrapped[0]
}

func paneVisibleContentHeight(placement panePlacement) int {
	innerHeight := placement.height - paneStyle.GetVerticalFrameSize() - 1
	if innerHeight < 1 {
		return 1
	}
	return innerHeight
}

func paneScrollableContentHeight(placement panePlacement) int {
	height := paneVisibleContentHeight(placement)
	if height >= 2 {
		return height - 1
	}
	return height
}

func paneScrollMaxForLineCount(placement panePlacement, lineCount int) int {
	height := paneVisibleContentHeight(placement)
	if lineCount <= height {
		return 0
	}
	return maxInt(0, lineCount-paneScrollableContentHeight(placement))
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
