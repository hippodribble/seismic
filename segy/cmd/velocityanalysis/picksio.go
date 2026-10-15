package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"github.com/hippodribble/seismic/segy/velocitywidgets"
)

type pickExportFile struct {
	Version         int                                       `json:"version"`
	CDPPicks        map[string][]velocitywidgets.EnsemblePick `json:"cdp_picks"`
	CDPTopMutePicks map[string][]velocitywidgets.EnsemblePick `json:"cdp_top_mute_picks,omitempty"`
}

func exportSemblancePicks() {
	if winMain == nil {
		return
	}
	snapshotCurrentSemblancePicks()
	snapshotCurrentCDPTopMutePicks()

	saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, winMain)
			return
		}
		if writer == nil {
			return
		}
		defer writer.Close()

		payload := pickExportFile{
			Version:         2,
			CDPPicks:        make(map[string][]velocitywidgets.EnsemblePick, len(semblancePicksByCDP)),
			CDPTopMutePicks: make(map[string][]velocitywidgets.EnsemblePick, len(topMutePicksByCDP)),
		}
		for cdp, picks := range semblancePicksByCDP {
			payload.CDPPicks[strconv.FormatInt(cdp, 10)] = append([]velocitywidgets.EnsemblePick(nil), picks...)
		}
		for cdp, picks := range topMutePicksByCDP {
			payload.CDPTopMutePicks[strconv.FormatInt(cdp, 10)] = append([]velocitywidgets.EnsemblePick(nil), picks...)
		}

		blob, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr != nil {
			dialog.ShowError(marshalErr, winMain)
			return
		}
		if _, writeErr := writer.Write(blob); writeErr != nil {
			dialog.ShowError(writeErr, winMain)
			return
		}
		saveLastPickURI(writer.URI())
		semblancePicksDirty = false
		topMutePicksDirty = false
		pushStatus(fmt.Sprintf("Exported picks for %d CDPs", len(payload.CDPPicks)))
	}, winMain)
	saveDialog.SetFileName("semblance-picks.json")
	saveDialog.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
	saveDialog.Show()
}

func importSemblancePicks() {
	if winMain == nil {
		return
	}

	if hasUnsavedSemblancePicks() {
		dialog.NewConfirm(
			"Overwrite Unsaved Picks?",
			"You have unsaved pick edits in memory. Importing will replace current picks. Continue?",
			func(ok bool) {
				if !ok {
					return
				}
				runSemblancePickImportDialog()
			},
			winMain,
		).Show()
		return
	}

	runSemblancePickImportDialog()
}

func runSemblancePickImportDialog() {
	if winMain == nil {
		return
	}

	openDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, winMain)
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()

		total, importErr := importSemblancePicksFromReader(reader)
		if importErr != nil {
			dialog.ShowError(importErr, winMain)
			return
		}
		saveLastPickURI(reader.URI())
		pushStatus(fmt.Sprintf("Imported %d picks across %d CDPs", total, len(semblancePicksByCDP)))
	}, winMain)
	openDialog.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
	openDialog.Show()
}

func importSemblancePicksFromReader(reader io.Reader) (int, error) {
	if reader == nil {
		return 0, fmt.Errorf("nil reader")
	}
	data, readErr := io.ReadAll(reader)
	if readErr != nil {
		return 0, readErr
	}

	var payload pickExportFile
	if unmarshalErr := json.Unmarshal(data, &payload); unmarshalErr != nil {
		return 0, unmarshalErr
	}

	loaded := make(map[int64][]velocitywidgets.EnsemblePick, len(payload.CDPPicks))
	loadedTopMute := make(map[int64][]velocitywidgets.EnsemblePick, len(payload.CDPTopMutePicks))
	total := 0
	for key, picks := range payload.CDPPicks {
		cdp, parseErr := strconv.ParseInt(strings.TrimSpace(key), 10, 64)
		if parseErr != nil {
			return 0, fmt.Errorf("invalid CDP key %q: %w", key, parseErr)
		}
		loaded[cdp] = append([]velocitywidgets.EnsemblePick(nil), picks...)
		total += len(picks)
	}
	for key, picks := range payload.CDPTopMutePicks {
		cdp, parseErr := strconv.ParseInt(strings.TrimSpace(key), 10, 64)
		if parseErr != nil {
			return 0, fmt.Errorf("invalid top mute CDP key %q: %w", key, parseErr)
		}
		loadedTopMute[cdp] = append([]velocitywidgets.EnsemblePick(nil), picks...)
	}

	semblancePicksByCDP = loaded
	topMutePicksByCDP = loadedTopMute
	semblancePicksDirty = false
	topMutePicksDirty = false
	markStackDirtyFull()
	refreshStackPickMarkers()
	if displayedSemblanceGather != nil && semblanceImageView != nil {
		semblanceImageView.SetPicks(semblancePicksByCDP[displayedSemblanceGather.CDP])
		refreshCDPPreviewFromOriginalAsync()
		updateNMOIndicatorForCurrent()
	}
	if displayedCDPGather != nil && cdpImageView != nil {
		cdpImageView.SetPicks(topMutePicksByCDP[displayedCDPGather.CDP])
	}

	return total, nil
}

func saveLastPickURI(uri fyne.URI) {
	if uri == nil || fyne.CurrentApp() == nil {
		return
	}
	fyne.CurrentApp().Preferences().SetString(prefLastPickURI, uri.String())
}

func tryRestoreLastPickFile() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	uriStr := strings.TrimSpace(app.Preferences().String(prefLastPickURI))
	if uriStr == "" {
		return
	}

	uri, err := storage.ParseURI(uriStr)
	if err != nil {
		return
	}
	if strings.EqualFold(uri.Scheme(), "file") {
		if _, statErr := os.Stat(uri.Path()); statErr != nil {
			pushStatus("Last pick file not found; skipping pick restore")
			return
		}
	}

	reader, openErr := storage.Reader(uri)
	if openErr != nil {
		pushStatus(fmt.Sprintf("Could not open last pick file: %v", openErr))
		return
	}
	defer reader.Close()

	var (
		total     int
		importErr error
	)
	done := make(chan struct{})
	fyne.Do(func() {
		total, importErr = importSemblancePicksFromReader(reader)
		close(done)
	})
	<-done
	if importErr != nil {
		pushStatus(fmt.Sprintf("Could not restore picks: %v", importErr))
		return
	}
	pushStatus(fmt.Sprintf("Restored picks from %s (%d picks across %d CDPs)", uri.Name(), total, len(semblancePicksByCDP)))
}

func hasUnsavedSemblancePicks() bool {
	if !semblancePicksDirty {
		return topMutePicksDirty
	}
	return true
}

func snapshotCurrentSemblancePicks() {
	if displayedSemblanceGather == nil || semblanceImageView == nil {
		return
	}
	semblancePicksByCDP[displayedSemblanceGather.CDP] = semblanceImageView.Picks()
}

func snapshotCurrentCDPTopMutePicks() {
	if displayedCDPGather == nil || cdpImageView == nil {
		return
	}
	topMutePicksByCDP[displayedCDPGather.CDP] = cdpImageView.Picks()
}
