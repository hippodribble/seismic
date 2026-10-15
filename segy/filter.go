package segy

import (
	"fmt"
	"math"
)

// ApplyHighCutToCDPGather applies a zero-phase high-cut filter to all traces in a CDP gather.
// The filter is a 2nd-order Butterworth low-pass applied forward and backward.
// A new gather is returned; the input gather is not modified.
func ApplyHighCutToCDPGather(gather CDPGather, highCutHz float64) (CDPGather, error) {
	if gather.SampleIntervalMS <= 0 {
		return CDPGather{}, fmt.Errorf("sample interval must be > 0 ms")
	}
	if highCutHz <= 0 {
		return CDPGather{}, fmt.Errorf("high-cut frequency must be > 0 Hz")
	}

	fs := 1000.0 / gather.SampleIntervalMS
	nyquist := fs * 0.5
	if highCutHz >= nyquist {
		return CDPGather{}, fmt.Errorf("high-cut frequency %g Hz must be less than Nyquist %g Hz", highCutHz, nyquist)
	}

	b0, b1, b2, a1, a2 := butterworthLowPassBiquad(highCutHz, fs)

	out := CDPGather{
		CDP:              gather.CDP,
		SampleIntervalMS: gather.SampleIntervalMS,
		Traces:           make([]GatherTrace, len(gather.Traces)),
	}

	for i, tr := range gather.Traces {
		filtered := filtfiltBiquad(tr.Samples, b0, b1, b2, a1, a2)
		out.Traces[i] = GatherTrace{
			Header:  tr.Header,
			Samples: filtered,
		}
	}

	return out, nil
}

func butterworthLowPassBiquad(fcHz, fsHz float64) (b0, b1, b2, a1, a2 float64) {
	// RBJ-style biquad design with Butterworth Q for a 2nd-order low-pass section.
	q := 1.0 / math.Sqrt2
	w0 := 2.0 * math.Pi * fcHz / fsHz
	cosw0 := math.Cos(w0)
	sinw0 := math.Sin(w0)
	alpha := sinw0 / (2.0 * q)

	b0u := (1.0 - cosw0) * 0.5
	b1u := 1.0 - cosw0
	b2u := (1.0 - cosw0) * 0.5
	a0u := 1.0 + alpha
	a1u := -2.0 * cosw0
	a2u := 1.0 - alpha

	b0 = b0u / a0u
	b1 = b1u / a0u
	b2 = b2u / a0u
	a1 = a1u / a0u
	a2 = a2u / a0u
	return
}

func filtfiltBiquad(in []float32, b0, b1, b2, a1, a2 float64) []float32 {
	if len(in) == 0 {
		return []float32{}
	}

	forward := biquadFilter(in, b0, b1, b2, a1, a2)
	reverseInPlace(forward)
	backward := biquadFilter(forward, b0, b1, b2, a1, a2)
	reverseInPlace(backward)
	return backward
}

func biquadFilter(in []float32, b0, b1, b2, a1, a2 float64) []float32 {
	out := make([]float32, len(in))

	x1 := 0.0
	x2 := 0.0
	y1 := 0.0
	y2 := 0.0

	for i := range in {
		x0 := float64(in[i])
		y0 := b0*x0 + b1*x1 + b2*x2 - a1*y1 - a2*y2
		out[i] = float32(y0)

		x2 = x1
		x1 = x0
		y2 = y1
		y1 = y0
	}

	return out
}

func reverseInPlace(a []float32) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}
