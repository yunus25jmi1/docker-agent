package completion

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestTabVsEnterBehavior(t *testing.T) {
	t.Run("Enter closes completion popup", func(t *testing.T) {
		c := New().(*manager)
		c.items = []Item{
			{Label: "exit", Description: "Exit", Value: "/exit"},
		}
		c.filterItems("")
		c.visible = true

		// Press Enter
		result, _ := c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		// Verify completion popup is closed
		assert.False(t, result.(*manager).visible, "Enter should close completion popup")
	})

	t.Run("Tab closes completion popup", func(t *testing.T) {
		c := New().(*manager)
		c.items = []Item{
			{Label: "exit", Description: "Exit", Value: "/exit"},
		}
		c.filterItems("")
		c.visible = true

		// Press Tab
		result, _ := c.Update(tea.KeyPressMsg{Code: tea.KeyTab})

		// Verify completion popup is closed
		assert.False(t, result.(*manager).visible, "Tab should close completion popup")
	})

	t.Run("Tab does not trigger Execute function", func(t *testing.T) {
		c := New().(*manager)
		c.items = []Item{
			{
				Label:       "export",
				Description: "Export session",
				Value:       "/export",
				Execute: func() tea.Cmd {
					// This should not be called for Tab
					t.Error("Tab should not trigger Execute function")
					return nil
				},
			},
		}
		c.filterItems("")
		c.visible = true

		// Press Tab (should autocomplete but not execute)
		c.Update(tea.KeyPressMsg{Code: tea.KeyTab})

		// If we reach here without t.Error being called, the test passes
	})

	t.Run("Enter triggers Execute function", func(t *testing.T) {
		c := New().(*manager)
		c.items = []Item{
			{
				Label:       "browse",
				Description: "Browse files",
				Value:       "@",
				Execute: func() tea.Cmd {
					return nil
				},
			},
		}
		c.filterItems("")
		c.visible = true

		// Press Enter (should execute)
		_, cmd := c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		// The Execute function is called when the command is run by the tea runtime
		// For this test, we verify that a command is returned (which will call Execute when run)
		assert.NotNil(t, cmd, "Enter should return a command that will execute the item")
	})

	t.Run("Escape closes popup without executing", func(t *testing.T) {
		c := New().(*manager)
		executed := false
		c.items = []Item{
			{
				Label:       "exit",
				Description: "Exit",
				Value:       "/exit",
				Execute: func() tea.Cmd {
					executed = true
					return nil
				},
			},
		}
		c.filterItems("")
		c.visible = true

		// Press Escape
		c.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		// Verify popup is closed and Execute was NOT called
		assert.False(t, c.visible, "Escape should close completion popup")
		assert.False(t, executed, "Escape should not trigger Execute function")
	})
}
