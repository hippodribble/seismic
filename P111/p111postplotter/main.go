package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	layermanager "github.com/hippodribble/drawing/layermanager"
)

var statusCh chan string
var winMain fyne.Window
var manager *layermanager.LayerManager
var bOpen *widget.Button

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
	status := widget.NewLabel("Ready")
	go func() {
		for msg := range statusCh {
			fyne.Do(func() {
				status.SetText(msg)
			})
		}
	}()

	bOpen=widget.NewButton("Open", func() {
			folderOpen()
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
	)

	manager = layermanager.NewLayerManager()
	manager.SetHighlight(true)
	manager.SetStatusChannel(statusCh)
	manager.SetMode(layermanager.ModeNone)

	statusBar := container.NewBorder(nil, nil, nil, nil, status)
	return container.NewBorder(menu, statusBar, nil, nil, manager)
}

func monitorStatusChannel(ch chan string) {
	go func() {
		for msg := range ch {
			fyne.Do(func() {
				// Here you can add any additional logic to handle the status messages
				// For example, you could log them or update a status bar in the GUI
				statusCh <- msg
			})
		}
	}()
}