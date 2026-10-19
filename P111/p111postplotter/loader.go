package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"github.com/hippodribble/drawing/layermanager"
	p111 "github.com/hippodribble/seismic/P111"
)

var spfiles []p111.P111File

func folderOpen() {
	dlg := dialog.NewFolderOpen(
		func(lu fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, winMain)
				return
			}
			if lu == nil {
				return
			}
			// Load P1 files from the selected folder
			go loadP1FilesFromFolder(lu)
		}, winMain,
	)
	dlg.Show()
}

func loadP1FilesFromFolder(lu fyne.ListableURI) {
	fyne.Do(func() {bOpen.Disable()})
	defer fyne.Do(func() { bOpen.Enable() })
	spfiles = []p111.P111File{}

	tempfiles, err := lu.List()
	if err != nil {
		dialog.ShowError(err, winMain)
		return
	}

	files := []fyne.URI{}
	for _, uri := range tempfiles {
		if strings.HasSuffix(strings.ToLower(uri.Name()), "11") {
			files = append(files, uri)
		}
	}

	jobs := make(chan fyne.URI, runtime.NumCPU())

	wg := &sync.WaitGroup{}
	wg.Add(len(files))

	for _, file := range files {
		jobs <- file
		go func(f fyne.URI, wg *sync.WaitGroup) {
			defer wg.Done()
			defer func() {
				u := <-jobs
				statusCh <- fmt.Sprintf("Finished processing %s", u.Name())
			}()
			linename, sps, err := p111.LoadSPsFromP111File(f.Path(), "S02")
			if err != nil {
				dialog.ShowError(err, winMain)
				return
			}
			spfiles = append(spfiles, p111.P111File{
				LineName: linename,
				SPs:      sps,
			})
		}(file, wg)
	}
	wg.Wait()

	fyne.Do(func() { plotFiles() })

}

func plotFiles() {
	if len(spfiles) == 0 {
		dialog.ShowInformation("No files loaded", "Please load P111 files first.", winMain)
		return
	}

	layer := layermanager.NewGeometryLayer()
	for _, spfile := range spfiles {

		points := []layermanager.Point{}
		for _, sp := range spfile.SPs {
			points = append(points, sp.Location)
		}
		pl := layermanager.NewPolyline(points)
		pl.ID = spfile.LineName

		layer.AddPolyline(pl.DouglasPuecker(3.0))
	}

	manager.ClearLayers()
	manager.AddLayer(layer)
	manager.ShowAll()
}
