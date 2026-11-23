// Package buildinfo holds what the build says about the binary: the release it was built from.
package buildinfo

// Version is the release tag the image was built from (v2.9.0), set with -ldflags "-X" by the Dockerfile; a build from a checkout says dev.
var Version = "dev"

// Commit is the git commit the image was built from (the full hash), set with -ldflags "-X" by the Dockerfile; a build from a checkout says unknown.
var Commit = "unknown"
