package main

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/hippodribble/seismic/segy"
	"golang.org/x/text/encoding/charmap"
)

const (
	prefPrimaryField     = "popup.primary.field"
	prefPrimaryMode      = "popup.primary.mode"
	prefPrimaryMin       = "popup.primary.min"
	prefPrimaryMax       = "popup.primary.max"
	prefSecondaryEnabled = "popup.secondary.enabled"
	prefSecondaryField   = "popup.secondary.field"
	prefSecondaryMode    = "popup.secondary.mode"
	prefSecondaryMin     = "popup.secondary.min"
	prefSecondaryMax     = "popup.secondary.max"

	prefSortField1 = "popup.sort.field1"
	prefSortOrder1 = "popup.sort.order1"
	prefSortField2 = "popup.sort.field2"
	prefSortOrder2 = "popup.sort.order2"

	prefPalette      = "popup.render.palette"
	prefClip         = "popup.render.clip"
	prefNegClip      = "popup.render.negclip"
	prefPosClip      = "popup.render.posclip"
	prefTimeStartMs  = "popup.render.time_start_ms"
	prefTimeEndMs    = "popup.render.time_end_ms"
	prefZeroCentered = "popup.render.zero_centered"
	prefMaxSamples   = "popup.render.max_samples"

	prefGainMode      = "popup.gain.mode"
	prefTSquaredScale = "popup.gain.tsquared_scale"
	prefAGCWindow     = "popup.gain.agc_window"
	prefAGCFloor      = "popup.gain.agc_floor"

	prefLastSEGYURI        = "app.last_segy_uri"
	prefHasRenderableState = "app.render_state.available"
)

var (
	winMain fyne.Window

	centre                    *fyne.Container
	statusLabel               *widget.Label
	renderButton              *widget.Button
	textHeaderButton          *widget.Button
	reelHeaderButton          *widget.Button
	traceInspectCheck         *widget.Check
	traceHeaderPanel          *widget.Card
	traceHeaderHolder         *fyne.Container
	controlsPopup             *widget.PopUp
	loadedFile                *segy.File
	loadedCatalog             *segy.TraceCatalog
	loadedSummary             segy.TraceHeaderRangeSummary
	displayedEntries          []segy.TraceIndexEntry
	lastHoveredTraceIndex     int
	lastHoveredSampleIndex    int
	hasLastHoverPosition      bool
	lastHoverXFraction        float32
	lastHoverYFraction        float32
	displayedStartSampleIndex int
	displayedSampleCount      int
	fieldLabels               []string
	labelToField              map[string]segy.TraceHeaderField
	paletteOptions            []string

	primaryFieldSelect   *widget.Select
	primaryModeSelect    *widget.Select
	primaryMinEntry      *widget.Entry
	primaryMaxEntry      *widget.Entry
	secondaryEnableCheck *widget.Check
	secondaryFieldSelect *widget.Select
	secondaryModeSelect  *widget.Select
	secondaryMinEntry    *widget.Entry
	secondaryMaxEntry    *widget.Entry

	sortField1Select *widget.Select
	sortOrder1Select *widget.Select
	sortField2Select *widget.Select
	sortOrder2Select *widget.Select

	paletteSelect      *widget.Select
	zeroCenteredCheck  *widget.Check
	clipEntry          *widget.Entry
	negClipEntry       *widget.Entry
	posClipEntry       *widget.Entry
	timeStartEntry     *widget.Entry
	timeEndEntry       *widget.Entry
	maxSamplesEntry    *widget.Entry
	gainModeSelect     *widget.Select
	tSquaredScaleEntry *widget.Entry
	agcWindowEntry     *widget.Entry
	agcFloorEntry      *widget.Entry
	gainTSquaredCard   *widget.Card
	gainAGCCard        *widget.Card
)

func main() {
	a := app.NewWithID("com.github.hippodribble.seismic.segytest")
	fmt.Println(a.Metadata())
	initOptions()

	winMain = a.NewWindow("SEGY Load & Display Test")
	winMain.SetContent(gui())
	winMain.Resize(fyne.NewSize(1500, 1000))
	winMain.SetFullScreen(true)
	installKeyboardNavigation()
	tryRestoreLastSEGYSession()
	winMain.ShowAndRun()
}

func installKeyboardNavigation() {
	if winMain == nil {
		return
	}

	winMain.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev == nil {
			return
		}

		switch ev.Name {
		case fyne.KeyRight:
			shiftPrimaryRangeBySpan(+1)
		case fyne.KeyLeft:
			shiftPrimaryRangeBySpan(-1)
		case fyne.KeyHome:
			anchorPrimaryRangeToBoundary(false)
		case fyne.KeyEnd:
			anchorPrimaryRangeToBoundary(true)
		}
	})
}

func shiftPrimaryRangeBySpan(direction int64) {
	if direction == 0 || loadedCatalog == nil || loadedFile == nil {
		return
	}
	if primaryModeSelect == nil || primaryModeSelect.Selected != "Range" {
		return
	}

	minValue, err := parseInt64Entry("primary min", primaryMinEntry)
	if err != nil {
		statusLabel.SetText(fmt.Sprintf("Primary range navigation skipped: %v", err))
		return
	}
	maxValue, err := parseInt64Entry("primary max", primaryMaxEntry)
	if err != nil {
		statusLabel.SetText(fmt.Sprintf("Primary range navigation skipped: %v", err))
		return
	}
	if maxValue < minValue {
		statusLabel.SetText("Primary range navigation skipped: max must be >= min")
		return
	}

	span := maxValue - minValue + 1
	if span <= 0 {
		return
	}
	shift := span * direction

	newMin := minValue + shift
	newMax := maxValue + shift

	if field, ok := labelToField[primaryFieldSelect.Selected]; ok {
		if r, hasRange := loadedSummary.Ranges[field]; hasRange {
			if newMin < r.Min {
				newMin = r.Min
				newMax = newMin + span - 1
			}
			if newMax > r.Max {
				newMax = r.Max
				newMin = newMax - span + 1
				if newMin < r.Min {
					newMin = r.Min
				}
			}
		}
	}

	if newMax < newMin {
		return
	}

	primaryMinEntry.SetText(strconv.FormatInt(newMin, 10))
	primaryMaxEntry.SetText(strconv.FormatInt(newMax, 10))

	renderSelection()
}

func anchorPrimaryRangeToBoundary(toEnd bool) {
	if loadedCatalog == nil || loadedFile == nil {
		return
	}
	if primaryModeSelect == nil || primaryModeSelect.Selected != "Range" {
		return
	}

	field, ok := labelToField[primaryFieldSelect.Selected]
	if !ok {
		return
	}
	r, ok := loadedSummary.Ranges[field]
	if !ok {
		return
	}

	minValue, err := parseInt64Entry("primary min", primaryMinEntry)
	if err != nil {
		statusLabel.SetText(fmt.Sprintf("Primary range navigation skipped: %v", err))
		return
	}
	maxValue, err := parseInt64Entry("primary max", primaryMaxEntry)
	if err != nil {
		statusLabel.SetText(fmt.Sprintf("Primary range navigation skipped: %v", err))
		return
	}
	if maxValue < minValue {
		statusLabel.SetText("Primary range navigation skipped: max must be >= min")
		return
	}

	span := maxValue - minValue + 1
	total := r.Max - r.Min + 1
	if span <= 0 {
		return
	}
	if span > total {
		span = total
	}

	newMin := r.Min
	newMax := r.Min + span - 1
	if toEnd {
		newMax = r.Max
		newMin = r.Max - span + 1
	}

	primaryMinEntry.SetText(strconv.FormatInt(newMin, 10))
	primaryMaxEntry.SetText(strconv.FormatInt(newMax, 10))
	renderSelection()
}

func initOptions() {
	lastHoveredTraceIndex = -1
	lastHoveredSampleIndex = -1
	hasLastHoverPosition = false
	lastHoverXFraction = 0
	lastHoverYFraction = 0

	labelToField = map[string]segy.TraceHeaderField{
		"EP":     segy.TraceHeaderFieldEP,
		"TRACR":  segy.TraceHeaderFieldTRACR,
		"TRACL":  segy.TraceHeaderFieldTRACL,
		"FLDR":   segy.TraceHeaderFieldFLDR,
		"CDP":    segy.TraceHeaderFieldCDP,
		"CDPT":   segy.TraceHeaderFieldCDPT,
		"OFFSET": segy.TraceHeaderFieldOFFSET,
		"SX":     segy.TraceHeaderFieldSX,
		"SY":     segy.TraceHeaderFieldSY,
		"GX":     segy.TraceHeaderFieldGX,
		"GY":     segy.TraceHeaderFieldGY,
	}

	fieldLabels = []string{"EP", "TRACR", "TRACL", "FLDR", "CDP", "CDPT", "OFFSET", "SX", "SY", "GX", "GY"}
	paletteOptions = []string{"Grayscale", "Grayscale (Inverted)", "Blue-White-Red", "Blue-White-Red (Inverted)"}
}

func gui() fyne.CanvasObject {
	statusLabel = widget.NewLabel("Load a SEG-Y file to build a catalog.")
	loadButton := widget.NewButton("Load SEGY", func() {
		loader()
	})
	textHeaderButton = widget.NewButton("Character Header/s", func() {
		showTextHeadersPopup()
	})
	textHeaderButton.Disable()
	reelHeaderButton = widget.NewButton("Reel Header", func() {
		showReelHeaderPopup()
	})
	reelHeaderButton.Disable()
	traceInspectCheck = widget.NewCheck("Trace inspect", func(checked bool) {
		updateTraceHeaderPanelVisibility(checked)
	})
	traceInspectCheck.SetChecked(false)
	controlsButton := widget.NewButton("Controls", func() {
		toggleControlsPopup()
	})

	primaryFieldSelect = widget.NewSelect(fieldLabels, func(label string) {
		if loadedCatalog != nil {
			applyFieldRangeDefaults(label, primaryMinEntry, primaryMaxEntry)
		}
	})
	primaryFieldSelect.SetSelected("EP")
	primaryModeSelect = widget.NewSelect([]string{"Range", "Exact"}, nil)
	primaryModeSelect.SetSelected("Range")
	primaryMinEntry = widget.NewEntry()
	primaryMaxEntry = widget.NewEntry()

	secondaryEnableCheck = widget.NewCheck("Use secondary filter", nil)
	secondaryEnableCheck.SetChecked(false)
	secondaryFieldSelect = widget.NewSelect(fieldLabels, func(label string) {
		if loadedCatalog != nil {
			applyFieldRangeDefaults(label, secondaryMinEntry, secondaryMaxEntry)
		}
	})
	secondaryFieldSelect.SetSelected("OFFSET")
	secondaryModeSelect = widget.NewSelect([]string{"Range", "Exact"}, nil)
	secondaryModeSelect.SetSelected("Range")
	secondaryMinEntry = widget.NewEntry()
	secondaryMaxEntry = widget.NewEntry()

	sortChoices := append([]string{"(none)"}, fieldLabels...)
	sortField1Select = widget.NewSelect(sortChoices, nil)
	sortField1Select.SetSelected("EP")
	sortOrder1Select = widget.NewSelect([]string{"Asc", "Desc"}, nil)
	sortOrder1Select.SetSelected("Asc")
	sortField2Select = widget.NewSelect(sortChoices, nil)
	sortField2Select.SetSelected("OFFSET")
	sortOrder2Select = widget.NewSelect([]string{"Asc", "Desc"}, nil)
	sortOrder2Select.SetSelected("Asc")

	paletteSelect = widget.NewSelect(paletteOptions, nil)
	paletteSelect.SetSelected("Blue-White-Red")
	zeroCenteredCheck = widget.NewCheck("Zero-centered mapping", nil)
	zeroCenteredCheck.SetChecked(true)
	clipEntry = widget.NewEntry()
	clipEntry.SetText("2.0")
	negClipEntry = widget.NewEntry()
	negClipEntry.SetText("2.0")
	posClipEntry = widget.NewEntry()
	posClipEntry.SetText("2.0")
	timeStartEntry = widget.NewEntry()
	timeStartEntry.SetPlaceHolder("optional")
	timeEndEntry = widget.NewEntry()
	timeEndEntry.SetPlaceHolder("optional")
	maxSamplesEntry = widget.NewEntry()
	maxSamplesEntry.SetText(strconv.FormatInt(segy.DefaultMaxImageSamples, 10))
	gainModeSelect = widget.NewSelect([]string{"None", "T-Squared", "AGC"}, func(_ string) {
		updateGainControlsState()
	})
	gainModeSelect.SetSelected("None")
	tSquaredScaleEntry = widget.NewEntry()
	tSquaredScaleEntry.SetText("1.0")
	agcWindowEntry = widget.NewEntry()
	agcWindowEntry.SetText("101")
	agcFloorEntry = widget.NewEntry()
	agcFloorEntry.SetText("1e-6")

	gainTSquaredCard = widget.NewCard("T-Squared Gain", "", widget.NewForm(
		widget.NewFormItem("Scale", tSquaredScaleEntry),
	))
	gainAGCCard = widget.NewCard("AGC", "", widget.NewForm(
		widget.NewFormItem("Window samples", agcWindowEntry),
		widget.NewFormItem("Floor", agcFloorEntry),
	))

	applyPersistedPopupSettings()
	attachPopupSettingsPersistence()
	attachEnterToRenderHandlers()
	updateGainControlsState()

	renderButton = widget.NewButton("Render Ensemble", func() {
		renderSelection()
	})
	renderButton.Disable()

	filters := container.NewGridWithColumns(2,
		widget.NewCard("Primary filter", "", container.NewVBox(
			widget.NewForm(
				widget.NewFormItem("Field", primaryFieldSelect),
				widget.NewFormItem("Mode", primaryModeSelect),
				widget.NewFormItem("Min / Value", primaryMinEntry),
				widget.NewFormItem("Max", primaryMaxEntry),
			),
		)),
		widget.NewCard("Secondary filter", "", container.NewVBox(
			secondaryEnableCheck,
			widget.NewForm(
				widget.NewFormItem("Field", secondaryFieldSelect),
				widget.NewFormItem("Mode", secondaryModeSelect),
				widget.NewFormItem("Min / Value", secondaryMinEntry),
				widget.NewFormItem("Max", secondaryMaxEntry),
			),
		)),
	)

	sorting := widget.NewCard("Sorting", "", widget.NewForm(
		widget.NewFormItem("Primary sort field", sortField1Select),
		widget.NewFormItem("Primary sort order", sortOrder1Select),
		widget.NewFormItem("Secondary sort field", sortField2Select),
		widget.NewFormItem("Secondary sort order", sortOrder2Select),
	))

	renderingForm := widget.NewForm(
		widget.NewFormItem("Palette", paletteSelect),
		widget.NewFormItem("Default clip percentile", clipEntry),
		widget.NewFormItem("Negative clip percentile", negClipEntry),
		widget.NewFormItem("Positive clip percentile", posClipEntry),
		widget.NewFormItem("Display start (ms)", timeStartEntry),
		widget.NewFormItem("Display end (ms)", timeEndEntry),
		widget.NewFormItem("Gain mode", gainModeSelect),
		widget.NewFormItem("Max image samples", maxSamplesEntry),
		widget.NewFormItem("", zeroCenteredCheck),
	)

	rendering := widget.NewCard("Rendering", "", container.NewVBox(
		renderingForm,
		gainTSquaredCard,
		gainAGCCard,
	))

	controlsPanel := container.NewVBox(
		widget.NewCard("Primary/Secondary Filters", "", filters),
		sorting,
		rendering,
		container.NewHBox(layout.NewSpacer(), renderButton),
	)

	controlsPopup = widget.NewPopUp(container.NewScroll(controlsPanel), winMain.Canvas())
	updateControlsPopupGeometry()
	controlsPopup.Hide()

	traceHeaderHolder = container.NewVBox(widget.NewLabel("Move the mouse over a trace to view header values."))
	traceHeaderScroll := container.NewVScroll(traceHeaderHolder)
	traceHeaderScroll.SetMinSize(fyne.NewSize(315, 720))
	traceHeaderPanel = widget.NewCard("Trace Header", "Read-only", traceHeaderScroll)
	traceHeaderPanel.Hide()

	top := container.NewHBox(
		loadButton,
		textHeaderButton,
		reelHeaderButton,
		traceInspectCheck,
		controlsButton,
		widget.NewSeparator(),
		statusLabel,
	)

	centre = container.NewStack(widget.NewLabel("No image rendered yet."))
	b := container.NewBorder(top, nil, nil, traceHeaderPanel, centre)
	return b
}

func toggleControlsPopup() {
	if controlsPopup == nil {
		return
	}

	if controlsPopup.Visible() {
		controlsPopup.Hide()
		return
	}

	updateControlsPopupGeometry()
	controlsPopup.Show()
}

func updateControlsPopupGeometry() {
	if controlsPopup == nil || winMain == nil {
		return
	}

	canvasSize := winMain.Canvas().Size()
	const margin = float32(12)
	const topOffset = float32(50)

	maxWidth := canvasSize.Width * 0.25
	if maxWidth < 360 {
		maxWidth = 360
	}
	availableHeight := canvasSize.Height - topOffset - margin
	if availableHeight < 320 {
		availableHeight = 320
	}

	controlsPopup.Resize(fyne.NewSize(maxWidth, availableHeight))

	popupSize := controlsPopup.Size()
	x := canvasSize.Width - popupSize.Width - margin
	if x < 0 {
		x = 0
	}
	y := topOffset
	controlsPopup.Move(fyne.NewPos(x, y))
}

func loader() {
	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, winMain)
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()

		uri := reader.URI()
		if uri == nil {
			return
		}

		filename := strings.ToLower(uri.Name())
		if !strings.HasSuffix(filename, ".segy") && !strings.HasSuffix(filename, ".sgy") {
			dialog.ShowError(errors.New("only .segy and .sgy files are supported"), winMain)
			return
		}

		winMain.SetTitle(uri.Name())
		if loadErr := loadSEGYFromURI(uri, true); loadErr != nil {
			dialog.ShowError(loadErr, winMain)
			return
		}
	}, winMain)

	fileDialog.SetTitleText("Open SEG-Y file")
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".segy", ".sgy"}))
	fileDialog.Show()
}

func loadSEGYFromURI(uri fyne.URI, resetRanges bool) error {
	if uri == nil {
		return fmt.Errorf("nil uri")
	}

	f, openErr := segy.Open(uri)
	if openErr != nil {
		return openErr
	}

	catalog, catalogErr := f.BuildTraceCatalog()
	if catalogErr != nil {
		return catalogErr
	}

	loadedFile = f
	loadedCatalog = catalog
	loadedSummary = catalog.HeaderRangeSummary()
	renderButton.Enable()
	textHeaderButton.Enable()
	reelHeaderButton.Enable()

	if resetRanges {
		applyFieldRangeDefaults(primaryFieldSelect.Selected, primaryMinEntry, primaryMaxEntry)
		applyFieldRangeDefaults(secondaryFieldSelect.Selected, secondaryMinEntry, secondaryMaxEntry)
	} else {
		applyFieldRangeDefaultsIfEmpty(primaryFieldSelect.Selected, primaryMinEntry, primaryMaxEntry)
		applyFieldRangeDefaultsIfEmpty(secondaryFieldSelect.Selected, secondaryMinEntry, secondaryMaxEntry)
	}

	// Clear any previously rendered image/selection; it no longer applies to the new file.
	displayedEntries = nil
	lastHoveredTraceIndex = -1
	lastHoveredSampleIndex = -1
	hasLastHoverPosition = false
	lastHoverXFraction = 0
	lastHoverYFraction = 0
	displayedStartSampleIndex = 0
	displayedSampleCount = 0
	if centre != nil {
		centre.Objects = []fyne.CanvasObject{widget.NewLabel("No image rendered yet.")}
		centre.Refresh()
	}
	if traceHeaderHolder != nil {
		traceHeaderHolder.Objects = []fyne.CanvasObject{widget.NewLabel("Move the mouse over a rendered trace to view header values.")}
		traceHeaderHolder.Refresh()
	}

	count := len(catalog.Entries)
	statusLabel.SetText(fmt.Sprintf("Loaded %s: %d traces cataloged", uri.Name(), count))
	fmt.Println(loadedSummary)

	if p := prefs(); p != nil {
		p.SetString(prefLastSEGYURI, uri.String())
	}

	return nil
}

func tryRestoreLastSEGYSession() {
	p := prefs()
	if p == nil {
		return
	}

	uriString := strings.TrimSpace(p.String(prefLastSEGYURI))
	if uriString == "" {
		return
	}

	uri, parseErr := storage.ParseURI(uriString)
	if parseErr != nil {
		dialog.ShowError(fmt.Errorf("parse last SEG-Y URI: %w", parseErr), winMain)
		return
	}

	if strings.EqualFold(uri.Scheme(), "file") {
		if _, statErr := os.Stat(uri.Path()); statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				dialog.ShowInformation("Last SEG-Y Missing", fmt.Sprintf("Last SEG-Y file not found:\n%s", uri.Path()), winMain)
				return
			}
			dialog.ShowError(fmt.Errorf("access last SEG-Y file: %w", statErr), winMain)
			return
		}
	}

	if loadErr := loadSEGYFromURI(uri, false); loadErr != nil {
		dialog.ShowError(fmt.Errorf("open last SEG-Y file: %w", loadErr), winMain)
		return
	}

	if p.BoolWithFallback(prefHasRenderableState, false) {
		renderSelection()
	}
}

func renderSelection() {
	if loadedFile == nil || loadedCatalog == nil {
		dialog.ShowInformation("No SEG-Y loaded", "Load a SEG-Y file first.", winMain)
		return
	}

	predicates, err := buildPredicates()
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	sortKeys, err := buildSortKeys()
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	entries, err := loadedCatalog.FindWhereSorted(predicates, sortKeys...)
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}
	if len(entries) == 0 {
		dialog.ShowInformation("No traces", "The current filters returned no traces.", winMain)
		return
	}

	maxSamples, err := parseInt64Entry("max image samples", maxSamplesEntry)
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	defaultClip, err := parseFloatEntry("default clip percentile", clipEntry)
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	negClip, err := parseFloatEntry("negative clip percentile", negClipEntry)
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	posClip, err := parseFloatEntry("positive clip percentile", posClipEntry)
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	startMs, endMs, rangeSet, err := parseTimeRangeEntries()
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	gain, err := buildGainOptions()
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	opts := segy.EnsembleImageOptions{
		Palette:                selectedPalette(paletteSelect.Selected),
		PercentileClip:         defaultClip,
		ZeroCentered:           zeroCenteredCheck.Checked,
		ZeroIndex:              127,
		NegativePercentileClip: negClip,
		PositivePercentileClip: posClip,
		Gain:                   gain,
	}

	ensemble, err := loadedFile.ReadEnsembleSamples(entries, maxSamples)
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	displayStartIdx := 0
	displaySampleCount := ensemble.SamplesPerTrace

	if rangeSet {
		var endIdx int
		ensemble, displayStartIdx, endIdx, err = subsetEnsembleByTimeRange(ensemble, loadedFile.BinaryHeader.SampleIntervalMicrosec, startMs, endMs)
		if err != nil {
			dialog.ShowError(err, winMain)
			return
		}
		displaySampleCount = endIdx - displayStartIdx
	}

	img, _, err := segy.RenderEnsembleImage(ensemble, opts)
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	displayedEntries = make([]segy.TraceIndexEntry, len(entries))
	copy(displayedEntries, entries)
	lastHoveredTraceIndex = -1
	lastHoveredSampleIndex = -1
	displayedStartSampleIndex = displayStartIdx
	displayedSampleCount = displaySampleCount

	traceImage := newTraceImageWidget(img, func(xFraction, yFraction float32) {
		onTraceImageHovered(xFraction, yFraction)
	})

	centre.Objects = []fyne.CanvasObject{traceImage}
	centre.Refresh()

	// Prime the status with hover-style values for the newly rendered range.
	if hasLastHoverPosition {
		onTraceImageHovered(lastHoverXFraction, lastHoverYFraction)
	} else {
		onTraceImageHovered(0, 0)
	}
	if p := prefs(); p != nil {
		p.SetBool(prefHasRenderableState, true)
	}
}

func parseTimeRangeEntries() (float64, float64, bool, error) {
	startRaw := strings.TrimSpace(timeStartEntry.Text)
	endRaw := strings.TrimSpace(timeEndEntry.Text)

	if startRaw == "" && endRaw == "" {
		return 0, 0, false, nil
	}

	startMs := 0.0
	if startRaw != "" {
		parsedStart, err := strconv.ParseFloat(startRaw, 64)
		if err != nil {
			return 0, 0, false, fmt.Errorf("invalid display start (ms): %q", startRaw)
		}
		startMs = parsedStart
	}
	if startMs < 0 {
		return 0, 0, false, fmt.Errorf("display start/end must be >= 0")
	}

	if endRaw == "" {
		return startMs, -1, true, nil
	}

	endMs, err := strconv.ParseFloat(endRaw, 64)
	if err != nil {
		return 0, 0, false, fmt.Errorf("invalid display end (ms): %q", endRaw)
	}
	if startMs < 0 || endMs < 0 {
		return 0, 0, false, fmt.Errorf("display start/end must be >= 0")
	}
	if endMs <= startMs {
		return 0, 0, false, fmt.Errorf("display end (ms) must be greater than start (ms)")
	}

	return startMs, endMs, true, nil
}

func subsetEnsembleByTimeRange(samples segy.EnsembleSamples, sampleIntervalMicrosec uint16, startMs, endMs float64) (segy.EnsembleSamples, int, int, error) {
	if samples.TraceCount == 0 || samples.SamplesPerTrace == 0 {
		return samples, 0, 0, fmt.Errorf("empty ensemble samples")
	}
	if sampleIntervalMicrosec == 0 {
		return samples, 0, 0, fmt.Errorf("cannot apply time range: sample interval is zero")
	}

	dtUs := float64(sampleIntervalMicrosec)
	startUs := startMs * 1000.0
	endUs := endMs * 1000.0
	if endMs < 0 {
		endUs = float64(samples.SamplesPerTrace) * dtUs
	}

	startIdx := int(math.Floor(startUs / dtUs))
	endIdx := int(math.Ceil(endUs / dtUs))

	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > samples.SamplesPerTrace {
		endIdx = samples.SamplesPerTrace
	}
	if startIdx >= samples.SamplesPerTrace {
		return samples, 0, 0, fmt.Errorf("display start is beyond trace length")
	}
	if endIdx <= startIdx {
		return samples, 0, 0, fmt.Errorf("display range is empty after applying sample interval")
	}

	out := segy.EnsembleSamples{
		Traces:            make([][]float32, samples.TraceCount),
		TraceCount:        samples.TraceCount,
		SamplesPerTrace:   endIdx - startIdx,
		TotalImageSamples: int64(samples.TraceCount) * int64(endIdx-startIdx),
	}

	for i := 0; i < samples.TraceCount; i++ {
		trace := samples.Traces[i]
		if len(trace) < endIdx {
			return samples, 0, 0, fmt.Errorf("trace %d shorter than expected sample range", i+1)
		}
		out.Traces[i] = trace[startIdx:endIdx]
	}

	return out, startIdx, endIdx, nil
}

func onTraceImageHovered(xFraction, yFraction float32) {
	if len(displayedEntries) == 0 {
		return
	}

	hasLastHoverPosition = true
	lastHoverXFraction = xFraction
	lastHoverYFraction = yFraction

	idx := traceIndexFromFraction(xFraction, len(displayedEntries))
	sampleIdx := sampleIndexFromFraction(yFraction, displayedSampleCount)
	if idx == lastHoveredTraceIndex && sampleIdx == lastHoveredSampleIndex {
		return
	}
	lastHoveredTraceIndex = idx
	lastHoveredSampleIndex = sampleIdx

	entry := displayedEntries[idx]
	header := entry.Header
	absoluteSampleIndex := displayedStartSampleIndex + sampleIdx
	timeMs := sampleIndexToMilliseconds(absoluteSampleIndex, loadedFile)
	primaryDisplay := selectedSortFieldDisplay(sortField1Select, header)
	secondaryDisplay := selectedSortFieldDisplay(sortField2Select, header)

	status := fmt.Sprintf("t=%0.3f ms  %s", timeMs, primaryDisplay)
	if secondaryDisplay != "" {
		status += fmt.Sprintf("  %s", secondaryDisplay)
	}
	statusLabel.SetText(status)

	if traceInspectCheck != nil && traceInspectCheck.Checked {
		updateTraceHeaderPanel(entry, idx, xFraction)
	}
}

func traceIndexFromFraction(fraction float32, count int) int {
	if count <= 1 {
		return 0
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}

	idx := int(fraction * float32(count))
	if idx >= count {
		idx = count - 1
	}
	return idx
}

func sampleIndexFromFraction(fraction float32, count int) int {
	if count <= 1 {
		return 0
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}

	idx := int(fraction * float32(count))
	if idx >= count {
		idx = count - 1
	}
	return idx
}

func sampleIndexToMilliseconds(sampleIndex int, f *segy.File) float64 {
	if f == nil {
		return 0
	}
	dt := float64(f.BinaryHeader.SampleIntervalMicrosec)
	if dt <= 0 {
		return 0
	}
	return float64(sampleIndex) * dt / 1000.0
}

func selectedSortFieldDisplay(s *widget.Select, h segy.TraceHeader) string {
	if s == nil {
		return "sort1=n/a"
	}
	selected := strings.TrimSpace(s.Selected)
	if selected == "" || selected == "(none)" {
		return ""
	}
	field, ok := labelToField[selected]
	if !ok {
		return ""
	}
	value := traceHeaderFieldValueForDisplay(field, h)
	return fmt.Sprintf("%s=%d", selected, value)
}

func traceHeaderFieldValueForDisplay(field segy.TraceHeaderField, h segy.TraceHeader) int64 {
	switch field {
	case segy.TraceHeaderFieldEP:
		return int64(h.EnergySourcePointNumber)
	case segy.TraceHeaderFieldTRACR:
		return int64(h.TraceSequenceFile)
	case segy.TraceHeaderFieldTRACL:
		return int64(h.TraceSequenceLine)
	case segy.TraceHeaderFieldFLDR:
		return int64(h.FieldRecordNumber)
	case segy.TraceHeaderFieldCDP:
		return int64(h.CDP)
	case segy.TraceHeaderFieldCDPT:
		return int64(h.TraceNumberWithinCDP)
	case segy.TraceHeaderFieldOFFSET:
		return int64(h.SourceReceiverOffset)
	case segy.TraceHeaderFieldSX:
		return int64(h.SourceX)
	case segy.TraceHeaderFieldSY:
		return int64(h.SourceY)
	case segy.TraceHeaderFieldGX:
		return int64(h.GroupX)
	case segy.TraceHeaderFieldGY:
		return int64(h.GroupY)
	default:
		return 0
	}
}

func updateTraceHeaderPanel(entry segy.TraceIndexEntry, index int, fraction float32) {
	if traceHeaderHolder == nil {
		return
	}

	rows := make([]traceHeaderRow, 0, 8+len(traceHeaderRows(entry.Header)))
	rows = append(rows,
		traceHeaderRow{Name: "Image lateral fraction", Value: fmt.Sprintf("%.4f", fraction)},
		traceHeaderRow{Name: "Rendered trace index", Value: fmt.Sprintf("%d of %d", index+1, len(displayedEntries))},
		traceHeaderRow{Name: "TraceNumber", Value: strconv.Itoa(entry.TraceNumber)},
		traceHeaderRow{Name: "HeaderOffset", Value: strconv.FormatInt(entry.HeaderOffset, 10)},
		traceHeaderRow{Name: "DataOffset", Value: strconv.FormatInt(entry.DataOffset, 10)},
		traceHeaderRow{Name: "DataByteLength", Value: strconv.FormatInt(entry.DataByteLength, 10)},
		traceHeaderRow{Name: "SampleCount", Value: strconv.FormatUint(uint64(entry.SampleCount), 10)},
		traceHeaderRow{Name: "SampleFormatCode", Value: strconv.FormatUint(uint64(entry.SampleFormatCode), 10)},
	)
	rows = append(rows, derivedScaledCoordinateRows(entry.Header, entry.Header.Raw[:], byteOrderForDisplay())...)
	rows = append(rows, derivedTrace3DRows(entry.Header.Raw[:], byteOrderForDisplay())...)
	rows = append(rows, traceHeaderRows(entry.Header)...)

	grid := buildCompactTraceHeaderGrid(rows)

	traceHeaderHolder.Objects = []fyne.CanvasObject{grid}
	traceHeaderHolder.Refresh()
}

func buildCompactTraceHeaderGrid(rows []traceHeaderRow) fyne.CanvasObject {
	objects := make([]fyne.CanvasObject, 0, len(rows)*2)
	for _, row := range rows {
		name := canvas.NewText(row.Name, theme.Color(theme.ColorNameForeground))
		name.TextStyle = fyne.TextStyle{Bold: true}
		name.TextSize = 12

		value := canvas.NewText(row.Value, theme.Color(theme.ColorNameForeground))
		value.TextSize = 12

		objects = append(objects, name, value)
	}

	return container.New(layout.NewGridLayoutWithColumns(2), objects...)
}

func updateTraceHeaderPanelVisibility(visible bool) {
	if traceHeaderPanel == nil {
		return
	}

	if visible {
		traceHeaderPanel.Show()
	} else {
		traceHeaderPanel.Hide()
	}
	traceHeaderPanel.Refresh()
}

type traceHeaderRow struct {
	Name  string
	Value string
}

var traceHeaderByteRanges = map[string]string{
	"TraceSequenceLine":       "b1-4",
	"TraceSequenceFile":       "b5-8",
	"FieldRecordNumber":       "b9-12",
	"TraceNumberWithinRecord": "b13-16",
	"EnergySourcePointNumber": "b17-20",
	"CDP":                     "b21-24",
	"TraceNumberWithinCDP":    "b25-28",
	"SourceReceiverOffset":    "b37-40",
	"CoordinateScalar":        "b71-72",
	"SourceX":                 "b73-76",
	"SourceY":                 "b77-80",
	"GroupX":                  "b81-84",
	"GroupY":                  "b85-88",
	"CoordinateUnits":         "b89-90",
	"SamplesInTrace":          "b115-116",
	"SampleIntervalMicrosec":  "b117-118",
	"DelayRecordingTime":      "b109-110",
	"YearDataRecorded":        "b157-158",
	"DayOfYear":               "b159-160",
	"HourOfDay":               "b161-162",
	"MinuteOfHour":            "b163-164",
	"SecondOfMinute":          "b165-166",
}

func traceHeaderRows(h segy.TraceHeader) []traceHeaderRow {
	rows := make([]traceHeaderRow, 0)
	v := reflect.ValueOf(h)
	t := reflect.TypeOf(h)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Name == "Raw" {
			continue
		}
		rows = append(rows, traceHeaderRow{
			Name:  annotatedTraceHeaderName(field.Name),
			Value: fmt.Sprintf("%v", v.Field(i).Interface()),
		})
	}

	return rows
}

func derivedTrace3DRows(raw []byte, order binary.ByteOrder) []traceHeaderRow {
	if len(raw) < segy.TraceHeaderByteLength {
		return nil
	}
	if order == nil {
		order = binary.BigEndian
	}

	cdpX := int32(order.Uint32(raw[180:184]))
	cdpY := int32(order.Uint32(raw[184:188]))
	inline := int32(order.Uint32(raw[188:192]))
	crossline := int32(order.Uint32(raw[192:196]))

	return []traceHeaderRow{
		{Name: "CDP_X (b181-184)", Value: strconv.FormatInt(int64(cdpX), 10)},
		{Name: "CDP_Y (b185-188)", Value: strconv.FormatInt(int64(cdpY), 10)},
		{Name: "Inline_3D (b189-192)", Value: strconv.FormatInt(int64(inline), 10)},
		{Name: "Crossline_3D (b193-196)", Value: strconv.FormatInt(int64(crossline), 10)},
	}
}

func derivedScaledCoordinateRows(h segy.TraceHeader, raw []byte, order binary.ByteOrder) []traceHeaderRow {
	if len(raw) < segy.TraceHeaderByteLength {
		return nil
	}
	if order == nil {
		order = binary.BigEndian
	}

	scaleFactor := coordinateScaleFactor(h.CoordinateScalar)
	unitsLabel := coordinateUnitsLabel(h.CoordinateUnits)

	cdpXRaw := int32(order.Uint32(raw[180:184]))
	cdpYRaw := int32(order.Uint32(raw[184:188]))

	return []traceHeaderRow{
		{Name: "Coordinate scalar applied", Value: fmt.Sprintf("%d -> factor %s", h.CoordinateScalar, strconv.FormatFloat(scaleFactor, 'f', -1, 64))},
		{Name: "Coordinate units", Value: unitsLabel},
		{Name: "SourceX scaled", Value: strconv.FormatFloat(applyCoordinateScale(h.SourceX, scaleFactor), 'f', -1, 64)},
		{Name: "SourceY scaled", Value: strconv.FormatFloat(applyCoordinateScale(h.SourceY, scaleFactor), 'f', -1, 64)},
		{Name: "GroupX scaled", Value: strconv.FormatFloat(applyCoordinateScale(h.GroupX, scaleFactor), 'f', -1, 64)},
		{Name: "GroupY scaled", Value: strconv.FormatFloat(applyCoordinateScale(h.GroupY, scaleFactor), 'f', -1, 64)},
		{Name: "CDP_X scaled", Value: strconv.FormatFloat(applyCoordinateScale(cdpXRaw, scaleFactor), 'f', -1, 64)},
		{Name: "CDP_Y scaled", Value: strconv.FormatFloat(applyCoordinateScale(cdpYRaw, scaleFactor), 'f', -1, 64)},
	}
}

func coordinateScaleFactor(scalar int16) float64 {
	if scalar > 0 {
		return float64(scalar)
	}
	if scalar < 0 {
		return 1.0 / float64(-scalar)
	}
	return 1.0
}

func applyCoordinateScale(raw int32, scaleFactor float64) float64 {
	return float64(raw) * scaleFactor
}

func coordinateUnitsLabel(units int16) string {
	switch units {
	case 1:
		return "1 (length: meters or feet per measurement system)"
	case 2:
		return "2 (seconds of arc)"
	case 3:
		return "3 (decimal degrees)"
	case 4:
		return "4 (DMS)"
	default:
		return fmt.Sprintf("%d (unknown)", units)
	}
}

func byteOrderForDisplay() binary.ByteOrder {
	if loadedFile == nil || loadedFile.ByteOrder == nil {
		return binary.BigEndian
	}
	return loadedFile.ByteOrder
}

func annotatedTraceHeaderName(name string) string {
	byteRange, ok := traceHeaderByteRanges[name]
	if !ok {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, byteRange)
}

func applyFieldRangeDefaults(label string, minEntry, maxEntry *widget.Entry) {
	field, ok := labelToField[label]
	if !ok {
		return
	}
	rangeInfo, ok := loadedSummary.Ranges[field]
	if !ok {
		return
	}
	minEntry.SetText(strconv.FormatInt(rangeInfo.Min, 10))
	maxEntry.SetText(strconv.FormatInt(rangeInfo.Max, 10))
}

func applyFieldRangeDefaultsIfEmpty(label string, minEntry, maxEntry *widget.Entry) {
	if strings.TrimSpace(minEntry.Text) != "" && strings.TrimSpace(maxEntry.Text) != "" {
		return
	}
	applyFieldRangeDefaults(label, minEntry, maxEntry)
}

func buildPredicates() ([]segy.TraceQueryPredicate, error) {
	predicates := make([]segy.TraceQueryPredicate, 0, 2)

	primary, err := buildPredicate(primaryFieldSelect, primaryModeSelect, primaryMinEntry, primaryMaxEntry)
	if err != nil {
		return nil, fmt.Errorf("primary filter: %w", err)
	}
	predicates = append(predicates, primary)

	if secondaryEnableCheck.Checked {
		secondary, err := buildPredicate(secondaryFieldSelect, secondaryModeSelect, secondaryMinEntry, secondaryMaxEntry)
		if err != nil {
			return nil, fmt.Errorf("secondary filter: %w", err)
		}
		predicates = append(predicates, secondary)
	}

	return predicates, nil
}

func buildPredicate(fieldSelect, modeSelect *widget.Select, minEntry, maxEntry *widget.Entry) (segy.TraceQueryPredicate, error) {
	field, ok := labelToField[fieldSelect.Selected]
	if !ok {
		return segy.TraceQueryPredicate{}, fmt.Errorf("invalid field selection")
	}

	minValue, err := parseInt64Entry("min/value", minEntry)
	if err != nil {
		return segy.TraceQueryPredicate{}, err
	}

	if modeSelect.Selected == "Exact" {
		return segy.EqualPredicate(field, minValue), nil
	}

	maxValue, err := parseInt64Entry("max", maxEntry)
	if err != nil {
		return segy.TraceQueryPredicate{}, err
	}
	if maxValue < minValue {
		return segy.TraceQueryPredicate{}, fmt.Errorf("max must be >= min")
	}

	return segy.RangePredicate(field, minValue, maxValue), nil
}

func buildSortKeys() ([]segy.TraceSortKey, error) {
	keys := make([]segy.TraceSortKey, 0, 2)

	appendKey := func(fieldLabel, orderLabel string) error {
		if fieldLabel == "(none)" {
			return nil
		}
		field, ok := labelToField[fieldLabel]
		if !ok {
			return fmt.Errorf("invalid sort field %q", fieldLabel)
		}
		if orderLabel == "Desc" {
			keys = append(keys, segy.Desc(field))
			return nil
		}
		keys = append(keys, segy.Asc(field))
		return nil
	}

	if err := appendKey(sortField1Select.Selected, sortOrder1Select.Selected); err != nil {
		return nil, err
	}
	if err := appendKey(sortField2Select.Selected, sortOrder2Select.Selected); err != nil {
		return nil, err
	}

	return keys, nil
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

func parseInt64Entry(name string, e *widget.Entry) (int64, error) {
	v := strings.TrimSpace(e.Text)
	if v == "" {
		return 0, fmt.Errorf("%s is empty", name)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %q", name, v)
	}
	return n, nil
}

func parseFloatEntry(name string, e *widget.Entry) (float64, error) {
	v := strings.TrimSpace(e.Text)
	if v == "" {
		return 0, fmt.Errorf("%s is empty", name)
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %q", name, v)
	}
	return n, nil
}

func showTextHeadersPopup() {
	if loadedFile == nil {
		dialog.ShowInformation("No SEG-Y loaded", "Load a SEG-Y file first.", winMain)
		return
	}

	var b strings.Builder
	headers := make([]segy.TextualHeader, 0, 1+len(loadedFile.ExtendedTextualHeader))
	headers = append(headers, loadedFile.TextualHeader)
	headers = append(headers, loadedFile.ExtendedTextualHeader...)

	for i, h := range headers {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if i == 0 {
			fmt.Fprintf(&b, "Main textual header (encoding: %s)\n", textualEncodingName(h.Encoding))
		} else {
			fmt.Fprintf(&b, "Extended textual header %d (encoding: %s)\n", i, textualEncodingName(h.Encoding))
		}
		b.WriteString(formatTextualHeaderLines(h))
	}

	showReadOnlyTextPopup("Character Header/s", b.String(), fyne.NewSize(980, 760))
}

func showReelHeaderPopup() {
	if loadedFile == nil {
		dialog.ShowInformation("No SEG-Y loaded", "Load a SEG-Y file first.", winMain)
		return
	}

	bh := loadedFile.BinaryHeader
	var b strings.Builder
	fmt.Fprintf(&b, "SEG-Y binary reel header\n")
	fmt.Fprintf(&b, "Byte order: %T\n\n", loadedFile.ByteOrder)

	fmt.Fprintf(&b, "JobID: %d\n", bh.JobID)
	fmt.Fprintf(&b, "LineNumber: %d\n", bh.LineNumber)
	fmt.Fprintf(&b, "ReelNumber: %d\n", bh.ReelNumber)
	fmt.Fprintf(&b, "DataTracesPerEnsemble: %d\n", bh.DataTracesPerEnsemble)
	fmt.Fprintf(&b, "AuxTracesPerEnsemble: %d\n", bh.AuxTracesPerEnsemble)
	fmt.Fprintf(&b, "SampleIntervalMicrosec: %d\n", bh.SampleIntervalMicrosec)
	fmt.Fprintf(&b, "SamplesPerTrace: %d\n", bh.SamplesPerTrace)
	fmt.Fprintf(&b, "DataSampleFormatCode: %d\n", bh.DataSampleFormatCode)
	fmt.Fprintf(&b, "EnsembleFold: %d\n", bh.EnsembleFold)
	fmt.Fprintf(&b, "TraceSortingCode: %d\n", bh.TraceSortingCode)
	fmt.Fprintf(&b, "SEGYRevision: %d\n", bh.SEGYRevision)
	fmt.Fprintf(&b, "FixedLengthTraceFlag: %d\n", bh.FixedLengthTraceFlag)
	fmt.Fprintf(&b, "ExtendedTextHeadersCount: %d\n", bh.ExtendedTextHeadersCount)

	b.WriteString("\nRaw 400-byte binary header (hex dump):\n")
	b.WriteString(hex.Dump(bh.Raw[:]))

	showReadOnlyTextPopup("Reel Header", b.String(), fyne.NewSize(980, 760))
}

func showReadOnlyTextPopup(title, content string, size fyne.Size) {
	text := widget.NewTextGridFromString(content)
	scroll := container.NewScroll(text)
	scroll.SetMinSize(size)
	dlg := dialog.NewCustom(title, "Close", scroll, winMain)
	dlg.Resize(size)
	dlg.Show()
}

func textualEncodingName(enc segy.TextualHeaderEncoding) string {
	switch enc {
	case segy.TextualHeaderEncodingUTF8:
		return "UTF-8"
	case segy.TextualHeaderEncodingEBCDIC:
		return "EBCDIC (CP037 decoded)"
	default:
		return "Unknown"
	}
}

func formatTextualHeaderLines(h segy.TextualHeader) string {
	const lineWidth = 80
	const lineCount = segy.TextualHeaderByteLength / lineWidth
	var b strings.Builder

	for i := 0; i < lineCount; i++ {
		start := i * lineWidth
		end := start + lineWidth
		chunk := h.Data[start:end]
		fmt.Fprintf(&b, "C%02d %s\n", i+1, sanitizeHeaderChunk(chunk, h.Encoding))
	}

	return strings.TrimSuffix(b.String(), "\n")
}

func sanitizeHeaderChunk(chunk []byte, enc segy.TextualHeaderEncoding) string {
	decoded := chunk
	if enc == segy.TextualHeaderEncodingEBCDIC {
		if converted, err := charmap.CodePage037.NewDecoder().Bytes(chunk); err == nil {
			decoded = converted
		}
	}

	out := make([]byte, len(decoded))
	for i, c := range decoded {
		if c == 0 {
			out[i] = ' '
			continue
		}
		if c >= 32 && c <= 126 {
			out[i] = c
			continue
		}
		out[i] = '.'
	}
	return string(out)
}

func prefs() fyne.Preferences {
	if fyne.CurrentApp() == nil {
		return nil
	}
	return fyne.CurrentApp().Preferences()
}

func applyPersistedPopupSettings() {
	p := prefs()
	if p == nil {
		return
	}

	setSelectFromPref(primaryFieldSelect, p.StringWithFallback(prefPrimaryField, "EP"))
	setSelectFromPref(primaryModeSelect, p.StringWithFallback(prefPrimaryMode, "Range"))
	primaryMinEntry.SetText(p.StringWithFallback(prefPrimaryMin, ""))
	primaryMaxEntry.SetText(p.StringWithFallback(prefPrimaryMax, ""))

	secondaryEnableCheck.SetChecked(p.BoolWithFallback(prefSecondaryEnabled, false))
	setSelectFromPref(secondaryFieldSelect, p.StringWithFallback(prefSecondaryField, "OFFSET"))
	setSelectFromPref(secondaryModeSelect, p.StringWithFallback(prefSecondaryMode, "Range"))
	secondaryMinEntry.SetText(p.StringWithFallback(prefSecondaryMin, ""))
	secondaryMaxEntry.SetText(p.StringWithFallback(prefSecondaryMax, ""))

	setSelectFromPref(sortField1Select, p.StringWithFallback(prefSortField1, "EP"))
	setSelectFromPref(sortOrder1Select, p.StringWithFallback(prefSortOrder1, "Asc"))
	setSelectFromPref(sortField2Select, p.StringWithFallback(prefSortField2, "OFFSET"))
	setSelectFromPref(sortOrder2Select, p.StringWithFallback(prefSortOrder2, "Asc"))

	setSelectFromPref(paletteSelect, p.StringWithFallback(prefPalette, "Blue-White-Red"))
	clipEntry.SetText(p.StringWithFallback(prefClip, "2.0"))
	negClipEntry.SetText(p.StringWithFallback(prefNegClip, "2.0"))
	posClipEntry.SetText(p.StringWithFallback(prefPosClip, "2.0"))
	timeStartEntry.SetText(p.StringWithFallback(prefTimeStartMs, ""))
	timeEndEntry.SetText(p.StringWithFallback(prefTimeEndMs, ""))
	maxSamplesEntry.SetText(p.StringWithFallback(prefMaxSamples, strconv.FormatInt(segy.DefaultMaxImageSamples, 10)))
	zeroCenteredCheck.SetChecked(p.BoolWithFallback(prefZeroCentered, true))

	setSelectFromPref(gainModeSelect, p.StringWithFallback(prefGainMode, "None"))
	tSquaredScaleEntry.SetText(p.StringWithFallback(prefTSquaredScale, "1.0"))
	agcWindowEntry.SetText(p.StringWithFallback(prefAGCWindow, "101"))
	agcFloorEntry.SetText(p.StringWithFallback(prefAGCFloor, "1e-6"))
}

func attachPopupSettingsPersistence() {
	p := prefs()
	if p == nil {
		return
	}

	primaryFieldSelect.OnChanged = func(label string) {
		if loadedCatalog != nil {
			applyFieldRangeDefaults(label, primaryMinEntry, primaryMaxEntry)
		}
		setSelectFromValue(sortField1Select, label)
		p.SetString(prefPrimaryField, label)
	}
	primaryModeSelect.OnChanged = func(v string) { p.SetString(prefPrimaryMode, v) }
	primaryMinEntry.OnChanged = func(v string) { p.SetString(prefPrimaryMin, v) }
	primaryMaxEntry.OnChanged = func(v string) { p.SetString(prefPrimaryMax, v) }

	secondaryEnableCheck.OnChanged = func(v bool) { p.SetBool(prefSecondaryEnabled, v) }
	secondaryFieldSelect.OnChanged = func(label string) {
		if loadedCatalog != nil {
			applyFieldRangeDefaults(label, secondaryMinEntry, secondaryMaxEntry)
		}
		setSelectFromValue(sortField2Select, label)
		p.SetString(prefSecondaryField, label)
	}
	secondaryModeSelect.OnChanged = func(v string) { p.SetString(prefSecondaryMode, v) }
	secondaryMinEntry.OnChanged = func(v string) { p.SetString(prefSecondaryMin, v) }
	secondaryMaxEntry.OnChanged = func(v string) { p.SetString(prefSecondaryMax, v) }

	sortField1Select.OnChanged = func(v string) { p.SetString(prefSortField1, v) }
	sortOrder1Select.OnChanged = func(v string) { p.SetString(prefSortOrder1, v) }
	sortField2Select.OnChanged = func(v string) { p.SetString(prefSortField2, v) }
	sortOrder2Select.OnChanged = func(v string) { p.SetString(prefSortOrder2, v) }

	paletteSelect.OnChanged = func(v string) { p.SetString(prefPalette, v) }
	clipEntry.OnChanged = func(v string) { p.SetString(prefClip, v) }
	negClipEntry.OnChanged = func(v string) { p.SetString(prefNegClip, v) }
	posClipEntry.OnChanged = func(v string) { p.SetString(prefPosClip, v) }
	timeStartEntry.OnChanged = func(v string) { p.SetString(prefTimeStartMs, v) }
	timeEndEntry.OnChanged = func(v string) { p.SetString(prefTimeEndMs, v) }
	maxSamplesEntry.OnChanged = func(v string) { p.SetString(prefMaxSamples, v) }
	zeroCenteredCheck.OnChanged = func(v bool) { p.SetBool(prefZeroCentered, v) }

	gainModeSelect.OnChanged = func(v string) {
		updateGainControlsState()
		p.SetString(prefGainMode, v)
	}
	tSquaredScaleEntry.OnChanged = func(v string) { p.SetString(prefTSquaredScale, v) }
	agcWindowEntry.OnChanged = func(v string) { p.SetString(prefAGCWindow, v) }
	agcFloorEntry.OnChanged = func(v string) { p.SetString(prefAGCFloor, v) }
}

func attachEnterToRenderHandlers() {
	entries := []*widget.Entry{
		primaryMinEntry,
		primaryMaxEntry,
		secondaryMinEntry,
		secondaryMaxEntry,
		clipEntry,
		negClipEntry,
		posClipEntry,
		timeStartEntry,
		timeEndEntry,
		maxSamplesEntry,
		tSquaredScaleEntry,
		agcWindowEntry,
		agcFloorEntry,
	}

	onSubmit := func(_ string) {
		if loadedFile == nil || loadedCatalog == nil {
			return
		}
		renderSelection()
	}

	for _, e := range entries {
		if e == nil {
			continue
		}
		e.OnSubmitted = onSubmit
	}
}

func setSelectFromPref(s *widget.Select, value string) {
	setSelectFromValue(s, value)
}

func setSelectFromValue(s *widget.Select, value string) {
	if s == nil {
		return
	}
	for _, opt := range s.Options {
		if opt == value {
			s.SetSelected(value)
			return
		}
	}
}

func updateGainControlsState() {
	if gainModeSelect == nil || tSquaredScaleEntry == nil || agcWindowEntry == nil || agcFloorEntry == nil || gainTSquaredCard == nil || gainAGCCard == nil {
		return
	}

	switch gainModeSelect.Selected {
	case "T-Squared":
		tSquaredScaleEntry.Enable()
		agcWindowEntry.Disable()
		agcFloorEntry.Disable()
		gainTSquaredCard.Show()
		gainAGCCard.Hide()
	case "AGC":
		tSquaredScaleEntry.Disable()
		agcWindowEntry.Enable()
		agcFloorEntry.Enable()
		gainTSquaredCard.Hide()
		gainAGCCard.Show()
	default:
		tSquaredScaleEntry.Disable()
		agcWindowEntry.Disable()
		agcFloorEntry.Disable()
		gainTSquaredCard.Hide()
		gainAGCCard.Hide()
	}
}

func buildGainOptions() (segy.GainOptions, error) {
	switch gainModeSelect.Selected {
	case "T-Squared":
		scale, err := parseFloatEntry("t-squared scale", tSquaredScaleEntry)
		if err != nil {
			return segy.GainOptions{}, err
		}
		return segy.GainOptions{
			Mode:              segy.GainModeTSquared,
			TSquaredScale:     scale,
			SampleIntervalSec: float64(loadedFile.BinaryHeader.SampleIntervalMicrosec) * 1e-6,
		}, nil
	case "AGC":
		window, err := parseInt64Entry("AGC window samples", agcWindowEntry)
		if err != nil {
			return segy.GainOptions{}, err
		}
		if window <= 0 {
			return segy.GainOptions{}, fmt.Errorf("AGC window samples must be > 0")
		}
		floor, err := parseFloatEntry("AGC floor", agcFloorEntry)
		if err != nil {
			return segy.GainOptions{}, err
		}
		if floor <= 0 {
			return segy.GainOptions{}, fmt.Errorf("AGC floor must be > 0")
		}
		return segy.GainOptions{
			Mode:             segy.GainModeAGC,
			AGCWindowSamples: int(window),
			AGCFloor:         floor,
		}, nil
	default:
		return segy.GainOptions{Mode: segy.GainModeNone}, nil
	}
}

type traceImageWidget struct {
	widget.BaseWidget
	img     *canvas.Image
	onHover func(xFraction, yFraction float32)
}

func newTraceImageWidget(src image.Image, onHover func(xFraction, yFraction float32)) *traceImageWidget {
	img := canvas.NewImageFromImage(src)
	img.FillMode = canvas.ImageFillStretch
	img.ScaleMode = canvas.ImageScaleFastest

	w := &traceImageWidget{img: img, onHover: onHover}
	w.ExtendBaseWidget(w)
	return w
}

func (w *traceImageWidget) MouseIn(_ *desktop.MouseEvent) {}

func (w *traceImageWidget) MouseMoved(ev *desktop.MouseEvent) {
	if w == nil || w.onHover == nil {
		return
	}

	size := w.Size()
	if size.Width <= 0 || size.Height <= 0 {
		return
	}

	// desktop.MouseEvent positions are already widget-local.
	localX := ev.Position.X
	localY := ev.Position.Y
	xFraction := localX / size.Width
	yFraction := localY / size.Height
	if xFraction < 0 {
		xFraction = 0
	}
	if xFraction > 1 {
		xFraction = 1
	}
	if yFraction < 0 {
		yFraction = 0
	}
	if yFraction > 1 {
		yFraction = 1
	}
	w.onHover(xFraction, yFraction)
}

func (w *traceImageWidget) MouseOut() {}

func (w *traceImageWidget) Tapped(_ *fyne.PointEvent) {}

func (w *traceImageWidget) TappedSecondary(_ *fyne.PointEvent) {}

func (w *traceImageWidget) CreateRenderer() fyne.WidgetRenderer {
	return &traceImageRenderer{widget: w, objects: []fyne.CanvasObject{w.img}}
}

type traceImageRenderer struct {
	widget  *traceImageWidget
	objects []fyne.CanvasObject
}

func (r *traceImageRenderer) Layout(size fyne.Size) {
	r.widget.img.Move(fyne.NewPos(0, 0))
	r.widget.img.Resize(size)
}

func (r *traceImageRenderer) MinSize() fyne.Size {
	return fyne.NewSize(64, 64)
}

func (r *traceImageRenderer) Refresh() {
	r.widget.img.Refresh()
}

func (r *traceImageRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *traceImageRenderer) Destroy() {}

func (r *traceImageRenderer) BackgroundColor() color.Color {
	return color.Transparent
}
