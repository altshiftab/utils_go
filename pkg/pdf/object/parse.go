package object

import (
	"bytes"
	"fmt"
	"strconv"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// maxDepth bounds nesting, so a hostile file cannot exhaust the stack.
const maxDepth = 256

func isWhitespace(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

func isDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func isRegular(c byte) bool {
	return !isWhitespace(c) && !isDelimiter(c)
}

type parser struct {
	data     []byte
	position int
}

func (p *parser) syntaxError(format string, arguments ...any) error {
	return altshiftErrors.NewWithTrace(fmt.Errorf("%w: at offset %d: %s", ErrSyntax, p.position, fmt.Sprintf(format, arguments...)))
}

// skipWhitespace skips whitespace and comments.
func (p *parser) skipWhitespace() {
	for p.position < len(p.data) {
		c := p.data[p.position]
		switch {
		case isWhitespace(c):
			p.position++
		case c == '%':
			for p.position < len(p.data) && p.data[p.position] != '\n' && p.data[p.position] != '\r' {
				p.position++
			}
		default:
			return
		}
	}
}

func (p *parser) regularRun() []byte {
	start := p.position
	for p.position < len(p.data) && isRegular(p.data[p.position]) {
		p.position++
	}
	return p.data[start:p.position]
}

// keyword reads the next run of regular characters without consuming it unless it is want.
func (p *parser) acceptKeyword(want string) bool {
	p.skipWhitespace()
	start := p.position
	if string(p.regularRun()) == want {
		return true
	}
	p.position = start
	return false
}

func (p *parser) parseObject(depth int) (Object, error) {
	if depth > maxDepth {
		return nil, p.syntaxError("nesting deeper than %d", maxDepth)
	}
	p.skipWhitespace()
	if p.position >= len(p.data) {
		return nil, p.syntaxError("unexpected end of data")
	}

	switch c := p.data[p.position]; {
	case c == '/':
		return p.parseName()
	case c == '(':
		return p.parseLiteralString()
	case c == '<':
		if p.position+1 < len(p.data) && p.data[p.position+1] == '<' {
			return p.parseDictionary(depth)
		}
		return p.parseHexString()
	case c == '[':
		return p.parseArray(depth)
	case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return p.parseNumberOrReference()
	case isRegular(c):
		start := p.position
		switch keyword := string(p.regularRun()); keyword {
		case "true":
			return Boolean(true), nil
		case "false":
			return Boolean(false), nil
		case "null":
			return nil, nil
		default:
			p.position = start
			return nil, p.syntaxError("unexpected keyword %q", keyword)
		}
	default:
		return nil, p.syntaxError("unexpected %q", c)
	}
}

func hexValue(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

func (p *parser) parseName() (Object, error) {
	p.position++
	raw := p.regularRun()
	var name []byte
	for i := 0; i < len(raw); i++ {
		if raw[i] != '#' {
			name = append(name, raw[i])
			continue
		}
		if i+2 >= len(raw) {
			return nil, p.syntaxError("truncated #-escape in name")
		}
		high, okHigh := hexValue(raw[i+1])
		low, okLow := hexValue(raw[i+2])
		if !okHigh || !okLow {
			return nil, p.syntaxError("bad #-escape in name")
		}
		name = append(name, high<<4|low)
		i += 2
	}
	return Name(name), nil
}

func (p *parser) parseLiteralString() (Object, error) {
	p.position++
	var value []byte
	for depth := 1; ; {
		if p.position >= len(p.data) {
			return nil, p.syntaxError("unterminated string")
		}
		c := p.data[p.position]
		p.position++
		switch c {
		case '(':
			depth++
			value = append(value, c)
		case ')':
			depth--
			if depth == 0 {
				return &String{Value: value}, nil
			}
			value = append(value, c)
		case '\r':
			// An unescaped end-of-line in a string reads as a line feed, whichever one it is.
			if p.position < len(p.data) && p.data[p.position] == '\n' {
				p.position++
			}
			value = append(value, '\n')
		case '\\':
			if p.position >= len(p.data) {
				return nil, p.syntaxError("unterminated string escape")
			}
			escaped := p.data[p.position]
			p.position++
			switch escaped {
			case 'n':
				value = append(value, '\n')
			case 'r':
				value = append(value, '\r')
			case 't':
				value = append(value, '\t')
			case 'b':
				value = append(value, '\b')
			case 'f':
				value = append(value, '\f')
			case '\r':
				if p.position < len(p.data) && p.data[p.position] == '\n' {
					p.position++
				}
			case '\n':
			case '0', '1', '2', '3', '4', '5', '6', '7':
				octal := int(escaped - '0')
				for range 2 {
					if p.position < len(p.data) && p.data[p.position] >= '0' && p.data[p.position] <= '7' {
						octal = octal*8 + int(p.data[p.position]-'0')
						p.position++
					}
				}
				value = append(value, byte(octal))
			default:
				// An unknown escape is the character itself.
				value = append(value, escaped)
			}
		default:
			value = append(value, c)
		}
	}
}

func (p *parser) parseHexString() (Object, error) {
	p.position++
	var value []byte
	var high byte
	half := false
	for {
		if p.position >= len(p.data) {
			return nil, p.syntaxError("unterminated hex string")
		}
		c := p.data[p.position]
		p.position++
		if c == '>' {
			break
		}
		if isWhitespace(c) {
			continue
		}
		nibble, ok := hexValue(c)
		if !ok {
			return nil, p.syntaxError("bad digit %q in hex string", c)
		}
		if half {
			value = append(value, high<<4|nibble)
		} else {
			high = nibble
		}
		half = !half
	}
	// An odd final digit is followed by an implied zero.
	if half {
		value = append(value, high<<4)
	}
	return &String{Value: value, Hex: true}, nil
}

func (p *parser) parseArray(depth int) (Object, error) {
	p.position++
	array := Array{}
	for {
		p.skipWhitespace()
		if p.position >= len(p.data) {
			return nil, p.syntaxError("unterminated array")
		}
		if p.data[p.position] == ']' {
			p.position++
			return array, nil
		}
		element, err := p.parseObject(depth + 1)
		if err != nil {
			return nil, err
		}
		array = append(array, element)
	}
}

func (p *parser) parseDictionary(depth int) (Object, error) {
	p.position += 2
	dictionary := Dictionary{}
	for {
		p.skipWhitespace()
		if p.position+1 < len(p.data) && p.data[p.position] == '>' && p.data[p.position+1] == '>' {
			p.position += 2
			return dictionary, nil
		}
		if p.position >= len(p.data) {
			return nil, p.syntaxError("unterminated dictionary")
		}
		if p.data[p.position] != '/' {
			return nil, p.syntaxError("dictionary key is not a name")
		}
		key, err := p.parseName()
		if err != nil {
			return nil, err
		}
		value, err := p.parseObject(depth + 1)
		if err != nil {
			return nil, err
		}
		dictionary[key.(Name)] = value
	}
}

func parseInteger(text []byte) (int, bool) {
	if len(text) == 0 {
		return 0, false
	}
	for _, c := range text {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	value, err := strconv.Atoi(string(text))
	return value, err == nil
}

func (p *parser) parseNumberOrReference() (Object, error) {
	text := p.regularRun()
	if bytes.ContainsRune(text, '.') {
		value, err := strconv.ParseFloat(string(text), 64)
		if err != nil {
			return nil, p.syntaxError("bad number %q", text)
		}
		return Real(value), nil
	}
	value, err := strconv.ParseInt(string(text), 10, 64)
	if err != nil {
		return nil, p.syntaxError("bad number %q", text)
	}

	// "N G R" is a reference; anything else leaves the integer alone.
	if number, ok := parseInteger(text); ok {
		after := p.position
		p.skipWhitespace()
		if generation, ok := parseInteger(p.regularRun()); ok && p.acceptKeyword("R") {
			return &Reference{Number: number, Generation: generation}, nil
		}
		p.position = after
	}
	return Integer(value), nil
}

// Parse reads one object from the start of data and returns it with the number of bytes it took.
func Parse(data []byte) (Object, int, error) {
	p := &parser{data: data}
	value, err := p.parseObject(0)
	if err != nil {
		return nil, 0, err
	}
	return value, p.position, nil
}

// LengthResolver turns a stream's /Length, which may be a reference, into a number of bytes.
type LengthResolver func(length Object) (int, error)

// ParseIndirect reads "N G obj ... endobj" at offset, its stream data included.
func ParseIndirect(data []byte, offset int, resolveLength LengthResolver) (*Indirect, error) {
	if offset < 0 || offset >= len(data) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: offset %d is outside the data", ErrSyntax, offset))
	}
	p := &parser{data: data, position: offset}

	p.skipWhitespace()
	number, ok := parseInteger(p.regularRun())
	if !ok {
		return nil, p.syntaxError("no object number")
	}
	p.skipWhitespace()
	generation, ok := parseInteger(p.regularRun())
	if !ok {
		return nil, p.syntaxError("no generation number")
	}
	if !p.acceptKeyword("obj") {
		return nil, p.syntaxError("no obj keyword")
	}

	value, err := p.parseObject(0)
	if err != nil {
		return nil, err
	}

	if p.acceptKeyword("stream") {
		dictionary, ok := value.(Dictionary)
		if !ok {
			return nil, p.syntaxError("stream without a dictionary")
		}
		// The keyword is followed by CRLF or LF; a lone CR is tolerated.
		if p.position < len(p.data) && p.data[p.position] == '\r' {
			p.position++
		}
		if p.position < len(p.data) && p.data[p.position] == '\n' {
			p.position++
		}
		if resolveLength == nil {
			resolveLength = func(length Object) (int, error) {
				if integer, ok := length.(Integer); ok {
					return int(integer), nil
				}
				return 0, altshiftErrors.NewWithTrace(fmt.Errorf("%w: stream length is not a direct integer", ErrSyntax))
			}
		}
		length, err := resolveLength(dictionary["Length"])
		if err != nil {
			return nil, fmt.Errorf("resolve length: %w", err)
		}
		// Compared this way round so that a huge /Length cannot overflow the sum.
		if length < 0 || length > len(p.data)-p.position {
			return nil, p.syntaxError("stream length %d runs past the data", length)
		}
		streamData := p.data[p.position : p.position+length]
		p.position += length
		if !p.acceptKeyword("endstream") {
			return nil, p.syntaxError("no endstream after %d bytes of stream", length)
		}
		value = &Stream{Dictionary: dictionary, Data: streamData}
	}

	if !p.acceptKeyword("endobj") {
		return nil, p.syntaxError("no endobj")
	}
	return &Indirect{Number: number, Generation: generation, Value: value}, nil
}
