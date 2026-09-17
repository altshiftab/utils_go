package endpoint

import (
	"strings"
	"unicode"
)

// titleCase uppercases the first rune of s, leaving the rest unchanged.
func titleCase(s string) string {
	if s == "" {
		return s
	}

	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}

// makePathPart turns a path into the part of a name that says which resource it names:
// "/api/order/bucket-file" becomes "OrderBucketFile".
func makePathPart(path string) string {
	segments := strings.Split(
		strings.ReplaceAll(
			strings.TrimPrefix(
				path,
				"/api/",
			),
			"/",
			"-",
		),
		"-",
	)

	var casedSegments []string
	for _, segment := range segments {
		casedSegments = append(casedSegments, titleCase(segment))
	}

	return strings.ReplaceAll(strings.Join(casedSegments, ""), ".", "")
}

// OperationName is what an endpoint is called by whoever generates from it: "getOrderBucketFile"
// for a GET of /api/order/bucket-file, "queryOrders" for a QUERY of /api/orders.
//
// It lives here, rather than in any one generator, so that an operation carries one name wherever
// it is named -- the function a frontend calls and the identifier a documented operation goes by
// are the same string, and stay the same string. A generator that derived its own would name the
// same operation two ways, and nothing would notice.
//
// The name is derived, so it can collide: two paths differing only in punctuation reduce to one
// name. Hint.OperationId is the override for that, and a generator for whom names must be unique
// is expected to check rather than to assume.
func OperationName(method string, path string) string {
	return strings.ToLower(method) + makePathPart(path)
}
