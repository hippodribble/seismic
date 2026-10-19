# Velocity Analysis

A desktop tool for interactive seismic velocity interpretation and stack review.

## Overview

This app lets you:

- open a SEG-Y or SU file
- browse through CDPs
- inspect the raw CDP gather and semblance panel together
- pick velocity functions and top-mute values
- preview NMO correction and muted gathers
- build a stacked section across the line
- save and reload pick files

## Quick start

From the repository root, run:

```bash
go run ./segy/cmd/velocityanalysis
```

Then:

1. click `SEG-Y File` to open a seismic file
2. pick a CDP with the selector or keyboard arrows
3. switch between `Pick mode` and `Mute mode`
4. drag or click to add/edit picks on the semblance panel
5. use `NMO preview` and `Build Stack` to review the result

## Main workflow

### CDP and semblance

- The left panel shows the current CDP gather.
- The center/right panel shows the semblance display used for picking.
- Use the CDP selector to move through the line.
- Arrow keys move the active CDP by the current step value.

### Velocity picks

- In `Pick mode`, click or drag on the semblance plot to place velocity picks.
- Pick values are shown against the selected CDP and used to build the NMO preview.
- The app keeps the original gather untouched and applies the preview dynamically.

### Top-mute picks

- In `Mute mode`, pick the top mute on the CDP gather.
- Muting is applied in the processing preview and to the semblance calculation.

### Stack review

- `Build Stack` creates a stacked section for the full line.
- `Export Stack` writes the current stacked section to `.segy`, `.sgy`, or `.su`.
- The stack panel shows picked CDP markers and active cursor guides.
- Click on the stack view to jump to the corresponding CDP.

## Pick management

- `Export Picks` saves current velocity and mute picks.
- `Import Picks` reloads them later.
- `Clear V Picks` / `Clear Mute Picks` remove picks for the current CDP.
- `Clear All V` / `Clear All Mute` remove them across the whole file.

## Notes

- The app persists the last opened file and last pick file.
- `Auto Stack` can be enabled to rebuild the stack after pick edits complete.
- The current view is synchronized vertically across the main panels for easier comparison.

## Keyboard shortcuts

- Left / Right arrow: move active CDP
- Up / Down arrow: pan vertically in the view

## Status

This is a workflow-focused interpretation tool intended for active seismic velocity analysis and QC.
