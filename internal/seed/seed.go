// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package seed

import (
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
)

// Domain is the fixed prefix that separates stage-seed digests from any other
// SHA-256 use. See the package documentation for the full byte layout.
const Domain = "mpg/stage-seed/v1"

// Derive returns the two uint64 values of the stage seed for world, stage,
// and version. They are the arguments to rand.NewPCG, and may also key
// coordinate noise. It panics if stage or version is empty.
func Derive(world uint64, stage, version string) (seed1, seed2 uint64) {
	if stage == "" {
		panic("seed: empty stage name")
	}
	if version == "" {
		panic("seed: empty algorithm version")
	}
	msg := make([]byte, 0, 8+len(Domain)+8+8+len(stage)+8+len(version))
	msg = appendString(msg, Domain)
	msg = binary.BigEndian.AppendUint64(msg, world)
	msg = appendString(msg, stage)
	msg = appendString(msg, version)
	sum := sha256.Sum256(msg)
	return binary.BigEndian.Uint64(sum[0:8]), binary.BigEndian.Uint64(sum[8:16])
}

// PCG returns a new PCG source seeded with Derive(world, stage, version).
// The caller owns it. It panics if stage or version is empty.
func PCG(world uint64, stage, version string) *rand.PCG {
	return rand.NewPCG(Derive(world, stage, version))
}

// Rand returns a new generator over PCG(world, stage, version).
// The caller owns it. It panics if stage or version is empty.
func Rand(world uint64, stage, version string) *rand.Rand {
	return rand.New(PCG(world, stage, version))
}

// appendString appends s to b as a big-endian uint64 length followed by the
// bytes of s.
func appendString(b []byte, s string) []byte {
	b = binary.BigEndian.AppendUint64(b, uint64(len(s)))
	return append(b, s...)
}
