package testonlyread_test

import (
	"testing"

	"testonlyread"
)

func TestLevel(t *testing.T) {
	var h testonlyread.Exposed = testonlyread.MakeHidden()
	if h.Level != 2 {
		t.Fatal("level")
	}
}
