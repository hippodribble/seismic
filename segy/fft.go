package segy

import (
	"fmt"
	"math"
	"math/cmplx"
)

// ShiftTraceBySamplesFrequencyDomain shifts a real-valued trace by a fractional
// number of samples using phase multiplication in the frequency domain.
//
// Positive shiftSamples delays the trace in time (moves events to larger sample
// indices). Circular wrap is removed by zeroing the wrapped edge so the result
// is truncated instead of periodic.
func ShiftTraceBySamplesFrequencyDomain(samples []float32, shiftSamples float64) ([]float32, error) {
	if len(samples) == 0 {
		return []float32{}, nil
	}
	if math.IsNaN(shiftSamples) || math.IsInf(shiftSamples, 0) {
		return nil, fmt.Errorf("shift samples must be finite")
	}

	n := len(samples)
	padN := nextPowerOfTwo(n)
	if padN < 1 {
		return nil, fmt.Errorf("invalid trace length %d", n)
	}

	in := make([]complex128, padN)
	for i := range samples {
		in[i] = complex(float64(samples[i]), 0)
	}

	spectrum := fft(in)
	for k := range spectrum {
		phase := -2.0 * math.Pi * shiftSamples * float64(k) / float64(padN)
		spectrum[k] *= cmplx.Exp(complex(0, phase))
	}

	outComplex := ifft(spectrum)
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = float32(real(outComplex[i]))
	}

	// Remove periodic wrap by zeroing the edge where wrapped energy appears.
	wrapped := int(math.Ceil(math.Abs(shiftSamples)))
	if wrapped > n {
		wrapped = n
	}
	if shiftSamples > 0 {
		for i := 0; i < wrapped; i++ {
			out[i] = 0
		}
	} else if shiftSamples < 0 {
		for i := n - wrapped; i < n; i++ {
			out[i] = 0
		}
	}

	return out, nil
}

// ShiftEnsembleSamplesBySamplesFrequencyDomain shifts all traces in an ensemble
// by a fractional number of samples using frequency-domain phase multiplication.
//
// Positive shiftSamples delays traces in time. Wrapped edges are zeroed, so the
// result is truncated instead of circular.
func ShiftEnsembleSamplesBySamplesFrequencyDomain(ensemble EnsembleSamples, shiftSamples float32) (EnsembleSamples, error) {
	out := EnsembleSamples{
		Traces:            make([][]float32, len(ensemble.Traces)),
		TraceCount:        ensemble.TraceCount,
		SamplesPerTrace:   ensemble.SamplesPerTrace,
		TotalImageSamples: ensemble.TotalImageSamples,
	}

	shift := float64(shiftSamples)
	for i := range ensemble.Traces {
		shifted, err := ShiftTraceBySamplesFrequencyDomain(ensemble.Traces[i], shift)
		if err != nil {
			return EnsembleSamples{}, fmt.Errorf("shift trace %d: %w", i, err)
		}
		out.Traces[i] = shifted
	}

	if out.TraceCount == 0 {
		out.TraceCount = len(out.Traces)
	}
	if out.SamplesPerTrace == 0 && len(out.Traces) > 0 {
		out.SamplesPerTrace = len(out.Traces[0])
	}
	if out.TotalImageSamples == 0 && out.TraceCount > 0 && out.SamplesPerTrace > 0 {
		out.TotalImageSamples = int64(out.TraceCount) * int64(out.SamplesPerTrace)
	}

	return out, nil
}
