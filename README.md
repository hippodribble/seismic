# seismic

A Go module for reading, querying, and visualizing SEG-Y seismic data.

## Included tools

- `segy`: core SEG-Y parsing and trace/header utilities
- `segy/cmd/segyviewer`: interactive SEG-Y viewer for browsing traces and render settings
- `segy/cmd/velocityanalysis`: velocity-picking and stack review workflow for CDP analysis
- `P190`: additional project-specific utilities and examples

## Quick start

```bash
go run ./segy/cmd/segyviewer
```

or

```bash
go run ./segy/cmd/velocityanalysis
```

## Repository layout

```text
.
├── P190/
├── segy/
│   ├── cmd/
│   │   ├── segyviewer/
│   │   └── velocityanalysis/
│   └── ...
├── go.mod
├── LICENSE
└── README.md
```

## Notes

- The `segy` package provides file and trace catalog support for SEG-Y workflows.
- The viewer app is aimed at inspection and QC.
- The velocity-analysis app is aimed at interpretation workflows, including picks, NMO preview, and stack review.
