# P190 Parser and Shapefile Export

This repository includes a Go package in [p190.go](p190.go) that reads P1/90 style text data, groups records into preplot lines, and can export those lines to an ESRI polyline shapefile.

## What [p190.go](p190.go) provides

The package name is P190 and it defines three main types:

1. NodeRecord
2. PreplotLine
3. P190File

### NodeRecord

NodeRecord is a parsed point record with:

1. Record metadata (Type, LineName, IDs)
2. Point number (Point)
3. Coordinates (Easting, Northing)

The method FromP190Text parses a fixed-width P190 text row and fills the struct.

Expected fixed-width slices used by parser:

1. Record type: character 1
2. Line name: characters 2-13
3. Point number: characters 20-25
4. Easting: characters 47-55
5. Northing: characters 56-64

If numeric fields cannot be parsed, parsing stops with a fatal log.

### PreplotLine

PreplotLine represents one line (track) and its ordered point records.

Fields:

1. Name string
2. Records *[]NodeRecord

Constructor:

1. NewPreplotLine(name string) PreplotLine

### P190File

P190File is the main container for file import and export.

Fields:

1. Name string: input file path
2. preplotrectype string: one-character record prefix to keep (for example S or P)
3. Lines []PreplotLine: parsed output grouped by line name

Constructor:

1. NewP190File(name, preplotrectype string) P190File

Methods:

1. Load()
2. ToShapeFile(filename string)

## Load flow

Load does the following:

1. Opens Name and scans line-by-line
2. Keeps only rows that start with preplotrectype
3. Parses each selected row into NodeRecord
4. Groups consecutive records by LineName into PreplotLine entries
5. Stores result in P190File.Lines

Important behavior:

1. Grouping is based on consecutive line name changes, not global regrouping.
2. Invalid files or parse failures currently terminate via log.Fatalln.

## Shapefile export

ToShapeFile writes one polyline geometry per PreplotLine using github.com/jonas-p/go-shp.

Outputs created:

1. filename.shp
2. filename.dbf
3. Other companion shapefile parts created by the library

Each written feature includes a LINE text attribute containing the source line name.

## Minimal usage

```go
package main

import "github.com/hippodribble/seismic/P190"

func main() {
	p := P190.NewP190File("survey.p190", "S")
	p.Load()
	p.ToShapeFile("survey_lines")
}
```

## How GUI code uses it

In [cmd/dialog.go](cmd/dialog.go), the application:

1. Loads file text
2. Detects the most common one-character record prefix
3. Calls NewP190File(path, prefix)
4. Calls Load()
5. Converts loaded points into drawable lines for the viewport

## Notes and limitations

1. Parsing assumes fixed-width input and minimum row length.
2. Coordinate parsing assumes Easting/Northing are in the expected positions.
3. Error handling is currently process-terminating; callers do not receive recoverable errors.
