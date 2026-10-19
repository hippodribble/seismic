package segy

import (
	"fmt"
)

// Autocorrelation computes the autocorrelation of a real-valued trace.
//
// With twoSided=false, the result is one-sided and contains lags from 0 to maxLag.
// With twoSided=true, the result is two-sided and contains lags from -maxLag to +maxLag.
// The calculation is unnormalized and uses the standard raw sum definition:
// r[k] = sum_i x[i] * x[i+k] for valid i.
func Autocorrelation(trace []float32, maxLag int, twoSided bool) ([]float32, error) {
	if maxLag < 0 {
		return nil, fmt.Errorf("max lag must be >= 0")
	}
	if len(trace) == 0 {
		return nil, fmt.Errorf("trace must not be empty")
	}
	if maxLag >= len(trace) {
		maxLag = len(trace) - 1
	}

	if !twoSided {
		out := make([]float32, maxLag+1)
		for lag := 0; lag <= maxLag; lag++ {
			var acc float64
			for i := 0; i+lag < len(trace); i++ {
				acc += float64(trace[i]) * float64(trace[i+lag])
			}
			out[lag] = float32(acc)
		}
		return out, nil
	}

	out := make([]float32, 2*maxLag+1)
	for lag := -maxLag; lag <= maxLag; lag++ {
		var acc float64
		for i := 0; i < len(trace); i++ {
			j := i + lag
			if j < 0 || j >= len(trace) {
				continue
			}
			acc += float64(trace[i]) * float64(trace[j])
		}
		out[lag+maxLag] = float32(acc)
	}
	return out, nil
}

// NormalizedAutocorrelation computes the normalized autocorrelation of a real-valued trace.
// The zero-lag value is normalized to 1.0, and the result is scaled by the zero-lag energy.
// If the zero-lag energy is zero, a zero slice is returned.
func NormalizedAutocorrelation(trace []float32, maxLag int, twoSided bool) ([]float32, error) {
	raw, err := Autocorrelation(trace, maxLag, twoSided)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}

	lag0Index := 0
	if twoSided {
		lag0Index = maxLag
		if maxLag >= len(trace) {
			lag0Index = len(trace) - 1
		}
	}
	if lag0Index >= len(raw) {
		lag0Index = len(raw) - 1
	}

	divisor := float64(raw[lag0Index])
	if divisor == 0 {
		zero := make([]float32, len(raw))
		return zero, nil
	}

	out := make([]float32, len(raw))
	for i := range raw {
		out[i] = float32(float64(raw[i]) / divisor)
	}
	return out, nil
}

// AutocorrelationForTraceSet computes autocorrelation for each trace in a generic trace ensemble.
// This works for CDP gathers, shot records, stack ensembles, or any collection of traces with a Samples slice.
func AutocorrelationForTraceSet(traces []GatherTrace, maxLag int, twoSided bool) ([][]float32, error) {
	if maxLag < 0 {
		return nil, fmt.Errorf("max lag must be >= 0")
	}
	if len(traces) == 0 {
		return nil, fmt.Errorf("trace set must not be empty")
	}

	out := make([][]float32, len(traces))
	for i, tr := range traces {
		ac, err := Autocorrelation(tr.Samples, maxLag, twoSided)
		if err != nil {
			return nil, fmt.Errorf("trace %d: %w", i, err)
		}
		out[i] = ac
	}
	return out, nil
}

// NormalizedAutocorrelationForTraceSet computes normalized autocorrelation for each trace in a generic trace ensemble.
func NormalizedAutocorrelationForTraceSet(traces []GatherTrace, maxLag int, twoSided bool) ([][]float32, error) {
	if maxLag < 0 {
		return nil, fmt.Errorf("max lag must be >= 0")
	}
	if len(traces) == 0 {
		return nil, fmt.Errorf("trace set must not be empty")
	}

	out := make([][]float32, len(traces))
	for i, tr := range traces {
		ac, err := NormalizedAutocorrelation(tr.Samples, maxLag, twoSided)
		if err != nil {
			return nil, fmt.Errorf("trace %d: %w", i, err)
		}
		out[i] = ac
	}
	return out, nil
}
