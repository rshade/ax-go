package schema

import (
	"strconv"
	"strings"
	"testing"
)

func FuzzEnumCanonicalise(f *testing.F) {
	flagTypes := []string{"string", "int", "int8", "uint16", "int64", "uint"}
	for _, seed := range []string{"", "03", "-0", "+5", "300", "9223372036854775808"} {
		for index := range flagTypes {
			f.Add(uint8(index), seed)
		}
	}

	f.Fuzz(func(t *testing.T, typeIndex uint8, value string) {
		flagType := flagTypes[int(typeIndex)%len(flagTypes)]
		canonical, err := CanonicalEnumValue(flagType, value)
		if err != nil {
			return
		}

		again, err := CanonicalEnumValue(flagType, canonical)
		if err != nil {
			t.Fatalf("CanonicalEnumValue(%q, %q) rejected its own output: %v", flagType, canonical, err)
		}
		if again != canonical {
			t.Fatalf("CanonicalEnumValue(%q) not idempotent: %q -> %q -> %q", flagType, value, canonical, again)
		}

		if flagType == "string" {
			return
		}
		if strings.HasPrefix(flagType, "uint") {
			original, _ := strconv.ParseUint(value, 0, 64)
			roundTrip, parseErr := strconv.ParseUint(canonical, 10, 64)
			if parseErr != nil || roundTrip != original {
				t.Fatalf("canonical %q does not parse back to %d", canonical, original)
			}
			return
		}
		original, _ := strconv.ParseInt(value, 0, 64)
		roundTrip, parseErr := strconv.ParseInt(canonical, 10, 64)
		if parseErr != nil || roundTrip != original {
			t.Fatalf("canonical %q does not parse back to %d", canonical, original)
		}
	})
}
