// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package seed derives per-stage seeds from the world seed.
//
// Every stage that draws random numbers owns a local [math/rand/v2] PCG
// source built from its own stage seed. Nothing in mpg draws from a global
// source, so consuming values in one stage cannot perturb another.
//
// # Derivation
//
// A stage seed is the SHA-256 digest of this message, with every integer in
// big-endian byte order:
//
//	len(Domain)   uint64, 8 bytes
//	Domain        the ASCII bytes of "mpg/stage-seed/v1"
//	world         uint64, 8 bytes (the world seed; fixed width, no length)
//	len(stage)    uint64, 8 bytes
//	stage         the bytes of the stage name (e.g. "elevation")
//	len(version)  uint64, 8 bytes
//	version       the bytes of the stage's algorithm version (e.g. "1")
//
// Strings are hashed as their raw bytes, with no normalization or
// terminator. The length prefixes keep field boundaries unambiguous, so the
// pair ("ab", "c") never collides with ("a", "bc").
//
// The first 16 bytes of the 32-byte digest are read as two big-endian uint64
// values: bytes 0–7 give Seed1 and bytes 8–15 give Seed2. The remaining 16
// bytes are discarded. The PCG source is rand.NewPCG(Seed1, Seed2).
//
// The stage name and version must be non-empty; the functions panic
// otherwise. Both are compile-time constants of the calling stage, so an
// empty one is a programming error, not bad input.
//
// Changing any byte of this layout, including the domain string, changes
// every stage seed and every generated world. A new layout takes a new domain
// string.
package seed
