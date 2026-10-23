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
	"github.com/hippodribble/seismic/P190"
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
			go loadP1FilesFromFolder(lu)
		}, winMain,
	)
	dlg.Show()
}

func loadP1FilesFromFolder(lu fyne.ListableURI) {
	fyne.Do(func() { bOpen.Disable() })
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

	n := float64(len(files))
	count := 0.0

	defer func() {
		progressCh <- 0.0
	}()

	jobs := make(chan fyne.URI, runtime.NumCPU()*2/3)

	wg := &sync.WaitGroup{}
	wg.Add(len(files))

	for _, file := range files {

		jobs <- file
		go func(f fyne.URI, wg *sync.WaitGroup) {
			defer wg.Done()
			defer func() {
				u := <-jobs
				statusCh <- fmt.Sprintf("Processed %s (%d/%d)", u.Name(), int(count), int(n))
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

			count++
			progressCh <- count / n

		}(file, wg)
	}
	wg.Wait()

	fyne.Do(func() {
		plotFiles()
		winMain.SetTitle(fmt.Sprintf("P1/11 Post-Plot - %s", lu.Path()))
	})
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

func exportPolylines() {
	if len(spfiles) == 0 {
		statusCh <- "No files loaded. Please load P111 files first."
		return
	}

	pls := []P190.PreplotLine{}
	for _, spfile := range spfiles {
		nrs := []P190.NodeRecord{}
		for _, sp := range spfile.SPs {
			nr := P190.NodeRecord{LineName: spfile.LineName, Easting: sp.Location.X, Northing: sp.Location.Y}
			nrs = append(nrs, nr)
		}
		pl := P190.NewPreplotLine(spfile.LineName)
		pl.Records = &nrs
		pls = append(pls, pl)
	}
	dlg := dialog.NewFileSave(
		func(lu fyne.URIWriteCloser, err error) {
			if err != nil {
				dialog.ShowError(err, winMain)
				return
			}
			if lu == nil {
				return
			}
			file := P190.NewP190File(lu.URI().Path(), "V")
			file.Lines = pls
			err = file.ToShapeFile(lu.URI().Path())
			if err != nil {
				statusCh <- fmt.Sprintf("Error exporting to %s: %v", lu.URI().Path(), err)
				return
			}
			statusCh <- fmt.Sprintf("Exported %d lines to %s", len(pls), lu.URI().Path())
		}, winMain,
	)
	dlg.SetFileName("export.shp")
	dlg.Show()

}
