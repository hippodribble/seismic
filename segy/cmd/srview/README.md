# SR View

`srview` is a small seismic QC application for reviewing shot-domain data using the `segy` module. It is intended to present a shot record on one side and the corresponding receiver gather on the other side, with linked selection and shared cursor feedback.

## Basic behavior

- load SEG-Y or SU data through the `segy` package
- browse by shot and by receiver
- update the opposite panel when a trace is selected
- highlight the selected trace as the reference trace
- mirror mouse position in the status area
- keep horizontal and vertical cross-hairs aligned with the cursor
- preserve the last file and last selections between runs
- provide an options panel for gain selection and parameter tuning

## Goal

This command is meant to be a smaller companion app to the existing viewer tools in `segy/cmd`, focused on quick shot/receiver review rather than full interpretation workflows.
