// Package netbind makes every socket of a program that must send from one local address: the
// source address a router steers into a VPN tunnel.
//
// On a host without policy routing, the source address of a packet is what decides whether it
// travels through the tunnel or straight out of the home connection. A socket that binds the
// wildcard gets the interface's primary address and leaves unprotected, with no error and nothing
// in a log to say so. So the sockets are made here, every bind and connect is checked afterwards
// against the address that was asked for, and a name is resolved through a DNS server reached
// from that address, or through the system resolver only when the program says so, or not at all.
//
// The check after the fact is deliberate: it costs nothing per connection, and it catches the case
// where the binding was configured correctly and something else, a redirect, a fallback or a
// future edit, got a socket anyway. The netbindtest package is the other half: a test that fails
// the build when any other package of the program opens a socket of its own.
package netbind
