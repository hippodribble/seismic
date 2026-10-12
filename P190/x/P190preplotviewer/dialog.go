package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/hippodribble/drawing/layermanager"
	"github.com/hippodribble/seismic/P190"
)

var p190Text string
var filename string

func allowedSailFileSuffixes() []string {
	return []string{".p190", ".p90", ".90"}
}

func hasAllowedSailSuffix(name string) bool {
	lowerName := strings.ToLower(name)
	for _, suffix := range allowedSailFileSuffixes() {
		if strings.HasSuffix(lowerName, suffix) {
			return true
		}
	}
	return false
}

func loadSailLines() {
	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.NewError(err, nil).Show()
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

		filename := uri.Name()
		if !hasAllowedSailSuffix(filename) {
			dialog.NewError(errors.New("only .p190, .p90, and .90 files are supported"), nil).Show()
			return
		}

		status.SetText("Loading sail file: " + filename)
		loadP190FromFile(reader)
	}, winMain)

	fileDialog.SetTitleText("Open sail file")
	fileDialog.SetFilter(storage.NewExtensionFileFilter(allowedSailFileSuffixes()))
	fileDialog.Show()
}

// loadP190FromFile reads the contents of a P190 file as simple text.
// It resets any existing data and drawing layers
func loadP190FromFile(reader fyne.URIReadCloser) {
	Bytes, err := io.ReadAll(reader)
	if err != nil {
		dialog.NewError(err, nil).Show()
	}

	p190Text = string(Bytes)
	filename = reader.URI().Path()
	p190File := P190.NewP190File(filename, P190.MostCommonPrefix(p190Text)) // Automatically determine the record type based on the most common prefix in the file
	err = p190File.Load()
	if err != nil {
		dialog.NewError(err, nil).Show()
		return
	}

	polylineLayer := layermanager.NewGeometryLayer()

	thepolylines := []layermanager.Polyline{}
	for _, line := range p190File.Lines {
		thepoints := []layermanager.Point{}
		for _, r := range *line.Records {
			thepoints = append(thepoints, layermanager.Point{X: r.Easting, Y: r.Northing})
		}
		polyline := layermanager.Polyline{Points: thepoints, ID: line.Name}
		thepolylines = append(thepolylines, polyline)
		polylineLayer.AddPolyline(polyline)
	}
	viewport.ClearLayers()
	polylineLayer.AttachTo(viewport)
	status.SetText(fmt.Sprintf("Loaded %d polylines from %s", len(thepolylines), filename))
	viewport.ShowAll()
}

func showP190TextDialog() {
	if p190Text == "" {
		dialog.NewInformation("No P190 file loaded", "Please load a P190 file first.", winMain).Show()
		return
	}
	label := widget.NewLabel(p190Text)
	label.TextStyle.Monospace = true

	dlg := dialog.NewCustom("P190 File Contents "+filename, "Close", container.NewVScroll(label), winMain)
	dlg.Resize(fyne.NewSize(800, 1000))
	dlg.Show()
}
