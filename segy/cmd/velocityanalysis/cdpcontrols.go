package main

import (
	"fmt"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/hippodribble/seismic/segy"
	"github.com/hippodribble/seismic/segy/velocitywidgets"
)

var cdpSelector *velocitywidgets.CDPSelector
var pickModeCheck *widget.Check
var muteModeCheck *widget.Check
var nmoPreviewCheck *widget.Check
var pickLockCheck *widget.Check
var autoStackCheck *widget.Check
var nmoStateLabel *widget.Label
var stackStateLabel *widget.Label
var suppressPickControlEvents bool
var autoStackEnabled bool

func buildTopRightControls() *fyne.Container {
	cdpSelector = velocitywidgets.NewCDPSelector(velocitywidgets.CDPSelectorOptions{
		StepValues:       []int{10, 20, 50, 100},
		DefaultStep:      10,
		MaxEntryOptions:  5000,
		MinInputWidth:    100,
		ShowPreviewLabel: true,
	})
	restoreCDPSelectorStepPreference()
	if cdpSelector.StepEntry != nil {
		prev := cdpSelector.StepEntry.OnChanged
		cdpSelector.StepEntry.OnChanged = func(text string) {
			if prev != nil {
				prev(text)
			}
			saveCDPStepPreference(text)
		}
	}
	cdpSelector.SetOnCommit(func(cdp int) {
		saveCDPSelectedPreference(cdp)
		storeCurrentCDPGather(int64(cdp))
	})

	pickModeCheck = widget.NewCheck("Pick mode", func(enabled bool) {
		if suppressPickControlEvents {
			return
		}
		if enabled && muteModeCheck != nil {
			suppressPickControlEvents = true
			muteModeCheck.SetChecked(false)
			suppressPickControlEvents = false
			setCDPMutePickMode(false)
			saveCDPMuteModePreference(false)
		}
		setSemblancePickEditMode(enabled)
		savePickModePreference(enabled)
	})
	pickModeCheck.SetChecked(readPickModePreference())

	muteModeCheck = widget.NewCheck("Mute mode", func(enabled bool) {
		if suppressPickControlEvents {
			return
		}
		if enabled && pickModeCheck != nil {
			suppressPickControlEvents = true
			pickModeCheck.SetChecked(false)
			suppressPickControlEvents = false
			setSemblancePickEditMode(false)
			savePickModePreference(false)
		}
		setCDPMutePickMode(enabled)
		saveCDPMuteModePreference(enabled)
	})
	muteModeCheck.SetChecked(readCDPMuteModePreference())

	nmoPreviewCheck = widget.NewCheck("NMO preview", func(enabled bool) {
		if suppressPickControlEvents {
			return
		}
		setCDPNMOPreviewEnabled(enabled)
		saveCDPNMOPreviewPreference(enabled)
	})
	nmoPreviewCheck.SetChecked(readCDPNMOPreviewPreference())

	pickLockCheck = widget.NewCheck("Lock picks", func(locked bool) {
		if suppressPickControlEvents {
			return
		}
		setCurrentCDPPickLocked(locked)
	})

	autoStackCheck = widget.NewCheck("Auto Stack", func(enabled bool) {
		autoStackEnabled = enabled
		if enabled {
			pushStatus("Auto Stack enabled")
			return
		}
		pushStatus("Auto Stack disabled")
	})
	autoStackCheck.SetChecked(false)

	nmoStateLabel = widget.NewLabel("NMO: idle")
	nmoStateLabel.Importance = widget.MediumImportance
	stackStateLabel = widget.NewLabel("Stack: idle")
	stackStateLabel.Importance = widget.MediumImportance

	btnExport := widget.NewButton("Export Picks", exportSemblancePicks)
	btnImport := widget.NewButton("Import Picks", importSemblancePicks)
	btnClearVelocity := widget.NewButton("Clear V Picks", clearCurrentCDPVelocityPicks)
	btnClearMute := widget.NewButton("Clear Mute Picks", clearCurrentCDPMutePicks)
	btnClearAllVelocity := widget.NewButton("Clear All V", func() {
		confirmClearAllPicks("Clear All Velocity Picks?", "This will remove all velocity picks across all CDPs.", clearAllVelocityPicks)
	})
	btnClearAllMute := widget.NewButton("Clear All Mute", func() {
		confirmClearAllPicks("Clear All Top Mute Picks?", "This will remove all top mute picks across all CDPs.", clearAllMutePicks)
	})
	btnBuildStack := widget.NewButton("Build Stack", buildWholeLineStackAsync)

	row1 := container.NewHBox(cdpSelector.CanvasObject(), pickModeCheck, muteModeCheck, nmoPreviewCheck, pickLockCheck, autoStackCheck, btnClearVelocity, btnClearMute, btnClearAllVelocity, btnClearAllMute, btnBuildStack, btnImport, btnExport)
	row2 := container.NewHBox(nmoStateLabel, stackStateLabel)
	controls := container.NewVBox(row1, row2)

	setCDPNMOPreviewEnabled(readCDPNMOPreviewPreference())
	setCDPMutePickMode(readCDPMuteModePreference())
	syncPickLockCheck(false)
	updateNMOIndicatorForCurrent()
	setStackStateLabel("Stack: idle")
	return container.NewStack(controls)
}

func readPickModePreference() bool {
	app := fyne.CurrentApp()
	if app == nil {
		return false
	}
	return app.Preferences().BoolWithFallback(prefSemblancePickEditMode, false)
}

func savePickModePreference(enabled bool) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	app.Preferences().SetBool(prefSemblancePickEditMode, enabled)
}

func readCDPMuteModePreference() bool {
	app := fyne.CurrentApp()
	if app == nil {
		return false
	}
	return app.Preferences().BoolWithFallback(prefCDPMutePickMode, false)
}

func saveCDPMuteModePreference(enabled bool) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	app.Preferences().SetBool(prefCDPMutePickMode, enabled)
}

func readCDPNMOPreviewPreference() bool {
	app := fyne.CurrentApp()
	if app == nil {
		return true
	}
	return app.Preferences().BoolWithFallback(prefCDPNMOPreviewEnabled, true)
}

func saveCDPNMOPreviewPreference(enabled bool) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	app.Preferences().SetBool(prefCDPNMOPreviewEnabled, enabled)
}

func syncPickLockCheck(locked bool) {
	if pickLockCheck == nil {
		return
	}
	fyne.Do(func() {
		suppressPickControlEvents = true
		pickLockCheck.SetChecked(locked)
		suppressPickControlEvents = false
	})
}

func setNMOStateLabel(text string) {
	if nmoStateLabel == nil {
		return
	}
	fyne.Do(func() {
		nmoStateLabel.SetText(text)
	})
}

func setStackStateLabel(text string) {
	if stackStateLabel == nil {
		return
	}
	fyne.Do(func() {
		stackStateLabel.SetText(text)
	})
}

func refreshCDPControlsFromCatalog(cat *segy.TraceCatalog) {
	if cdpSelector == nil {
		return
	}

	if cat == nil {
		fyne.Do(func() { cdpSelector.Disable() })
		return
	}

	summary := cat.HeaderRangeSummary()
	rng, ok := summary.Ranges[segy.TraceHeaderFieldCDP]
	if !ok {
		fyne.Do(func() { cdpSelector.Disable() })
		return
	}

	minCDP := int(rng.Min)
	maxCDP := int(rng.Max)
	preferredCDP, hasPreferredCDP := readPreferredCDPInRange(minCDP, maxCDP)
	fyne.Do(func() {
		cdpSelector.SetRange(minCDP, maxCDP)
		if hasPreferredCDP {
			cdpSelector.CDPEntry.SetText(strconv.Itoa(preferredCDP))
		}
	})
}

func readPreferredCDPInRange(minCDP, maxCDP int) (int, bool) {
	app := fyne.CurrentApp()
	if app == nil {
		return 0, false
	}
	text := app.Preferences().StringWithFallback(prefCDPSelectedValue, "")
	if text == "" {
		return 0, false
	}
	v, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}
	if v < minCDP {
		v = minCDP
	}
	if v > maxCDP {
		v = maxCDP
	}
	return v, true
}

func restoreCDPSelectorStepPreference() {
	if cdpSelector == nil || cdpSelector.StepEntry == nil {
		return
	}
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	text := app.Preferences().StringWithFallback(prefCDPStepValue, "")
	if text == "" {
		return
	}
	if v, err := strconv.Atoi(text); err != nil || v <= 0 {
		return
	}
	cdpSelector.StepEntry.SetText(text)
}

func saveCDPStepPreference(text string) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	if v, err := strconv.Atoi(text); err != nil || v <= 0 {
		return
	}
	app.Preferences().SetString(prefCDPStepValue, text)
}

func saveCDPSelectedPreference(cdp int) {
	if cdp <= 0 {
		return
	}
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	app.Preferences().SetString(prefCDPSelectedValue, strconv.Itoa(cdp))
}

func persistCDPSelectorPreferences() {
	if cdpSelector == nil {
		return
	}
	if cdpSelector.StepEntry != nil {
		saveCDPStepPreference(cdpSelector.StepEntry.Text)
	}
	if cdpSelector.CDPEntry != nil {
		if v, err := strconv.Atoi(cdpSelector.CDPEntry.Text); err == nil && v > 0 {
			saveCDPSelectedPreference(v)
		}
	}
}

func storeCurrentCDPGather(cdp int64) {
	if cdp <= 0 {
		currentCDPGather = nil
		clearEnsembleImageView()
		return
	}
	if loadedFile == nil {
		currentCDPGather = nil
		clearEnsembleImageView()
		return
	}

	gather, err := loadedFile.BuildCDPGatherByCDP(cdp, true)
	if err != nil {
		currentCDPGather = nil
		clearEnsembleImageView()
		pushStatus(fmt.Sprintf("CDP %d selection failed: %v", cdp, err))
		return
	}

	currentCDPGather = &gather
	updateEnsembleImageViewFromGather(currentCDPGather)
	pushStatus(fmt.Sprintf("Selected CDP %d (%d traces)", gather.CDP, len(gather.Traces)))
}

func stepActiveCDPBySelectorStep(direction int) {
	if direction == 0 || cdpSelector == nil || loadedFile == nil {
		return
	}

	step := defaultCDPStep
	if cdpSelector.StepEntry != nil {
		if v, err := strconv.Atoi(cdpSelector.StepEntry.Text); err == nil && v > 0 {
			step = v
		}
	}

	current := 0
	if cdpSelector.CDPEntry != nil {
		if v, err := strconv.Atoi(cdpSelector.CDPEntry.Text); err == nil && v > 0 {
			current = v
		}
	}
	if current <= 0 && currentCDPGather != nil {
		current = int(currentCDPGather.CDP)
	}
	if current <= 0 {
		return
	}

	target := current + direction*step
	if cdpSelector.Slider != nil {
		minCDP := int(math.Round(cdpSelector.Slider.Min))
		maxCDP := int(math.Round(cdpSelector.Slider.Max))
		if target < minCDP {
			target = minCDP
		}
		if target > maxCDP {
			target = maxCDP
		}
	}
	if target == current {
		return
	}

	if cdpSelector.CDPEntry != nil {
		cdpSelector.CDPEntry.SetText(strconv.Itoa(target))
		return
	}

	saveCDPSelectedPreference(target)
	storeCurrentCDPGather(int64(target))
}

func confirmClearAllPicks(title, message string, onConfirm func()) {
	if onConfirm == nil {
		return
	}
	if winMain == nil {
		onConfirm()
		return
	}
	dialog.NewConfirm(title, message, func(ok bool) {
		if !ok {
			return
		}
		onConfirm()
	}, winMain).Show()
}
