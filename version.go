// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mpg

import (
	"github.com/maloquacious/semver"
)

var (
	version = semver.Version{
		Major:      0,
		Minor:      9,
		Patch:      7,
		PreRelease: "alpha",
		Build:      semver.Commit(),
	}
)

func Version() semver.Version {
	return version
}
