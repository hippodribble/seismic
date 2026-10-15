# SEG-Y Viewer

A desktop viewer for exploring SEG-Y seismic data with Fyne.

## Overview

This application loads a SEG-Y or SU-style seismic file, builds a trace catalog, and lets you inspect and render selected subsets of the data. It is designed for interactive review and QC of seismic gathers, headers, and display settings.

Typical use cases include:

- opening a SEG-Y file from disk
- browsing trace-header ranges and summaries
- selecting subsets by field and range
- rendering ensembles as image panels
- applying gain, clipping, and display transforms
- exporting rendered data or selected metadata

## Quick start

From the repository root, run:

```bash
go run ./segy/cmd/segyviewer
```

Then:

1. open a SEG-Y file from the file dialog
2. choose the primary and optional secondary display filters
3. adjust rendering options such as palette, clipping, and time window
4. render the selection and inspect the output
5. use the trace inspector and header panel to inspect individual traces

## Main features

### File loading and trace catalog

- opens SEG-Y files through the Fyne storage layer
- parses file and trace headers
- builds an in-memory catalog for rapid filtering and selection
- keeps the last used SEG-Y file for quick reopen

### Data selection and filtering

- query by trace-header fields such as CDP, EP, source/receiver values, and offsets
- apply primary and secondary filters with range or exact-match logic
- sort by multiple trace fields before rendering

### Display controls

- render a trace selection as a seismic image panel
- choose display palette and zero-centering options
- set clip limits and time-range windows
- apply gain controls including $t^2$ scaling and AGC-style adjustments
- limit the number of samples used in a render for performance

### Header inspection

- inspect the textual and binary SEG-Y headers
- browse trace headers for hovered or selected traces
- review summary statistics for the loaded file

### Export

- export rendered selections and header-included output where supported
- keep the workflow suitable for review, QC, and downstream processing preparation

## Keyboard controls

The viewer supports a few quick navigation keys:

- Left / Right arrow: shift the primary range selection
- Home / End: jump to the start or end of the primary range boundary

## Notes

- The viewer is optimized for interactive review and range-based selection workflows.
- It is suitable for QC tasks and quick inspection, especially when working with large SEG-Y datasets.
- The app stores user preferences for the last file and render settings.

## Typical workflow

1. Load a file.
2. Review the available header ranges.
3. Filter to a target subset using primary and secondary selectors.
4. Sort the subset as needed.
5. Render the selected ensemble.
6. Inspect the display and trace metadata.
7. Export or continue with downstream processing.
