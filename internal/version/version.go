// Package version holds the build version, set via -ldflags.
package version

// Version is overridden at build time:
// -ldflags "-X netwatch/internal/version.Version=1.4.0"
var Version = "1.4.0-dev"
