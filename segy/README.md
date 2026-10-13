# segy package

The segy package provides core types and helpers for working with industry-standard SEG-Y files.

Note: an example SEG-Y viewer application is available in [x/main.go](x/main.go).

It includes:

- File-level modeling with a platform-independent file location (`fyne.URI`)
- Textual and binary file header parsing
- Trace header parsing
- Trace catalog/index construction for large files
- Database-style trace selection by header values, including multi-predicate filtering

## API quick reference

| API | Returns | Purpose |
| --- | --- | --- |
| `Open(uri fyne.URI)` | `(*File, error)` | Open a SEG-Y file and parse textual/binary headers. |
| `OpenWithByteOrder(uri, order)` | `(*File, error)` | Open a SEG-Y file with explicit byte order (`binary.BigEndian` or `binary.LittleEndian`). |
| `(*File).BuildTraceCatalog()` | `(*TraceCatalog, error)` | Scan traces and build in-memory indexes. |
| `BuildTraceCatalogFromReader(uri, reader)` | `(*TraceCatalog, error)` | Build catalog from a stream already at byte zero. |
| `BuildTraceCatalogFromReaderWithByteOrder(uri, reader, order)` | `(*TraceCatalog, error)` | Build catalog from a stream with explicit byte order. |
| `(*TraceCatalog).FindEqual(field, value)` | `[]TraceIndexEntry` | Exact match query on one indexed header field. |
| `(*TraceCatalog).FindRange(field, min, max)` | `[]TraceIndexEntry` | Inclusive range query on one indexed header field. |
| `(*TraceCatalog).FindWhere(predicates...)` | `([]TraceIndexEntry, error)` | Multi-predicate query with AND semantics. |
| `(*TraceCatalog).FindRangeSorted(field, min, max, keys...)` | `([]TraceIndexEntry, error)` | Range query and multi-key sorting in one call. |
| `(*TraceCatalog).FindWhereSorted(predicates, keys...)` | `([]TraceIndexEntry, error)` | Multi-predicate query and multi-key sorting in one call. |
| `SortTraceIndexEntries(entries, keys...)` | `error` | Sort an extracted trace set in place by one or more keys. |
| `Asc(field)` / `Desc(field)` | `TraceSortKey` | Build ascending/descending sort keys. |
| `GroupByField(entries, field)` | `map[int64][]TraceIndexEntry` | Group a result set by any indexed field. |
| `GroupByEP(entries)` / `GroupByCDP(entries)` | `map[int64][]TraceIndexEntry` | Convenience grouping by EP or CDP. |
| `(*TraceCatalog).HeaderRangeSummary()` | `TraceHeaderRangeSummary` | Min/max summary for each indexed header field. |
| `(TraceHeaderRangeSummary).String()` | `string` | One line per header field with min/max values. |
| `(*File).ReadTraceFromIndex(entry)` | `(TraceHeader, []byte, error)` | Read one trace using a catalog entry. |
| `(*File).ReadTraceAtOffset(offset, sampleCount)` | `(TraceHeader, []byte, error)` | Read one trace directly by header byte offset. |
| `DecodeTraceSamples(data, formatCode, order)` | `([]float32, error)` | Decode raw trace sample bytes to float32 values. |
| `(*File).DecodeTraceData(data)` | `([]float32, error)` | Decode raw trace bytes using file format/byte order settings. |
| `(*File).ReadDecodedTraceFromIndex(entry)` | `(TraceHeader, []float32, error)` | Read and decode one trace from a catalog entry. |
| `(*File).ReadDecodedTraceAtOffset(offset, sampleCount)` | `(TraceHeader, []float32, error)` | Read and decode one trace directly by byte offset. |
| `(*File).ReadEnsembleSamples(entries, maxTotal)` | `(EnsembleSamples, error)` | Decode many traces to a rectangular 2D sample set with a hard sample-count guard. |
| `RenderEnsembleImage(samples, opts)` | `(*image.Paletted, EnsembleImageStats, error)` | Convert decoded 2D samples to a paletted image with clipping and zero-centering support. |
| `(*File).RenderEnsembleFromEntries(entries, maxTotal, opts)` | `(*image.Paletted, EnsembleImageStats, error)` | One-call read/decode/render from selected trace entries with threshold guard. |
| `(*File).QueryAndRenderEnsemble(predicates, sortKeys, maxTotal, opts)` | `(*image.Paletted, EnsembleImageStats, error)` | One-call query, optional sort, threshold check, and image render. |
| `GrayscalePalette256()` | `color.Palette` | 256-color grayscale palette from black to white. |
| `BlueWhiteRedPalette256()` | `color.Palette` | 256-color diverging palette from blue through white to red. |
| `InvertPalette(palette)` | `color.Palette` | Returns a reversed copy of a palette. |
| `MustPalette256(palette)` | `color.Palette` | Normalizes palette to 256 entries (panics if empty). |

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
