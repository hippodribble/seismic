package main

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/hippodribble/seismic/segy"
)

var controlsWindow fyne.Window
var controlsWindowVisible bool
var paletteSelect *widget.Select
var highCutEntry *widget.Entry
var gainModeSelect *widget.Select
var tSquaredScaleEntry *widget.Entry
var agcWindowEntry *widget.Entry
var agcFloorEntry *widget.Entry
var smoothingEntry *widget.Entry
var semblanceMinVelEntry *widget.Entry
var semblanceMaxVelEntry *widget.Entry
var semblanceVelStepEntry *widget.Entry
var semblancePowerEntry *widget.Entry
var semblanceClipEntry *widget.Entry
var semblanceStretchMuteEntry *widget.Entry
var stackOverlayEnabledCheck *widget.Check
var stackOverlayOpacitySlider *widget.Slider
var stackOverlayOpacityLabel *widget.Label

var gainTSquaredCard *widget.Card
var gainAGCCard *widget.Card

var suppressControlEvents bool
var suppressControlReprocess bool
var lastNyquistHz float64
var highCutAuto bool = true

const (
	defaultSemblancePalette    = "Blue-Red-White"
	defaultGainMode            = "None"
	defaultTSquaredScale       = "1.0"
	defaultAGCWindow           = "101"
	defaultAGCFloor            = "1e-6"
	defaultSmoothingWindow     = "1"
	defaultSemblanceVMin       = "1200"
	defaultSemblanceVMax       = "4500"
	defaultSemblanceDV         = "50"
	defaultSemblancePower      = "1.0"
	defaultSemblanceClip       = "2.5"
	defaultStretchMute         = "0"
	defaultCDPStep             = 10
	defaultStackOverlayOpacity = "0"
	prefStackOverlayEnabled    = "velocityanalysis.ctrl.stack_overlay_enabled"
	prefStackOverlayOpacity    = "velocityanalysis.ctrl.stack_overlay_opacity"
)

func initControlsPopup() {
	if winMain == nil || controlsWindow != nil {
		return
	}

	suppressControlReprocess = true
	defer func() { suppressControlReprocess = false }()

	paletteSelect = widget.NewSelect([]string{
		"Blue-Red-White",
		"Blue-White-Red",
		"Blue-White-Red (Inverted)",
		"Grayscale",
		"Grayscale (Inverted)",
	}, func(_ string) { reprocessCurrentCDPAsync() })
	paletteSelect.SetSelected(defaultSemblancePalette)

	highCutEntry = widget.NewEntry()
	highCutEntry.SetPlaceHolder("Nyquist")
	highCutEntry.OnChanged = func(_ string) {
		if suppressControlEvents {
			return
		}
		highCutAuto = false
	}
	highCutEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	gainModeSelect = widget.NewSelect([]string{"None", "T-Squared", "AGC"}, func(_ string) {
		updateGainControlsState()
		reprocessCurrentCDPAsync()
	})
	gainModeSelect.SetSelected(defaultGainMode)

	tSquaredScaleEntry = widget.NewEntry()
	tSquaredScaleEntry.SetText(defaultTSquaredScale)
	tSquaredScaleEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	agcWindowEntry = widget.NewEntry()
	agcWindowEntry.SetText(defaultAGCWindow)
	agcWindowEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	agcFloorEntry = widget.NewEntry()
	agcFloorEntry.SetText(defaultAGCFloor)
	agcFloorEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	smoothingEntry = widget.NewEntry()
	smoothingEntry.SetText(defaultSmoothingWindow)
	smoothingEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	semblanceMinVelEntry = widget.NewEntry()
	semblanceMinVelEntry.SetText(defaultSemblanceVMin)
	semblanceMinVelEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	semblanceMaxVelEntry = widget.NewEntry()
	semblanceMaxVelEntry.SetText(defaultSemblanceVMax)
	semblanceMaxVelEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	semblanceVelStepEntry = widget.NewEntry()
	semblanceVelStepEntry.SetText(defaultSemblanceDV)
	semblanceVelStepEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	semblancePowerEntry = widget.NewEntry()
	semblancePowerEntry.SetText(defaultSemblancePower)
	semblancePowerEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	semblanceClipEntry = widget.NewEntry()
	semblanceClipEntry.SetText(defaultSemblanceClip)
	semblanceClipEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	semblanceStretchMuteEntry = widget.NewEntry()
	semblanceStretchMuteEntry.SetText(defaultStretchMute)
	semblanceStretchMuteEntry.OnSubmitted = func(_ string) { reprocessCurrentCDPAsync() }

	stackOverlayEnabledCheck = widget.NewCheck("Enable Stack Velocity Overlay", func(enabled bool) {
		if suppressControlEvents {
			return
		}
		refreshStackPickMarkers()
	})

	stackOverlayOpacityLabel = widget.NewLabel("0%")
	stackOverlayOpacitySlider = widget.NewSlider(0, 60)
	stackOverlayOpacitySlider.Step = 1
	stackOverlayOpacitySlider.OnChanged = func(value float64) {
		stackOverlayOpacityLabel.SetText(fmt.Sprintf("%.0f%%", value))
		if suppressControlEvents {
			return
		}
		refreshStackPickMarkers()
	}

	gainTSquaredCard = widget.NewCard("T-Squared", "", widget.NewForm(
		widget.NewFormItem("Scale", tSquaredScaleEntry),
	))
	gainAGCCard = widget.NewCard("AGC", "", widget.NewForm(
		widget.NewFormItem("Window", agcWindowEntry),
		widget.NewFormItem("Floor", agcFloorEntry),
	))

	updateGainControlsState()

	controlsForm := widget.NewForm(
		widget.NewFormItem("Semblance Palette", paletteSelect),
		widget.NewFormItem("High-Cut (Hz)", highCutEntry),
		widget.NewFormItem("Gain", gainModeSelect),
		widget.NewFormItem("Smoothing", smoothingEntry),
		widget.NewFormItem("Semblance Vmin", semblanceMinVelEntry),
		widget.NewFormItem("Semblance Vmax", semblanceMaxVelEntry),
		widget.NewFormItem("Semblance dV", semblanceVelStepEntry),
		widget.NewFormItem("Semblance Power", semblancePowerEntry),
		widget.NewFormItem("Semblance Clip (%)", semblanceClipEntry),
		widget.NewFormItem("Stretch Mute", semblanceStretchMuteEntry),
		widget.NewFormItem("Stack Velocity Overlay", container.NewVBox(stackOverlayEnabledCheck, container.NewHBox(stackOverlayOpacitySlider, stackOverlayOpacityLabel))),
	)

	applyButton := widget.NewButton("Apply", reprocessCurrentCDPAsync)
	resetButton := widget.NewButton("Reset Preferences", func() {
		resetPreferencesToDefaults()
		reprocessCurrentCDPAsync()
	})
	buttons := container.NewHBox(applyButton, resetButton)
	content := container.NewVBox(
		widget.NewLabel("Velocity Analysis Controls"),
		controlsForm,
		gainTSquaredCard,
		gainAGCCard,
		buttons,
	)

	controlsWindow = fyne.CurrentApp().NewWindow("Velocity Analysis Controls")
	controlsWindow.SetContent(container.NewScroll(content))
	controlsWindow.Resize(fyne.NewSize(420, 700))
	controlsWindow.SetOnClosed(func() {
		persistControlPreferences()
		controlsWindowVisible = false
		controlsWindow = nil
	})
	restoreControlPreferences()
	updateGainControlsState()
	controlsWindowVisible = false
}

func toggleControlsPopup() {
	if controlsWindow == nil {
		initControlsPopup()
	}
	if controlsWindow == nil {
		return
	}
	if controlsWindowVisible {
		controlsWindowVisible = false
		controlsWindow.Hide()
		return
	}
	controlsWindowVisible = true
	controlsWindow.Show()
	controlsWindow.RequestFocus()
}

func updateGainControlsState() {
	if gainModeSelect == nil || tSquaredScaleEntry == nil || agcWindowEntry == nil || agcFloorEntry == nil || gainTSquaredCard == nil || gainAGCCard == nil {
		return
	}
	mode := strings.TrimSpace(gainModeSelect.Selected)
	switch mode {
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

func selectedSemblancePalette() color.Palette {
	if paletteSelect == nil {
		return segy.BlueRedWhitePalette256()
	}
	switch paletteSelect.Selected {
	case "Blue-Red-White":
		return segy.BlueRedWhitePalette256()
	case "Blue-White-Red":
		return blueWhiteRedPalette(false)
	case "Blue-White-Red (Inverted)":
		return blueWhiteRedPalette(true)
	case "Grayscale":
		return grayscalePalette(false)
	case "Grayscale (Inverted)":
		return grayscalePalette(true)
	default:
		return segy.BlueRedWhitePalette256()
	}
}

func selectedGainOptions(sampleIntervalMS float64) (segy.GainOptions, error) {
	if gainModeSelect == nil {
		return segy.GainOptions{Mode: segy.GainModeNone}, nil
	}
	switch gainModeSelect.Selected {
	case "T-Squared":
		scale, err := parsePositiveFloat(tSquaredScaleEntry, "t-squared scale")
		if err != nil {
			return segy.GainOptions{}, err
		}
		return segy.GainOptions{Mode: segy.GainModeTSquared, TSquaredScale: scale, SampleIntervalSec: sampleIntervalMS / 1000.0}, nil
	case "AGC":
		window, err := parsePositiveInt(agcWindowEntry, "AGC window")
		if err != nil {
			return segy.GainOptions{}, err
		}
		floor, err := parsePositiveFloat(agcFloorEntry, "AGC floor")
		if err != nil {
			return segy.GainOptions{}, err
		}
		return segy.GainOptions{Mode: segy.GainModeAGC, AGCWindowSamples: window, AGCFloor: floor}, nil
	default:
		return segy.GainOptions{Mode: segy.GainModeNone}, nil
	}
}

func selectedSemblanceVelocityParams() (float64, float64, float64, error) {
	vmin, err := parsePositiveFloat(semblanceMinVelEntry, "semblance minimum velocity")
	if err != nil {
		return 0, 0, 0, err
	}
	vmax, err := parsePositiveFloat(semblanceMaxVelEntry, "semblance maximum velocity")
	if err != nil {
		return 0, 0, 0, err
	}
	step, err := parsePositiveFloat(semblanceVelStepEntry, "semblance velocity step")
	if err != nil {
		return 0, 0, 0, err
	}
	if vmax < vmin {
		return 0, 0, 0, fmt.Errorf("semblance maximum velocity must be >= minimum velocity")
	}
	return vmin, vmax, step, nil
}

func selectedSmoothingWindow() (int, error) {
	if smoothingEntry == nil {
		return 1, nil
	}
	v, err := parsePositiveInt(smoothingEntry, "smoothing")
	if err != nil {
		return 0, err
	}
	return v, nil
}

func selectedSemblancePower() (float64, error) {
	if semblancePowerEntry == nil {
		return 1.0, nil
	}
	v, err := parsePositiveFloat(semblancePowerEntry, "semblance power")
	if err != nil {
		return 0, err
	}
	return v, nil
}

func selectedSemblancePercentileClip() (float64, error) {
	if semblanceClipEntry == nil {
		return 0, nil
	}
	text := strings.TrimSpace(semblanceClipEntry.Text)
	if text == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("semblance clip must be a number")
	}
	if v < 0 {
		return 0, fmt.Errorf("semblance clip must be >= 0")
	}
	if v > 50 {
		return 0, fmt.Errorf("semblance clip must be <= 50")
	}
	return v, nil
}

func selectedSemblanceStretchMuteFraction() (float64, error) {
	if semblanceStretchMuteEntry == nil {
		return 0, nil
	}
	text := strings.TrimSpace(semblanceStretchMuteEntry.Text)
	if text == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("stretch mute must be a number")
	}
	if v < 0 {
		return 0, fmt.Errorf("stretch mute must be >= 0")
	}
	return v, nil
}

func selectedHighCutHz(sampleIntervalMS float64) (float64, error) {
	if sampleIntervalMS <= 0 {
		return 0, fmt.Errorf("invalid sample interval")
	}
	nyquist := 500.0 / sampleIntervalMS
	lastNyquistHz = nyquist
	if highCutEntry == nil {
		return nyquist, nil
	}

	text := strings.TrimSpace(highCutEntry.Text)
	if text == "" {
		return nyquist, nil
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("high-cut must be numeric")
	}
	if v <= 0 {
		return 0, fmt.Errorf("high-cut must be > 0")
	}
	if v > nyquist {
		return 0, fmt.Errorf("high-cut %.3f Hz exceeds Nyquist %.3f Hz", v, nyquist)
	}
	if math.Abs(v-nyquist) < 1e-9 {
		return nyquist, nil
	}
	return v, nil
}

func restoreControlPreferences() {
	suppressControlReprocess = true
	defer func() { suppressControlReprocess = false }()

	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	prefs := app.Preferences()

	if paletteSelect != nil {
		paletteSelect.SetSelected(prefs.StringWithFallback(prefSemblancePalette, paletteSelect.Selected))
	}
	if gainModeSelect != nil {
		gainModeSelect.SetSelected(prefs.StringWithFallback(prefGainMode, gainModeSelect.Selected))
	}

	if tSquaredScaleEntry != nil {
		tSquaredScaleEntry.SetText(prefs.StringWithFallback(prefTSquaredScale, tSquaredScaleEntry.Text))
	}
	if agcWindowEntry != nil {
		agcWindowEntry.SetText(prefs.StringWithFallback(prefAGCWindow, agcWindowEntry.Text))
	}
	if agcFloorEntry != nil {
		agcFloorEntry.SetText(prefs.StringWithFallback(prefAGCFloor, agcFloorEntry.Text))
	}
	if smoothingEntry != nil {
		smoothingEntry.SetText(prefs.StringWithFallback(prefSmoothingWindow, smoothingEntry.Text))
	}
	if semblanceMinVelEntry != nil {
		semblanceMinVelEntry.SetText(prefs.StringWithFallback(prefSemblanceMinVelocity, semblanceMinVelEntry.Text))
	}
	if semblanceMaxVelEntry != nil {
		semblanceMaxVelEntry.SetText(prefs.StringWithFallback(prefSemblanceMaxVelocity, semblanceMaxVelEntry.Text))
	}
	if semblanceVelStepEntry != nil {
		semblanceVelStepEntry.SetText(prefs.StringWithFallback(prefSemblanceStep, semblanceVelStepEntry.Text))
	}
	if semblancePowerEntry != nil {
		semblancePowerEntry.SetText(prefs.StringWithFallback(prefSemblancePower, semblancePowerEntry.Text))
	}
	if semblanceClipEntry != nil {
		semblanceClipEntry.SetText(prefs.StringWithFallback(prefSemblanceClip, semblanceClipEntry.Text))
	}
	if semblanceStretchMuteEntry != nil {
		semblanceStretchMuteEntry.SetText(prefs.StringWithFallback(prefSemblanceStretchMute, semblanceStretchMuteEntry.Text))
	}
	if stackOverlayEnabledCheck != nil {
		stackOverlayEnabledCheck.SetChecked(prefs.BoolWithFallback(prefStackOverlayEnabled, false))
	}
	if stackOverlayOpacitySlider != nil {
		text := prefs.StringWithFallback(prefStackOverlayOpacity, defaultStackOverlayOpacity)
		if v, err := strconv.ParseFloat(text, 64); err == nil {
			suppressControlEvents = true
			if v < 0 {
				v = 0
			}
			if v > 60 {
				v = 60
			}
			stackOverlayOpacitySlider.SetValue(v)
			suppressControlEvents = false
			stackOverlayOpacityLabel.SetText(fmt.Sprintf("%.0f%%", v))
		}
	}

	highCutAuto = prefs.BoolWithFallback(prefHighCutAuto, true)
	if highCutEntry != nil {
		highCutText := prefs.StringWithFallback(prefHighCutHz, highCutEntry.Text)
		suppressControlEvents = true
		if highCutAuto {
			highCutEntry.SetText("")
		} else {
			highCutEntry.SetText(highCutText)
		}
		suppressControlEvents = false
	}
}

func persistControlPreferences() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	prefs := app.Preferences()

	if paletteSelect != nil {
		prefs.SetString(prefSemblancePalette, paletteSelect.Selected)
	}
	if gainModeSelect != nil {
		prefs.SetString(prefGainMode, gainModeSelect.Selected)
	}
	if highCutEntry != nil {
		prefs.SetString(prefHighCutHz, strings.TrimSpace(highCutEntry.Text))
	}
	prefs.SetBool(prefHighCutAuto, highCutAuto)

	if tSquaredScaleEntry != nil {
		prefs.SetString(prefTSquaredScale, strings.TrimSpace(tSquaredScaleEntry.Text))
	}
	if agcWindowEntry != nil {
		prefs.SetString(prefAGCWindow, strings.TrimSpace(agcWindowEntry.Text))
	}
	if agcFloorEntry != nil {
		prefs.SetString(prefAGCFloor, strings.TrimSpace(agcFloorEntry.Text))
	}
	if smoothingEntry != nil {
		prefs.SetString(prefSmoothingWindow, strings.TrimSpace(smoothingEntry.Text))
	}
	if semblanceMinVelEntry != nil {
		prefs.SetString(prefSemblanceMinVelocity, strings.TrimSpace(semblanceMinVelEntry.Text))
	}
	if semblanceMaxVelEntry != nil {
		prefs.SetString(prefSemblanceMaxVelocity, strings.TrimSpace(semblanceMaxVelEntry.Text))
	}
	if semblanceVelStepEntry != nil {
		prefs.SetString(prefSemblanceStep, strings.TrimSpace(semblanceVelStepEntry.Text))
	}
	if semblancePowerEntry != nil {
		prefs.SetString(prefSemblancePower, strings.TrimSpace(semblancePowerEntry.Text))
	}
	if semblanceClipEntry != nil {
		prefs.SetString(prefSemblanceClip, strings.TrimSpace(semblanceClipEntry.Text))
	}
	if semblanceStretchMuteEntry != nil {
		prefs.SetString(prefSemblanceStretchMute, strings.TrimSpace(semblanceStretchMuteEntry.Text))
	}
	if stackOverlayEnabledCheck != nil {
		prefs.SetBool(prefStackOverlayEnabled, stackOverlayEnabledCheck.Checked)
	}
	if stackOverlayOpacitySlider != nil {
		prefs.SetString(prefStackOverlayOpacity, fmt.Sprintf("%.0f", stackOverlayOpacitySlider.Value))
	}
}

func applyDefaultControlValues() {
	suppressControlReprocess = true
	defer func() { suppressControlReprocess = false }()

	highCutAuto = true

	if paletteSelect != nil {
		paletteSelect.SetSelected(defaultSemblancePalette)
	}
	if gainModeSelect != nil {
		gainModeSelect.SetSelected(defaultGainMode)
	}
	if tSquaredScaleEntry != nil {
		tSquaredScaleEntry.SetText(defaultTSquaredScale)
	}
	if agcWindowEntry != nil {
		agcWindowEntry.SetText(defaultAGCWindow)
	}
	if agcFloorEntry != nil {
		agcFloorEntry.SetText(defaultAGCFloor)
	}
	if smoothingEntry != nil {
		smoothingEntry.SetText(defaultSmoothingWindow)
	}
	if semblanceMinVelEntry != nil {
		semblanceMinVelEntry.SetText(defaultSemblanceVMin)
	}
	if semblanceMaxVelEntry != nil {
		semblanceMaxVelEntry.SetText(defaultSemblanceVMax)
	}
	if semblanceVelStepEntry != nil {
		semblanceVelStepEntry.SetText(defaultSemblanceDV)
	}
	if semblancePowerEntry != nil {
		semblancePowerEntry.SetText(defaultSemblancePower)
	}
	if semblanceClipEntry != nil {
		semblanceClipEntry.SetText(defaultSemblanceClip)
	}
	if semblanceStretchMuteEntry != nil {
		semblanceStretchMuteEntry.SetText(defaultStretchMute)
	}
	if stackOverlayEnabledCheck != nil {
		suppressControlEvents = true
		stackOverlayEnabledCheck.SetChecked(false)
		suppressControlEvents = false
	}
	if stackOverlayOpacitySlider != nil {
		suppressControlEvents = true
		stackOverlayOpacitySlider.SetValue(0)
		suppressControlEvents = false
		if stackOverlayOpacityLabel != nil {
			stackOverlayOpacityLabel.SetText("0%")
		}
	}
	if highCutEntry != nil {
		suppressControlEvents = true
		highCutEntry.SetText("")
		suppressControlEvents = false
	}
	updateGainControlsState()

	if cdpSelector != nil && cdpSelector.StepEntry != nil {
		cdpSelector.StepEntry.SetText(strconv.Itoa(defaultCDPStep))
	}
	refreshStackPickMarkers()
}

func resetPreferencesToDefaults() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	prefs := app.Preferences()

	prefs.SetString(prefSemblancePalette, defaultSemblancePalette)
	prefs.SetString(prefHighCutHz, "")
	prefs.SetBool(prefHighCutAuto, true)
	prefs.SetString(prefGainMode, defaultGainMode)
	prefs.SetString(prefTSquaredScale, defaultTSquaredScale)
	prefs.SetString(prefAGCWindow, defaultAGCWindow)
	prefs.SetString(prefAGCFloor, defaultAGCFloor)
	prefs.SetString(prefSmoothingWindow, defaultSmoothingWindow)
	prefs.SetString(prefSemblanceMinVelocity, defaultSemblanceVMin)
	prefs.SetString(prefSemblanceMaxVelocity, defaultSemblanceVMax)
	prefs.SetString(prefSemblanceStep, defaultSemblanceDV)
	prefs.SetString(prefSemblancePower, defaultSemblancePower)
	prefs.SetString(prefSemblanceClip, defaultSemblanceClip)
	prefs.SetString(prefSemblanceStretchMute, defaultStretchMute)
	prefs.SetString(prefCDPStepValue, strconv.Itoa(defaultCDPStep))
	prefs.SetString(prefCDPSelectedValue, "")

	applyDefaultControlValues()
	persistControlPreferences()
	saveCDPStepPreference(strconv.Itoa(defaultCDPStep))
}

func refreshHighCutDefaultForSampleInterval(sampleIntervalMS float64) {
	if highCutEntry == nil || sampleIntervalMS <= 0 {
		return
	}
	nyquist := 500.0 / sampleIntervalMS
	lastNyquistHz = nyquist
	if highCutAuto || strings.TrimSpace(highCutEntry.Text) == "" {
		suppressControlEvents = true
		highCutEntry.SetText(fmt.Sprintf("%.3f", nyquist))
		suppressControlEvents = false
		highCutAuto = true
	}
}

func parsePositiveFloat(entry *widget.Entry, label string) (float64, error) {
	if entry == nil {
		return 0, fmt.Errorf("%s entry is missing", label)
	}
	text := strings.TrimSpace(entry.Text)
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", label)
	}
	if v <= 0 {
		return 0, fmt.Errorf("%s must be > 0", label)
	}
	return v, nil
}

func parsePositiveInt(entry *widget.Entry, label string) (int, error) {
	if entry == nil {
		return 0, fmt.Errorf("%s entry is missing", label)
	}
	text := strings.TrimSpace(entry.Text)
	v, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", label)
	}
	if v <= 0 {
		return 0, fmt.Errorf("%s must be > 0", label)
	}
	return v, nil
}

func reprocessCurrentCDPAsync() {
	if suppressControlReprocess {
		return
	}
	persistControlPreferences()
	markStackDirtyFull()
	if currentCDPGather == nil {
		return
	}
	updateEnsembleImageViewFromGather(currentCDPGather)
}

func grayscalePalette(inverted bool) color.Palette {
	palette := make(color.Palette, 256)
	for i := 0; i < 256; i++ {
		v := uint8(i)
		if inverted {
			v = 255 - v
		}
		palette[i] = color.RGBA{R: v, G: v, B: v, A: 255}
	}
	return palette
}

func blueWhiteRedPalette(inverted bool) color.Palette {
	palette := make(color.Palette, 256)
	for i := 0; i < 256; i++ {
		t := float64(i) / 255.0
		if inverted {
			t = 1.0 - t
		}
		var r, g, b uint8
		if t < 0.5 {
			u := t / 0.5
			r = uint8(255 * u)
			g = uint8(255 * u)
			b = 255
		} else {
			u := (t - 0.5) / 0.5
			r = 255
			g = uint8(255 * (1.0 - u))
			b = uint8(255 * (1.0 - u))
		}
		palette[i] = color.RGBA{R: r, G: g, B: b, A: 255}
	}
	return palette
}
