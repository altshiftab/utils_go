package qr

// MaxVersion is the largest symbol this package draws: 57 modules a side,
// enough for 213 bytes at LevelM and well past a BankID payload's 108. The
// larger versions differ only in their tables, which are left out until
// something needs them.
const MaxVersion = 10

// blockSpec is how a symbol's codewords divide into error-correction blocks:
// group1Blocks blocks of group1Data data codewords, then group2Blocks of
// group2Data, each with ecPerBlock error-correction codewords.
type blockSpec struct {
	ecPerBlock   int
	group1Blocks int
	group1Data   int
	group2Blocks int
	group2Data   int
}

func (spec blockSpec) dataCodewords() int {
	return spec.group1Blocks*spec.group1Data + spec.group2Blocks*spec.group2Data
}

func (spec blockSpec) totalCodewords() int {
	return spec.dataCodewords() + (spec.group1Blocks+spec.group2Blocks)*spec.ecPerBlock
}

// blockSpecs is ISO/IEC 18004 table 9, indexed by version and then by level
// in the order L, M, Q, H.
var blockSpecs = [MaxVersion + 1][4]blockSpec{
	1:  {{7, 1, 19, 0, 0}, {10, 1, 16, 0, 0}, {13, 1, 13, 0, 0}, {17, 1, 9, 0, 0}},
	2:  {{10, 1, 34, 0, 0}, {16, 1, 28, 0, 0}, {22, 1, 22, 0, 0}, {28, 1, 16, 0, 0}},
	3:  {{15, 1, 55, 0, 0}, {26, 1, 44, 0, 0}, {18, 2, 17, 0, 0}, {22, 2, 13, 0, 0}},
	4:  {{20, 1, 80, 0, 0}, {18, 2, 32, 0, 0}, {26, 2, 24, 0, 0}, {16, 4, 9, 0, 0}},
	5:  {{26, 1, 108, 0, 0}, {24, 2, 43, 0, 0}, {18, 2, 15, 2, 16}, {22, 2, 11, 2, 12}},
	6:  {{18, 2, 68, 0, 0}, {16, 4, 27, 0, 0}, {24, 4, 19, 0, 0}, {28, 4, 15, 0, 0}},
	7:  {{20, 2, 78, 0, 0}, {18, 4, 31, 0, 0}, {18, 2, 14, 4, 15}, {26, 4, 13, 1, 14}},
	8:  {{24, 2, 97, 0, 0}, {22, 2, 38, 2, 39}, {22, 4, 18, 2, 19}, {26, 4, 14, 2, 15}},
	9:  {{30, 2, 116, 0, 0}, {22, 3, 36, 2, 37}, {20, 4, 16, 4, 17}, {24, 4, 12, 4, 13}},
	10: {{18, 2, 68, 2, 69}, {26, 4, 43, 1, 44}, {24, 6, 19, 2, 20}, {28, 6, 15, 2, 16}},
}

// totalCodewords is how many codewords each version holds, data and error
// correction together, whatever the level: the table above must agree with it.
var totalCodewords = [MaxVersion + 1]int{0, 26, 44, 70, 100, 134, 172, 196, 242, 292, 346}

// alignmentCenters is where alignment patterns are centred, on both axes, for
// each version. Version 1 has none.
var alignmentCenters = [MaxVersion + 1][]int{
	2:  {6, 18},
	3:  {6, 22},
	4:  {6, 26},
	5:  {6, 30},
	6:  {6, 34},
	7:  {6, 22, 38},
	8:  {6, 24, 42},
	9:  {6, 26, 46},
	10: {6, 28, 50},
}

// characterCountBits is the width of byte mode's length field.
func characterCountBits(version int) int {
	if version <= 9 {
		return 8
	}

	return 16
}
