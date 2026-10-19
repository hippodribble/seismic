package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/hippodribble/seismic/segy"
	"github.com/hippodribble/seismic/segy/velocitywidgets"
)

const (
	windowTitleBase      = "SR View"
	prefLastOpenedURI    = "srview.last_opened_uri"
	prefSelectedShot     = "srview.selected_shot"
	prefSelectedReceiver = "srview.selected_receiver"
	prefPalette          = "srview.render.palette"
	prefClip             = "srview.render.clip"
	prefGainMode         = "srview.gain.mode"
	prefTSquaredScale    = "srview.gain.tsquared_scale"
	prefAGCWindow        = "srview.gain.agc_window"
	prefAGCFloor         = "srview.gain.agc_floor"
	prefSplitOffset      = "srview.split.offset"
)

const (
	defaultPaletteName = "Blue-White-Red"
	defaultClip        = "2.0"
	defaultGainMode    = "None"
	defaultTSquared    = "1.0"
	defaultAGCWindow   = "101"
	defaultAGCFloor    = "1e-6"
)

type renderConfig struct {
	palette   string
	clip      string
	gainMode  string
	tSquared  string
	agcWindow string
	agcFloor  string
}

var (
	mainWin fyne.Window

	statusLabel *widget.Label
	progressBar *widget.ProgressBar
	statusCh    chan string

	btnOpen     *widget.Button
	btnControls *widget.Button
	shotSelect  *widget.Select
	recvSelect  *widget.Select
	shotView    *velocitywidgets.EnsembleImageView
	recvView    *velocitywidgets.EnsembleImageView
	mainSplit   *container.Split

	controlsWin fyne.Window

	paletteSelect  *widget.Select
	clipEntry      *widget.Entry
	gainModeSelect *widget.Select
	tSquaredEntry  *widget.Entry
	agcWindowEntry *widget.Entry
	agcFloorEntry  *widget.Entry
	tSquaredCard   *widget.Card
	agcCard        *widget.Card

	loadMu         sync.Mutex
	loadInProgress bool

	currentFile    *segy.File
	currentCatalog *segy.TraceCatalog
	currentURI     fyne.URI
	currentShots   []int64
	currentRecs    []int64
	selectedShot   int64
	selectedRec    int64
	shotEntries    []segy.TraceIndexEntry
	recEntries     []segy.TraceIndexEntry
	currentStatus  = "Ready"
	renderToken    uint64

	suppressSelection bool
	suppressControls  bool
	viewSync          = velocitywidgets.NewEnsembleViewSync()
)

func main() {
	app := app.NewWithID("com.github.hippodribble.seismic.srview")
	mainWin = app.NewWindow(windowTitleBase)
	mainWin.SetOnClosed(func() {
		persistAllPreferences()
		app.Quit()
	})
	mainWin.SetContent(buildUI())
	mainWin.Resize(fyne.NewSize(1400, 900))
	mainWin.SetFullScreen(true)
	go restoreLastOpenedFile()
	mainWin.ShowAndRun()
}

func buildUI() fyne.CanvasObject {
	statusLabel = widget.NewLabel(currentStatus)
	progressBar = widget.NewProgressBar()
	statusCh = make(chan string, 64)
	go watchStatus()

	btnOpen = widget.NewButton("Open File", openFileDialog)
	btnControls = widget.NewButton("Controls", toggleControlsWindow)

	shotSelect = widget.NewSelect(nil, func(value string) {
		if suppressSelection {
			return
		}
		v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return
		}
		selectShot(v)
	})
	shotSelect.Disable()

	recvSelect = widget.NewSelect(nil, func(value string) {
		if suppressSelection {
			return
		}
		v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return
		}
		selectReceiver(v)
	})
	recvSelect.Disable()

	shotView = newView()
	recvView = newView()
	viewSync.Register(shotView)
	viewSync.Register(recvView)
	shotView.SetOnViewChanged(func(minMS, maxMS float64) {
		viewSync.SyncFrom(shotView, minMS, maxMS)
	})
	recvView.SetOnViewChanged(func(minMS, maxMS float64) {
		viewSync.SyncFrom(recvView, minMS, maxMS)
	})
	wireViewCallbacks()

	controls := container.NewHBox(
		btnOpen,
		btnControls,
		widget.NewLabel("Shot"),
		shotSelect,
		widget.NewLabel("Receiver"),
		recvSelect,
	)
	content := container.NewHSplit(
		container.NewBorder(widget.NewLabel("Shot Record"), nil, nil, nil, shotView),
		container.NewBorder(widget.NewLabel("Common Receiver Gather"), nil, nil, nil, recvView),
	)
	content.Offset = 0.5
	mainSplit = content
	restoreSplitOffset()
	footer := container.NewBorder(nil, nil, nil, progressBar, statusLabel)

	initControlsWindow()
	restoreRenderPreferences()

	return container.NewBorder(controls, footer, nil, nil, content)
}

func newView() *velocitywidgets.EnsembleImageView {
	view := velocitywidgets.NewEnsembleImageView(velocitywidgets.EnsembleImageViewOptions{
		ShowTimeAxis:  true,
		AxisWidth:     64,
		ZoomFactor:    1.05,
		MinViewSpanMS: 20,
		MinSize:       fyne.NewSize(240, 240),
	})
	view.SetCursorGuideEnabled(true)
	view.SetHorizontalCursorGuideEnabled(true)
	return view
}

func wireViewCallbacks() {
	shotView.SetOnCursorMoved(func(info velocitywidgets.EnsembleCursorInfo) {
		syncCursors(info)
		if info.Inside {
			tracf := int64(0)
			idx := traceIndexAtFraction(shotEntries, info.XFrac)
			if idx >= 0 && idx < len(shotEntries) {
				tracf = traceValue(shotEntries[idx].Header, segy.TraceHeaderFieldTRACF)
			}
			setTransientStatus(fmt.Sprintf("Shot %d  TRACF %d  time %.1f ms", selectedShot, tracf, info.TimeMS))
		} else {
			restoreSelectionStatus()
		}
	})
	shotView.SetOnCursorTapped(func(info velocitywidgets.EnsembleCursorInfo) {
		if !info.Inside || len(shotEntries) == 0 {
			return
		}
		idx := traceIndexAtFraction(shotEntries, info.XFrac)
		if idx >= 0 {
			selectReceiver(traceValue(shotEntries[idx].Header, segy.TraceHeaderFieldTRACF))
		}
	})

	recvView.SetOnCursorMoved(func(info velocitywidgets.EnsembleCursorInfo) {
		syncCursors(info)
		if info.Inside {
			ep := int64(0)
			idx := traceIndexAtFraction(recEntries, info.XFrac)
			if idx >= 0 && idx < len(recEntries) {
				ep = traceValue(recEntries[idx].Header, segy.TraceHeaderFieldEP)
			}
			setTransientStatus(fmt.Sprintf("Receiver %d  EP %d  time %.1f ms", selectedRec, ep, info.TimeMS))
		} else {
			restoreSelectionStatus()
		}
	})
	recvView.SetOnCursorTapped(func(info velocitywidgets.EnsembleCursorInfo) {
		if !info.Inside || len(recEntries) == 0 {
			return
		}
		idx := traceIndexAtFraction(recEntries, info.XFrac)
		if idx >= 0 {
			selectShot(traceValue(recEntries[idx].Header, segy.TraceHeaderFieldEP))
		}
	})
}

func openFileDialog() {
	if mainWin == nil {
		return
	}
	d := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, mainWin)
			return
		}
		if reader == nil {
			return
		}
		uri := reader.URI()
		reader.Close()
		loadFileFromURI(uri)
	}, mainWin)
	d.SetTitleText("Open SEG-Y/SU file")
	d.SetFilter(storage.NewExtensionFileFilter([]string{".segy", ".sgy", ".su"}))
	d.Show()
}

func loadFileFromURI(uri fyne.URI) {
	if uri == nil {
		return
	}
	if !beginLoad() {
		pushStatus("A load is already in progress")
		return
	}
	setLoadUI(true)
	setProgress(0)
	pushStatus(fmt.Sprintf("Loading %s ...", uri.Name()))

	go func() {
		defer finishLoad()
		defer setProgress(0)

		file, catalog, err := openAndCatalog(uri)
		if err != nil {
			fyne.Do(func() { dialog.ShowError(err, mainWin) })
			pushStatus(fmt.Sprintf("Failed loading %s", uri.Name()))
			return
		}

		currentFile = file
		currentCatalog = catalog
		currentURI = uri
		persistLastOpenedURI(uri)
		fyne.Do(func() {
			setMainWindowTitleForURI(uri)
			refreshOptionsFromCatalog()
			restoreSelectionFromPreferences()
			renderCurrentSelection()
		})
		pushStatus(fmt.Sprintf("Loaded %s", uri.Name()))
	}()
}

func openAndCatalog(uri fyne.URI) (*segy.File, *segy.TraceCatalog, error) {
	name := strings.ToLower(uri.Name())
	var (
		file *segy.File
		err  error
	)
	if strings.HasSuffix(name, ".su") {
		file, err = segy.OpenSU(uri)
	} else {
		file, err = segy.Open(uri)
	}
	if err != nil {
		return nil, nil, err
	}

	if strings.HasSuffix(name, ".su") {
		file.TraceCatalog, err = file.BuildTraceCatalogTraceOnlyWithProgress(func(p segy.CatalogBuildProgress) {
			updateProgressFromCatalog(p)
		})
	} else {
		file.TraceCatalog, err = file.BuildTraceCatalogWithProgress(func(p segy.CatalogBuildProgress) {
			updateProgressFromCatalog(p)
		})
	}
	if err != nil {
		return nil, nil, err
	}
	return file, file.TraceCatalog, nil
}

func updateProgressFromCatalog(progress segy.CatalogBuildProgress) {
	if progress.TotalBytes > 0 {
		setProgress(float64(progress.BytesProcessed) / float64(progress.TotalBytes))
	}
}

func refreshOptionsFromCatalog() {
	if currentCatalog == nil {
		currentShots = nil
		currentRecs = nil
		updateSelectionControls()
		return
	}
	currentShots = uniqueValues(currentCatalog.Entries, segy.TraceHeaderFieldEP)
	currentRecs = uniqueValues(currentCatalog.Entries, segy.TraceHeaderFieldTRACF)
	updateSelectionControls()
}

func restoreSelectionFromPreferences() {
	selectedShot = prefInt64(prefSelectedShot)
	selectedRec = prefInt64(prefSelectedReceiver)
	if len(currentShots) > 0 && !containsInt64(currentShots, selectedShot) {
		selectedShot = currentShots[0]
	}
	if len(currentRecs) > 0 && !containsInt64(currentRecs, selectedRec) {
		selectedRec = firstReceiverForShot(selectedShot)
	}
	if selectedShot == 0 && len(currentShots) > 0 {
		selectedShot = currentShots[0]
	}
	if selectedRec == 0 && len(currentRecs) > 0 {
		selectedRec = currentRecs[0]
	}
	updateSelectionControls()
}

func selectShot(shot int64) {
	if shot <= 0 {
		return
	}
	selectedShot = shot
	if selectedRec == 0 {
		selectedRec = firstReceiverForShot(shot)
	}
	updateSelectionControls()
	renderCurrentSelection()
}

func selectReceiver(receiver int64) {
	if receiver <= 0 {
		return
	}
	selectedRec = receiver
	updateSelectionControls()
	renderCurrentSelection()
}

func updateSelectionControls() {
	if shotSelect != nil {
		suppressSelection = true
		shotSelect.SetOptions(int64Strings(currentShots))
		if selectedShot > 0 {
			shotSelect.SetSelected(strconv.FormatInt(selectedShot, 10))
		}
		if len(currentShots) == 0 {
			shotSelect.Disable()
		} else {
			shotSelect.Enable()
		}
		suppressSelection = false
	}
	if recvSelect != nil {
		suppressSelection = true
		recvSelect.SetOptions(int64Strings(currentRecs))
		if selectedRec > 0 {
			recvSelect.SetSelected(strconv.FormatInt(selectedRec, 10))
		}
		if len(currentRecs) == 0 {
			recvSelect.Disable()
		} else {
			recvSelect.Enable()
		}
		suppressSelection = false
	}
}

func renderCurrentSelection() {
	if currentFile == nil || currentCatalog == nil || selectedShot == 0 || selectedRec == 0 {
		clearViews()
		return
	}
	cfg := snapshotRenderConfig()
	token := atomic.AddUint64(&renderToken, 1)
	pushStatus(fmt.Sprintf("Rendering shot %d / receiver %d ...", selectedShot, selectedRec))

	go func(localToken uint64, config renderConfig) {

		shot, err := currentCatalog.FindWhereSorted([]segy.TraceQueryPredicate{segy.EqualPredicate(segy.TraceHeaderFieldEP, selectedShot)}, segy.Asc(segy.TraceHeaderFieldTRACF))
		if err != nil {
			if localToken == atomic.LoadUint64(&renderToken) {
				pushStatus(fmt.Sprintf("Shot lookup failed: %v", err))
			}
			return
		}
		rec, err := currentCatalog.FindWhereSorted([]segy.TraceQueryPredicate{segy.EqualPredicate(segy.TraceHeaderFieldTRACF, selectedRec)}, segy.Asc(segy.TraceHeaderFieldEP))
		if err != nil {
			if localToken == atomic.LoadUint64(&renderToken) {
				pushStatus(fmt.Sprintf("Receiver lookup failed: %v", err))
			}
			return
		}

		shotOpts, err := buildImageOptions(config, sampleIntervalMS(shot))
		if err != nil {
			if localToken == atomic.LoadUint64(&renderToken) {
				pushStatus(err.Error())
			}
			return
		}
		recOpts, err := buildImageOptions(config, sampleIntervalMS(rec))
		if err != nil {
			if localToken == atomic.LoadUint64(&renderToken) {
				pushStatus(err.Error())
			}
			return
		}

		shotImage, shotStats, err := currentFile.RenderEnsembleFromEntries(shot, segy.DefaultMaxImageSamples, shotOpts)
		if err != nil {
			if localToken == atomic.LoadUint64(&renderToken) {
				pushStatus(fmt.Sprintf("Shot render failed: %v", err))
			}
			return
		}
		recImage, recStats, err := currentFile.RenderEnsembleFromEntries(rec, segy.DefaultMaxImageSamples, recOpts)
		if err != nil {
			if localToken == atomic.LoadUint64(&renderToken) {
				pushStatus(fmt.Sprintf("Receiver render failed: %v", err))
			}
			return
		}

		if localToken != atomic.LoadUint64(&renderToken) {
			return
		}

		fyne.Do(func() {
			shotEntries = append([]segy.TraceIndexEntry(nil), shot...)
			recEntries = append([]segy.TraceIndexEntry(nil), rec...)
			setImage(shotView, shotImage, shotStats)
			setImage(recvView, recImage, recStats)
			viewSync.ApplyCurrent(shotView)
			viewSync.ApplyCurrent(recvView)
			shotView.SetActiveVerticalMarkerFraction(markerFractionForSelection(shotEntries, segy.TraceHeaderFieldTRACF, selectedRec))
			recvView.SetActiveVerticalMarkerFraction(markerFractionForSelection(recEntries, segy.TraceHeaderFieldEP, selectedShot))
			updateSelectionStatus()
		})
		pushStatus(fmt.Sprintf("Shot %d (%d traces) | Receiver %d (%d traces)", selectedShot, len(shot), selectedRec, len(rec)))
	}(token, cfg)
}

func setImage(view *velocitywidgets.EnsembleImageView, img *image.Paletted, stats segy.EnsembleImageStats) {
	if view == nil {
		return
	}
	if img == nil || stats.Height == 0 {
		view.Clear()
		return
	}
	view.SetImage(img, sampleIntervalMSFromStats(stats), stats.Height)
}

func sampleIntervalMSFromStats(stats segy.EnsembleImageStats) float64 {
	if stats.Height <= 0 {
		return 1
	}
	if len(shotEntries) > 0 {
		return sampleIntervalMS(shotEntries)
	}
	if len(recEntries) > 0 {
		return sampleIntervalMS(recEntries)
	}
	return 1
}

func clearViews() {
	shotEntries = nil
	recEntries = nil
	if shotView != nil {
		shotView.Clear()
		shotView.SetActiveVerticalMarkerFraction(math.NaN())
	}
	if recvView != nil {
		recvView.Clear()
		recvView.SetActiveVerticalMarkerFraction(math.NaN())
	}
	updateSelectionStatus()
}

func updateSelectionStatus() {
	if selectedShot > 0 && selectedRec > 0 {
		pushStatus(fmt.Sprintf("Shot %d | Receiver %d", selectedShot, selectedRec))
		return
	}
	pushStatus("Ready")
}

func syncCursors(info velocitywidgets.EnsembleCursorInfo) {
	if shotView == nil || recvView == nil {
		return
	}
	if !info.Inside {
		shotView.ClearCursorGuideTime()
		recvView.ClearCursorGuideTime()
		return
	}
	shotView.SetCursorGuideTimeMS(info.TimeMS)
	recvView.SetCursorGuideTimeMS(info.TimeMS)
}

func buildImageOptions(config renderConfig, sampleMS float64) (segy.EnsembleImageOptions, error) {
	clip, err := strconv.ParseFloat(config.clip, 64)
	if err != nil {
		return segy.EnsembleImageOptions{}, fmt.Errorf("clip percentile must be numeric")
	}
	if clip < 0 {
		return segy.EnsembleImageOptions{}, fmt.Errorf("clip percentile must be non-negative")
	}
	gain := segy.GainOptions{Mode: segy.GainModeNone}
	switch config.gainMode {
	case "T-Squared":
		scale, err := strconv.ParseFloat(config.tSquared, 64)
		if err != nil || scale <= 0 {
			return segy.EnsembleImageOptions{}, fmt.Errorf("T-Squared scale must be > 0")
		}
		gain = segy.GainOptions{Mode: segy.GainModeTSquared, TSquaredScale: scale, SampleIntervalSec: sampleMS / 1000.0}
	case "AGC":
		window, err := strconv.Atoi(config.agcWindow)
		if err != nil || window <= 0 {
			return segy.EnsembleImageOptions{}, fmt.Errorf("AGC window must be > 0")
		}
		floor, err := strconv.ParseFloat(config.agcFloor, 64)
		if err != nil || floor <= 0 {
			return segy.EnsembleImageOptions{}, fmt.Errorf("AGC floor must be > 0")
		}
		gain = segy.GainOptions{Mode: segy.GainModeAGC, AGCWindowSamples: window, AGCFloor: floor}
	}
	return segy.EnsembleImageOptions{
		Palette:        selectedPalette(config.palette),
		PercentileClip: clip,
		ZeroCentered:   true,
		ZeroIndex:      127,
		Gain:           gain,
	}, nil
}

func selectedPalette(name string) color.Palette {
	switch name {
	case "Grayscale":
		return segy.GrayscalePalette256()
	case "Grayscale (Inverted)":
		return segy.InvertPalette(segy.GrayscalePalette256())
	case "Blue-White-Red (Inverted)":
		return segy.InvertPalette(segy.BlueWhiteRedPalette256())
	default:
		return segy.BlueWhiteRedPalette256()
	}
}

func snapshotRenderConfig() renderConfig {
	cfg := renderConfig{
		palette:   defaultPaletteName,
		clip:      defaultClip,
		gainMode:  defaultGainMode,
		tSquared:  defaultTSquared,
		agcWindow: defaultAGCWindow,
		agcFloor:  defaultAGCFloor,
	}
	if paletteSelect != nil {
		cfg.palette = paletteSelect.Selected
	}
	if clipEntry != nil {
		cfg.clip = strings.TrimSpace(clipEntry.Text)
	}
	if gainModeSelect != nil {
		cfg.gainMode = gainModeSelect.Selected
	}
	if tSquaredEntry != nil {
		cfg.tSquared = strings.TrimSpace(tSquaredEntry.Text)
	}
	if agcWindowEntry != nil {
		cfg.agcWindow = strings.TrimSpace(agcWindowEntry.Text)
	}
	if agcFloorEntry != nil {
		cfg.agcFloor = strings.TrimSpace(agcFloorEntry.Text)
	}
	return cfg
}

func uniqueValues(entries []segy.TraceIndexEntry, field segy.TraceHeaderField) []int64 {
	seen := make(map[int64]struct{}, len(entries))
	values := make([]int64, 0, len(entries))
	for i := range entries {
		v := traceValue(entries[i].Header, field)
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values
}

func int64Strings(values []int64) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.FormatInt(value, 10))
	}
	return out
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func traceValue(header segy.TraceHeader, field segy.TraceHeaderField) int64 {
	switch field {
	case segy.TraceHeaderFieldEP:
		return int64(header.EnergySourcePointNumber)
	case segy.TraceHeaderFieldTRACF:
		return int64(header.TraceNumberWithinRecord)
	case segy.TraceHeaderFieldTRACR:
		return int64(header.TraceSequenceLine)
	case segy.TraceHeaderFieldOFFSET:
		return int64(header.SourceReceiverOffset)
	case segy.TraceHeaderFieldCDP:
		return int64(header.CDP)
	case segy.TraceHeaderFieldSX:
		return int64(header.SourceX)
	case segy.TraceHeaderFieldSY:
		return int64(header.SourceY)
	case segy.TraceHeaderFieldGX:
		return int64(header.GroupX)
	case segy.TraceHeaderFieldGY:
		return int64(header.GroupY)
	default:
		return 0
	}
}

func traceIndexAtFraction(entries []segy.TraceIndexEntry, xFrac float64) int {
	if len(entries) == 0 {
		return -1
	}
	if xFrac < 0 {
		xFrac = 0
	}
	if xFrac > 1 {
		xFrac = 1
	}
	idx := int(math.Round(xFrac * float64(len(entries)-1)))
	if idx < 0 || idx >= len(entries) {
		return -1
	}
	return idx
}

func markerFractionForSelection(entries []segy.TraceIndexEntry, field segy.TraceHeaderField, value int64) float64 {
	if len(entries) == 0 {
		return math.NaN()
	}
	idx := -1
	for i := range entries {
		if traceValue(entries[i].Header, field) == value {
			idx = i
			break
		}
	}
	if idx < 0 {
		return math.NaN()
	}
	if len(entries) == 1 {
		return 0
	}
	return float64(idx) / float64(len(entries)-1)
}

func sampleIntervalMS(entries []segy.TraceIndexEntry) float64 {
	if len(entries) == 0 {
		return 1
	}
	ms := float64(entries[0].Header.SampleIntervalMicrosec) / 1000.0
	if ms <= 0 && currentFile != nil && currentFile.BinaryHeader.SampleIntervalMicrosec > 0 {
		ms = float64(currentFile.BinaryHeader.SampleIntervalMicrosec) / 1000.0
	}
	if ms <= 0 {
		ms = 1
	}
	return ms
}

func firstReceiverForShot(shot int64) int64 {
	if currentCatalog == nil || shot <= 0 {
		return 0
	}
	entries, err := currentCatalog.FindWhereSorted([]segy.TraceQueryPredicate{segy.EqualPredicate(segy.TraceHeaderFieldEP, shot)}, segy.Asc(segy.TraceHeaderFieldTRACF))
	if err != nil || len(entries) == 0 {
		return 0
	}
	return int64(entries[0].Header.TraceNumberWithinRecord)
}

func initControlsWindow() {
	if mainWin == nil || controlsWin != nil {
		return
	}
	paletteSelect = widget.NewSelect([]string{"Blue-White-Red", "Blue-White-Red (Inverted)", "Grayscale", "Grayscale (Inverted)"}, func(string) {
		if suppressControls {
			return
		}
		renderCurrentSelection()
	})
	clipEntry = widget.NewEntry()
	clipEntry.OnSubmitted = func(string) {
		if !suppressControls {
			renderCurrentSelection()
		}
	}
	gainModeSelect = widget.NewSelect([]string{"None", "T-Squared", "AGC"}, func(string) {
		updateGainControlState()
		if !suppressControls {
			renderCurrentSelection()
		}
	})
	tSquaredEntry = widget.NewEntry()
	tSquaredEntry.OnSubmitted = func(string) {
		if !suppressControls {
			renderCurrentSelection()
		}
	}
	agcWindowEntry = widget.NewEntry()
	agcWindowEntry.OnSubmitted = func(string) {
		if !suppressControls {
			renderCurrentSelection()
		}
	}
	agcFloorEntry = widget.NewEntry()
	agcFloorEntry.OnSubmitted = func(string) {
		if !suppressControls {
			renderCurrentSelection()
		}
	}
	tSquaredCard = widget.NewCard("T-Squared", "", widget.NewForm(widget.NewFormItem("Scale", tSquaredEntry)))
	agcCard = widget.NewCard("AGC", "", widget.NewForm(
		widget.NewFormItem("Window", agcWindowEntry),
		widget.NewFormItem("Floor", agcFloorEntry),
	))

	content := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("Palette", paletteSelect),
			widget.NewFormItem("Clip percentile", clipEntry),
			widget.NewFormItem("Gain mode", gainModeSelect),
		),
		tSquaredCard,
		agcCard,
		container.NewHBox(
			widget.NewButton("Apply", renderCurrentSelection),
			widget.NewButton("Reset Defaults", func() {
				suppressControls = true
				paletteSelect.SetSelected(defaultPaletteName)
				clipEntry.SetText(defaultClip)
				gainModeSelect.SetSelected(defaultGainMode)
				tSquaredEntry.SetText(defaultTSquared)
				agcWindowEntry.SetText(defaultAGCWindow)
				agcFloorEntry.SetText(defaultAGCFloor)
				suppressControls = false
				updateGainControlState()
				renderCurrentSelection()
			}),
		),
	)
	controlsWin = fyne.CurrentApp().NewWindow("SR View Controls")
	controlsWin.SetContent(container.NewScroll(content))
	controlsWin.Resize(fyne.NewSize(420, 420))
	controlsWin.SetOnClosed(func() {
		controlsWin = nil
		persistRenderPreferences()
	})
}

func toggleControlsWindow() {
	if controlsWin == nil {
		initControlsWindow()
	}
	if controlsWin == nil {
		return
	}
	controlsWin.Show()
	controlsWin.RequestFocus()
}

func updateGainControlState() {
	if gainModeSelect == nil || tSquaredEntry == nil || agcWindowEntry == nil || agcFloorEntry == nil || tSquaredCard == nil || agcCard == nil {
		return
	}
	switch gainModeSelect.Selected {
	case "T-Squared":
		tSquaredEntry.Enable()
		agcWindowEntry.Disable()
		agcFloorEntry.Disable()
		tSquaredCard.Show()
		agcCard.Hide()
	case "AGC":
		tSquaredEntry.Disable()
		agcWindowEntry.Enable()
		agcFloorEntry.Enable()
		tSquaredCard.Hide()
		agcCard.Show()
	default:
		tSquaredEntry.Disable()
		agcWindowEntry.Disable()
		agcFloorEntry.Disable()
		tSquaredCard.Hide()
		agcCard.Hide()
	}
}

func restoreRenderPreferences() {
	if fyne.CurrentApp() == nil || paletteSelect == nil {
		return
	}
	prefs := fyne.CurrentApp().Preferences()
	suppressControls = true
	paletteSelect.SetSelected(prefs.StringWithFallback(prefPalette, defaultPaletteName))
	clipEntry.SetText(prefs.StringWithFallback(prefClip, defaultClip))
	gainModeSelect.SetSelected(prefs.StringWithFallback(prefGainMode, defaultGainMode))
	tSquaredEntry.SetText(prefs.StringWithFallback(prefTSquaredScale, defaultTSquared))
	agcWindowEntry.SetText(prefs.StringWithFallback(prefAGCWindow, defaultAGCWindow))
	agcFloorEntry.SetText(prefs.StringWithFallback(prefAGCFloor, defaultAGCFloor))
	suppressControls = false
	updateGainControlState()
}

func persistRenderPreferences() {
	if fyne.CurrentApp() == nil {
		return
	}
	prefs := fyne.CurrentApp().Preferences()
	if paletteSelect != nil {
		prefs.SetString(prefPalette, paletteSelect.Selected)
	}
	if clipEntry != nil {
		prefs.SetString(prefClip, clipEntry.Text)
	}
	if gainModeSelect != nil {
		prefs.SetString(prefGainMode, gainModeSelect.Selected)
	}
	if tSquaredEntry != nil {
		prefs.SetString(prefTSquaredScale, tSquaredEntry.Text)
	}
	if agcWindowEntry != nil {
		prefs.SetString(prefAGCWindow, agcWindowEntry.Text)
	}
	if agcFloorEntry != nil {
		prefs.SetString(prefAGCFloor, agcFloorEntry.Text)
	}
}

func persistAllPreferences() {
	persistRenderPreferences()
	if fyne.CurrentApp() == nil {
		return
	}
	prefs := fyne.CurrentApp().Preferences()
	if mainSplit != nil {
		prefs.SetString(prefSplitOffset, strconv.FormatFloat(mainSplit.Offset, 'f', 6, 64))
	}
	if currentURI != nil {
		prefs.SetString(prefLastOpenedURI, currentURI.String())
	}
	if selectedShot > 0 {
		prefs.SetString(prefSelectedShot, strconv.FormatInt(selectedShot, 10))
	}
	if selectedRec > 0 {
		prefs.SetString(prefSelectedReceiver, strconv.FormatInt(selectedRec, 10))
	}
}

func restoreSplitOffset() {
	if fyne.CurrentApp() == nil || mainSplit == nil {
		return
	}
	text := strings.TrimSpace(fyne.CurrentApp().Preferences().String(prefSplitOffset))
	if text == "" {
		return
	}
	offset, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return
	}
	if offset < 0.05 {
		offset = 0.05
	}
	if offset > 0.95 {
		offset = 0.95
	}
	mainSplit.Offset = offset
}

func restoreLastOpenedFile() {
	if fyne.CurrentApp() == nil {
		return
	}
	uriString := strings.TrimSpace(fyne.CurrentApp().Preferences().String(prefLastOpenedURI))
	if uriString == "" {
		return
	}
	uri, err := storage.ParseURI(uriString)
	if err != nil {
		return
	}
	if strings.EqualFold(uri.Scheme(), "file") {
		if _, err := os.Stat(uri.Path()); err != nil {
			return
		}
	}
	loadFileFromURI(uri)
}

func persistLastOpenedURI(uri fyne.URI) {
	if fyne.CurrentApp() == nil || uri == nil {
		return
	}
	fyne.CurrentApp().Preferences().SetString(prefLastOpenedURI, uri.String())
}

func setMainWindowTitleForURI(uri fyne.URI) {
	if mainWin == nil {
		return
	}
	if uri == nil {
		mainWin.SetTitle(windowTitleBase)
		return
	}
	path := strings.TrimSpace(uri.Path())
	if path == "" {
		path = uri.String()
	}
	mainWin.SetTitle(fmt.Sprintf("%s - %s", windowTitleBase, path))
}

func prefInt64(key string) int64 {
	if fyne.CurrentApp() == nil {
		return 0
	}
	value := strings.TrimSpace(fyne.CurrentApp().Preferences().String(key))
	if value == "" {
		return 0
	}
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func setLoadUI(loading bool) {
	fyne.Do(func() {
		if btnOpen == nil {
			return
		}
		if loading {
			btnOpen.Disable()
		} else {
			btnOpen.Enable()
		}
	})
}

func beginLoad() bool {
	loadMu.Lock()
	defer loadMu.Unlock()
	if loadInProgress {
		return false
	}
	loadInProgress = true
	return true
}

func finishLoad() {
	loadMu.Lock()
	loadInProgress = false
	loadMu.Unlock()
	setLoadUI(false)
}

func setProgress(value float64) {
	if progressBar == nil {
		return
	}
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	fyne.Do(func() { progressBar.SetValue(value) })
}

func pushStatus(message string) {
	currentStatus = message
	if statusCh != nil {
		statusCh <- message
	}
}

func restoreSelectionStatus() {
	if selectedShot > 0 && selectedRec > 0 {
		pushStatus(fmt.Sprintf("Shot %d | Receiver %d", selectedShot, selectedRec))
		return
	}
	pushStatus("Ready")
}

func setTransientStatus(message string) {
	fyne.Do(func() {
		if statusLabel != nil {
			statusLabel.SetText(message)
		}
	})
}

func watchStatus() {
	for message := range statusCh {
		text := message
		fyne.Do(func() {
			if statusLabel != nil {
				statusLabel.SetText(text)
			}
		})
	}
}
