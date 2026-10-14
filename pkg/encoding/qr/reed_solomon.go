package qr

// The field QR codes compute in: GF(2⁸) reduced by x⁸ + x⁴ + x³ + x² + 1,
// with α = 2 as its generator.
const fieldPolynomial = 0x11d

var (
	exponents  [510]byte
	logarithms [256]int
)

func init() {
	value := 1
	for power := range 255 {
		exponents[power] = byte(value)
		exponents[power+255] = byte(value)
		logarithms[value] = power

		value <<= 1
		if value&0x100 != 0 {
			value ^= fieldPolynomial
		}
	}
}

func multiply(a byte, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}

	return exponents[logarithms[a]+logarithms[b]]
}

// generatorPolynomial is ∏ (x + αⁱ) for i below degree, highest power first,
// its leading coefficient 1.
func generatorPolynomial(degree int) []byte {
	polynomial := []byte{1}
	for i := range degree {
		next := make([]byte, len(polynomial)+1)
		for j, coefficient := range polynomial {
			next[j] ^= coefficient
			next[j+1] ^= multiply(coefficient, exponents[i])
		}
		polynomial = next
	}

	return polynomial
}

// errorCorrection is the remainder of data·xⁿ divided by the generator of
// degree n: the n codewords a block's data is followed by.
func errorCorrection(data []byte, count int) []byte {
	generator := generatorPolynomial(count)

	remainder := make([]byte, count)
	for _, codeword := range data {
		factor := codeword ^ remainder[0]
		copy(remainder, remainder[1:])
		remainder[count-1] = 0
		for i := range count {
			remainder[i] ^= multiply(generator[i+1], factor)
		}
	}

	return remainder
}
