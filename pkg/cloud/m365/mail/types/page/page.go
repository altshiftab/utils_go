package page

// Page is one page of a listing: what it held, and the link that asks for the next one, which is
// empty on the last.
type Page[T any] struct {
	Items    []T
	NextLink string
}
