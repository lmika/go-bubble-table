package table

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/juju/ansiterm/tabwriter"
	"github.com/muesli/reflow/ansi"
)

// Row renderer.
type Row interface {
	// Render the row into the given tabwriter.
	// To render correctly, join each cell by a tab character '\t'.
	// Use `m.Cursor() == index` to determine if the row is selected.
	// Take a look at the `SimpleRow` implementation for an example.
	Render(w io.Writer, model Model, index int)
}

// SimpleRow is a set of cells that can be rendered into a table.
// It supports row highlight if selected.
type SimpleRow []any

// Render a simple row.
func (row SimpleRow) Render(w io.Writer, model Model, index int) {
	cells := make([]string, len(row))
	for i, v := range row {
		cells[i] = fmt.Sprintf("%v", v)
	}
	s := strings.Join(cells, "\t")
	if index == model.Cursor() {
		s = model.Styles.SelectedRow.Render(s)
	}
	fmt.Fprintln(w, s)
}

// Columns model.
type Columns interface {
	// Len return the number of columns within the model.
	Len() int

	// Header returns the header text for the column with the given index.
	Header(index int) string
}

// SimpleColumns is a column model backed by a slice
type SimpleColumns []string

func (sc SimpleColumns) Len() int {
	return len(sc)
}

func (sc SimpleColumns) Header(index int) string {
	return sc[index]
}

// New model.
func New(cols Columns, width, height int) Model {
	vp := viewport.New(width, maxInt(height-1, 0))
	tw := &tabwriter.Writer{}
	return Model{
		KeyMap:    DefaultKeyMap(),
		Styles:    DefaultStyles(),
		cols:      cols,
		header:    joinColumnHeaders(cols, ' '), // simple initial header view without tabwriter.
		viewPort:  vp,
		tabWriter: tw,
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Model of a table component.
type Model struct {
	KeyMap       KeyMap
	Styles       Styles
	cols         Columns
	rows         []Row
	header       string
	viewPort     viewport.Model
	tabWriter    *tabwriter.Writer
	cursor       int
	offset       uint
	contentWidth int
}

// KeyMap holds the key bindings for the table.
type KeyMap struct {
	End      key.Binding
	Home     key.Binding
	PageDown key.Binding
	PageUp   key.Binding
	Down     key.Binding
	Up       key.Binding
	Right    key.Binding
	Left     key.Binding
}

// DefaultKeyMap used by the `New` constructor.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		End: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "bottom"),
		),
		Home: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "top"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdown", "page down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "down"),
		),
		Up: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "up"),
		),
		Right: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("→", "right"),
		),
		Left: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("←", "left"),
		),
	}
}

// Styles holds the styling for the table.
type Styles struct {
	Title       lipgloss.Style
	SelectedRow lipgloss.Style
}

// DefaultStyles used by the `New` constructor.
func DefaultStyles() Styles {
	return Styles{
		Title:       lipgloss.NewStyle().Bold(true),
		SelectedRow: lipgloss.NewStyle().Foreground(lipgloss.Color("170")),
	}
}

// SetSize of the table and makes sure to update the view
// and the selected row does not go out of bounds.
func (m *Model) SetSize(width, height int) {
	m.viewPort.Width = width
	m.viewPort.Height = height - 1

	if m.cursor > m.viewPort.YOffset+m.viewPort.Height-1 {
		m.cursor = m.viewPort.YOffset + m.viewPort.Height - 1
		m.updateView()
	}
}

// Cursor returns the index of the selected row.
func (m Model) Cursor() int {
	return m.cursor
}

// SelectedRow returns the selected row.
// You can cast it to your own implementation.
func (m Model) SelectedRow() Row {
	return m.rows[m.cursor]
}

// SetRows of the table and makes sure to update the view
// and the selected row does not go out of bounds.
func (m *Model) SetRows(rows []Row) {
	m.rows = rows
	m.updateView()
}

func (m *Model) updateView() {
	var b strings.Builder
	m.tabWriter.Init(&b, 0, 4, 1, ' ', 0)

	fmt.Fprintln(m.tabWriter, m.Styles.Title.Render(joinColumnHeaders(m.cols, '\t')))

	// rendering the rows.
	for i, row := range m.rows {
		row.Render(m.tabWriter, *m, i)
	}

	m.tabWriter.Flush()

	content := b.String()
	m.contentWidth = lipgloss.Width(content)

	if m.offset > 0 {
		content = trucateOffset(content, m.offset)
	}

	// split table at first line-break to take header and rows apart.
	parts := strings.SplitN(content, "\n", 2)
	if len(parts) != 0 {
		m.header = parts[0]
		if len(parts) == 2 {
			m.viewPort.SetContent(strings.TrimRightFunc(parts[1], unicode.IsSpace))
		}
	}
}

// CursorIsAtTop of the table.
func (m Model) CursorIsAtTop() bool {
	return m.cursor == 0
}

// CursorIsAtBottom of the table.
func (m Model) CursorIsAtBottom() bool {
	return m.cursor == len(m.rows)-1
}

// CursorIsPastBottom of the table.
func (m Model) CursorIsPastBottom() bool {
	return m.cursor > len(m.rows)-1
}

// UpdateView forces an update of the view.
func (m *Model) UpdateView() {
	m.updateView()
}

// GoUp moves the selection to the previous row.
// It can not go above the first row.
func (m *Model) GoUp() {
	if m.CursorIsAtTop() {
		return
	}

	m.cursor--
	m.updateView()

	if m.cursor < m.viewPort.YOffset {
		m.viewPort.LineUp(1)
	}
}

// GoDown moves the selection to the next row.
// It can not go below the last row.
func (m *Model) GoDown() {
	if m.CursorIsAtBottom() {
		return
	}

	m.cursor++
	m.updateView()

	if m.cursor > m.viewPort.YOffset+m.viewPort.Height-1 {
		m.viewPort.LineDown(1)
	}
}

// GoPageUp moves the selection one page up.
// It can not go above the first row.
func (m *Model) GoPageUp() {
	if m.CursorIsAtTop() {
		return
	}

	m.cursor -= m.viewPort.Height
	if m.cursor < 0 {
		m.cursor = 0
	}

	m.updateView()

	m.viewPort.ViewUp()
}

// GoPageDown moves the selection one page down.
// It can not go below the last row.
func (m *Model) GoPageDown() {
	if m.CursorIsAtBottom() {
		return
	}

	m.cursor += m.viewPort.Height
	if m.CursorIsPastBottom() {
		m.cursor = len(m.rows) - 1
	}

	m.updateView()

	m.viewPort.ViewDown()
}

// GoTop moves the selection to the first row.
func (m *Model) GoTop() {
	if m.CursorIsAtTop() {
		return
	}

	m.cursor = 0
	m.updateView()
	m.viewPort.GotoTop()
}

// GoBottom moves the selection to the last row.
func (m *Model) GoBottom() {
	if m.CursorIsAtBottom() {
		return
	}

	m.cursor = len(m.rows) - 1
	m.updateView()
	m.viewPort.GotoBottom()
}

func (m *Model) GoRight() {
	if uint(m.viewPort.Width)+m.offset >= uint(m.contentWidth) {
		return
	}

	m.offset++
	m.updateView()
}

func (m *Model) GoLeft() {
	if m.offset == 0 {
		return
	}

	m.offset--
	m.updateView()
}

// Update tea.Model implementor.
// It handles the key events.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.KeyMap.Up):
			m.GoUp()
		case key.Matches(msg, m.KeyMap.Down):
			m.GoDown()
		case key.Matches(msg, m.KeyMap.PageUp):
			m.GoPageUp()
		case key.Matches(msg, m.KeyMap.PageDown):
			m.GoPageDown()
		case key.Matches(msg, m.KeyMap.Home):
			m.GoTop()
		case key.Matches(msg, m.KeyMap.End):
			m.GoBottom()
		case key.Matches(msg, m.KeyMap.Right):
			m.GoRight()
		case key.Matches(msg, m.KeyMap.Left):
			m.GoLeft()
		}
	}

	return m, nil
}

// View tea.Model implementors.
// It renders the table inside a viewport.
func (m Model) View() string {
	return lipgloss.NewStyle().MaxWidth(m.viewPort.Width).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			m.header,
			m.viewPort.View(),
		),
	)
}

var reANSISeq = regexp.MustCompile("^[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")

// trucateOffset trucates the beginning of the given block of text.
// It handles more than 1 cell wide charaters
// and preserves ANSI escape sequences.
//
// TODO: find a better way to keep ANSI escape sequences
// than having to use regexp to remove them, trim the line
// and restore them at the end.
func trucateOffset(s string, offset uint) string {
	if offset == 0 {
		return s
	}

	var buf strings.Builder
	lines := strings.Split(s, "\n")
	last := len(lines) - 1
	for i, s := range lines {
		ansiSeq := reANSISeq.FindString(s)
		if ansiSeq != "" {
			s = strings.Replace(s, ansiSeq, "", 1)
		}

		var cutset, spaces, chars uint
		for _, r := range s {
			w := ansi.PrintableRuneWidth(string(r))
			if w < 0 {
				continue
			}

			cutset += uint(w)
			chars++

			if cutset >= offset {
				spaces = cutset - offset
				break
			}
		}

		line := strings.Repeat(" ", int(spaces)) + string([]rune(s)[chars:])
		if ansiSeq != "" {
			line = ansiSeq + line
		}
		buf.WriteString(line)
		if i != last {
			buf.WriteRune('\n')
		}
	}
	return buf.String()
}

func joinColumnHeaders(cols Columns, separator rune) string {
	var colStr strings.Builder
	for i := 0; i < cols.Len(); i++ {
		if i > 0 {
			colStr.WriteRune(separator)
		}
		colStr.WriteString(cols.Header(i))
	}
	return colStr.String()
}
