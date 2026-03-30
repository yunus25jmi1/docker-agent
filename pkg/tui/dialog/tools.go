package dialog

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/docker/docker-agent/pkg/tools"
	"github.com/docker/docker-agent/pkg/tui/components/toolcommon"
	"github.com/docker/docker-agent/pkg/tui/styles"
)

// toolsDialog displays all tools available to the current agent.
type toolsDialog struct {
	readOnlyScrollDialog

	tools []tools.Tool
}

// NewToolsDialog creates a new dialog showing all available tools.
func NewToolsDialog(toolList []tools.Tool) Dialog {
	// Sort tools by category then name.
	sorted := make([]tools.Tool, len(toolList))
	copy(sorted, toolList)
	slices.SortFunc(sorted, func(a, b tools.Tool) int {
		if c := strings.Compare(strings.ToLower(a.Category), strings.ToLower(b.Category)); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.DisplayName()), strings.ToLower(b.DisplayName()))
	})

	d := &toolsDialog{tools: sorted}
	d.readOnlyScrollDialog = newReadOnlyScrollDialog(
		readOnlyScrollDialogSize{widthPercent: 70, minWidth: 50, maxWidth: 80, heightPercent: 80, heightMax: 40},
		d.renderLines,
	)
	return d
}

func (d *toolsDialog) renderLines(contentWidth, _ int) []string {
	title := fmt.Sprintf("Tools (%d)", len(d.tools))
	lines := []string{
		RenderTitle(title, contentWidth, styles.DialogTitleStyle),
		RenderSeparator(contentWidth),
		"",
	}

	if len(d.tools) == 0 {
		lines = append(lines, styles.MutedStyle.Render("No tools available."), "")
		return lines
	}

	var lastCategory string
	for i := range d.tools {
		t := &d.tools[i]
		cat := t.Category
		if cat == "" {
			cat = "Other"
		}
		if cat != lastCategory {
			if lastCategory != "" {
				lines = append(lines, "")
			}
			lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(styles.TextSecondary).Render(cat))
			lastCategory = cat
		}

		name := lipgloss.NewStyle().Foreground(styles.Highlight).Render("  " + t.DisplayName())
		if desc, _, _ := strings.Cut(t.Description, "\n"); desc != "" {
			separator := " • "
			separatorWidth := lipgloss.Width(separator)
			nameWidth := lipgloss.Width(name)
			availableWidth := contentWidth - nameWidth - separatorWidth
			if availableWidth > 0 {
				truncated := toolcommon.TruncateText(desc, availableWidth)
				name += styles.MutedStyle.Render(separator + truncated)
			}
		}
		lines = append(lines, name)
	}
	lines = append(lines, "")

	return lines
}
