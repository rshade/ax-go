package logcore

import (
	"context"
	"io"
	"sync"
	"testing"
)

// TestConcurrentEmitAndFlush covers L-09/C-09 concurrency safety under -race:
// multiple goroutines emit through one logger while another repeatedly calls
// Flush. This is the realistic shutdown shape — request handlers still logging
// while the shutdown path drains — and the sink slice is read by both.
func TestConcurrentEmitAndFlush(t *testing.T) {
	const (
		emitters        = 8
		linesPerEmitter = 50
		flushRounds     = 20
	)

	sink := &sanctioningSink{}
	logger := New(
		context.Background(),
		WithWriter(io.Discard),
		withSinks(sink),
		WithLabels(Labels{Application: "concurrent"}),
	)

	var wg sync.WaitGroup
	wg.Add(emitters + 1)

	for i := range emitters {
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			for n := range linesPerEmitter {
				logger.Info(ctx).Int("emitter", id).Int("n", n).Msg("concurrent")
			}
		}(i)
	}

	go func() {
		defer wg.Done()
		ctx := context.Background()
		for range flushRounds {
			if err := Flush(ctx, logger); err != nil {
				t.Errorf("Flush: %v", err)
				return
			}
		}
	}()

	wg.Wait()

	if got := len(sink.lines()); got != emitters*linesPerEmitter {
		t.Fatalf("sink received %d lines, want %d", got, emitters*linesPerEmitter)
	}
}

// TestConcurrentWithLabelsDerivation asserts deriving loggers concurrently is
// safe. Each derivation re-sanctions on the shared sink, so the label-sanctioning
// path is exercised from several goroutines at once.
func TestConcurrentWithLabelsDerivation(t *testing.T) {
	const derivations = 16

	sink := &sanctioningSink{}
	logger := New(context.Background(), WithWriter(io.Discard), withSinks(sink))

	var wg sync.WaitGroup
	wg.Add(derivations)
	for i := range derivations {
		go func(id int) {
			defer wg.Done()
			derived := logger.WithLabels(Labels{Application: "app", Host: "host"})
			derived.Info(context.Background()).Int("id", id).Msg("derived")
		}(i)
	}
	wg.Wait()

	// One sanction at construction plus one per derivation.
	if got := len(sink.sanctions()); got != derivations+1 {
		t.Fatalf("SanctionLabels called %d times, want %d", got, derivations+1)
	}
}
