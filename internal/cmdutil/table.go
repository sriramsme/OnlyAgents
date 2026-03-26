package cmdutil

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

func PrintTable(headers []string, rows [][]string, dimmed []bool) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				if n := lipgloss.Width(cell); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}

	pad := func(s string, w int) string {
		if n := lipgloss.Width(s); n < w {
			return s + strings.Repeat(" ", w-n)
		}
		return s
	}

	headerCells := make([]string, len(headers))
	sepCells := make([]string, len(headers))
	for i, h := range headers {
		headerCells[i] = StyleHeader.Render(pad(h, widths[i]))
		sepCells[i] = strings.Repeat("─", widths[i])
	}
	fmt.Println(strings.Join(headerCells, "  "))
	fmt.Println(StyleDim.Render(strings.Join(sepCells, "  ")))

	for i, row := range rows {
		cells := make([]string, len(headers))
		for j := range headers {
			cell := ""
			if j < len(row) {
				cell = pad(row[j], widths[j])
			}
			if i < len(dimmed) && dimmed[i] {
				cell = StyleDim.Render(cell)
			}
			cells[j] = cell
		}
		fmt.Println(strings.Join(cells, "  "))
	}
}
