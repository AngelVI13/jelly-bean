package jellybean

import (
	"encoding"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alexflint/go-scalar"
)

// Versioned is the interface that the destination struct should implement to
// make a version string appear at the top of the help message.
type Versioned interface {
	// Version returns the version string that will be printed on a line by itself
	// at the top of the help message.
	Version() string
}

// Described is the interface that the destination struct should implement to
// make a description string appear at the top of the help message.
type Described interface {
	// Description returns the string that will be printed on a line by itself
	// at the top of the help message.
	Description() string
}

type Parser struct {
	cmd         *command
	roots       []reflect.Value
	version     string
	description string
}

// val returns a reflect.Value corresponding to the current value for the
// given path
func (p *Parser) val(dest path) reflect.Value {
	v := p.roots[dest.root]
	for _, field := range dest.fields {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return reflect.Value{}
			}
			v = v.Elem()
		}

		v = v.FieldByIndex(field.Index)
	}
	return v
}

func parse(dests ...any) (*Parser, error) {
	name := filepath.Base(os.Args[0])

	// construct a parser
	p := Parser{
		cmd: &command{name: name},
	}

	// process each of the destination values
	for i, dest := range dests {
		t := reflect.TypeOf(dest)
		if t.Kind() != reflect.Ptr {
			return nil, fmt.Errorf("%s is not a pointer (did you forget an ampersand?)", t)
		}

		cmd, err := cmdFromStruct(name, path{root: i}, t)
		if err != nil {
			return nil, err
		}
		// make a list of roots
		for _, dest := range dests {
			p.roots = append(p.roots, reflect.ValueOf(dest))
		}

		// add nonzero field values as defaults
		for _, spec := range cmd.specs {
			if v := p.val(spec.dest); v.IsValid() && !isZero(v) {
				if defaultVal, ok := v.Interface().(encoding.TextMarshaler); ok {
					str, err := defaultVal.MarshalText()
					if err != nil {
						return nil, fmt.Errorf("%v: error marshaling default value to string: %v", spec.dest, err)
					}
					spec.defaultVal = string(str)
				} else {
					spec.defaultVal = fmt.Sprintf("%v", v)
				}
			}
		}

		p.cmd.specs = append(p.cmd.specs, cmd.specs...)
		p.cmd.subcommands = append(p.cmd.subcommands, cmd.subcommands...)

		if dest, ok := dest.(Versioned); ok {
			p.version = dest.Version()
		}
		if dest, ok := dest.(Described); ok {
			p.description = dest.Description()
		}
	}

	for _, s := range p.cmd.specs {
		log.Println(s.field.Name, s.help)
	}

	return &p, nil
}

// TODO: this is exaclty the same as NewParser from go-arg
// it will be best if we can just reuse Parser struct.
// Only problem is all the fields are unexported
func MustParse(dests ...any) {
	p, err := parse(dests...)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(p.description)
}

// path represents a sequence of steps to find the output location for an
// argument or subcommand in the final destination struct
type path struct {
	root   int                   // index of the destination struct
	fields []reflect.StructField // sequence of struct fields to traverse
}

// String gets a string representation of the given path
func (p path) String() string {
	s := "args"
	for _, f := range p.fields {
		s += "." + f.Name
	}
	return s
}

// Child gets a new path representing a child of this path.
func (p path) Child(f reflect.StructField) path {
	// copy the entire slice of fields to avoid possible slice overwrite
	subfields := make([]reflect.StructField, len(p.fields)+1)
	copy(subfields, p.fields)
	subfields[len(subfields)-1] = f
	return path{
		root:   p.root,
		fields: subfields,
	}
}

// spec represents a command line option
type spec struct {
	dest        path
	field       reflect.StructField // the struct field from which this option was created
	long        string              // the --long form for this option, or empty if none
	short       string              // the -s short form for this option, or empty if none
	cardinality cardinality         // determines how many tokens will be present (possible values: zero, one, multiple)
	required    bool                // if true, this option must be present on the command line
	positional  bool                // if true, this option will be looked for in the positional flags
	separate    bool                // if true, each slice and map entry will have its own --flag
	help        string              // the help text for this option
	env         string              // the name of the environment variable for this option, or empty for none
	defaultVal  string              // default value for this option
	placeholder string              // name of the data in help
}

// command represents a named subcommand, or the top-level command
type command struct {
	name        string
	help        string
	dest        path
	specs       []*spec
	subcommands []*command
	parent      *command
}

func cmdFromStruct(name string, dest path, t reflect.Type) (*command, error) {
	// commands can only be created from pointers to structs
	if t.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("subcommands must be pointers to structs but %s is a %s",
			dest, t.Kind())
	}

	t = t.Elem()
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("subcommands must be pointers to structs but %s is a pointer to %s",
			dest, t.Kind())
	}

	cmd := command{
		name: name,
		dest: dest,
	}

	var errs []string
	walkFields(t, func(field reflect.StructField, t reflect.Type) bool {
		// check for the ignore switch in the tag
		tag := field.Tag.Get("arg")
		if tag == "-" {
			return false
		}

		// if this is an embedded struct then recurse into its fields, even if
		// it is unexported, because exported fields on unexported embedded
		// structs are still writable
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			return true
		}

		// ignore any other unexported field
		if !isExported(field.Name) {
			return false
		}

		// duplicate the entire path to avoid slice overwrites
		subdest := dest.Child(field)
		spec := spec{
			dest:  subdest,
			field: field,
			long:  strings.ToLower(field.Name),
		}

		help, exists := field.Tag.Lookup("help")
		if exists {
			spec.help = help
		}

		defaultVal, hasDefault := field.Tag.Lookup("default")
		if hasDefault {
			spec.defaultVal = defaultVal
		}

		// Look at the tag
		var isSubcommand bool // tracks whether this field is a subcommand
		for _, key := range strings.Split(tag, ",") {
			if key == "" {
				continue
			}
			key = strings.TrimLeft(key, " ")
			var value string
			if pos := strings.Index(key, ":"); pos != -1 {
				value = key[pos+1:]
				key = key[:pos]
			}

			switch {
			case strings.HasPrefix(key, "---"):
				errs = append(errs, fmt.Sprintf("%s.%s: too many hyphens", t.Name(), field.Name))
			case strings.HasPrefix(key, "--"):
				spec.long = key[2:]
			case strings.HasPrefix(key, "-"):
				if len(key) != 2 {
					errs = append(errs, fmt.Sprintf("%s.%s: short arguments must be one character only",
						t.Name(), field.Name))
					return false
				}
				spec.short = key[1:]
			case key == "required":
				if hasDefault {
					errs = append(errs, fmt.Sprintf("%s.%s: 'required' cannot be used when a default value is specified",
						t.Name(), field.Name))
					return false
				}
				spec.required = true
			case key == "positional":
				spec.positional = true
			case key == "separate":
				spec.separate = true
			case key == "help": // deprecated
				spec.help = value
			case key == "env":
				// Use override name if provided
				if value != "" {
					spec.env = value
				} else {
					spec.env = strings.ToUpper(field.Name)
				}
			case key == "subcommand":
				// decide on a name for the subcommand
				cmdname := value
				if cmdname == "" {
					cmdname = strings.ToLower(field.Name)
				}

				// parse the subcommand recursively
				subcmd, err := cmdFromStruct(cmdname, subdest, field.Type)
				if err != nil {
					errs = append(errs, err.Error())
					return false
				}

				subcmd.parent = &cmd
				subcmd.help = field.Tag.Get("help")

				cmd.subcommands = append(cmd.subcommands, subcmd)
				isSubcommand = true
			default:
				errs = append(errs, fmt.Sprintf("unrecognized tag '%s' on field %s", key, tag))
				return false
			}
		}

		placeholder, hasPlaceholder := field.Tag.Lookup("placeholder")
		if hasPlaceholder {
			spec.placeholder = placeholder
		} else if spec.long != "" {
			spec.placeholder = strings.ToUpper(spec.long)
		} else {
			spec.placeholder = strings.ToUpper(spec.field.Name)
		}

		// Check whether this field is supported. It's good to do this here rather than
		// wait until ParseValue because it means that a program with invalid argument
		// fields will always fail regardless of whether the arguments it received
		// exercised those fields.
		if !isSubcommand {
			cmd.specs = append(cmd.specs, &spec)

			var err error
			spec.cardinality, err = cardinalityOf(field.Type)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s.%s: %s fields are not supported",
					t.Name(), field.Name, field.Type.String()))
				return false
			}
			if spec.cardinality == multiple && hasDefault {
				errs = append(errs, fmt.Sprintf("%s.%s: default values are not supported for slice or map fields",
					t.Name(), field.Name))
				return false
			}
		}

		// if this was an embedded field then we already returned true up above
		return false
	})

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "\n"))
	}

	// check that we don't have both positionals and subcommands
	var hasPositional bool
	for _, spec := range cmd.specs {
		if spec.positional {
			hasPositional = true
		}
	}
	if hasPositional && len(cmd.subcommands) > 0 {
		return nil, fmt.Errorf("%s cannot have both subcommands and positional arguments", dest)
	}

	return &cmd, nil
}

// walkFields calls a function for each field of a struct, recursively expanding struct fields.
func walkFields(t reflect.Type, visit func(field reflect.StructField, owner reflect.Type) bool) {
	walkFieldsImpl(t, visit, nil)
}

func walkFieldsImpl(t reflect.Type, visit func(field reflect.StructField, owner reflect.Type) bool, path []int) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		field.Index = make([]int, len(path)+1)
		copy(field.Index, append(path, i))
		expand := visit(field, t)
		if expand && field.Type.Kind() == reflect.Struct {
			var subpath []int
			if field.Anonymous {
				subpath = append(path, i)
			}
			walkFieldsImpl(field.Type, visit, subpath)
		}
	}
}

// cardinality tracks how many tokens are expected for a given spec
//  - zero is a boolean, which does to expect any value
//  - one is an ordinary option that will be parsed from a single token
//  - multiple is a slice or map that can accept zero or more tokens
type cardinality int

const (
	zero cardinality = iota
	one
	multiple
	unsupported
)

func (k cardinality) String() string {
	switch k {
	case zero:
		return "zero"
	case one:
		return "one"
	case multiple:
		return "multiple"
	case unsupported:
		return "unsupported"
	default:
		return fmt.Sprintf("unknown(%d)", int(k))
	}
}

// cardinalityOf returns true if the type can be parsed from a string
func cardinalityOf(t reflect.Type) (cardinality, error) {
	if scalar.CanParse(t) {
		if isBoolean(t) {
			return zero, nil
		}
		return one, nil
	}

	// look inside pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// look inside slice and map types
	switch t.Kind() {
	case reflect.Slice:
		if !scalar.CanParse(t.Elem()) {
			return unsupported, fmt.Errorf("cannot parse into %v because %v not supported", t, t.Elem())
		}
		return multiple, nil
	case reflect.Map:
		if !scalar.CanParse(t.Key()) {
			return unsupported, fmt.Errorf("cannot parse into %v because key type %v not supported", t, t.Elem())
		}
		if !scalar.CanParse(t.Elem()) {
			return unsupported, fmt.Errorf("cannot parse into %v because value type %v not supported", t, t.Elem())
		}
		return multiple, nil
	default:
		return unsupported, fmt.Errorf("cannot parse into %v", t)
	}
}

var textUnmarshalerType = reflect.TypeOf([]encoding.TextUnmarshaler{}).Elem()

// isBoolean returns true if the type can be parsed from a single string
func isBoolean(t reflect.Type) bool {
	switch {
	case t.Implements(textUnmarshalerType):
		return false
	case t.Kind() == reflect.Bool:
		return true
	case t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Bool:
		return true
	default:
		return false
	}
}

// isExported returns true if the struct field name is exported
func isExported(field string) bool {
	r, _ := utf8.DecodeRuneInString(field) // returns RuneError for empty string or invalid UTF8
	return unicode.IsLetter(r) && unicode.IsUpper(r)
}

// isZero returns true if v contains the zero value for its type
func isZero(v reflect.Value) bool {
	t := v.Type()
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
		return v.IsNil()
	}
	if !t.Comparable() {
		return false
	}
	return v.Interface() == reflect.Zero(t).Interface()
}
