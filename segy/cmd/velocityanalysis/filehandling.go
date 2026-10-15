package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"github.com/hippodribble/seismic/segy"
)

var loadStateMu sync.Mutex
var loadInProgress bool
var loadCancelRequested bool

func updateProgressBar(value float64) {
	if progress == nil {
		return
	}
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	fyne.Do(func() {
		progress.SetValue(value)
	})
}

func pushStatus(msg string) {
	if statuschannel != nil {
		statuschannel <- msg
	}
}

func setLoadControls(isLoading bool) {
	fyne.Do(func() {
		if btnOpenFile != nil {
			if isLoading {
				btnOpenFile.Disable()
			} else {
				btnOpenFile.Enable()
			}
		}
		if btnCancelLoad != nil {
			if isLoading {
				btnCancelLoad.Enable()
			} else {
				btnCancelLoad.Disable()
			}
		}
	})
}

func beginLoad() bool {
	loadStateMu.Lock()
	if loadInProgress {
		loadStateMu.Unlock()
		return false
	}
	loadInProgress = true
	loadCancelRequested = false
	loadStateMu.Unlock()
	setLoadControls(true)
	return true
}

func finishLoad() {
	loadStateMu.Lock()
	loadInProgress = false
	loadCancelRequested = false
	loadStateMu.Unlock()
	setLoadControls(false)
}

func shouldCancelLoad() bool {
	loadStateMu.Lock()
	defer loadStateMu.Unlock()
	return loadCancelRequested
}

func cancelCurrentLoad() {
	loadStateMu.Lock()
	active := loadInProgress
	loadCancelRequested = loadInProgress
	loadStateMu.Unlock()
	if active {
		pushStatus("Cancel requested...")
	}
}

func openFile() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}

	windows := app.Driver().AllWindows()
	if len(windows) == 0 {
		return
	}
	w := windows[0]

	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, w)
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

		name := strings.ToLower(uri.Name())
		isSU := strings.HasSuffix(name, ".su")
		if !strings.HasSuffix(name, ".segy") && !strings.HasSuffix(name, ".sgy") && !isSU {
			dialog.ShowError(errors.New("only .segy, .sgy and .su files are supported"), w)
			return
		}

		if !beginLoad() {
			pushStatus("A file load is already in progress")
			return
		}

		go func() {
			defer finishLoad()
			defer updateProgressBar(0)
			currentCDPGather = nil
			clearEnsembleImageView()
			pushStatus(fmt.Sprintf("Loading %s ...", uri.Name()))
			updateProgressBar(0)

			if loadErr := loadCatalogFromURIWithProgress(uri, true); loadErr != nil {
				if errors.Is(loadErr, segy.ErrCatalogBuildCanceled) {
					pushStatus(fmt.Sprintf("Canceled loading %s", uri.Name()))
					return
				}
				fyne.Do(func() {
					dialog.ShowError(loadErr, w)
				})
				pushStatus(fmt.Sprintf("Failed loading %s", uri.Name()))
				return
			}

			saveLastOpenedURI(uri)
			pushStatus(fmt.Sprintf("Loaded %s: %d traces cataloged", uri.Name(), len(catalog.Entries)))
		}()
	}, w)

	fileDialog.SetTitleText("Open SEG-Y/SU file")
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".segy", ".sgy", ".su"}))
	fileDialog.Show()
}

func loadCatalogFromURIWithProgress(uri fyne.URI, reportProgress bool) error {
	if uri == nil {
		return fmt.Errorf("nil uri")
	}

	name := strings.ToLower(uri.Name())
	isSU := strings.HasSuffix(name, ".su")
	if !strings.HasSuffix(name, ".segy") && !strings.HasSuffix(name, ".sgy") && !isSU {
		return errors.New("only .segy, .sgy and .su files are supported")
	}

	var (
		f            *segy.File
		openErr      error
		builtCatalog *segy.TraceCatalog
		catalogErr   error
	)

	if isSU {
		f, openErr = segy.OpenSU(uri)
	} else {
		f, openErr = segy.Open(uri)
	}
	if openErr != nil {
		return openErr
	}

	progressFn := segy.CatalogBuildProgressFunc(nil)
	if reportProgress {
		progressFn = func(p segy.CatalogBuildProgress) {
			if p.TotalBytes > 0 {
				updateProgressBar(float64(p.BytesProcessed) / float64(p.TotalBytes))
			}
			if p.Done {
				if p.Canceled {
					pushStatus(fmt.Sprintf("Cataloging canceled at %d traces", p.TraceCount))
				} else {
					pushStatus(fmt.Sprintf("Cataloged %d traces", p.TraceCount))
				}
			} else if p.TraceCount > 0 && p.TraceCount%2000 == 0 {
				if p.TotalBytes > 0 {
					pushStatus(fmt.Sprintf("Cataloging... %d traces (%.1f%%)", p.TraceCount, 100*float64(p.BytesProcessed)/float64(p.TotalBytes)))
				} else {
					pushStatus(fmt.Sprintf("Cataloging... %d traces", p.TraceCount))
				}
			}
		}
	}

	if isSU {
		builtCatalog, catalogErr = f.BuildTraceCatalogTraceOnlyWithProgressAndCancel(progressFn, shouldCancelLoad)
	} else {
		builtCatalog, catalogErr = f.BuildTraceCatalogWithProgressAndCancel(progressFn, shouldCancelLoad)
	}
	if catalogErr != nil {
		loadedFile = nil
		currentCDPGather = nil
		clearEnsembleImageView()
		return catalogErr
	}

	loadedFile = f
	catalog = builtCatalog
	setMainWindowTitleForFile(uri.Name())
	refreshCDPControlsFromCatalog(catalog)
	return nil
}

func setMainWindowTitleForFile(name string) {
	title := strings.TrimSpace(name)
	if title == "" {
		title = "Velocity Analysis"
	}
	fyne.Do(func() {
		if winMain != nil {
			winMain.SetTitle(title)
		}
	})
}

func saveLastOpenedURI(uri fyne.URI) {
	if uri == nil || fyne.CurrentApp() == nil {
		return
	}
	fyne.CurrentApp().Preferences().SetString(prefLastOpenedURI, uri.String())
}

func tryRestoreLastOpenedFile() {
	if fyne.CurrentApp() == nil {
		return
	}

	uriStr := strings.TrimSpace(fyne.CurrentApp().Preferences().String(prefLastOpenedURI))
	if uriStr == "" {
		return
	}

	uri, err := storage.ParseURI(uriStr)
	if err != nil {
		return
	}

	if strings.EqualFold(uri.Scheme(), "file") {
		if _, statErr := os.Stat(uri.Path()); statErr != nil {
			return
		}
	}

	if statuschannel != nil {
		statuschannel <- fmt.Sprintf("Restoring %s ...", uri.Name())
	}
	if !beginLoad() {
		return
	}
	defer finishLoad()
	defer updateProgressBar(0)

	if loadErr := loadCatalogFromURIWithProgress(uri, true); loadErr != nil {
		if errors.Is(loadErr, segy.ErrCatalogBuildCanceled) {
			if statuschannel != nil {
				statuschannel <- "Restore canceled"
			}
			return
		}
		if statuschannel != nil {
			statuschannel <- "Could not restore last opened file"
		}
		return
	}

	if statuschannel != nil {
		statuschannel <- fmt.Sprintf("Restored %s: %d traces cataloged", uri.Name(), len(catalog.Entries))
	}
	tryRestoreLastPickFile()
}
