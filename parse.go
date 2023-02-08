package jellybean

import (
	"fmt"
	"log"

	"github.com/alexflint/go-scalar"
	tea "github.com/charmbracelet/bubbletea"
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

	model := initialModel(p)
	program := tea.NewProgram(model)
	if _, err := program.Run(); err != nil {
		log.Fatal(err)
	}

	err = setValues(&model)
	if err != nil {
		log.Fatal(err)
	}
}

func setValues(m *model) error {
	for specIdx, inputIdx := range m.specToinputMap {
		spec := m.parser.cmd.specs[specIdx]
		value := m.inputs[inputIdx].Value()

		err := scalar.ParseValue(m.parser.val(spec.dest), value)
		if err != nil {
			return fmt.Errorf(
				"error processing value for %s: %s. %v",
				spec.field.Name, value, err,
			)
		}
	}

	return nil
}
