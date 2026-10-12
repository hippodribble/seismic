# P190 preplot parser and utilities

This package parses P1/90-style preplot text files, groups records into named line features, and can export them as ESRI polyline shapefiles.

The current code in this folder includes:

- the reusable Go library in [p190.go](p190.go)
- a Fyne-based viewer in [x/P190preplotviewer](x/P190preplotviewer)
- a command-line shapefile exporter in [x/P190preplot2shape](x/P190preplot2shape)

## Package overview

The library is in the `P190` package and exposes these main types:

- `NodeRecord`: a single parsed node record
- `PreplotLine`: a named collection of ordered nodes
- `P190File`: the file-level parser and export container

## Parsing behavior

`NodeRecord.FromP190Text` reads fixed-width P190 fields and fills the struct with:

- record type
- line name
- point number
- easting
- northing

The parser expects the standard fixed-width layout used by the source files, primarily:

- record type at column 1
- line name in columns 2-13
- point number in columns 20-25
- easting in columns 47-55
- northing in columns 56-64

`P190File.Load()` opens the file, filters on the selected record prefix, parses matching rows, and groups sequential records by line name into `PreplotLine` entries.

## Record-type detection

The viewer uses `P190.MostCommonPrefix(text)` to detect the most common record code in a file and pass that into `NewP190File(...)`.

Typical record prefixes include:

- `S`
- `G`
- `Q`
- `A`
- `T`
- `V`
- `E`
- `Z`
- `C`

This is useful because P190 preplot files often contain more than one record family, but one type usually dominates the actual line data.

## Shapefile export

`P190File.ToShapeFile(filename)` writes a polyline shapefile for each line using `github.com/jonas-p/go-shp`.

Each line becomes a single polyline, and the output includes a `LINE` attribute containing the line name. The method writes a `.shp` and renames the generated DBF output to match the target filename.

## Example usage

```go
package main

import (
    "fmt"
    "github.com/hippodribble/seismic/P190"
)

func main() {
    p := P190.NewP190File("survey.p190", "S")
    if err := p.Load(); err != nil {
        panic(err)
    }

    if err := p.ToShapeFile("survey_lines"); err != nil {
        panic(err)
    }

    fmt.Printf("Loaded %d lines\n", len(p.Lines))
}
```

## GUI viewer

The Fyne viewer in [x/P190preplotviewer/main.go](x/P190preplotviewer/main.go) lets you:

- open a `.p190`, `.p90`, or `.90` file
- detect the dominant record prefix automatically
- load the preplot lines into a viewport
- inspect the raw file text
- switch between standard and measurement/identify modes

This is intended for quick visual checking of line geometry before exporting or further processing.

## Command-line converter

The converter in [x/P190preplot2shape/main.go](x/P190preplot2shape/main.go) takes a file and writes a shapefile from it.

Example:

```bash
go run ./P190/x/P190preplot2shape -r S survey.p190
```

The `-r` flag selects the record type to parse; the final argument is the input file path.

## Notes and limitations

- The parser assumes fixed-width input and validates the expected numeric ranges.
- It groups consecutive records by line name, not by a separate global line index.
- Errors are returned to the caller instead of terminating the process, which is the current library behavior in the code in this folder.
- The helper tools are focused on reading and visualizing survey preplot geometry rather than full survey-data handling.
