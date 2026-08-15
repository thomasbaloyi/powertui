package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

type Cell struct {
	Ch    rune
	Style string
	Width int // 1 for normal, 2 for wide (emoji/CJK), 0 for continuation of wide
}

type ScreenBuffer struct {
	Width  int
	Height int
	Cells  [][]Cell
}

func NewScreenBuffer(width, height int) *ScreenBuffer {
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}
	sb := &ScreenBuffer{
		Width:  width,
		Height: height,
	}
	sb.Resize(width, height)
	return sb
}

func (sb *ScreenBuffer) Resize(width, height int) {
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}
	sb.Width = width
	sb.Height = height
	sb.Cells = make([][]Cell, height)
	for y := 0; y < height; y++ {
		sb.Cells[y] = make([]Cell, width)
		for x := 0; x < width; x++ {
			sb.Cells[y][x] = Cell{Ch: ' ', Style: "", Width: 1}
		}
	}
}

func (sb *ScreenBuffer) Clear() {
	for y := 0; y < sb.Height; y++ {
		for x := 0; x < sb.Width; x++ {
			sb.Cells[y][x] = Cell{Ch: ' ', Style: "", Width: 1}
		}
	}
}

func (sb *ScreenBuffer) SetCell(y, x int, r rune, style string) {
	if y < 0 || y >= sb.Height || x < 0 || x >= sb.Width {
		return
	}
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		w = 1
	}
	sb.Cells[y][x] = Cell{Ch: r, Style: style, Width: w}
	if w == 2 && x+1 < sb.Width {
		sb.Cells[y][x+1] = Cell{Ch: 0, Style: style, Width: 0}
	}
}

func (sb *ScreenBuffer) FillRect(y, x, h, w int, r rune, style string) {
	for row := y; row < y+h && row < sb.Height; row++ {
		if row < 0 {
			continue
		}
		for col := x; col < x+w && col < sb.Width; col++ {
			if col < 0 {
				continue
			}
			sb.SetCell(row, col, r, style)
		}
	}
}

func (sb *ScreenBuffer) DrawString(y, x int, text string, style string) int {
	if y < 0 || y >= sb.Height || x >= sb.Width {
		return x
	}
	curX := x
	inEscape := false
	curStyle := style

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\033' {
			inEscape = true
			var esc strings.Builder
			esc.WriteRune(r)
			for i+1 < len(runes) && runes[i+1] != 'm' {
				i++
				esc.WriteRune(runes[i])
			}
			if i+1 < len(runes) && runes[i+1] == 'm' {
				i++
				esc.WriteRune('m')
			}
			inEscape = false
			escStr := esc.String()
			if escStr == "\033[0m" {
				curStyle = style
			} else {
				curStyle = style + escStr
			}
			continue
		}
		if inEscape {
			continue
		}

		if curX < 0 {
			w := runewidth.RuneWidth(r)
			if w <= 0 {
				w = 1
			}
			curX += w
			continue
		}
		if curX >= sb.Width {
			break
		}

		w := runewidth.RuneWidth(r)
		if w <= 0 {
			w = 1
		}
		if curX+w > sb.Width {
			break
		}

		sb.Cells[y][curX] = Cell{Ch: r, Style: curStyle, Width: w}
		if w == 2 && curX+1 < sb.Width {
			sb.Cells[y][curX+1] = Cell{Ch: 0, Style: curStyle, Width: 0}
		}
		curX += w
	}
	return curX
}

func (sb *ScreenBuffer) DrawBox(y, x, h, w int, title string, style string) {
	if y >= sb.Height || x >= sb.Width || h < 2 || w < 2 {
		return
	}

	maxY := y + h - 1
	if maxY >= sb.Height {
		maxY = sb.Height - 1
	}
	maxX := x + w - 1
	if maxX >= sb.Width {
		maxX = sb.Width - 1
	}

	// Corners
	sb.SetCell(y, x, '┌', style)
	sb.SetCell(y, maxX, '┐', style)
	sb.SetCell(maxY, x, '└', style)
	sb.SetCell(maxY, maxX, '┘', style)

	// Horizontal borders
	for col := x + 1; col < maxX; col++ {
		sb.SetCell(y, col, '─', style)
		sb.SetCell(maxY, col, '─', style)
	}

	// Vertical borders
	for row := y + 1; row < maxY; row++ {
		sb.SetCell(row, x, '│', style)
		sb.SetCell(row, maxX, '│', style)
	}

	// Title
	if title != "" && w > 4 {
		titleStr := " " + title + " "
		availW := w - 4
		if runewidth.StringWidth(titleStr) > availW {
			titleStr = " " + runewidth.Truncate(title, availW-2, "…") + " "
		}
		sb.DrawString(y, x+2, titleStr, style)
	}
}

func (sb *ScreenBuffer) DrawDoubleBox(y, x, h, w int, title string, style string) {
	if y >= sb.Height || x >= sb.Width || h < 2 || w < 2 {
		return
	}

	maxY := y + h - 1
	if maxY >= sb.Height {
		maxY = sb.Height - 1
	}
	maxX := x + w - 1
	if maxX >= sb.Width {
		maxX = sb.Width - 1
	}

	// Corners
	sb.SetCell(y, x, '╔', style)
	sb.SetCell(y, maxX, '╗', style)
	sb.SetCell(maxY, x, '╚', style)
	sb.SetCell(maxY, maxX, '╝', style)

	// Horizontal borders
	for col := x + 1; col < maxX; col++ {
		sb.SetCell(y, col, '═', style)
		sb.SetCell(maxY, col, '═', style)
	}

	// Vertical borders
	for row := y + 1; row < maxY; row++ {
		sb.SetCell(row, x, '║', style)
		sb.SetCell(row, maxX, '║', style)
	}

	// Title
	if title != "" && w > 4 {
		titleStr := " " + title + " "
		availW := w - 4
		if runewidth.StringWidth(titleStr) > availW {
			titleStr = " " + runewidth.Truncate(title, availW-2, "…") + " "
		}
		sb.DrawString(y, x+2, titleStr, style)
	}
}

func (sb *ScreenBuffer) Flush() string {
	var out strings.Builder
	out.WriteString("\033[H") // Move to 1,1

	lastStyle := ""
	for y := 0; y < sb.Height; y++ {
		for x := 0; x < sb.Width; x++ {
			cell := sb.Cells[y][x]
			if cell.Width == 0 {
				continue // Skip secondary cell of wide character
			}
			if cell.Style != lastStyle {
				out.WriteString("\033[0m")
				if cell.Style != "" {
					out.WriteString(cell.Style)
				}
				lastStyle = cell.Style
			}
			if cell.Ch == 0 {
				out.WriteRune(' ')
			} else {
				out.WriteRune(cell.Ch)
			}
		}
		// Clear rest of line to avoid artifacting, then CRLF for raw mode!
		out.WriteString("\033[0m\033[K")
		lastStyle = ""
		if y < sb.Height-1 {
			out.WriteString("\r\n")
		}
	}
	return out.String()
}
