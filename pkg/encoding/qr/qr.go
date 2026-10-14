// Package qr encodes bytes as a QR code symbol: the modules, not an image, so
// that whatever draws it -- an SVG path, a terminal, a PNG -- decides how.
//
// It writes byte mode only, versions 1 to MaxVersion, at any of the four error
// correction levels, and chooses the mask the standard's penalty rules prefer.
// That covers what it was written for, a BankID payload of about a hundred
// bytes, and anything else of that size; it refuses rather than truncates what
// does not fit.
//
// The module layout follows ISO/IEC 18004. The tests hold it to libqrencode's
// output for the same data, level and mask, module for module.
package qr

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// ErrDataTooLong is returned for data that no version up to MaxVersion holds at
// the level asked for.
var ErrDataTooLong = errors.New("data too long")

// Level is how much of a symbol may be damaged and still read: about 7, 15, 25
// and 30 percent for L, M, Q and H, at the cost of room for data.
type Level int

const (
	LevelL Level = iota
	LevelM
	LevelQ
	LevelH
)

// formatBits is a level as the format information writes it, which is not the
// order the levels are listed in.
func (level Level) formatBits() int {
	switch level {
	case LevelL:
		return 1
	case LevelM:
		return 0
	case LevelQ:
		return 3
	case LevelH:
		return 2
	}

	return -1
}

func (level Level) valid() bool {
	return level >= LevelL && level <= LevelH
}

// Code is an encoded symbol.
type Code struct {
	Version int
	Level   Level
	// Mask is the mask pattern applied, 0 to 7.
	Mask int
	// Size is the number of modules a side.
	Size int

	// dark holds the modules row by row.
	dark []bool
}

// Dark says whether the module at column x and row y is dark. Anything outside
// the symbol is light, as the quiet zone around it is.
func (code *Code) Dark(x int, y int) bool {
	if code == nil || x < 0 || y < 0 || x >= code.Size || y >= code.Size {
		return false
	}

	return code.dark[y*code.Size+x]
}

// Rows renders the symbol as one string a row, '#' for a dark module and '.'
// for a light one, without a quiet zone.
func (code *Code) Rows() []string {
	if code == nil {
		return nil
	}

	rows := make([]string, code.Size)
	for y := range code.Size {
		var builder strings.Builder
		builder.Grow(code.Size)
		for x := range code.Size {
			if code.Dark(x, y) {
				builder.WriteByte('#')
			} else {
				builder.WriteByte('.')
			}
		}
		rows[y] = builder.String()
	}

	return rows
}

// Encode encodes data in the smallest version that holds it at the level.
func Encode(data []byte, level Level) (*Code, error) {
	version, err := chooseVersion(data, level)
	if err != nil {
		return nil, err
	}

	symbol := newSymbol(version, level, data)

	best, bestPenalty := 0, -1
	for mask := range 8 {
		symbol.applyMask(mask)
		symbol.drawFormat(mask)
		if penalty := symbol.penalty(); bestPenalty < 0 || penalty < bestPenalty {
			best, bestPenalty = mask, penalty
		}
		symbol.applyMask(mask)
	}

	return symbol.code(best), nil
}

// encodeWithMask is Encode with the mask given rather than chosen, which is
// what lets a test compare it with another encoder's choice.
func encodeWithMask(data []byte, level Level, mask int) (*Code, error) {
	if mask < 0 || mask > 7 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: a mask is 0 to 7", altshiftErrors.ErrValidationError),
			mask,
		)
	}

	version, err := chooseVersion(data, level)
	if err != nil {
		return nil, err
	}

	return newSymbol(version, level, data).code(mask), nil
}

// chooseVersion is the smallest version whose data codewords hold the data in
// byte mode at the level.
func chooseVersion(data []byte, level Level) (int, error) {
	if !level.valid() {
		return 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: not a level", altshiftErrors.ErrValidationError),
			int(level),
		)
	}

	for version := 1; version <= MaxVersion; version++ {
		capacityBits := blockSpecs[version][level].dataCodewords() * 8
		if 4+characterCountBits(version)+8*len(data) <= capacityBits {
			return version, nil
		}
	}

	return 0, altshiftErrors.NewWithTrace(ErrDataTooLong, len(data), int(level))
}

// bitWriter appends bits most significant first.
type bitWriter struct {
	bytes []byte
	count int
}

func (writer *bitWriter) write(value int, width int) {
	for i := width - 1; i >= 0; i-- {
		if writer.count%8 == 0 {
			writer.bytes = append(writer.bytes, 0)
		}
		if value>>i&1 == 1 {
			writer.bytes[len(writer.bytes)-1] |= 0x80 >> (writer.count % 8)
		}
		writer.count++
	}
}

// dataCodewords is the data as byte mode writes it -- the mode, the length,
// the bytes, a terminator -- padded to the version's data capacity.
func dataCodewords(data []byte, version int, spec blockSpec) []byte {
	capacity := spec.dataCodewords()

	writer := &bitWriter{}
	writer.write(0b0100, 4)
	writer.write(len(data), characterCountBits(version))
	for _, b := range data {
		writer.write(int(b), 8)
	}

	// The terminator is up to four zero bits, as many as there is room for,
	// and then zeros to the end of the byte.
	writer.write(0, min(4, capacity*8-writer.count))
	if writer.count%8 != 0 {
		writer.write(0, 8-writer.count%8)
	}

	for pad := 0; len(writer.bytes) < capacity; pad++ {
		if pad%2 == 0 {
			writer.write(0xec, 8)
		} else {
			writer.write(0x11, 8)
		}
	}

	return writer.bytes
}

// interleave splits the data codewords into blocks, adds each block's error
// correction, and interleaves the blocks: the first codeword of every block,
// then the second, and the error correction the same way after the data.
func interleave(data []byte, spec blockSpec) []byte {
	var blocks [][]byte
	offset := 0
	for range spec.group1Blocks {
		blocks = append(blocks, data[offset:offset+spec.group1Data])
		offset += spec.group1Data
	}
	for range spec.group2Blocks {
		blocks = append(blocks, data[offset:offset+spec.group2Data])
		offset += spec.group2Data
	}

	corrections := make([][]byte, len(blocks))
	for i, block := range blocks {
		corrections[i] = errorCorrection(block, spec.ecPerBlock)
	}

	result := make([]byte, 0, spec.totalCodewords())
	for i := range max(spec.group1Data, spec.group2Data) {
		for _, block := range blocks {
			if i < len(block) {
				result = append(result, block[i])
			}
		}
	}
	for i := range spec.ecPerBlock {
		for _, correction := range corrections {
			result = append(result, correction[i])
		}
	}

	return result
}

// symbol is a symbol being drawn: its modules, and which of them belong to the
// function patterns that data and masks leave alone.
type symbol struct {
	version  int
	level    Level
	size     int
	dark     []bool
	function []bool
}

func newSymbol(version int, level Level, data []byte) *symbol {
	size := 17 + 4*version
	symbol := &symbol{
		version:  version,
		level:    level,
		size:     size,
		dark:     make([]bool, size*size),
		function: make([]bool, size*size),
	}

	spec := blockSpecs[version][level]
	symbol.drawFunctionPatterns()
	symbol.drawCodewords(interleave(dataCodewords(data, version, spec), spec))

	return symbol
}

func (symbol *symbol) set(x int, y int, dark bool) {
	symbol.dark[y*symbol.size+x] = dark
}

func (symbol *symbol) setFunction(x int, y int, dark bool) {
	symbol.dark[y*symbol.size+x] = dark
	symbol.function[y*symbol.size+x] = true
}

func (symbol *symbol) isDark(x int, y int) bool {
	return symbol.dark[y*symbol.size+x]
}

func (symbol *symbol) drawFunctionPatterns() {
	size := symbol.size

	// Timing patterns first, so that the finders and alignment patterns drawn
	// over their ends win.
	for i := range size {
		symbol.setFunction(6, i, i%2 == 0)
		symbol.setFunction(i, 6, i%2 == 0)
	}

	symbol.drawFinder(3, 3)
	symbol.drawFinder(size-4, 3)
	symbol.drawFinder(3, size-4)

	centers := alignmentCenters[symbol.version]
	for i, cx := range centers {
		for j, cy := range centers {
			// The three corners the finders occupy have none.
			last := len(centers) - 1
			if (i == 0 && j == 0) || (i == 0 && j == last) || (i == last && j == 0) {
				continue
			}
			symbol.drawAlignment(cx, cy)
		}
	}

	// Reserved for the format and version information, drawn in later, with
	// the dark module beside the bottom-left finder.
	symbol.drawFormat(0)
	symbol.drawVersion()
}

// drawFinder draws a finder pattern centred at (cx, cy) and its light
// separator, as far as the symbol reaches.
func (symbol *symbol) drawFinder(cx int, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || y < 0 || x >= symbol.size || y >= symbol.size {
				continue
			}
			distance := max(abs(dx), abs(dy))
			symbol.setFunction(x, y, distance != 2 && distance != 4)
		}
	}
}

func (symbol *symbol) drawAlignment(cx int, cy int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			symbol.setFunction(cx+dx, cy+dy, max(abs(dx), abs(dy)) != 1)
		}
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}

	return value
}

// formatInformation is the 15 bits that say the level and the mask: five bits
// of data, ten of BCH error correction, masked so that they are never all
// light.
func formatInformation(level Level, mask int) int {
	data := level.formatBits()<<3 | mask
	remainder := data
	for range 10 {
		remainder = remainder<<1 ^ (remainder>>9)*0x537
	}

	return (data<<10 | remainder) ^ 0x5412
}

// versionInformation is the 18 bits versions 7 and up carry: six of version,
// twelve of BCH error correction.
func versionInformation(version int) int {
	remainder := version
	for range 12 {
		remainder = remainder<<1 ^ (remainder>>11)*0x1f25
	}

	return version<<12 | remainder
}

func bit(value int, index int) bool {
	return value>>index&1 == 1
}

// drawFormat draws both copies of the format information, and the dark module
// that sits beside the second.
func (symbol *symbol) drawFormat(mask int) {
	bits := formatInformation(symbol.level, mask)
	size := symbol.size

	for i := range 6 {
		symbol.setFunction(8, i, bit(bits, i))
	}
	symbol.setFunction(8, 7, bit(bits, 6))
	symbol.setFunction(8, 8, bit(bits, 7))
	symbol.setFunction(7, 8, bit(bits, 8))
	for i := 9; i < 15; i++ {
		symbol.setFunction(14-i, 8, bit(bits, i))
	}

	for i := range 8 {
		symbol.setFunction(size-1-i, 8, bit(bits, i))
	}
	for i := 8; i < 15; i++ {
		symbol.setFunction(8, size-15+i, bit(bits, i))
	}
	symbol.setFunction(8, size-8, true)
}

// drawVersion draws both copies of the version information, which only
// versions 7 and up have.
func (symbol *symbol) drawVersion() {
	if symbol.version < 7 {
		return
	}

	bits := versionInformation(symbol.version)
	for i := range 18 {
		a, b := symbol.size-11+i%3, i/3
		symbol.setFunction(a, b, bit(bits, i))
		symbol.setFunction(b, a, bit(bits, i))
	}
}

// drawCodewords places the codewords' bits in the zigzag the standard
// prescribes: up and down two columns at a time from the bottom right, passing
// over the vertical timing pattern and every function module. What is left
// over at the end -- the remainder bits -- stays light.
func (symbol *symbol) drawCodewords(codewords []byte) {
	size := symbol.size
	index := 0
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		upward := (right+1)&2 == 0
		for vertical := range size {
			y := vertical
			if upward {
				y = size - 1 - vertical
			}
			for j := range 2 {
				x := right - j
				if symbol.function[y*size+x] || index >= len(codewords)*8 {
					continue
				}
				symbol.set(x, y, codewords[index/8]>>(7-index%8)&1 == 1)
				index++
			}
		}
	}
}

// masks are the eight mask patterns, as conditions on a module's column and
// row: where one holds, the module is inverted.
var masks = [8]func(x int, y int) bool{
	func(x, y int) bool { return (x+y)%2 == 0 },
	func(_, y int) bool { return y%2 == 0 },
	func(x, _ int) bool { return x%3 == 0 },
	func(x, y int) bool { return (x+y)%3 == 0 },
	func(x, y int) bool { return (x/3+y/2)%2 == 0 },
	func(x, y int) bool { return x*y%2+x*y%3 == 0 },
	func(x, y int) bool { return (x*y%2+x*y%3)%2 == 0 },
	func(x, y int) bool { return ((x+y)%2+x*y%3)%2 == 0 },
}

// applyMask inverts the data modules the mask selects. Applying it twice undoes
// it.
func (symbol *symbol) applyMask(mask int) {
	condition := masks[mask]
	for y := range symbol.size {
		for x := range symbol.size {
			if !symbol.function[y*symbol.size+x] && condition(x, y) {
				symbol.dark[y*symbol.size+x] = !symbol.dark[y*symbol.size+x]
			}
		}
	}
}

// code finishes the symbol with a mask and its format information.
func (symbol *symbol) code(mask int) *Code {
	symbol.applyMask(mask)
	symbol.drawFormat(mask)

	return &Code{
		Version: symbol.version,
		Level:   symbol.level,
		Mask:    mask,
		Size:    symbol.size,
		dark:    symbol.dark,
	}
}

// Penalty weights, ISO/IEC 18004 section 7.8.3.
const (
	penaltyRun     = 3
	penaltyBlock   = 3
	penaltyFinder  = 40
	penaltyBalance = 10
)

// finderLike is the 1:1:3:1:1 ratio of a finder pattern with four light
// modules on one side, which a mask should not imitate.
var finderLike = [2][11]bool{
	{true, false, true, true, true, false, true, false, false, false, false},
	{false, false, false, false, true, false, true, true, true, false, true},
}

// penalty scores a masked symbol by the standard's four rules; the mask with
// the lowest score is the one used.
func (symbol *symbol) penalty() int {
	size := symbol.size
	score := 0

	line := make([]bool, size)
	for y := range size {
		for x := range size {
			line[x] = symbol.isDark(x, y)
		}
		score += linePenalty(line)
	}
	for x := range size {
		for y := range size {
			line[y] = symbol.isDark(x, y)
		}
		score += linePenalty(line)
	}

	return score + blockPenalty(symbol) + balancePenalty(symbol)
}

// linePenalty is the first and third rules over one row or column: a run of
// five or more modules of one colour, and anything that looks like a finder.
func linePenalty(line []bool) int {
	score := 0

	run := 1
	for i := 1; i <= len(line); i++ {
		if i < len(line) && line[i] == line[i-1] {
			run++
			continue
		}
		if run >= 5 {
			score += penaltyRun + run - 5
		}
		run = 1
	}

	for start := 0; start+11 <= len(line); start++ {
		for _, pattern := range finderLike {
			if slices.Equal(line[start:start+11], pattern[:]) {
				score += penaltyFinder
			}
		}
	}

	return score
}

// blockPenalty is the second rule: every two-by-two block of one colour.
func blockPenalty(symbol *symbol) int {
	score := 0
	for y := range symbol.size - 1 {
		for x := range symbol.size - 1 {
			color := symbol.isDark(x, y)
			if symbol.isDark(x+1, y) == color && symbol.isDark(x, y+1) == color && symbol.isDark(x+1, y+1) == color {
				score += penaltyBlock
			}
		}
	}

	return score
}

// balancePenalty is the fourth rule: ten points for each whole five percent
// the dark share is from half.
func balancePenalty(symbol *symbol) int {
	dark := 0
	for _, module := range symbol.dark {
		if module {
			dark++
		}
	}
	percent := dark * 100 / len(symbol.dark)

	return penaltyBalance * (abs(percent-50) / 5)
}
