package option

import (
	"fmt"
	"os"
	"strconv"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// Nargs describes how many arguments an option consumes.
type Nargs string

const (
	// NargsOne marks an option that consumes exactly one argument. It is the zero value, so an
	// option that does not say otherwise takes one argument.
	NargsOne Nargs = ""
	// NargsNone marks an option that takes no argument, such as a flag or a counter.
	NargsNone Nargs = "0"
	// NargsOptional marks an option whose argument may be omitted. Given without one, it is set to
	// its constant instead.
	NargsOptional Nargs = "?"
	// NargsAtLeastOne marks an option that may be given repeatedly, accumulating its arguments, and
	// must be given at least once.
	NargsAtLeastOne Nargs = "+"
	// NargsAny is NargsAtLeastOne without the obligation: it accumulates, and may be left out.
	NargsAny Nargs = "*"
)

// IsVariadic reports whether an arity accumulates its arguments rather than taking a fixed number.
func (nargs Nargs) IsVariadic() bool {
	return nargs == NargsAtLeastOne || nargs == NargsAny
}

// Minimum returns how many arguments an arity must be given.
func (nargs Nargs) Minimum() int {
	switch nargs {
	case NargsNone, NargsOptional, NargsAny:
		return 0
	default:
		return 1
	}
}

// Option is a single command-line option, addressable by a short name, a long name, or both.
type Option interface {
	Set(string) error
	GetShortName() string
	GetLongName() string
	GetUsage() string
	GetRequired() bool
	GetNargs() Nargs
	GetMetavar() string
}

// ChoicesProvider is implemented by options that accept only certain values. Every option built by
// this package implements it; a hand-written Option may.
type ChoicesProvider interface {
	GetChoices() []string
}

// DefaultProvider is implemented by options that are set to a value when no argument names them.
// The value is nil when no default was declared, which is what distinguishes having no default
// from defaulting to the empty string.
type DefaultProvider interface {
	GetDefault() *string
}

// ChoiceNormalizer is implemented by options whose values have more than one spelling, so that
// choices may be compared in a single form. An option that does not implement it has its choices
// compared against the argument as written.
type ChoiceNormalizer interface {
	NormalizeChoice(string) (string, error)
}

// ConstProvider is implemented by NargsOptional options, supplying the value to use when the
// option is given without an argument.
type ConstProvider interface {
	GetConst() string
}

// base holds the metadata shared by every option kind.
type base struct {
	ShortName rune
	LongName  string
	Usage     string
	Required  bool
	Nargs     Nargs
	Metavar   string
	// Choices restricts the accepted values. Any value is accepted while it is empty.
	Choices []string
	// Const is the value a NargsOptional option takes when given without an argument.
	Const string
	// Default is the value the option takes when no argument names it. A nil Default is no default
	// at all, which is what lets the empty string be one.
	Default *string
}

// GetShortName returns the option's short name, or the empty string when it has none. A zero
// ShortName must not render as "\x00": that is a name like any other, and every option lacking a
// short name would collide on it.
func (base *base) GetShortName() string {
	if base.ShortName == 0 {
		return ""
	}

	return string(base.ShortName)
}

func (base *base) GetLongName() string {
	return base.LongName
}

func (base *base) GetUsage() string {
	return base.Usage
}

func (base *base) GetRequired() bool {
	return base.Required
}

func (base *base) GetNargs() Nargs {
	return base.Nargs
}

func (base *base) GetMetavar() string {
	return base.Metavar
}

func (base *base) GetChoices() []string {
	return base.Choices
}

func (base *base) SetChoices(choices ...string) {
	base.Choices = choices
}

func (base *base) GetConst() string {
	return base.Const
}

func (base *base) SetConst(value string) {
	base.Const = value
}

func (base *base) GetDefault() *string {
	return base.Default
}

func (base *base) SetDefault(value string) {
	base.Default = &value
}

func (base *base) SetNargs(nargs Nargs) {
	base.Nargs = nargs
}

func (base *base) SetMetavar(metavar string) {
	base.Metavar = metavar
}

// WithChoices restricts an option to the given values and returns it, so that it may be declared
// inside an option list.
func WithChoices[T interface {
	Option
	SetChoices(...string)
}](opt T, choices ...string) T {
	opt.SetChoices(choices...)

	return opt
}

// WithDefault gives an option the value it takes when no argument names it, and returns it.
func WithDefault[T interface {
	Option
	SetDefault(string)
}](opt T, value string) T {
	opt.SetDefault(value)

	return opt
}

// WithMetavar names an option's argument in the help message, and returns it. A positional is
// named this way: its metavar is what the help calls it, having no option name to go by.
func WithMetavar[T interface {
	Option
	SetMetavar(string)
}](opt T, metavar string) T {
	opt.SetMetavar(metavar)

	return opt
}

// WithNargs sets how many arguments an option takes, and returns it. It is chiefly for positionals,
// which may take one, an optional one, or any number.
func WithNargs[T interface {
	Option
	SetNargs(Nargs)
}](opt T, nargs Nargs) T {
	opt.SetNargs(nargs)

	return opt
}

// WithOptionalArgument lets an option's argument be omitted, in which case it is set to
// constValue, and returns it.
func WithOptionalArgument[T interface {
	Option
	SetNargs(Nargs)
	SetConst(string)
}](opt T, constValue string) T {
	opt.SetNargs(NargsOptional)
	opt.SetConst(constValue)

	return opt
}

// IntOption stores the argument it is given as an int, replacing any previous value.
type IntOption struct {
	base
	Value *int
}

func (intOption *IntOption) Set(in string) error {
	if intOption.Value == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("value"))
	}

	value, err := strconv.Atoi(in)
	if err != nil {
		return motmedelErrors.NewWithTrace(fmt.Errorf("strconv atoi: %w", err), in)
	}

	*intOption.Value = value

	return nil
}

func NewIntOption(shortName rune, longName string, usage string, required bool, value *int) *IntOption {
	return &IntOption{
		base: base{
			ShortName: shortName,
			LongName:  longName,
			Usage:     usage,
			Required:  required,
			Nargs:     NargsOne,
			Metavar:   "INT",
		},
		Value: value,
	}
}

// NormalizeChoice renders an int the one way, so that a choice of "7" also admits "007" and "+7".
func (intOption *IntOption) NormalizeChoice(in string) (string, error) {
	value, err := strconv.Atoi(in)
	if err != nil {
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("strconv atoi: %w", err), in)
	}

	return strconv.Itoa(value), nil
}

// IntsOption appends each argument it is given to a slice of ints.
type IntsOption struct {
	base
	Value *[]int
}

func (intsOption *IntsOption) Set(in string) error {
	if intsOption.Value == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("value"))
	}

	value, err := strconv.Atoi(in)
	if err != nil {
		return motmedelErrors.NewWithTrace(fmt.Errorf("strconv atoi: %w", err), in)
	}

	*intsOption.Value = append(*intsOption.Value, value)

	return nil
}

func NewIntsOption(shortName rune, longName string, usage string, required bool, value *[]int) *IntsOption {
	return &IntsOption{
		base: base{
			ShortName: shortName,
			LongName:  longName,
			Usage:     usage,
			Required:  required,
			Nargs:     NargsAtLeastOne,
			Metavar:   "INT",
		},
		Value: value,
	}
}

// NormalizeChoice renders an int the one way, so that a choice of "7" also admits "007" and "+7".
func (intsOption *IntsOption) NormalizeChoice(in string) (string, error) {
	value, err := strconv.Atoi(in)
	if err != nil {
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("strconv atoi: %w", err), in)
	}

	return strconv.Itoa(value), nil
}

// StringOption stores the argument it is given, replacing any previous value.
type StringOption struct {
	base
	Value *string
}

func (stringOption *StringOption) Set(in string) error {
	if stringOption.Value == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("value"))
	}

	*stringOption.Value = in

	return nil
}

func NewStringOption(shortName rune, longName string, usage string, required bool, value *string) *StringOption {
	return &StringOption{
		base: base{
			ShortName: shortName,
			LongName:  longName,
			Usage:     usage,
			Required:  required,
			Nargs:     NargsOne,
			Metavar:   "STRING",
		},
		Value: value,
	}
}

// StringsOption appends each argument it is given to a slice of strings.
type StringsOption struct {
	base
	Value *[]string
}

func (stringsOption *StringsOption) Set(in string) error {
	if stringsOption.Value == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("value"))
	}

	*stringsOption.Value = append(*stringsOption.Value, in)

	return nil
}

func NewStringsOption(shortName rune, longName string, usage string, required bool, value *[]string) *StringsOption {
	return &StringsOption{
		base: base{
			ShortName: shortName,
			LongName:  longName,
			Usage:     usage,
			Required:  required,
			Nargs:     NargsAtLeastOne,
			Metavar:   "STRING",
		},
		Value: value,
	}
}

// BoolOption takes no argument and records that the option was present.
type BoolOption struct {
	base
	Value *bool
}

func (boolOption *BoolOption) Set(_ string) error {
	if boolOption.Value == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("value"))
	}

	*boolOption.Value = true

	return nil
}

func NewBoolOption(shortName rune, longName string, usage string, required bool, value *bool) *BoolOption {
	return &BoolOption{
		base: base{
			ShortName: shortName,
			LongName:  longName,
			Usage:     usage,
			Required:  required,
			Nargs:     NargsNone,
		},
		Value: value,
	}
}

// CountedOption takes no argument and counts how many times the option was given.
type CountedOption struct {
	base
	Count *int
}

func (countedOption *CountedOption) Set(_ string) error {
	if countedOption.Count == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("count"))
	}

	*countedOption.Count++

	return nil
}

func NewCountedOption(shortName rune, longName string, usage string, required bool, count *int) *CountedOption {
	return &CountedOption{
		base: base{
			ShortName: shortName,
			LongName:  longName,
			Usage:     usage,
			Required:  required,
			Nargs:     NargsNone,
		},
		Count: count,
	}
}

// FileOption opens the path it is given with Flag and Mode, as os.OpenFile does.
type FileOption struct {
	base
	File *os.File
	Flag int
	Mode os.FileMode
}

func (fileOption *FileOption) Set(in string) error {
	if fileOption.File == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("file"))
	}

	value, err := os.OpenFile(in, fileOption.Flag, fileOption.Mode)
	if err != nil {
		return motmedelErrors.NewWithTrace(fmt.Errorf("os open file: %w", err), in)
	}

	*fileOption.File = *value

	return nil
}

// NewFileOption returns a FileOption that opens the path for reading. Use NewFileOptionExtra to
// choose the flag and mode.
func NewFileOption(shortName rune, longName string, usage string, required bool, file *os.File) *FileOption {
	return NewFileOptionExtra(shortName, longName, usage, required, os.O_RDONLY, 0, file)
}

func NewFileOptionExtra(
	shortName rune,
	longName string,
	usage string,
	required bool,
	flag int,
	mode os.FileMode,
	file *os.File,
) *FileOption {
	return &FileOption{
		base: base{
			ShortName: shortName,
			LongName:  longName,
			Usage:     usage,
			Required:  required,
			Nargs:     NargsOne,
			Metavar:   "FILE",
		},
		File: file,
		Flag: flag,
		Mode: mode,
	}
}
