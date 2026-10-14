package qr

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// fixture is a symbol libqrencode 4.1.1 drew: the data, the level, the mask it
// chose, and its modules. testdata holds one per <name>.data and <name>.txt.
type fixture struct {
	name    string
	data    []byte
	level   Level
	mask    int
	version int
	rows    []string
}

func readFixtures(t *testing.T) []*fixture {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join("testdata", "*.txt"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}

	levels := map[string]Level{"L": LevelL, "M": LevelM, "Q": LevelQ, "H": LevelH}

	var fixtures []*fixture
	for _, path := range paths {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		data, err := os.ReadFile(strings.TrimSuffix(path, ".txt") + ".data")
		if err != nil {
			t.Fatalf("reading the data beside %s: %v", path, err)
		}

		lines := strings.Split(strings.TrimSpace(string(text)), "\n")
		var level string
		item := &fixture{name: filepath.Base(path), data: data, rows: lines[1:]}
		if _, err := fmt.Sscanf(lines[0], "%s %d %d", &level, &item.mask, &item.version); err != nil {
			t.Fatalf("reading the header of %s: %v", path, err)
		}
		item.level = levels[level]
		fixtures = append(fixtures, item)
	}

	return fixtures
}

// libqrencode chooses its mask by its own reading of the penalty rules, so the
// comparison is at its mask: what is held to it is everything else -- the
// encoding, the error correction, the interleaving, the placement and the
// function patterns, module for module.
func TestMatchesLibqrencode(t *testing.T) {
	t.Parallel()

	for _, item := range readFixtures(t) {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			code, err := encodeWithMask(item.data, item.level, item.mask)
			if err != nil {
				t.Fatalf("encodeWithMask: %v", err)
			}
			if code.Version != item.version || code.Size != len(item.rows) {
				t.Fatalf("version %d size %d, want %d and %d", code.Version, code.Size, item.version, len(item.rows))
			}

			rows := code.Rows()
			for y := range rows {
				if rows[y] != item.rows[y] {
					t.Errorf("row %d\n got %s\nwant %s", y, rows[y], item.rows[y])
				}
			}
		})
	}
}

// readFormat reads the level and mask back from a symbol's first copy of its
// format information, as a reader would.
func readFormat(code *Code) (int, int) {
	cells := [][2]int{{8, 0}, {8, 1}, {8, 2}, {8, 3}, {8, 4}, {8, 5}, {8, 7}, {8, 8}, {7, 8}}
	for i := 9; i < 15; i++ {
		cells = append(cells, [2]int{14 - i, 8})
	}

	bits := 0
	for index, cell := range cells {
		if code.Dark(cell[0], cell[1]) {
			bits |= 1 << index
		}
	}
	bits ^= 0x5412

	return bits >> 13, bits >> 10 & 7
}

func TestEncodeChoosesTheLeastPenalisedMask(t *testing.T) {
	t.Parallel()

	for _, item := range readFixtures(t) {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			code, err := Encode(item.data, item.level)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			// What Encode answers is the symbol at the mask it chose.
			forced, err := encodeWithMask(item.data, item.level, code.Mask)
			if err != nil {
				t.Fatalf("encodeWithMask: %v", err)
			}
			if !slices.Equal(code.Rows(), forced.Rows()) {
				t.Error("Encode differs from its own mask forced")
			}

			// The mask it says is the one its format information says.
			level, mask := readFormat(code)
			if level != item.level.formatBits() || mask != code.Mask {
				t.Errorf("format reads level %d mask %d, want %d and %d", level, mask, item.level.formatBits(), code.Mask)
			}

			// And no mask scores lower; among equal scores, the first wins.
			penalties := make([]int, 8)
			for mask := range 8 {
				version, err := chooseVersion(item.data, item.level)
				if err != nil {
					t.Fatalf("chooseVersion: %v", err)
				}
				candidate := newSymbol(version, item.level, item.data)
				candidate.applyMask(mask)
				candidate.drawFormat(mask)
				penalties[mask] = candidate.penalty()
			}
			lowest := slices.Index(penalties, slices.Min(penalties))
			if code.Mask != lowest {
				t.Errorf("mask %d, want %d: penalties %v", code.Mask, lowest, penalties)
			}
		})
	}
}

// The classic example of the standard and every tutorial: HELLO WORLD at 1-M,
// whose data codewords are followed by these ten.
func TestErrorCorrection(t *testing.T) {
	t.Parallel()

	data := []byte{32, 91, 11, 120, 209, 114, 220, 77, 67, 64, 236, 17, 236, 17, 236, 17}
	want := []byte{196, 35, 39, 119, 235, 215, 231, 226, 93, 23}

	if got := errorCorrection(data, 10); !bytes.Equal(got, want) {
		t.Errorf("errorCorrection = %v, want %v", got, want)
	}
}

func TestFormatAndVersionInformation(t *testing.T) {
	t.Parallel()

	// ISO/IEC 18004 annex C.
	formatCases := []struct {
		name  string
		level Level
		mask  int
		want  int
	}{
		{name: "L, mask 0", level: LevelL, mask: 0, want: 0x77c4},
		{name: "M, mask 0", level: LevelM, mask: 0, want: 0x5412},
		{name: "Q, mask 0", level: LevelQ, mask: 0, want: 0x355f},
		{name: "H, mask 0", level: LevelH, mask: 0, want: 0x1689},
		{name: "M, mask 5", level: LevelM, mask: 5, want: 0x40ce},
		{name: "H, mask 7", level: LevelH, mask: 7, want: 0x083b},
	}
	for _, testCase := range formatCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := formatInformation(testCase.level, testCase.mask); got != testCase.want {
				t.Errorf("formatInformation = %#x, want %#x", got, testCase.want)
			}
		})
	}

	// ISO/IEC 18004 annex D.
	versionCases := []struct {
		version int
		want    int
	}{
		{version: 7, want: 0x07c94},
		{version: 8, want: 0x085bc},
		{version: 9, want: 0x09a99},
		{version: 10, want: 0x0a4d3},
	}
	for _, testCase := range versionCases {
		t.Run(fmt.Sprintf("version %d", testCase.version), func(t *testing.T) {
			t.Parallel()

			if got := versionInformation(testCase.version); got != testCase.want {
				t.Errorf("versionInformation = %#x, want %#x", got, testCase.want)
			}
		})
	}
}

// Every level of a version must come to the version's codewords, which is the
// check that catches a mistyped row of the block table.
func TestBlockSpecs(t *testing.T) {
	t.Parallel()

	for version := 1; version <= MaxVersion; version++ {
		for level, spec := range blockSpecs[version] {
			if got := spec.totalCodewords(); got != totalCodewords[version] {
				t.Errorf("version %d level %d: %d codewords, want %d", version, level, got, totalCodewords[version])
			}
		}
	}
}

func TestEncode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		data        []byte
		level       Level
		wantVersion int
		wantErr     error
	}{
		{name: "nothing", data: nil, level: LevelM, wantVersion: 1},
		{name: "a BankID payload", data: bytes.Repeat([]byte("x"), 108), level: LevelM, wantVersion: 7},
		{name: "the most version 1 holds at L", data: bytes.Repeat([]byte("x"), 17), level: LevelL, wantVersion: 1},
		{name: "one more", data: bytes.Repeat([]byte("x"), 18), level: LevelL, wantVersion: 2},
		{name: "the most there is room for at M", data: bytes.Repeat([]byte("x"), 213), level: LevelM, wantVersion: 10},
		{name: "one more than there is room for", data: bytes.Repeat([]byte("x"), 214), level: LevelM, wantErr: ErrDataTooLong},
		{name: "not a level", data: []byte("x"), level: Level(9), wantErr: altshiftErrors.ErrValidationError},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, err := Encode(testCase.data, testCase.level)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("Encode = %v, want %v", err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if code.Version != testCase.wantVersion || code.Size != 17+4*testCase.wantVersion {
				t.Errorf("version %d size %d, want version %d", code.Version, code.Size, testCase.wantVersion)
			}
		})
	}

	if _, err := encodeWithMask([]byte("x"), LevelM, 8); !errors.Is(err, altshiftErrors.ErrValidationError) {
		t.Errorf("a mask of 8 = %v, want a validation error", err)
	}
}

func TestCode(t *testing.T) {
	t.Parallel()

	code, err := Encode([]byte("Hello"), LevelL)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// The top-left finder's corner is dark; past the edge is the quiet zone.
	if !code.Dark(0, 0) || code.Dark(-1, 0) || code.Dark(0, code.Size) {
		t.Error("Dark does not read the finder and the quiet zone")
	}

	var none *Code
	if none.Dark(0, 0) || none.Rows() != nil {
		t.Error("a nil code is not empty")
	}
}

func line(text string) []bool {
	modules := make([]bool, len(text))
	for i, character := range text {
		modules[i] = character == '#'
	}

	return modules
}

// grid makes a symbol of the modules drawn, '#' dark, for scoring.
func grid(rows ...string) *symbol {
	size := len(rows)
	drawn := &symbol{size: size, dark: make([]bool, size*size), function: make([]bool, size*size)}
	for y, row := range rows {
		copy(drawn.dark[y*size:], line(row))
	}

	return drawn
}

func TestLinePenalty(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		line string
		want int
	}{
		{name: "nothing four long", line: "####.####.", want: 0},
		{name: "a run of five", line: "#####.", want: 3},
		{name: "a run of seven", line: "#######", want: 5},
		{name: "two runs", line: ".....#.#####", want: 6},
		{name: "a finder with light after it", line: "#.###.#....", want: 40},
		{name: "a finder with light before it", line: "....#.###.#", want: 40},
		{name: "a finder with light on both sides counts twice", line: "....#.###.#....", want: 80},
		{name: "a finder with three light is none", line: "#.###.#...#", want: 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := linePenalty(line(testCase.line)); got != testCase.want {
				t.Errorf("linePenalty(%q) = %d, want %d", testCase.line, got, testCase.want)
			}
		})
	}
}

func TestBlockAndBalancePenalties(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		symbol      *symbol
		wantBlock   int
		wantBalance int
	}{
		{name: "all dark", symbol: grid("###", "###", "###"), wantBlock: 12, wantBalance: 100},
		{name: "a checkerboard", symbol: grid("#.#.", ".#.#", "#.#.", ".#.#"), wantBlock: 0, wantBalance: 0},
		{name: "one light block", symbol: grid("..#", "..#", "#.#"), wantBlock: 3, wantBalance: 10},
		{name: "a quarter dark", symbol: grid("#...", "#...", "#...", "#..."), wantBlock: 18, wantBalance: 50},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := blockPenalty(testCase.symbol); got != testCase.wantBlock {
				t.Errorf("blockPenalty = %d, want %d", got, testCase.wantBlock)
			}
			if got := balancePenalty(testCase.symbol); got != testCase.wantBalance {
				t.Errorf("balancePenalty = %d, want %d", got, testCase.wantBalance)
			}
		})
	}
}
