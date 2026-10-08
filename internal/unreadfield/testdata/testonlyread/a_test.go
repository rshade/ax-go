package testonlyread

import "testing"

func TestDebug(t *testing.T) {
	if !newOpts().debug {
		t.Fatal("debug unset")
	}
}
