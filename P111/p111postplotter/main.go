package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	layermanager "github.com/hippodribble/drawing/layermanager"
)

var statusCh chan string
var progressCh chan float64
var winMain fyne.Window
var manager *layermanager.LayerManager
var bOpen *widget.Button
var progress *widget.ProgressBar
var status *widget.Label

func main() {

	a := app.NewWithID("com.github.hippodribble.seismic.postplotter")
	winMain = a.NewWindow("P1/11 Post-Plot")
	winMain.SetContent(gui())
	winMain.Resize(fyne.NewSize(1000, 700))
	winMain.ShowAndRun()

}

func gui() fyne.CanvasObject {

	statusCh = make(chan string, 32)
	monitorStatusChannel(statusCh)

	progressCh = make(chan float64, 32)
	monitorProgressChannel(progressCh)

	status = widget.NewLabel("Ready")

	progress = widget.NewProgressBar()

	bOpen = widget.NewButton("Open", func() {
		folderOpen()
	})

	bExport := widget.NewButton("Export", func() {
		exportPolylines()
	})

	menu := container.NewHBox(
		bOpen,
		widget.NewCheck("ID", func(checked bool) {
			if checked {
				manager.SetMode(layermanager.ModeIdentify)
			} else {
				manager.SetMode(layermanager.ModeNone)
			}
		}),
		bExport,
	)

	manager = layermanager.NewLayerManager()
	manager.SetHighlight(true)
	manager.SetStatusChannel(statusCh)
	manager.SetMode(layermanager.ModeNone)

	statusBar := container.NewBorder(nil, nil, nil, progress, status)
	return container.NewBorder(menu, statusBar, nil, nil, manager)

}

func monitorStatusChannel(ch chan string) {
	go func() {
		for msg := range ch {
			fyne.Do(func() {
				status.SetText(msg)
			})
		}
	}()
}

func monitorProgressChannel(ch chan float64) {
	go func() {
		for update := range ch {
			fyne.Do(func() {
				progress.SetValue(update)
			})
		}
	}()
}
