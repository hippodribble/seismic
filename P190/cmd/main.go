package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/hippodribble/drawing"
)

var viewport *drawing.LayerManager
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
	proj := drawing.NewUnitProjection()
	viewport = drawing.NewLayerManager(proj, nil)
	viewport.SetStatusChannel(statusChannel)
	centre = container.NewStack(viewport)
	top = container.NewHBox(
		widget.NewButton("P1/90 Preplot", loadSailLines),
		widget.NewButton("P1/90 Text", showP190TextDialog),
		widget.NewButton("Clear Layers", viewport.ClearLayers),
		widget.NewButton("Show All", viewport.ShowAll),
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
