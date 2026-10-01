package conflict_behavior

// ConflictBehavior says what Graph does when an item of the same name already
// exists where one is created, uploaded or moved.
type ConflictBehavior string

const (
	Fail    ConflictBehavior = "fail"
	Replace ConflictBehavior = "replace"
	Rename  ConflictBehavior = "rename"
)
