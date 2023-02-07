package jellybean

import (
	"fmt"
	"log"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TODO: this is exaclty the same as NewParser from go-arg
// it will be best if we can just reuse Parser struct.
// Only problem is all the fields are unexported
func MustParse(dests ...any) {
	p, err := parse(dests...)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(p.description)

	program := tea.NewProgram(initialModel(p))
	if _, err := program.Run(); err != nil {
		log.Fatal(err)
	}
	log.Println("program closed")
}

type (
	errMsg error
)

const (
	hotPink  = lipgloss.Color("#FF06B7")
	darkGray = lipgloss.Color("#767676")
)

var (
	inputStyle    = lipgloss.NewStyle().Foreground(hotPink)
	continueStyle = lipgloss.NewStyle().Foreground(darkGray)
)

type model struct {
	inputs  []textinput.Model
	labels  []string
	focused int
	err     error
	parser  *Parser
}

func initialModel(p *Parser) model {
	var labels []string
	var inputs []textinput.Model

	for i, spec := range p.cmd.specs {
		input := textinput.New()
		input.SetValue(spec.defaultVal)
		input.Prompt = "> "

		if i == 0 {
			input.Focus()
		}

		inputs = append(inputs, input)
		labels = append(labels, spec.help)
	}

	return model{
		inputs:  inputs,
		labels:  labels,
		focused: 0,
		err:     nil,
		parser:  p,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd = make([]tea.Cmd, len(m.inputs))

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			if m.focused == len(m.inputs)-1 {
				return m, tea.Quit
			}
			m.nextInput()
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyShiftTab, tea.KeyCtrlP:
			m.prevInput()
		case tea.KeyTab, tea.KeyCtrlN:
			m.nextInput()
		}
		for i := range m.inputs {
			m.inputs[i].Blur()
		}
		m.inputs[m.focused].Focus()

	// We handle errors just like any other message
	case errMsg:
		m.err = msg
		return m, nil
	}

	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	out := ""
	for i, input := range m.inputs {
		style := continueStyle
		if m.focused == i {
			style = inputStyle
		}
		out += fmt.Sprintf("%s\n", style.Render(m.labels[i]))
		out += fmt.Sprintf("%s\n", input.View())
	}
	return out
}

// nextInput focuses the next input field
func (m *model) nextInput() {
	m.focused = (m.focused + 1) % len(m.inputs)
}

// prevInput focuses the previous input field
func (m *model) prevInput() {
	m.focused--
	// Wrap around
	if m.focused < 0 {
		m.focused = len(m.inputs) - 1
	}
}
