go 1.18

module golang.org/x/vgo

// This dependency is vulnerable to GO-2020-0006.
// The point of this commit is to serve as a test case for
// automated vulnerability scanning of the Go repos.
//
// Using the tour repo because it contains nothing
// important and is not imported by any of our other repos,
// which means any report should be limited to x/tour
// and not affect other users.
//
// Even if people did depend on x/tour, govulncheck would
// correctly identify that no code here calls the vulnerable
// symbols in github.com/miekg/dns. Only less precise
// scanners would suggest that there is a problem.
require github.com/miekg/dns v1.0.0

require (
	golang.org/x/crypto v0.14.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
)
