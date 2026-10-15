package segy

import (
	"fmt"
	"math"
)

// VelocitySample stores one time/velocity pair for a stacking velocity function.
// Time is expressed in milliseconds.
type VelocitySample struct {
	TimeMS           float64
	StackingVelocity float64
}

// VelocityFunction stores a stacking velocity function tied to one CDP value.
// Samples should be ordered by increasing time.
type VelocityFunction struct {
	CDP     int64
	Samples []VelocitySample
}

// VelocityExtrapolationMode controls velocity behavior outside sampled times.
type VelocityExtrapolationMode uint8

const (
	// VelocityExtrapolateHold holds the first/last sampled velocity outside range.
	VelocityExtrapolateHold VelocityExtrapolationMode = iota
	// VelocityExtrapolateLinear linearly extrapolates using end segments.
	VelocityExtrapolateLinear
)

// NMOOptions controls normal moveout correction behavior.
type NMOOptions struct {
	// ExtrapolationMode selects how velocity is evaluated before/after sampled times.
	ExtrapolationMode VelocityExtrapolationMode

	// MaxStretchFraction mutes output samples whose NMO stretch exceeds the threshold.
	// Stretch is defined as (tn - t0) / t0. Values <= 0 disable stretch muting.
	MaxStretchFraction float64
}

// GatherTrace stores one trace in a gather.
// Header supplies offset and other metadata; Samples holds amplitude values.
type GatherTrace struct {
	Header  TraceHeader
	Samples []float32
}

// CDPGather stores traces that share a common CDP value.
// SampleIntervalMS is the trace sampling interval in milliseconds.
type CDPGather struct {
	CDP              int64
	SampleIntervalMS float64
	Traces           []GatherTrace
}

// StretchMuteMask stores stretch-mute decisions for one gather.
// Muted[traceIndex][sampleIndex] is true when that output sample was muted by stretch criteria.
type StretchMuteMask struct {
	Muted [][]bool
}

// StackedTrace stores one stacked trace and minimal gather metadata.
type StackedTrace struct {
	CDP              int64
	SampleIntervalMS float64
	Samples          []float32
}

// SemblanceTrace stores semblance values versus time for one trial velocity.
type SemblanceTrace struct {
	Velocity float64
	Samples  []float32
}

// SemblanceGather stores semblance traces across trial velocities for one CDP gather.
// Trial velocities are ordered from lower to higher velocity.
type SemblanceGather struct {
	CDP              int64
	SampleIntervalMS float64
	Traces           []SemblanceTrace
}

// VelocityAxis returns trial velocities in gather order (low to high).
func (g SemblanceGather) VelocityAxis() []float64 {
	axis := make([]float64, len(g.Traces))
	for i, tr := range g.Traces {
		axis[i] = tr.Velocity
	}
	return axis
}

func defaultNMOOptions() NMOOptions {
	return NMOOptions{
		ExtrapolationMode:  VelocityExtrapolateHold,
		MaxStretchFraction: 0,
	}
}

// ApplyNMOToCDPGather applies hyperbolic normal moveout correction to a CDP gather.
// It returns a new gather with the same trace order and header values.
func ApplyNMOToCDPGather(gather CDPGather, velocity VelocityFunction) (CDPGather, error) {
	return ApplyNMOToCDPGatherWithOptions(gather, velocity, defaultNMOOptions())
}

// ApplyNMOToCDPGatherWithOptions applies hyperbolic normal moveout correction to a CDP gather.
// It returns a new gather with the same trace order and header values.
func ApplyNMOToCDPGatherWithOptions(gather CDPGather, velocity VelocityFunction, options NMOOptions) (CDPGather, error) {
	corrected, _, err := ApplyNMOToCDPGatherWithOptionsAndMask(gather, velocity, options)
	if err != nil {
		return CDPGather{}, err
	}
	return corrected, nil
}

// ApplyNMOToCDPGatherWithOptionsAndMask applies NMO and also returns the stretch-mute mask.
// The returned mask aligns with gather trace/sample indices.
func ApplyNMOToCDPGatherWithOptionsAndMask(gather CDPGather, velocity VelocityFunction, options NMOOptions) (CDPGather, StretchMuteMask, error) {
	if gather.SampleIntervalMS <= 0 {
		return CDPGather{}, StretchMuteMask{}, fmt.Errorf("sample interval must be > 0 ms")
	}
	if len(velocity.Samples) == 0 {
		return CDPGather{}, StretchMuteMask{}, fmt.Errorf("velocity function has no samples")
	}
	if options.MaxStretchFraction < 0 {
		return CDPGather{}, StretchMuteMask{}, fmt.Errorf("max stretch fraction must be >= 0")
	}

	samples, err := validateAndCopyVelocitySamples(velocity.Samples)
	if err != nil {
		return CDPGather{}, StretchMuteMask{}, err
	}

	normalizedVelocity := velocity
	normalizedVelocity.Samples = samples

	if velocity.CDP != 0 && gather.CDP != 0 && velocity.CDP != gather.CDP {
		return CDPGather{}, StretchMuteMask{}, fmt.Errorf("velocity CDP %d does not match gather CDP %d", velocity.CDP, gather.CDP)
	}

	out := CDPGather{
		CDP:              gather.CDP,
		SampleIntervalMS: gather.SampleIntervalMS,
		Traces:           make([]GatherTrace, len(gather.Traces)),
	}
	mask := StretchMuteMask{Muted: make([][]bool, len(gather.Traces))}

	for i, tr := range gather.Traces {
		if int64(tr.Header.CDP) != 0 && gather.CDP != 0 && int64(tr.Header.CDP) != gather.CDP {
			return CDPGather{}, StretchMuteMask{}, fmt.Errorf("trace %d has CDP %d, expected %d", i, tr.Header.CDP, gather.CDP)
		}

		corrected, muted, err := applyNMOToTrace(tr.Samples, float64(tr.Header.SourceReceiverOffset), gather.SampleIntervalMS, normalizedVelocity, options)
		if err != nil {
			return CDPGather{}, StretchMuteMask{}, fmt.Errorf("trace %d: %w", i, err)
		}

		out.Traces[i] = GatherTrace{
			Header:  tr.Header,
			Samples: corrected,
		}
		mask.Muted[i] = muted
	}

	return out, mask, nil
}

// BuildSemblanceGatherFromCDP computes a semblance gather from one CDP gather.
// Trial velocities are sampled uniformly in squared slowness (1/v^2), not in velocity.
// The returned gather is ordered from low velocity (start) to high velocity (end).
func BuildSemblanceGatherFromCDP(gather CDPGather, minVelocity, maxVelocity, velocityStep float64) (SemblanceGather, error) {
	return BuildSemblanceGatherFromCDPWithSmoothingPowerAndStretchMute(gather, minVelocity, maxVelocity, velocityStep, 1, 1.0, 0)
}

// BuildSemblanceGatherFromCDPWithSmoothing computes a semblance gather from one CDP gather.
// For each trial velocity, it forms separate numerator and denominator traces over time,
// applies running-average smoothing with windowLength samples to each trace, then divides
// smoothed numerator by smoothed denominator sample-by-sample.
// Trial velocities are sampled uniformly in squared slowness (1/v^2), not in velocity.
// The returned gather is ordered from low velocity (start) to high velocity (end).
func BuildSemblanceGatherFromCDPWithSmoothing(gather CDPGather, minVelocity, maxVelocity, velocityStep float64, windowLength int) (SemblanceGather, error) {
	return BuildSemblanceGatherFromCDPWithSmoothingPowerAndStretchMute(gather, minVelocity, maxVelocity, velocityStep, windowLength, 1.0, 0)
}

// BuildSemblanceGatherFromCDPWithSmoothingAndPower computes a semblance gather from one CDP gather.
// For each trial velocity, it forms separate numerator and denominator traces over time,
// applies running-average smoothing with windowLength samples to each trace, then divides
// smoothed numerator by smoothed denominator sample-by-sample and raises the result to
// semblancePower.
// Trial velocities are sampled uniformly in squared slowness (1/v^2), not in velocity.
// The returned gather is ordered from low velocity (start) to high velocity (end).
func BuildSemblanceGatherFromCDPWithSmoothingAndPower(gather CDPGather, minVelocity, maxVelocity, velocityStep float64, windowLength int, semblancePower float64) (SemblanceGather, error) {
	return BuildSemblanceGatherFromCDPWithSmoothingPowerAndStretchMute(gather, minVelocity, maxVelocity, velocityStep, windowLength, semblancePower, 0)
}

// BuildSemblanceGatherFromCDPWithSmoothingPowerAndStretchMute computes a semblance gather
// with optional stretch mute per trial velocity. When maxStretchFraction > 0, samples whose
// stretch ((tn-t0)/t0) exceeds the threshold are excluded from semblance accumulation.
func BuildSemblanceGatherFromCDPWithSmoothingPowerAndStretchMute(gather CDPGather, minVelocity, maxVelocity, velocityStep float64, windowLength int, semblancePower, maxStretchFraction float64) (SemblanceGather, error) {
	if gather.SampleIntervalMS <= 0 {
		return SemblanceGather{}, fmt.Errorf("sample interval must be > 0 ms")
	}
	if len(gather.Traces) == 0 {
		return SemblanceGather{}, fmt.Errorf("gather has no traces")
	}
	if windowLength <= 0 {
		return SemblanceGather{}, fmt.Errorf("semblance smoothing window must be > 0")
	}
	if semblancePower <= 0 {
		return SemblanceGather{}, fmt.Errorf("semblance power must be > 0")
	}
	if maxStretchFraction < 0 {
		return SemblanceGather{}, fmt.Errorf("max stretch fraction must be >= 0")
	}
	if minVelocity <= 0 || maxVelocity <= 0 {
		return SemblanceGather{}, fmt.Errorf("min and max velocity must be > 0")
	}
	if maxVelocity < minVelocity {
		return SemblanceGather{}, fmt.Errorf("max velocity must be >= min velocity")
	}
	if velocityStep <= 0 {
		return SemblanceGather{}, fmt.Errorf("velocity step must be > 0")
	}

	sampleCount := len(gather.Traces[0].Samples)
	if sampleCount == 0 {
		return SemblanceGather{}, fmt.Errorf("gather traces have zero samples")
	}
	for i := 1; i < len(gather.Traces); i++ {
		if len(gather.Traces[i].Samples) != sampleCount {
			return SemblanceGather{}, fmt.Errorf("trace %d sample count %d does not match trace 0 sample count %d", i, len(gather.Traces[i].Samples), sampleCount)
		}
	}

	trialCount := estimateTrialVelocityCount(minVelocity, maxVelocity, velocityStep)
	trialVelocities := buildSquaredSlownessVelocities(minVelocity, maxVelocity, trialCount)

	out := SemblanceGather{
		CDP:              gather.CDP,
		SampleIntervalMS: gather.SampleIntervalMS,
		Traces:           make([]SemblanceTrace, len(trialVelocities)),
	}

	for iv, velocity := range trialVelocities {
		numerator := make([]float32, sampleCount)
		denominator := make([]float32, sampleCount)
		semblance := make([]float32, sampleCount)

		for it := 0; it < sampleCount; it++ {
			t0MS := float64(it) * gather.SampleIntervalMS
			t0Sec := t0MS / 1000.0

			sum := 0.0
			sumSq := 0.0
			n := 0

			for _, tr := range gather.Traces {
				offset := math.Abs(float64(tr.Header.SourceReceiverOffset))
				tnSec := math.Sqrt(t0Sec*t0Sec + (offset*offset)/(velocity*velocity))
				tnMS := tnSec * 1000.0

				if maxStretchFraction > 0 {
					if t0MS <= 0 {
						if tnMS > 0 {
							continue
						}
					} else {
						stretch := (tnMS - t0MS) / t0MS
						if stretch > maxStretchFraction {
							continue
						}
					}
				}

				a, ok := sampleTraceLinearIfInRange(tr.Samples, tnMS/gather.SampleIntervalMS)
				if !ok {
					continue
				}

				sum += a
				sumSq += a * a
				n++
			}

			if n >= 2 && sumSq > 0 {
				numerator[it] = float32((sum * sum) / float64(n))
				denominator[it] = float32(sumSq)
			}
		}

		smoothedNumerator, err := SmoothTraceTimeRunningAverage(numerator, windowLength)
		if err != nil {
			return SemblanceGather{}, fmt.Errorf("velocity %.3f numerator smoothing failed: %w", velocity, err)
		}
		smoothedDenominator, err := SmoothTraceTimeRunningAverage(denominator, windowLength)
		if err != nil {
			return SemblanceGather{}, fmt.Errorf("velocity %.3f denominator smoothing failed: %w", velocity, err)
		}

		for it := 0; it < sampleCount; it++ {
			den := float64(smoothedDenominator[it])
			if den <= 0 {
				continue
			}
			s := float64(smoothedNumerator[it]) / den
			if s < 0 {
				s = 0
			}
			if s > 1 {
				s = 1
			}
			if semblancePower != 1.0 {
				s = math.Pow(s, semblancePower)
			}
			semblance[it] = float32(s)
		}

		out.Traces[iv] = SemblanceTrace{Velocity: velocity, Samples: semblance}
	}

	return out, nil
}

// StackCDPGatherMean stacks a gather horizontally using arithmetic mean by sample.
// If muteMask is provided, muted samples are excluded from the sample-wise count N.
// When no unmuted samples contribute at a time sample, stacked output is set to zero.
func StackCDPGatherMean(gather CDPGather, muteMask *StretchMuteMask) (StackedTrace, error) {
	if gather.SampleIntervalMS <= 0 {
		return StackedTrace{}, fmt.Errorf("sample interval must be > 0 ms")
	}
	if len(gather.Traces) == 0 {
		return StackedTrace{}, fmt.Errorf("gather has no traces")
	}

	sampleCount := len(gather.Traces[0].Samples)
	for i := 1; i < len(gather.Traces); i++ {
		if len(gather.Traces[i].Samples) != sampleCount {
			return StackedTrace{}, fmt.Errorf("trace %d sample count %d does not match trace 0 sample count %d", i, len(gather.Traces[i].Samples), sampleCount)
		}
	}

	if muteMask != nil {
		if len(muteMask.Muted) != len(gather.Traces) {
			return StackedTrace{}, fmt.Errorf("mute mask trace count %d does not match gather trace count %d", len(muteMask.Muted), len(gather.Traces))
		}
		for i := range muteMask.Muted {
			if len(muteMask.Muted[i]) != sampleCount {
				return StackedTrace{}, fmt.Errorf("mute mask trace %d sample count %d does not match gather sample count %d", i, len(muteMask.Muted[i]), sampleCount)
			}
		}
	}

	stacked := make([]float32, sampleCount)
	for is := 0; is < sampleCount; is++ {
		sum := 0.0
		n := 0

		for it := range gather.Traces {
			if muteMask != nil && muteMask.Muted[it][is] {
				continue
			}
			sum += float64(gather.Traces[it].Samples[is])
			n++
		}

		if n > 0 {
			stacked[is] = float32(sum / float64(n))
		} else {
			stacked[is] = 0
		}
	}

	return StackedTrace{
		CDP:              gather.CDP,
		SampleIntervalMS: gather.SampleIntervalMS,
		Samples:          stacked,
	}, nil
}

// BuildCDPGatherByCDP builds one decoded CDP gather using an exact CDP filter only.
// When sortByOffset is true, traces are sorted by SourceReceiverOffset.
// When sortByOffset is false, trace order is preserved from the catalog selection.
func (f *File) BuildCDPGatherByCDP(cdp int64, sortByOffset bool) (CDPGather, error) {
	if f == nil {
		return CDPGather{}, fmt.Errorf("nil segy file")
	}

	catalog := f.TraceCatalog
	if catalog == nil {
		built, err := f.BuildTraceCatalog()
		if err != nil {
			return CDPGather{}, err
		}
		catalog = built
	}

	entries := catalog.FindEqual(TraceHeaderFieldCDP, cdp)
	if len(entries) == 0 {
		return CDPGather{}, fmt.Errorf("no traces found for CDP %d", cdp)
	}

	if sortByOffset {
		if err := SortTraceIndexEntries(entries, Asc(TraceHeaderFieldOFFSET)); err != nil {
			return CDPGather{}, err
		}
	}

	return f.BuildCDPGatherFromEntries(entries)
}

// BuildCDPGatherFromEntries reads and decodes traces referenced by entries into one CDP gather.
// All entries must belong to one CDP and share one sample interval.
func (f *File) BuildCDPGatherFromEntries(entries []TraceIndexEntry) (CDPGather, error) {
	if f == nil {
		return CDPGather{}, fmt.Errorf("nil segy file")
	}
	if len(entries) == 0 {
		return CDPGather{}, fmt.Errorf("entries is empty")
	}

	gather := CDPGather{Traces: make([]GatherTrace, len(entries))}

	var (
		cdpSet bool
		dtSet  bool
		cdp    int64
		dtMS   float64
	)

	for i, entry := range entries {
		header, samples, err := f.ReadDecodedTraceFromIndex(entry)
		if err != nil {
			return CDPGather{}, fmt.Errorf("read decoded trace %d: %w", i, err)
		}

		traceCDP := int64(header.CDP)
		if !cdpSet {
			cdp = traceCDP
			cdpSet = true
		} else if traceCDP != cdp {
			return CDPGather{}, fmt.Errorf("entries contain multiple CDP values (%d and %d)", cdp, traceCDP)
		}

		traceDtMS, err := resolveTraceSampleIntervalMS(header, f.BinaryHeader)
		if err != nil {
			return CDPGather{}, fmt.Errorf("trace %d: %w", i, err)
		}
		if !dtSet {
			dtMS = traceDtMS
			dtSet = true
		} else if math.Abs(traceDtMS-dtMS) > 1e-9 {
			return CDPGather{}, fmt.Errorf("entries contain mixed sample intervals (%g ms and %g ms)", dtMS, traceDtMS)
		}

		gather.Traces[i] = GatherTrace{Header: header, Samples: samples}
	}

	gather.CDP = cdp
	gather.SampleIntervalMS = dtMS
	return gather, nil
}

func applyNMOToTrace(samples []float32, offset, dtMS float64, velocity VelocityFunction, options NMOOptions) ([]float32, []bool, error) {
	if dtMS <= 0 {
		return nil, nil, fmt.Errorf("sample interval must be > 0")
	}
	if len(samples) == 0 {
		return []float32{}, []bool{}, nil
	}

	out := make([]float32, len(samples))
	muted := make([]bool, len(samples))
	x := math.Abs(offset)

	for i := range out {
		t0MS := float64(i) * dtMS
		v, err := stackingVelocityAtTimeMS(velocity, t0MS, options.ExtrapolationMode)
		if err != nil {
			return nil, nil, err
		}
		if v <= 0 {
			return nil, nil, fmt.Errorf("non-positive stacking velocity %g at t0=%g ms", v, t0MS)
		}

		t0Sec := t0MS / 1000.0
		tnSec := math.Sqrt(t0Sec*t0Sec + (x*x)/(v*v))
		tnMS := tnSec * 1000.0

		if options.MaxStretchFraction > 0 {
			if t0MS <= 0 {
				if tnMS > 0 {
					out[i] = 0
					muted[i] = true
					continue
				}
			} else {
				stretch := (tnMS - t0MS) / t0MS
				if stretch > options.MaxStretchFraction {
					out[i] = 0
					muted[i] = true
					continue
				}
			}
		}

		out[i] = sampleTraceLinear(samples, tnMS/dtMS)
	}

	return out, muted, nil
}

func validateAndCopyVelocitySamples(samples []VelocitySample) ([]VelocitySample, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("velocity function has no samples")
	}

	out := make([]VelocitySample, len(samples))
	copy(out, samples)

	for i := range out {
		if out[i].StackingVelocity <= 0 {
			return nil, fmt.Errorf("velocity sample %d has non-positive velocity %g", i, out[i].StackingVelocity)
		}
		if i > 0 && out[i].TimeMS < out[i-1].TimeMS {
			return nil, fmt.Errorf("velocity samples must be ordered by increasing time")
		}
	}

	return out, nil
}

func stackingVelocityAtTimeMS(vf VelocityFunction, timeMS float64, mode VelocityExtrapolationMode) (float64, error) {
	if len(vf.Samples) == 0 {
		return 0, fmt.Errorf("velocity function has no samples")
	}

	if len(vf.Samples) == 1 {
		return vf.Samples[0].StackingVelocity, nil
	}

	if timeMS <= vf.Samples[0].TimeMS {
		if mode == VelocityExtrapolateLinear {
			return linearVelocity(vf.Samples[0], vf.Samples[1], timeMS), nil
		}
		return vf.Samples[0].StackingVelocity, nil
	}

	last := vf.Samples[len(vf.Samples)-1]
	if timeMS >= last.TimeMS {
		if mode == VelocityExtrapolateLinear {
			return linearVelocity(vf.Samples[len(vf.Samples)-2], last, timeMS), nil
		}
		return last.StackingVelocity, nil
	}

	for i := 1; i < len(vf.Samples); i++ {
		left := vf.Samples[i-1]
		right := vf.Samples[i]
		if timeMS <= right.TimeMS {
			dt := right.TimeMS - left.TimeMS
			if dt <= 0 {
				return right.StackingVelocity, nil
			}
			f := (timeMS - left.TimeMS) / dt
			return left.StackingVelocity + f*(right.StackingVelocity-left.StackingVelocity), nil
		}
	}

	return last.StackingVelocity, nil
}

func linearVelocity(left, right VelocitySample, timeMS float64) float64 {
	dt := right.TimeMS - left.TimeMS
	if dt <= 0 {
		return right.StackingVelocity
	}
	f := (timeMS - left.TimeMS) / dt
	return left.StackingVelocity + f*(right.StackingVelocity-left.StackingVelocity)
}

func resolveTraceSampleIntervalMS(header TraceHeader, bh BinaryHeader) (float64, error) {
	dtUS := header.SampleIntervalMicrosec
	if dtUS == 0 {
		dtUS = bh.SampleIntervalMicrosec
	}
	if dtUS == 0 {
		return 0, fmt.Errorf("sample interval is zero in trace and binary header")
	}
	return float64(dtUS) / 1000.0, nil
}

func estimateTrialVelocityCount(minVelocity, maxVelocity, velocityStep float64) int {
	if maxVelocity <= minVelocity {
		return 1
	}

	count := int(math.Floor((maxVelocity-minVelocity)/velocityStep)) + 1
	if count < 2 {
		count = 2
	}

	lastUniformVelocity := minVelocity + float64(count-1)*velocityStep
	if lastUniformVelocity < maxVelocity {
		count++
	}

	return count
}

func buildSquaredSlownessVelocities(minVelocity, maxVelocity float64, count int) []float64 {
	if count <= 1 || minVelocity == maxVelocity {
		return []float64{minVelocity}
	}

	slow2Start := 1.0 / (minVelocity * minVelocity)
	slow2End := 1.0 / (maxVelocity * maxVelocity)

	out := make([]float64, count)
	for i := 0; i < count; i++ {
		f := float64(i) / float64(count-1)
		slow2 := slow2Start + f*(slow2End-slow2Start)
		out[i] = 1.0 / math.Sqrt(slow2)
	}

	return out
}

// sampleTraceLinearIfInRange samples a trace at a fractional index using linear interpolation.
func sampleTraceLinearIfInRange(samples []float32, idx float64) (float64, bool) {
	n := len(samples)
	if n == 0 {
		return 0, false
	}
	if idx < 0 || idx > float64(n-1) {
		return 0, false
	}

	i0 := int(math.Floor(idx))
	i1 := i0 + 1
	if i1 >= n {
		return float64(samples[i0]), true
	}

	w := idx - float64(i0)
	v := (1.0-w)*float64(samples[i0]) + w*float64(samples[i1])
	return v, true
}

func sampleTraceLinear(samples []float32, idx float64) float32 {
	n := len(samples)
	if n == 0 {
		return 0
	}
	if idx < 0 || idx > float64(n-1) {
		return 0
	}

	i0 := int(math.Floor(idx))
	i1 := i0 + 1
	if i1 >= n {
		return samples[i0]
	}

	w := float32(idx - float64(i0))
	return (1-w)*samples[i0] + w*samples[i1]
}
