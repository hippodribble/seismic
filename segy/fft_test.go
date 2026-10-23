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

func TestShiftEnsembleSamplesBySamplesFrequencyDomain_PositiveTruncated(t *testing.T) {
	in := EnsembleSamples{
		Traces: [][]float32{
			{1, 2, 3, 4, 5, 6, 7, 8},
			{10, 20, 30, 40, 50, 60, 70, 80},
		},
		TraceCount:        2,
		SamplesPerTrace:   8,
		TotalImageSamples: 16,
	}

	out, err := ShiftEnsembleSamplesBySamplesFrequencyDomain(in, 2)
	if err != nil {
		t.Fatalf("ShiftEnsembleSamplesBySamplesFrequencyDomain returned error: %v", err)
	}

	if out.TraceCount != in.TraceCount || out.SamplesPerTrace != in.SamplesPerTrace || out.TotalImageSamples != in.TotalImageSamples {
		t.Fatalf("metadata mismatch: got (%d,%d,%d) want (%d,%d,%d)", out.TraceCount, out.SamplesPerTrace, out.TotalImageSamples, in.TraceCount, in.SamplesPerTrace, in.TotalImageSamples)
	}

	want0 := []float32{0, 0, 1, 2, 3, 4, 5, 6}
	want1 := []float32{0, 0, 10, 20, 30, 40, 50, 60}
	for i := range want0 {
		if math.Abs(float64(out.Traces[0][i]-want0[i])) > 1e-5 {
			t.Fatalf("trace0 mismatch at %d: got %v want %v", i, out.Traces[0][i], want0[i])
		}
		if math.Abs(float64(out.Traces[1][i]-want1[i])) > 1e-5 {
			t.Fatalf("trace1 mismatch at %d: got %v want %v", i, out.Traces[1][i], want1[i])
		}
	}
}

func TestShiftEnsembleSamplesBySamplesFrequencyDomain_InvalidShift(t *testing.T) {
	in := EnsembleSamples{Traces: [][]float32{{1, 2, 3, 4}}}
	if _, err := ShiftEnsembleSamplesBySamplesFrequencyDomain(in, float32(math.NaN())); err == nil {
		t.Fatalf("expected error for NaN shift")
	}
}
