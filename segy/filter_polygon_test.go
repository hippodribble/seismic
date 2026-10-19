package segy

import (
	"math"
	"testing"
)

func TestApplyPolygonalFrequencyFilter_AllPass(t *testing.T) {
	in := []float32{0, 1, -2, 3, -1, 2, 0.5, -0.5}
	out, err := ApplyPolygonalFrequencyFilter(in, 4.0, []float64{0, 25, 50, 125}, []float64{1, 1, 1, 1})
	if err != nil {
		t.Fatalf("ApplyPolygonalFrequencyFilter returned error: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("length mismatch: got %d want %d", len(out), len(in))
	}
	for i := range in {
		if math.Abs(float64(out[i]-in[i])) > 1e-6 {
			t.Fatalf("all-pass output mismatch at index %d: got %v want %v", i, out[i], in[i])
		}
	}
}

func TestApplyPolygonalFrequencyFilter_ZeroResponse(t *testing.T) {
	in := []float32{0.5, -0.25, 0.75, -1, 0.25, -0.5, 1, -0.75}
	out, err := ApplyPolygonalFrequencyFilter(in, 4.0, []float64{0, 25, 50, 125}, []float64{0, 0, 0, 0})
	if err != nil {
		t.Fatalf("ApplyPolygonalFrequencyFilter returned error: %v", err)
	}
	for i := range out {
		if out[i] != 0 {
			t.Fatalf("expected zero output, got %v at index %d", out[i], i)
		}
	}
}
