// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package config

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Seed is the world seed. In JSON it is a decimal string, such as
// "18446744073709551615", so readers that parse numbers as doubles keep every
// bit. Only the canonical form is accepted: no sign, no leading zeros, no
// space.
type Seed uint64

// MarshalJSON writes s as a quoted decimal string.
func (s Seed) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, strconv.FormatUint(uint64(s), 10)), nil
}

// UnmarshalJSON reads a quoted decimal string. A JSON number is rejected.
func (s *Seed) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil || (len(b) > 0 && b[0] != '"') {
		return fmt.Errorf("config: seed must be a decimal string such as \"42\", not %s", b)
	}
	v, err := strconv.ParseUint(str, 10, 64)
	if err != nil || strconv.FormatUint(v, 10) != str {
		return fmt.Errorf("config: seed %q must be a decimal integer from 0 to 18446744073709551615, without sign or leading zeros", str)
	}
	*s = Seed(v)
	return nil
}
