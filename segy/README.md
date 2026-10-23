# segy package

The segy package provides core types and helpers for working with industry-standard SEG-Y files.

Note: an example SEG-Y viewer application is available in [cmd/segyviewer/main.go](cmd/segyviewer/main.go).

It includes:

- File-level modeling with a platform-independent file location (`fyne.URI`)
- Textual and binary file header parsing
- Trace header parsing
- Trace catalog/index construction for large files
- Database-style trace selection by header values, including multi-predicate filtering

## API quick reference

This section reflects the current exported API surface of `github.com/hippodribble/seismic/segy`.

### Open and header parsing

| API | Returns | Purpose |
| --- | --- | --- |
| `Open(uri)` | `(*File, error)` | Open SEG-Y with default conventions. |
| `OpenWithByteOrder(uri, byteOrder)` | `(*File, error)` | Open with explicit endianness. |
| `OpenSEGYWithByteOrderAndFormat(uri, byteOrder, sampleFormatCode)` | `(*File, error)` | Open SEG-Y with explicit byte order and sample format override. |
| `OpenSU(uri)` | `(*File, error)` | Open Seismic Unix style trace stream. |
| `OpenSUWithByteOrder(uri, byteOrder)` | `(*File, error)` | Open SU with explicit endianness. |
| `OpenSUWithByteOrderAndFormat(uri, byteOrder, sampleFormatCode)` | `(*File, error)` | Open SU with explicit endianness and sample format override. |
| `ReadHeaders(uri, r)` | `(*File, error)` | Parse SEG-Y/SU headers from an existing reader. |
| `ReadHeadersWithByteOrder(uri, r, byteOrder)` | `(*File, error)` | Parse headers from reader with explicit endianness. |

### Catalog building and progress/cancel variants

| API family | Returns | Purpose |
| --- | --- | --- |
| `(*File).BuildTraceCatalog*` | `(*TraceCatalog, error)` | Build a catalog from an opened file, with standard and trace-only variants plus progress/cancel callbacks. |
| `BuildTraceCatalogFromReader*` | `(*TraceCatalog, error)` | Build a standard catalog directly from an `io.Reader`, including byte-order, sample-format, progress, and cancel variants. |
| `BuildTraceCatalogTraceOnlyFromReader*` | `(*TraceCatalog, error)` | Build a trace-only catalog from an `io.Reader`, including byte-order, progress, and cancel variants. |

### Querying, sorting, grouping, and summaries

| API | Returns | Purpose |
| --- | --- | --- |
| `(*TraceCatalog).FindEqual(field, value)` | `[]TraceIndexEntry` | Exact match by indexed field. |
| `(*TraceCatalog).FindRange(field, minValue, maxValue)` | `[]TraceIndexEntry` | Inclusive range query by indexed field. |
| `(*TraceCatalog).FindWhere(predicates...)` | `([]TraceIndexEntry, error)` | Multi-predicate AND query. |
| `(*TraceCatalog).FindRangeSorted(field, minValue, maxValue, keys...)` | `([]TraceIndexEntry, error)` | Range query plus sorting. |
| `(*TraceCatalog).FindWhereSorted(predicates, keys...)` | `([]TraceIndexEntry, error)` | Predicate query plus sorting. |
| `(*TraceCatalog).FindByTRACR(tracr)` / `FindByTRACF(tracf)` / `FindEnsembleByEP(ep)` | `[]TraceIndexEntry` | Convenience lookups by common fields. |
| `SortTraceIndexEntries(entries, keys...)` | `error` | In-place sort for extracted entry slices. |
| `Asc(field)` / `Desc(field)` | `TraceSortKey` | Build sort keys. |
| `EqualPredicate(field, value)` / `RangePredicate(field, minValue, maxValue)` | `TraceQueryPredicate` | Build reusable predicates. |
| `GroupByField(entries, field)` / `GroupByEP(entries)` / `GroupByCDP(entries)` | `map[int64][]TraceIndexEntry` | Group query results by field. |
| `(*TraceCatalog).HeaderRangeSummary()` | `TraceHeaderRangeSummary` | Min/max summary for indexed fields. |
| `(TraceHeaderRangeSummary).String()` | `string` | Render summary as text. |

### Trace reading and sample decoding

| API | Returns | Purpose |
| --- | --- | --- |
| `(*File).ReadTraceFromIndex(entry)` | `(TraceHeader, []byte, error)` | Read one raw trace by catalog entry. |
| `(*File).ReadTraceAtOffset(headerOffset, sampleCount)` | `(TraceHeader, []byte, error)` | Read one raw trace by byte offset. |
| `DecodeTraceSamples(traceData, sampleFormatCode, byteOrder)` | `([]float32, error)` | Decode raw sample bytes into `float32`. |
| `(*File).DecodeTraceData(traceData)` | `([]float32, error)` | Decode raw sample bytes using file settings. |
| `(*File).ReadDecodedTraceFromIndex(entry)` | `(TraceHeader, []float32, error)` | Read and decode by catalog entry. |
| `(*File).ReadDecodedTraceAtOffset(headerOffset, sampleCount)` | `(TraceHeader, []float32, error)` | Read and decode by offset. |
| `ReadTraceHeader(r)` / `ReadTraceHeaderWithByteOrder(r, byteOrder)` | `(TraceHeader, error)` | Decode one trace header from stream bytes. |

### Custom trace-header values

| API | Returns | Purpose |
| --- | --- | --- |
| `DecodeTraceHeaderValue(data, valueType, byteOrder)` | `(any, error)` | Decode one typed value from bytes. |
| `ReadCustomTraceValuesFromBytes(traceHeader, fields, byteOrder)` | `(map[string]any, error)` | Decode multiple custom values from one header slice. |
| `(*File).ReadTraceHeaderValueAtOffset(headerOffset, valueOffset, valueType)` | `(any, error)` | Read one custom value at byte offset from file. |
| `(*File).ReadCustomTraceValuesAtOffset(headerOffset, fields)` | `(map[string]any, error)` | Read many custom values from one header in file. |
| `(*File).BuildCustomTraceCatalog(entries, fields)` | `([]CustomTraceIndexEntry, error)` | Attach custom decoded fields to catalog entries. |

### Gather construction, velocity analysis, and stacking

| API | Returns | Purpose |
| --- | --- | --- |
| `(*File).BuildCDPGatherByCDP(cdp, sortByOffset)` | `(CDPGather, error)` | Build a decoded CDP gather from catalog selection. |
| `(*File).BuildCDPGatherFromEntries(entries)` | `(CDPGather, error)` | Build decoded gather from explicit trace entries. |
| `ApplyNMOToCDPGather(gather, velocity)` | `(CDPGather, error)` | Apply NMO with default options. |
| `ApplyNMOToCDPGatherWithOptions(gather, velocity, options)` | `(CDPGather, error)` | Apply NMO with explicit options. |
| `ApplyNMOToCDPGatherWithOptionsAndMask(gather, velocity, options)` | `(CDPGather, StretchMuteMask, error)` | Apply NMO and return stretch-mute mask. |
| `BuildSemblanceGatherFromCDP*` | `(SemblanceGather, error)` | Build semblance gathers using base, smoothing, power, and stretch-mute variants. |
| `(SemblanceGather).VelocityAxis()` | `[]float64` | Trial velocity axis in gather order. |
| `StackCDPGatherMean(gather, muteMask)` | `(StackedTrace, error)` | Mean stack with optional mute mask. |

### Filtering, autocorrelation, smoothing, and time shifts

| API | Returns | Purpose |
| --- | --- | --- |
| `ApplyHighCutToCDPGather(gather, highCutHz)` | `(CDPGather, error)` | Zero-phase high-cut filter on gather traces. |
| `ApplyPolygonalFrequencyFilter(samples, sampleIntervalMS, frequencies, amplitudes)` | `([]float32, error)` | Polygonal frequency-domain amplitude filter for one trace. |
| `ApplyPolygonalFrequencyFilterToCDPGather(gather, frequencies, amplitudes)` | `(CDPGather, error)` | Polygonal filter applied to every trace in gather. |
| `Autocorrelation(trace, maxLag, twoSided)` | `([]float32, error)` | Raw autocorrelation for one trace. |
| `NormalizedAutocorrelation(trace, maxLag, twoSided)` | `([]float32, error)` | Autocorrelation normalized by lag-0 energy. |
| `AutocorrelationForTraceSet(traces, maxLag, twoSided)` | `([][]float32, error)` | Raw autocorrelation for multiple traces. |
| `NormalizedAutocorrelationForTraceSet(traces, maxLag, twoSided)` | `([][]float32, error)` | Normalized autocorrelation for multiple traces. |
| `SmoothTraceTimeRunningAverage(trace, windowLength)` | `([]float32, error)` | Running-average smoothing for one trace. |
| `SmoothEnsembleSamplesTimeRunningAverage(samples, windowLength)` | `(EnsembleSamples, error)` | Running-average smoothing for all traces in an ensemble. |
| `ShiftTraceBySamplesFrequencyDomain(samples, shiftSamples)` | `([]float32, error)` | Frequency-domain fractional sample shift with non-wrapping truncation. |
| `ShiftEnsembleSamplesBySamplesFrequencyDomain(ensemble, shiftSamples)` | `(EnsembleSamples, error)` | Apply the same frequency-domain shift to every trace in an ensemble. |

### Ensemble decode/render and palettes

| API | Returns | Purpose |
| --- | --- | --- |
| `(*File).ReadEnsembleSamples(entries, maxTotalImageSamples)` | `(EnsembleSamples, error)` | Decode selected traces into rectangular sample matrix. |
| `RenderEnsembleImage(samples, opts)` | `(*image.Paletted, EnsembleImageStats, error)` | Map decoded samples to palette image with clipping/gain options. |
| `(*File).RenderEnsembleFromEntries(entries, maxTotalImageSamples, opts)` | `(*image.Paletted, EnsembleImageStats, error)` | Read/decode/render in one call. |
| `(*File).QueryAndRenderEnsemble(predicates, sortKeys, maxTotalImageSamples, opts)` | `(*image.Paletted, EnsembleImageStats, error)` | Query/sort/read/decode/render in one call. |
| `GrayscalePalette256()` | `color.Palette` | 256-step grayscale palette. |
| `BlueWhiteRedPalette256()` | `color.Palette` | 256-step blue-white-red diverging palette. |
| `BlueRedWhitePalette256()` | `color.Palette` | 256-step blue-red-white diverging palette. |
| `ThreePointHSVPalette256(low, mid, high)` | `color.Palette` | 256-step palette interpolated from three HSV anchors. |
| `InvertPalette(p)` | `color.Palette` | Reverse palette order. |
| `MustPalette256(p)` | `color.Palette` | Normalize to 256 entries (panics if empty). |

### Export

| API | Returns | Purpose |
| --- | --- | --- |
| `(*File).ExportTraces(w, entries, options)` | `(int, error)` | Write selected traces to output stream with configurable header inclusion. |

## Recommended workflow

For large files, this sequence is typically the fastest and simplest:

1. Open the file and parse top-level headers with `Open`.
2. Build one in-memory trace catalog with `BuildTraceCatalog`.
3. Extract the working set with `FindEqual`, `FindRange`, or `FindWhere`, then sort/group as needed.
4. Read only selected traces with `ReadTraceFromIndex` or `ReadTraceAtOffset`.

```go
uri := storage.NewFileURI("/path/to/data.segy")
f, err := segy.Open(uri)
if err != nil {
	return err
}

catalog, err := f.BuildTraceCatalog()
if err != nil {
	return err
}

entries, err := catalog.FindWhereSorted(
	[]segy.TraceQueryPredicate{
		segy.RangePredicate(segy.TraceHeaderFieldCDP, 1000, 1200),
		segy.RangePredicate(segy.TraceHeaderFieldTRACR, 1, 48),
	},
	segy.Asc(segy.TraceHeaderFieldCDP),
	segy.Asc(segy.TraceHeaderFieldOFFSET),
)
if err != nil {
	return err
}

if len(entries) > 0 {
	traceHeader, traceData, readErr := f.ReadTraceFromIndex(entries[0])
	if readErr != nil {
		return readErr
	}
	_ = traceHeader
	_ = traceData
}
```

## Main types

### File

`File` represents a SEG-Y file and stores:

- `URI` as a `fyne.URI` (from Fyne storage API)
- `TextualHeader` (3200-byte character header)
- `BinaryHeader` (400-byte binary header)
- `ExtendedTextualHeader` (zero or more 3200-byte headers)
- Optional `TraceCatalog` for indexed trace retrieval

### TextualHeader

`TextualHeader` stores:

- `Encoding` as `TextualHeaderEncoding` (`Unknown`, `EBCDIC`, `UTF8`)
- Raw `Data` as `[3200]byte`

### BinaryHeader

`BinaryHeader` contains commonly used SEG-Y binary header fields (sample interval, sample format code, trace counts, revision, etc.) and the original raw 400 bytes.

### TraceHeader

`TraceHeader` contains commonly used fields from the 240-byte trace header, including:

- `TraceSequenceLine` (`tracl`)
- `TraceSequenceFile` (`tracr`)
- `FieldRecordNumber` (`fldr`)
- `EnergySourcePointNumber` (`ep`)
- `CDP`, `TraceNumberWithinCDP`
- `SourceReceiverOffset`, `SourceX`, `SourceY`, `GroupX`, `GroupY`
- `SamplesInTrace`, `SampleIntervalMicrosec`
- Raw bytes as `[240]byte`

## Load a SEG-Y file

Use `Open` to load file headers via Fyne URI storage:

```go
package main

import (
	"log"

	"fyne.io/fyne/v2/storage"
	"github.com/hippodribble/seismic/segy"
)

func main() {
	uri := storage.NewFileURI("/path/to/data.segy")
	f, err := segy.Open(uri)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("sample format code: %d", f.BinaryHeader.DataSampleFormatCode)
	log.Printf("samples/trace (binary default): %d", f.BinaryHeader.SamplesPerTrace)
}
```

If you need non-standard little-endian support, open explicitly with byte order:

```go
f, err := segy.OpenWithByteOrder(uri, binary.LittleEndian)
if err != nil {
	return err
}
_ = f
```

## Build the trace catalog

The trace catalog performs one sequential scan and records trace byte locations and parsed headers.

```go
catalog, err := f.BuildTraceCatalog()
if err != nil {
	return err
}

// Number of indexed traces
count := len(catalog.Entries)
_ = count
```

Each catalog entry contains:

- `HeaderOffset` and `DataOffset` byte locations
- `DataByteLength`
- `SampleCount`, `SampleFormatCode`, `SampleByteLength`
- Parsed `TraceHeader`

## Custom non-standard trace fields

Some SEG-Y files include application-specific values outside the standard trace header fields. The package supports this by pairing a 1-based SEG-Y trace-header byte position with a numeric type and decoding the value at that position.

Use `TraceHeaderValueType` to describe the field width and representation:

```go
fields := []segy.CustomTraceField{
	{Name: "myFloat", Offset: 73, Type: segy.TraceHeaderValueTypeFloat32},
	{Name: "myInt16", Offset: 41, Type: segy.TraceHeaderValueTypeInt16},
	{Name: "myUint32", Offset: 121, Type: segy.TraceHeaderValueTypeUint32},
}
```

Then decode one trace header or build a custom catalog:

```go
// Decode custom values from one trace header slice.
headerValues, err := segy.ReadCustomTraceValuesFromBytes(
	traceHeader.Raw[:],
	fields,
	file.ByteOrder,
)
if err != nil {
	return err
}

// Or build a custom catalog with the standard trace catalog entries.
customEntries, err := file.BuildCustomTraceCatalog(file.TraceCatalog.Entries, fields)
if err != nil {
	return err
}

_ = headerValues
_ = customEntries
```

The custom values are stored as `map[string]any`, so each entry can contain a mix of numeric types such as `float32`, `int16`, `uint32`, etc. This is useful when a file contains vendor-specific metadata, auxiliary trace bookkeeping, or other non-standard fields not represented in the built-in `TraceHeader` struct.

## Query helpers

### Single-field queries

```go
// All traces with ep = 1001
byEP := catalog.FindEqual(segy.TraceHeaderFieldEP, 1001)

// All traces with tracr = 24
byTRACR := catalog.FindByTRACR(24)

// Range query (inclusive)
byRange := catalog.FindRange(segy.TraceHeaderFieldEP, 1000, 1100)

_, _, _ = byEP, byTRACR, byRange
```

### Multi-predicate query (AND semantics)

Use `FindWhere` with one or more predicates.

```go
matches, err := catalog.FindWhere(
	segy.EqualPredicate(segy.TraceHeaderFieldEP, 1001),
	segy.RangePredicate(segy.TraceHeaderFieldTRACR, 1, 48),
)
if err != nil {
	return err
}

_ = matches
```

Notes:

- Predicates are combined with logical AND.
- `FindWhere()` with no predicates returns all indexed traces.
- Range predicates are inclusive: `min <= value <= max`.

### Multi-key sorting of extracted traces

After selecting traces, you can sort by one or more header fields.

Sort keys are applied in order, and each key can be ascending or descending.

```go
// Extract a CDP range, then sort by CDP and offset (ascending)
entries := catalog.FindRange(segy.TraceHeaderFieldCDP, 1000, 1200)

err = segy.SortTraceIndexEntries(
	entries,
	segy.Asc(segy.TraceHeaderFieldCDP),
	segy.Asc(segy.TraceHeaderFieldOFFSET),
)
if err != nil {
	return err
}

_ = entries
```

## Decode trace samples to float32

Trace payload reads return raw bytes. To convert samples to `[]float32`, decode using the SEG-Y sample format code and byte order.

```go
entry := entries[0]

// One-call read + decode
traceHeader, samples, err := f.ReadDecodedTraceFromIndex(entry)
if err != nil {
	return err
}

_ = traceHeader
_ = samples
```

You can also decode manually:

```go
traceHeader, raw, err := f.ReadTraceFromIndex(entry)
if err != nil {
	return err
}

samples, err := f.DecodeTraceData(raw)
if err != nil {
	return err
}

_ = traceHeader
_ = samples
```

Supported sample format codes for decoding are: `1`, `2`, `3`, `5`, `6`, and `8`.

## Apply normal moveout (NMO) to a CDP gather

You can build a decoded CDP gather from selected entries and apply NMO using a stacking velocity function.

```go
// Select one CDP gather
entries := catalog.FindEqual(segy.TraceHeaderFieldCDP, 1205)
if len(entries) == 0 {
	return fmt.Errorf("no traces for CDP 1205")
}

gather, err := f.BuildCDPGatherFromEntries(entries)
if err != nil {
	return err
}

vf := segy.VelocityFunction{
	CDP: 1205,
	Samples: []segy.VelocitySample{
		{TimeMS: 0, StackingVelocity: 1500},
		{TimeMS: 500, StackingVelocity: 1900},
		{TimeMS: 1000, StackingVelocity: 2400},
		{TimeMS: 1500, StackingVelocity: 2800},
	},
}

nmoGather, err := segy.ApplyNMOToCDPGatherWithOptions(
	gather,
	vf,
	segy.NMOOptions{
		ExtrapolationMode: segy.VelocityExtrapolateLinear,
		MaxStretchFraction: 0.35,
	},
)
if err != nil {
	return err
}

_ = nmoGather
```

Notes:

- NMO output is a new gather; input gather is not modified.
- Trace order is preserved.
- Stretch mute is disabled when `MaxStretchFraction <= 0`.
- Extrapolation mode can be `VelocityExtrapolateHold` (default behavior) or `VelocityExtrapolateLinear`.

If you want to reuse the stretch-mute decisions in later steps (for example stacking), use the mask-returning variant:

```go
nmoGather, muteMask, err := segy.ApplyNMOToCDPGatherWithOptionsAndMask(
	gather,
	vf,
	segy.NMOOptions{
		ExtrapolationMode: segy.VelocityExtrapolateLinear,
		MaxStretchFraction: 0.35,
	},
)
if err != nil {
	return err
}

stacked, err := segy.StackCDPGatherMean(nmoGather, &muteMask)
if err != nil {
	return err
}

_ = stacked
```

## Build a semblance gather from a CDP gather

You can compute a semblance panel from one CDP gather by scanning trial velocities.
Velocity sampling is uniform in squared slowness (`1/v^2`) for better shaping, and output order is low velocity to high velocity.

```go
// Starting from the same gather created above.
semblance, err := segy.BuildSemblanceGatherFromCDP(
	gather,
	1400, // min velocity
	3600, // max velocity
	50,   // nominal velocity step
)
if err != nil {
	return err
}

// Velocity axis for plotting columns/traces.
velocityAxis := semblance.VelocityAxis()

_ = velocityAxis
_ = semblance
```

Notes:

- `velocityStep` controls trial density; sampling is mapped to uniform squared-slowness spacing.
- Each semblance trace corresponds to one trial velocity.
- Semblance values are clamped to `[0, 1]`.

## Apply a high-cut filter to a CDP gather

You can apply a high-cut filter directly to a decoded gather.
The implementation uses a 2nd-order Butterworth low-pass applied forward and backward to preserve phase.

```go
filteredGather, err := segy.ApplyHighCutToCDPGather(
	gather,
	80, // high-cut frequency in Hz
)
if err != nil {
	return err
}

_ = filteredGather
```

Notes:

- Output is a new gather; input gather is not modified.
- `highCutHz` must be greater than zero and lower than Nyquist.
- Forward + reverse application provides zero-phase filtering.

## Build a paletted image from an ensemble

Use a threshold guard when extracting an ensemble to avoid accidental oversized renders.

```go
matches, err := catalog.FindWhere(
	segy.RangePredicate(segy.TraceHeaderFieldCDP, 1000, 1200),
)
if err != nil {
	return err
}

ensemble, err := f.ReadEnsembleSamples(matches, segy.DefaultMaxImageSamples)
if err != nil {
	return err
}

opts := segy.EnsembleImageOptions{
	Palette:                myPalette256, // must be length 256
	ZeroCentered:           true,
	ZeroIndex:              127,
	NegativePercentileClip: 2.0,
	PositivePercentileClip: 2.0,
	Gain: segy.GainOptions{
		Mode:          segy.GainModeTSquared,
		TSquaredScale: 1.0,
	},
}

img, stats, err := segy.RenderEnsembleImage(ensemble, opts)
if err != nil {
	return err
}

_ = img
_ = stats
```

You can do query + optional sorting + render in a single call:

```go
predicates := []segy.TraceQueryPredicate{
	segy.RangePredicate(segy.TraceHeaderFieldCDP, 1000, 1200),
}
sortKeys := []segy.TraceSortKey{
	segy.Asc(segy.TraceHeaderFieldCDP),
	segy.Asc(segy.TraceHeaderFieldOFFSET),
}

img, stats, err := f.QueryAndRenderEnsemble(
	predicates,
	sortKeys,
	segy.DefaultMaxImageSamples,
	opts,
)
if err != nil {
	return err
}

_ = img
_ = stats
```

Notes:

- `ReadEnsembleSamples` pads shorter traces with zeros to create rectangular image input.
- If `maxTotal` is exceeded, `ReadEnsembleSamples` returns `TooManySamplesError` before reading trace payloads.
- `RenderEnsembleImage` requires a palette of exactly 256 colors.
- In zero-centered mode, value `0` maps to `ZeroIndex` and negative/positive clips are applied independently.
- In non-zero-centered mode, `PercentileClip` is applied symmetrically to lower and upper tails.
- `Gain.Mode` is optional and defaults to `GainModeNone`.
- `GainModeTSquared` applies gain $1 + scale \times t^2$ before clipping.
- `GainModeAGC` applies windowed RMS normalization with `AGCWindowSamples` and `AGCFloor`.

AGC example:

```go
opts.Gain = segy.GainOptions{
	Mode:             segy.GainModeAGC,
	AGCWindowSamples: 101,
	AGCFloor:         1e-6,
}
```

You can also query and sort in one call:

```go
entries, err := catalog.FindRangeSorted(
	segy.TraceHeaderFieldCDP,
	1000,
	1200,
	segy.Asc(segy.TraceHeaderFieldCDP),
	segy.Asc(segy.TraceHeaderFieldOFFSET),
)
if err != nil {
	return err
}

_ = entries
```

For multi-predicate queries with sorting:

```go
predicates := []segy.TraceQueryPredicate{
	segy.RangePredicate(segy.TraceHeaderFieldCDP, 1000, 1200),
	segy.RangePredicate(segy.TraceHeaderFieldTRACR, 1, 48),
}

entries, err := catalog.FindWhereSorted(
	predicates,
	segy.Asc(segy.TraceHeaderFieldCDP),
	segy.Asc(segy.TraceHeaderFieldOFFSET),
)
if err != nil {
	return err
}

_ = entries
```

Notes:

- `SortTraceIndexEntries` sorts in place.
- If no sort keys are provided, sorting falls back to file order (`TraceNumber`).
- Use `segy.Desc(...)` for descending sort on any key.

## Read a trace by indexed byte offset

Once you have a `TraceIndexEntry`, read that exact trace from the file:

```go
entry := matches[0]
th, data, err := f.ReadTraceFromIndex(entry)
if err != nil {
	return err
}

_ = th
_ = data // raw trace sample bytes
```

You can also read directly by header offset:

```go
th, data, err := f.ReadTraceAtOffset(entry.HeaderOffset, entry.SampleCount)
if err != nil {
	return err
}

_, _ = th, data
```

## Indexed fields

Current indexed header fields are:

- `ep`
- `tracr`
- `tracl`
- `fldr`
- `cdp`
- `cdpt`
- `offset`
- `sx`
- `sy`
- `gx`
- `gy`

Use the `TraceHeaderField...` constants when constructing queries.

## Group query results into ensembles

You can post-process query matches into ensemble groups keyed by EP or CDP with built-in helpers.

```go
// Example: query first, then group
matches, err := catalog.FindWhere(
	segy.RangePredicate(segy.TraceHeaderFieldEP, 1000, 1100),
	segy.RangePredicate(segy.TraceHeaderFieldTRACR, 1, 48),
)
if err != nil {
	return err
}

// Convenience helpers
ensemblesByEP := segy.GroupByEP(matches)
ensemblesByCDP := segy.GroupByCDP(matches)

// Generic helper if you want any indexed field
ensemblesByTRACR := segy.GroupByField(matches, segy.TraceHeaderFieldTRACR)

// Read one trace from one EP ensemble
for _, ensemble := range ensemblesByEP {
	if len(ensemble) == 0 {
		continue
	}
	traceHeader, traceData, readErr := f.ReadTraceFromIndex(ensemble[0])
	if readErr != nil {
		return readErr
	}
	_ = traceHeader
	_ = traceData
	break
}

_ = ensemblesByCDP
_ = ensemblesByTRACR
```

Tip: if you need deterministic trace order within each ensemble, sort each group by `TraceNumber`, `TraceSequenceFile` (`tracr`), or offset before processing.
