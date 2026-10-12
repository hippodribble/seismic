# segy package

The segy package provides core types and helpers for working with industry-standard SEG-Y files.

It includes:

- File-level modeling with a platform-independent file location (`fyne.URI`)
- Textual and binary file header parsing
- Trace header parsing
- Trace catalog/index construction for large files
- Database-style trace selection by header values, including multi-predicate filtering

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
