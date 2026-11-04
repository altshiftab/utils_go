package page

// Page is one page of a listing: what it held, and the token that asks for the
// next one, which is empty on the last.
//
// ResultSizeEstimate is Gmail's guess at how many the whole listing holds, which
// it says plainly is an estimate: it is a size for a reader to be told, not a
// count to stop paging on.
type Page[T any] struct {
	Items              []T
	NextPageToken      string
	ResultSizeEstimate int
}
