package segy

import (
	"math"
	"testing"
)

func TestShiftTraceBySamplesFrequencyDomain_ZeroShift(t *testing.T) {
	in := []float32{0, 1, -2, 3, -1, 2, 0.5, -0.5}
	out, err := ShiftTraceBySamplesFrequencyDomain(in, 0)
	if err != nil {
		t.Fatalf("ShiftTraceBySamplesFrequencyDomain returned error: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("length mismatch: got %d want %d", len(out), len(in))
	}
	for i := range in {
		if math.Abs(float64(out[i]-in[i])) > 1e-6 {
			t.Fatalf("zero-shift mismatch at %d: got %v want %v", i, out[i], in[i])
		}
	}
}

func TestShiftTraceBySamplesFrequencyDomain_IntegerShiftPositiveTruncated(t *testing.T) {
	in := []float32{1, 2, 3, 4, 5, 6, 7, 8}
	shift := 2.0
	out, err := ShiftTraceBySamplesFrequencyDomain(in, shift)
	if err != nil {
		t.Fatalf("ShiftTraceBySamplesFrequencyDomain returned error: %v", err)
	}

	want := []float32{0, 0, 1, 2, 3, 4, 5, 6}
	for i := range want {
		if math.Abs(float64(out[i]-want[i])) > 1e-5 {
			t.Fatalf("integer-shift mismatch at %d: got %v want %v", i, out[i], want[i])
		}
	}
}

func TestShiftTraceBySamplesFrequencyDomain_IntegerShiftNegativeTruncated(t *testing.T) {
	in := []float32{1, 2, 3, 4, 5, 6, 7, 8}
	shift := -3.0
	out, err := ShiftTraceBySamplesFrequencyDomain(in, shift)
	if err != nil {
		t.Fatalf("ShiftTraceBySamplesFrequencyDomain returned error: %v", err)
	}

	want := []float32{4, 5, 6, 7, 8, 0, 0, 0}
	for i := range want {
		if math.Abs(float64(out[i]-want[i])) > 1e-5 {
			t.Fatalf("negative-shift mismatch at %d: got %v want %v", i, out[i], want[i])
		}
	}
}

func TestShiftTraceBySamplesFrequencyDomain_InvalidShift(t *testing.T) {
	in := []float32{1, 2, 3, 4}
	if _, err := ShiftTraceBySamplesFrequencyDomain(in, math.NaN()); err == nil {
		t.Fatalf("expected error for NaN shift")
	}
	if _, err := ShiftTraceBySamplesFrequencyDomain(in, math.Inf(1)); err == nil {
		t.Fatalf("expected error for +Inf shift")
	}
}
