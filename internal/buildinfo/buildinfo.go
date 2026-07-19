// Package buildinfo exposes the version identity of the sinet binary.
//
// The release artifact is one static binary (Spec S01.5); every mode of that
// binary reports the identity stamped here.
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// version is the release version. Overridden at release build time via:
//
//	-ldflags "-X <module>/internal/buildinfo.version=v0.x.y"
var version = "v0.0.0-dev"

// Version returns the release version string.
func Version() string { return version }

// String returns the one-line binary identity: version, VCS revision when
// built from a git checkout, Go toolchain, and target platform.
func String() string {
	var b strings.Builder
	b.WriteString(version)
	if rev, dirty, ok := vcsRevision(); ok {
		b.WriteString(" (")
		b.WriteString(rev)
		if dirty {
			b.WriteString("+dirty")
		}
		b.WriteString(")")
	}
	b.WriteString(" ")
	b.WriteString(runtime.Version())
	b.WriteString(" ")
	b.WriteString(runtime.GOOS + "/" + runtime.GOARCH)
	return b.String()
}

func vcsRevision() (rev string, dirty bool, ok bool) {
	info, infoOK := debug.ReadBuildInfo()
	if !infoOK {
		return "", false, false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 12 {
				rev = rev[:12]
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	return rev, dirty, rev != ""
}
