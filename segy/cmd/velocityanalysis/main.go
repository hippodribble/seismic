package main

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/hippodribble/seismic/segy"
	"github.com/hippodribble/seismic/segy/velocitywidgets"
)

var progress *widget.ProgressBar
var status *widget.Label
var statuschannel chan string
var top, bottom, left, right, centre *fyne.Container
var catalog *segy.TraceCatalog
var loadedFile *segy.File
var winMain fyne.Window
var btnOpenFile *widget.Button
var btnCancelLoad *widget.Button
var topCentre *fyne.Container
var currentCDPGather *segy.CDPGather
var ensembleImageView *velocitywidgets.EnsembleImageView

const prefLastOpenedURI = "velocityanalysis.last_opened_uri"
const prefLastPickURI = "velocityanalysis.last_pick_uri"

const (
	prefCDPStepValue          = "velocityanalysis.cdp.step"
	prefCDPSelectedValue      = "velocityanalysis.cdp.selected"
	prefSemblancePalette      = "velocityanalysis.ctrl.semblance_palette"
	prefHighCutHz             = "velocityanalysis.ctrl.high_cut_hz"
	prefHighCutAuto           = "velocityanalysis.ctrl.high_cut_auto"
	prefGainMode              = "velocityanalysis.ctrl.gain_mode"
	prefTSquaredScale         = "velocityanalysis.ctrl.tsquared_scale"
	prefAGCWindow             = "velocityanalysis.ctrl.agc_window"
	prefAGCFloor              = "velocityanalysis.ctrl.agc_floor"
	prefSmoothingWindow       = "velocityanalysis.ctrl.smoothing_window"
	prefSemblanceMinVelocity  = "velocityanalysis.ctrl.semblance_vmin"
	prefSemblanceMaxVelocity  = "velocityanalysis.ctrl.semblance_vmax"
	prefSemblanceStep         = "velocityanalysis.ctrl.semblance_dv"
	prefSemblancePower        = "velocityanalysis.ctrl.semblance_power"
	prefSemblanceClip         = "velocityanalysis.ctrl.semblance_clip"
	prefSemblanceStretchMute  = "velocityanalysis.ctrl.semblance_stretch_mute"
	prefSemblancePickEditMode = "velocityanalysis.semblance.pick_edit_mode"
	prefCDPNMOPreviewEnabled  = "velocityanalysis.cdp.nmo_preview_enabled"
	prefCDPMutePickMode       = "velocityanalysis.cdp.mute_pick_mode"
	prefEnsembleUpperSplit    = "velocityanalysis.layout.ensemble_upper_split"
	prefEnsembleMainSplit     = "velocityanalysis.layout.ensemble_main_split"
)

func main() {
	a := app.NewWithID("com.github.hippodribble.seismic.velocityanalysis")
	winMain = a.NewWindow("Velocity Analysis")
	var closeOnce sync.Once
	winMain.SetOnClosed(func() {
		closeOnce.Do(func() {
			persistControlPreferences()
			persistCDPSelectorPreferences()
			persistEnsembleSplitPreferences()
			a.Quit()
		})
	})
	winMain.SetContent(gui())
	installGlobalKeyHandlers()
	winMain.Resize(fyne.NewSize(800, 1600))
	winMain.SetFullScreen(true)
	go tryRestoreLastOpenedFile()
	winMain.ShowAndRun()
}

func gui() fyne.CanvasObject {
	status = widget.NewLabel("Ready")
	progress = widget.NewProgressBar()
	statuschannel = make(chan string, 100)

	go monitorStatus()

	topleft := container.NewHBox(
		func() *widget.Button {
			btnOpenFile = widget.NewButton("SEG-Y File", openFile)
			return btnOpenFile
		}(),
		func() *widget.Button {
			btnCancelLoad = widget.NewButton("Cancel Load", cancelCurrentLoad)
			btnCancelLoad.Disable()
			return btnCancelLoad
		}(),
		widget.NewButton("Controls", toggleControlsPopup),
	)
	topCentre = buildTopRightControls()

	top = container.NewBorder(nil, nil, topleft, nil, topCentre)

	bottom = container.NewBorder(nil, nil, nil, progress, status)

	left = container.NewVBox()

	right = container.NewVBox()

	centre = buildCentreView()
	initControlsPopup()

	return container.NewBorder(top, bottom, left, right, centre)
}

func monitorStatus() {
	for update := range statuschannel {
		fyne.Do(func() { status.SetText(update) })
	}
}

func installGlobalKeyHandlers() {
	if winMain == nil || winMain.Canvas() == nil {
		return
	}

	winMain.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev == nil {
			return
		}

		if focused := winMain.Canvas().Focused(); focused != nil {
			switch focused.(type) {
			case *widget.Entry, *widget.SelectEntry:
				return
			}
		}

		switch ev.Name {
		case fyne.KeyLeft:
			stepActiveCDPBySelectorStep(-1)
		case fyne.KeyRight:
			stepActiveCDPBySelectorStep(1)
		}
	})
}
