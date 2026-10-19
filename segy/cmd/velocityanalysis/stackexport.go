package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"github.com/hippodribble/seismic/segy"
)

type stackExportTrace struct {
	CDP         int64
	Samples     []float32
	Template    segy.TraceHeader
	HasTemplate bool
}

func exportStackSection() {
	if winMain == nil {
		return
	}
	if loadedFile == nil {
		dialog.ShowError(fmt.Errorf("load a SEG-Y/SU file before exporting stack"), winMain)
		return
	}

	saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, winMain)
			return
		}
		if writer == nil {
			return
		}
		defer writer.Close()

		traces, sampleIntervalMS, snapshotErr := stackExportSnapshot()
		if snapshotErr != nil {
			dialog.ShowError(snapshotErr, winMain)
			setStackExportAvailable(false)
			return
		}

		sampleIntervalUS, intervalErr := stackSampleIntervalMicroseconds(sampleIntervalMS)
		if intervalErr != nil {
			dialog.ShowError(intervalErr, winMain)
			return
		}

		name := ""
		if uri := writer.URI(); uri != nil {
			name = strings.ToLower(strings.TrimSpace(uri.Name()))
		}
		isSU := strings.HasSuffix(name, ".su")
		isSEGY := strings.HasSuffix(name, ".segy") || strings.HasSuffix(name, ".sgy")
		if !isSU && !isSEGY {
			dialog.ShowError(fmt.Errorf("choose a .su, .segy or .sgy filename"), winMain)
			return
		}

		var byteOrder binary.ByteOrder = binary.BigEndian
		if loadedFile != nil && loadedFile.ByteOrder != nil {
			byteOrder = loadedFile.ByteOrder
		}

		var writeErr error
		if isSU {
			writeErr = writeStackSU(writer, traces, sampleIntervalUS, byteOrder)
		} else {
			writeErr = writeStackSEGY(writer, traces, sampleIntervalUS, byteOrder)
		}
		if writeErr != nil {
			dialog.ShowError(writeErr, winMain)
			return
		}
		setStackExportAvailable(true)
		pushStatus(fmt.Sprintf("Exported stack: %d traces (%s)", len(traces), strings.ToUpper(strings.TrimPrefix(name[strings.LastIndex(name, "."):], "."))))
	}, winMain)
	saveDialog.SetFileName("stack.segy")
	saveDialog.SetFilter(storage.NewExtensionFileFilter([]string{".segy", ".sgy", ".su"}))
	saveDialog.Show()
}

func stackExportSnapshot() ([]stackExportTrace, float64, error) {
	if len(stackCDPAxis) == 0 {
		return nil, 0, fmt.Errorf("build stack before exporting")
	}
	if len(stackTraceCacheByCDP) == 0 {
		return nil, 0, fmt.Errorf("no stack traces available for export")
	}

	templateByCDP := make(map[int64]segy.TraceHeader)
	if catalog != nil {
		for i := range catalog.Entries {
			cdp := int64(catalog.Entries[i].Header.CDP)
			if cdp <= 0 {
				continue
			}
			if _, ok := templateByCDP[cdp]; ok {
				continue
			}
			templateByCDP[cdp] = catalog.Entries[i].Header
		}
	}

	traces := make([]stackExportTrace, 0, len(stackCDPAxis))
	sampleIntervalMS := 0.0
	maxSamples := 0

	for _, cdp := range stackCDPAxis {
		entry, ok := stackTraceCacheByCDP[cdp]
		if !ok || len(entry.trace) == 0 {
			continue
		}
		if sampleIntervalMS <= 0 {
			sampleIntervalMS = entry.dtMS
		}
		if math.Abs(entry.dtMS-sampleIntervalMS) > 1e-9 {
			return nil, 0, fmt.Errorf("stack traces have inconsistent sample intervals")
		}
		if len(entry.trace) > maxSamples {
			maxSamples = len(entry.trace)
		}
		trace := stackExportTrace{
			CDP:     cdp,
			Samples: append([]float32(nil), entry.trace...),
		}
		if template, ok := templateByCDP[cdp]; ok {
			trace.Template = template
			trace.HasTemplate = true
		}
		traces = append(traces, trace)
	}

	if len(traces) == 0 {
		return nil, 0, fmt.Errorf("stack trace cache is empty")
	}
	if sampleIntervalMS <= 0 {
		return nil, 0, fmt.Errorf("stack sample interval is invalid")
	}
	if maxSamples <= 0 {
		return nil, 0, fmt.Errorf("stack sample count is invalid")
	}
	if maxSamples > math.MaxUint16 {
		return nil, 0, fmt.Errorf("stack trace length %d exceeds SEG-Y limit %d", maxSamples, math.MaxUint16)
	}

	for i := range traces {
		if len(traces[i].Samples) == maxSamples {
			continue
		}
		padded := make([]float32, maxSamples)
		copy(padded, traces[i].Samples)
		traces[i].Samples = padded
	}

	return traces, sampleIntervalMS, nil
}

func stackSampleIntervalMicroseconds(sampleIntervalMS float64) (uint16, error) {
	if sampleIntervalMS <= 0 {
		return 0, fmt.Errorf("sample interval must be > 0")
	}
	micro := int(math.Round(sampleIntervalMS * 1000.0))
	if micro <= 0 {
		return 0, fmt.Errorf("sample interval rounds to 0 microseconds")
	}
	if micro > math.MaxUint16 {
		return 0, fmt.Errorf("sample interval %d microseconds exceeds SEG-Y limit %d", micro, math.MaxUint16)
	}
	return uint16(micro), nil
}

func writeStackSEGY(w io.Writer, traces []stackExportTrace, sampleIntervalUS uint16, byteOrder binary.ByteOrder) error {
	if w == nil {
		return fmt.Errorf("nil writer")
	}
	if len(traces) == 0 {
		return fmt.Errorf("no traces to export")
	}
	if byteOrder == nil {
		byteOrder = binary.BigEndian
	}

	textual := defaultStackTextualHeader()
	extended := []segy.TextualHeader(nil)
	if loadedFile != nil {
		if !allZeroBytes(loadedFile.TextualHeader.Data[:]) {
			copy(textual, loadedFile.TextualHeader.Data[:])
		}
		extended = loadedFile.ExtendedTextualHeader
	}
	if _, err := w.Write(textual); err != nil {
		return fmt.Errorf("write textual header: %w", err)
	}

	binaryHeader := make([]byte, segy.BinaryHeaderByteLength)
	if loadedFile != nil {
		copy(binaryHeader, loadedFile.BinaryHeader.Raw[:])
	}
	samplesPerTrace := uint16(len(traces[0].Samples))
	byteOrder.PutUint16(binaryHeader[12:14], 1)
	byteOrder.PutUint16(binaryHeader[16:18], sampleIntervalUS)
	byteOrder.PutUint16(binaryHeader[18:20], sampleIntervalUS)
	byteOrder.PutUint16(binaryHeader[20:22], samplesPerTrace)
	byteOrder.PutUint16(binaryHeader[22:24], samplesPerTrace)
	byteOrder.PutUint16(binaryHeader[24:26], 5)
	byteOrder.PutUint16(binaryHeader[302:304], 1)
	byteOrder.PutUint16(binaryHeader[304:306], uint16(len(extended)))
	if _, err := w.Write(binaryHeader); err != nil {
		return fmt.Errorf("write binary header: %w", err)
	}

	for i := range extended {
		if _, err := w.Write(extended[i].Data[:]); err != nil {
			return fmt.Errorf("write extended textual header %d: %w", i+1, err)
		}
	}

	return writeStackTraceStream(w, traces, sampleIntervalUS, byteOrder)
}

func writeStackSU(w io.Writer, traces []stackExportTrace, sampleIntervalUS uint16, byteOrder binary.ByteOrder) error {
	if w == nil {
		return fmt.Errorf("nil writer")
	}
	if len(traces) == 0 {
		return fmt.Errorf("no traces to export")
	}
	if byteOrder == nil {
		byteOrder = binary.BigEndian
	}
	return writeStackTraceStream(w, traces, sampleIntervalUS, byteOrder)
}

func writeStackTraceStream(w io.Writer, traces []stackExportTrace, sampleIntervalUS uint16, byteOrder binary.ByteOrder) error {
	for i := range traces {
		header, err := stackTraceHeaderBytes(i+1, traces[i], sampleIntervalUS, byteOrder)
		if err != nil {
			return err
		}
		if _, err := w.Write(header); err != nil {
			return fmt.Errorf("write trace %d header: %w", i+1, err)
		}

		sampleBytes := make([]byte, len(traces[i].Samples)*4)
		for s, sample := range traces[i].Samples {
			byteOrder.PutUint32(sampleBytes[s*4:s*4+4], math.Float32bits(sample))
		}
		if _, err := w.Write(sampleBytes); err != nil {
			return fmt.Errorf("write trace %d samples: %w", i+1, err)
		}
	}
	return nil
}

func stackTraceHeaderBytes(traceIndex int, tr stackExportTrace, sampleIntervalUS uint16, byteOrder binary.ByteOrder) ([]byte, error) {
	if len(tr.Samples) == 0 {
		return nil, fmt.Errorf("trace %d has no samples", traceIndex)
	}
	if len(tr.Samples) > math.MaxUint16 {
		return nil, fmt.Errorf("trace %d sample count %d exceeds SEG-Y limit %d", traceIndex, len(tr.Samples), math.MaxUint16)
	}
	if tr.CDP < math.MinInt32 || tr.CDP > math.MaxInt32 {
		return nil, fmt.Errorf("trace %d has out-of-range CDP %d", traceIndex, tr.CDP)
	}

	header := make([]byte, segy.TraceHeaderByteLength)
	if tr.HasTemplate {
		copy(header, tr.Template.Raw[:])
	}

	traceNumber := uint32(traceIndex)
	byteOrder.PutUint32(header[0:4], traceNumber)
	byteOrder.PutUint32(header[4:8], traceNumber)
	byteOrder.PutUint32(header[20:24], uint32(int32(tr.CDP)))
	byteOrder.PutUint32(header[24:28], 1)
	byteOrder.PutUint16(header[28:30], 1)
	byteOrder.PutUint16(header[34:36], 1)
	byteOrder.PutUint32(header[36:40], 0)
	byteOrder.PutUint16(header[114:116], uint16(len(tr.Samples)))
	byteOrder.PutUint16(header[116:118], sampleIntervalUS)

	return header, nil
}

func defaultStackTextualHeader() []byte {
	out := make([]byte, segy.TextualHeaderByteLength)
	for i := range out {
		out[i] = ' '
	}

	lines := map[int]string{
		1:  "STACK EXPORT GENERATED BY VELOCITYANALYSIS",
		2:  "FORMAT: IEEE FLOAT32 STACK TRACES",
		3:  fmt.Sprintf("DATE: %s", time.Now().UTC().Format("2006-01-02 15:04:05 UTC")),
		40: "END TEXTUAL HEADER",
	}
	for line, text := range lines {
		if line < 1 || line > 40 {
			continue
		}
		record := fmt.Sprintf("C%02d %-76s", line, text)
		if len(record) > 80 {
			record = record[:80]
		}
		offset := (line - 1) * 80
		copy(out[offset:offset+80], []byte(record))
	}

	return out
}

func allZeroBytes(b []byte) bool {
	for i := range b {
		if b[i] != 0 {
			return false
		}
	}
	return true
}
