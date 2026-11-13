// Package version holds the build version, set via -ldflags.
package version

// Version is overridden at build time:
// -ldflags "-X sismon/internal/version.Version=1.7.0"
var Version = "1.7.0-dev"
