// Package outbound_guard keeps a program's outbound connections to the public internet, for the
// ones it makes to targets a user or a datastore chose: a feed address to fetch, a host to scan.
//
// It refuses loopback, private, link-local, cloud metadata, reserved, documentation, multicast and
// the other address ranges the public internet does not route. Check accepts a raw IP address, a
// hostname, a host:port or a URL; a hostname is resolved and every answer must be public, and a
// failed or empty lookup is refused. DialControl makes the same check where a net.Dialer connects,
// which also catches a redirect, or a name that resolves to an internal address only when dialled.
//
// A scanner engine with a connection policy of its own can be given DeniedPrefixStrings, so that
// it refuses the same ranges when it connects.
package outbound_guard
