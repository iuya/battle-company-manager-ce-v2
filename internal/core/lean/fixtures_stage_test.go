package lean

import "testing"

// decodeFixture is the decoder under test(bind, defaults, resolution, validation, Decode).
func decodeFixture(t *testing.T, raw []byte) Report {
	_, _, rep := Decode(fixtureFile, raw, newFakeLower())
	return rep
}
