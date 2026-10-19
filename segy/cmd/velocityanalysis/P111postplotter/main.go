package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	layermanager "github.com/hippodribble/drawing/layermanager"
)

func main() {
	a := app.NewWithID("com.github.hippodribble.seismic.postplotter")
	w := a.NewWindow("Layer Manager")
	w.SetContent(gui())
	w.Resize(fyne.NewSize(1000, 700))
	w.ShowAndRun()
}

func gui() fyne.CanvasObject {
	statusCh := make(chan string, 32)
	status := widget.NewLabel("Ready")
	go func() {
		for msg := range statusCh {
			fyne.Do(func() {
				status.SetText(msg)
			})
		}
	}()

	menu := container.NewHBox(
		widget.NewButton("Open", func() { statusCh <- "Open file" }),
		widget.NewButton("Save", func() { statusCh <- "Save project" }),
		widget.NewButton("Zoom In", func() { statusCh <- "Zoom in" }),
		widget.NewButton("Zoom Out", func() { statusCh <- "Zoom out" }),
	)

	manager := layermanager.NewLayerManager()
	manager.SetStatusChannel(statusCh)
	manager.SetMode(layermanager.ModeNone)

	layer := layermanager.NewGeometryLayer()
	layer.AddPolyline(layermanager.NewPolyline([]layermanager.Point{
		{X: 0, Y: 0},
		{X: 160, Y: 80},
		{X: 300, Y: 70},
		{X: 420, Y: 0},
		{X: 380, Y: -120},
		{X: 200, Y: -140},
		{X: 0, Y: 0},
	}))
	manager.AddLayer(layer)
	manager.ShowAll()

	statusBar := container.NewBorder(nil, nil, nil, nil, status)
	return container.NewBorder(menu, statusBar, nil, nil, manager)
}
