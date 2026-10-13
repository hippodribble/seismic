package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	layermanager "github.com/hippodribble/drawing/layermanager"
)

var viewport *layermanager.LayerManager
var top *fyne.Container
var centre *fyne.Container
var status = widget.NewLabel("Status: Ready")
var bottom = container.NewVBox(status)
var statusChannel = make(chan string)
var winMain fyne.Window

func main() {
	ap := app.NewWithID("canvas.test")
	winMain = ap.NewWindow("Canvas Test")
	winMain.Resize(fyne.NewSize(1500, 1000))
	winMain.SetContent(gui())
	winMain.ShowAndRun()
}

func gui() fyne.CanvasObject {
	go monitorStatusUpdates()
	viewport = layermanager.NewLayerManager()
	viewport.SetStatusChannel(statusChannel)
	frame := canvas.NewRectangle(color.Transparent)
	frame.StrokeWidth = 1
	frame.CornerRadius = 10
	frame.StrokeColor = theme.Color(theme.ColorNameForeground)
	centre = container.NewStack(frame, viewport)
	top = container.NewHBox(
		widget.NewButton("P1/90 Preplot", loadSailLines),
		widget.NewButton("P1/90 Text", showP190TextDialog),
		widget.NewButton("Clear", viewport.ClearLayers),
		widget.NewButton("Fit To Screen", viewport.ShowAll),
		widget.NewCheck("Measure", func(b bool) {
			if b {
				viewport.SetMode(layermanager.ModeMeasure)
			} else {
				viewport.SetMode(layermanager.ModeNone)
			}
		}),
		widget.NewCheck("ID", func(checked bool) {
			if checked {
				viewport.SetMode(layermanager.ModeIdentify)
			} else {
				viewport.SetMode(layermanager.ModeNone)
			}
		}),
	)

	b := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		centre,
	)
	return b
}

func monitorStatusUpdates() {
	for update := range statusChannel {
		fyne.Do(func() { status.SetText(update) })
	}
}
