# P190 preplot parser and utilities

This release adds a reusable P190 parser and related tooling for handling preplot survey data.

## Added

- fixed-width P190 record parsing for node data
- grouping of sequential records into named preplot lines
- file loading and line assembly in the `P190` package
- ESRI shapefile export for preplot geometry
- record-type detection to infer the dominant P190 record prefix automatically
- a Fyne-based viewer for opening and inspecting P190/P90 files
- a command-line converter for exporting preplot files to shapefiles
- updated package documentation covering usage and workflow

## Improvements

- clearer separation between parsing logic, line grouping, and export functions
- easier inspection of raw preplot text before export
- support for working with `.p190`, `.p90`, and `.90` input files in the viewer

## Notes

- parsing is designed around fixed-width P1/90-style records
- line grouping preserves sequential record ordering by line name
- the project remains focused on survey preplot geometry and shapefile export workflows
