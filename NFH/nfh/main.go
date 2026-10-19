package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var mainWin fyne.Window
var status *widget.Label
var centre, top, bottom *fyne.Container
var progress *widget.ProgressBar
var statusChannel chan string
var progressChannel chan float64

func main() {
	ap := app.NewWithID("com.github.hippodribble.segy.nfh")
	mainWin = ap.NewWindow("NFH")
	mainWin.Resize(fyne.NewSize(800, 600))
	mainWin.SetContent(gui())
	mainWin.ShowAndRun()
}

func gui() fyne.CanvasObject {
	top = makeTopButtons()
	centre = container.NewStack()
	bottom = makeBottom()
	return container.NewBorder(top, bottom, nil, nil, centre)
}

func monitorStatus(ch chan string) {
	for update := range ch {
		fyne.Do(func() { status.SetText(update) })
	}
}

func report(update string) {
	statusChannel <- update
}

func notify(title, content string) {
	fyne.CurrentApp().SendNotification(&fyne.Notification{
		Title:   title,
		Content: content,
	})
}

func makeTopButtons() *fyne.Container {
	return container.NewHBox(
		widget.NewButton("Open", func() {
			loadTestFile()
		}),
		widget.NewButton("Notify", func() {
			notify("NFH Assessment", "still going. not finished yet")
			report("sent notification")
			setProgress(.5)
		}),
	)
}

func makeBottom() *fyne.Container {

	progress = widget.NewProgressBar()
	progress.Hide()
	progressChannel = make(chan float64, 1000)
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(150, 2))
	go monitorProgress(progressChannel)

	status = widget.NewLabel("ready")
	statusChannel = make(chan string)
	go monitorStatus(statusChannel)

	return container.NewBorder(nil, nil, nil, container.NewStack(r, progress), status)
}

func monitorProgress(ch chan float64) {
	for update := range ch {
		fyne.Do(func() {
			setProgress(update)
		})
	}
}

func setProgress(update float64) {
	if update <= 0.0000001 || update >= .9999999 {
		if !progress.Hidden {
			progress.Hide()
		}
	} else {
		if progress.Hidden {
			progress.Show()
		}
		progress.SetValue(update)
	}
}
