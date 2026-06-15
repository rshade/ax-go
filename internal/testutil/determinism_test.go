package testutil

import "testing"

type tbSpy struct {
	*testing.T

	errorfCalled bool
}

func (s *tbSpy) Errorf(format string, args ...any) {
	s.errorfCalled = true
	s.T.Logf("spy caught Errorf: "+format, args...)
}

func TestCompareOutputsDetectsPayloadEntityIDDivergence(t *testing.T) {
	run1 := []byte(
		`{"data":{"entity_id":"019744d2-1a5f-7000-8000-000000000001"},` +
			`"meta":{"trace_id":"a","span_id":"b","idempotency_key":"c"}}`,
	)
	run2 := []byte(
		`{"data":{"entity_id":"019744d2-1a5f-7000-8000-000000000002"},` +
			`"meta":{"trace_id":"d","span_id":"e","idempotency_key":"f"}}`,
	)

	spy := &tbSpy{T: t}
	CompareOutputs(spy, run1, run2, ModeBoundedJSON)
	if !spy.errorfCalled {
		t.Fatal("CompareOutputs did not report a payload entity_id divergence")
	}
}
