package github

import "errors"

// ErrUnexpectedCommitCount reports an answer that does not name exactly the one
// commit that was asked for.
var ErrUnexpectedCommitCount = errors.New("unexpected number of commits")

// ErrGraphql reports a GraphQL request the API refused as a whole, rather than answering in part.
var ErrGraphql = errors.New("graphql request refused")
