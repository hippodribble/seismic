package segy

import (
	"fmt"
	"math"
	"math/cmplx"
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

// ApplyPolygonalFrequencyFilter applies a zero-phase, piecewise-linear amplitude mask in the frequency domain.
// The filter is defined by four frequency/amplitude pairs, where amplitudes are in the range [0,1].
// The spectrum is multiplied by the interpolated amplitude response and then transformed back to the time domain.
// The input is treated as real-valued data, so a real FFT is used internally.
func ApplyPolygonalFrequencyFilter(samples []float32, sampleIntervalMS float64, frequencies []float64, amplitudes []float64) ([]float32, error) {
	if sampleIntervalMS <= 0 {
		return nil, fmt.Errorf("sample interval must be > 0 ms")
	}
	if len(samples) == 0 {
		return []float32{}, nil
	}
	if len(frequencies) != 4 || len(amplitudes) != 4 {
		return nil, fmt.Errorf("polygonal filter requires exactly four frequency and amplitude values")
	}

	freqs, gain, err := validatePolygonalFilter(frequencies, amplitudes)
	if err != nil {
		return nil, err
	}

	fsHz := 1000.0 / sampleIntervalMS
	paddingN := nextPowerOfTwo(len(samples))
	if paddingN < 1 {
		paddingN = 1
	}

	in := make([]complex128, paddingN)
	for i := range samples {
		in[i] = complex(float64(samples[i]), 0)
	}

	spectrum := fft(in)
	response := make([]float64, paddingN)
	for k := 0; k < paddingN; k++ {
		freqHz := float64(k) * fsHz / float64(paddingN)
		if k > paddingN/2 {
			freqHz = fsHz - freqHz
		}
		response[k] = polygonAmplitude(freqHz, freqs, gain)
	}

	filtered := make([]complex128, paddingN)
	for k := range spectrum {
		filtered[k] = spectrum[k] * complex(response[k], 0)
	}

	inv := ifft(filtered)
	out := make([]float32, len(samples))
	for i := range out {
		out[i] = float32(real(inv[i]))
	}
	return out, nil
}

// ApplyPolygonalFrequencyFilterToCDPGather applies the polygonal frequency response to every trace in a CDP gather.
func ApplyPolygonalFrequencyFilterToCDPGather(gather CDPGather, frequencies []float64, amplitudes []float64) (CDPGather, error) {
	if gather.SampleIntervalMS <= 0 {
		return CDPGather{}, fmt.Errorf("sample interval must be > 0 ms")
	}

	out := CDPGather{
		CDP:              gather.CDP,
		SampleIntervalMS: gather.SampleIntervalMS,
		Traces:           make([]GatherTrace, len(gather.Traces)),
	}
	for i, tr := range gather.Traces {
		filtered, err := ApplyPolygonalFrequencyFilter(tr.Samples, gather.SampleIntervalMS, frequencies, amplitudes)
		if err != nil {
			return CDPGather{}, err
		}
		out.Traces[i] = GatherTrace{Header: tr.Header, Samples: filtered}
	}
	return out, nil
}

func validatePolygonalFilter(frequencies, amplitudes []float64) ([]float64, []float64, error) {
	if len(frequencies) != 4 || len(amplitudes) != 4 {
		return nil, nil, fmt.Errorf("polygonal filter requires exactly four frequency and amplitude values")
	}
	freqs := append([]float64(nil), frequencies...)
	amp := append([]float64(nil), amplitudes...)

	for i := range amp {
		if amp[i] < 0 || amp[i] > 1 {
			return nil, nil, fmt.Errorf("polygonal filter amplitudes must be in the range [0, 1]")
		}
	}
	for i := 1; i < len(freqs); i++ {
		if freqs[i] < freqs[i-1] {
			return nil, nil, fmt.Errorf("polygonal filter frequencies must be in ascending order")
		}
	}
	for i := range freqs {
		if freqs[i] < 0 {
			return nil, nil, fmt.Errorf("polygonal filter frequencies must be non-negative")
		}
	}
	return freqs, amp, nil
}

func polygonAmplitude(freqHz float64, frequencies, amplitudes []float64) float64 {
	if len(frequencies) == 0 || len(amplitudes) == 0 {
		return 1
	}
	if freqHz <= frequencies[0] {
		return amplitudes[0]
	}
	if freqHz >= frequencies[len(frequencies)-1] {
		return amplitudes[len(amplitudes)-1]
	}

	for i := 0; i < len(frequencies)-1; i++ {
		f0, f1 := frequencies[i], frequencies[i+1]
		if freqHz >= f0 && freqHz <= f1 {
			if f1 == f0 {
				return amplitudes[i]
			}
			t := (freqHz - f0) / (f1 - f0)
			return amplitudes[i] + t*(amplitudes[i+1]-amplitudes[i])
		}
	}
	return amplitudes[len(amplitudes)-1]
}

func nextPowerOfTwo(v int) int {
	if v <= 1 {
		return 1
	}
	p := 1
	for p < v {
		p <<= 1
	}
	return p
}

func fft(x []complex128) []complex128 {
	n := len(x)
	if n == 1 {
		return append([]complex128(nil), x...)
	}
	if n&(n-1) != 0 {
		panic("fft requires power-of-two length")
	}
	arr := append([]complex128(nil), x...)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			arr[i], arr[j] = arr[j], arr[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		angle := -2.0 * math.Pi / float64(size)
		wlen := complex(math.Cos(angle), math.Sin(angle))
		for i := 0; i < n; i += size {
			w := complex(1, 0)
			for j := 0; j < size/2; j++ {
				u := arr[i+j]
				v := arr[i+j+size/2] * w
				arr[i+j] = u + v
				arr[i+j+size/2] = u - v
				w *= wlen
			}
		}
	}
	return arr
}

func ifft(x []complex128) []complex128 {
	n := len(x)
	if n == 1 {
		return append([]complex128(nil), x...)
	}
	conj := make([]complex128, n)
	for i := range x {
		conj[i] = cmplx.Conj(x[i])
	}
	transformed := fft(conj)
	out := make([]complex128, n)
	for i := range transformed {
		out[i] = cmplx.Conj(transformed[i]) / complex(float64(n), 0)
	}
	return out
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
