package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/hippodribble/seismic/segy"
	"github.com/hippodribble/seismic/segy/velocitywidgets"
)

var semblanceRenderSeq uint64
var stackRenderSeq uint64
var cdpImageView *velocitywidgets.EnsembleImageView
var semblanceImageView *velocitywidgets.EnsembleImageView
var stackImageView *velocitywidgets.EnsembleImageView
var displayedCDPGather *segy.CDPGather
var originalCDPGatherForPreview *segy.CDPGather
var displayedSemblanceGather *segy.SemblanceGather
var stackCDPAxis []int64
var lastCDPCursorStatus string
var lastSemblanceCursorStatus string
var lastStackCursorStatus string
var ensembleViewSyncState = velocitywidgets.NewEnsembleViewSync()
var semblancePicksByCDP = map[int64][]velocitywidgets.EnsemblePick{}
var topMutePicksByCDP = map[int64][]velocitywidgets.EnsemblePick{}
var lockedPicksByCDP = map[int64]bool{}
var semblancePicksDirty bool
var topMutePicksDirty bool
var cdpPreviewSeq uint64
var cdpNMOPreviewEnabled = true
var pickModeRequested bool
var muteModeRequested bool
var upperEnsembleSplit *container.Split
var mainEnsembleSplit *container.Split

func shouldAutoStackAfterPickEdit(previousCount, newCount int, dragCompleted bool) bool {
	if previousCount < 0 {
		previousCount = 0
	}
	if newCount < 0 {
		newCount = 0
	}
	if previousCount > newCount {
		return true
	}
	return dragCompleted
}

type stackTraceCacheEntry struct {
	trace []float32
	dtMS  float64
}

var stackTraceCacheByCDP = map[int64]stackTraceCacheEntry{}
var stackCacheCatalogCDPs []int64
var stackCacheProcessingSignature string
var stackCacheReady bool
var stackDirtyFull = true
var stackDirtyRangeSet bool
var stackDirtyRangeMin int64
var stackDirtyRangeMax int64
var stackBaseImage *image.Paletted
var stackBaseSampleIntervalMS float64
var stackBaseSampleCount int
var stackOverlayRefreshInProgress bool

func buildCentreView() *fyne.Container {
	cdpImageView = velocitywidgets.NewEnsembleImageView(velocitywidgets.EnsembleImageViewOptions{
		ShowTimeAxis:  false,
		AxisWidth:     0,
		ZoomFactor:    1.05,
		MinViewSpanMS: 20,
		MinSize:       fyne.NewSize(240, 220),
	})
	cdpImageView.SetPickOverlayEnabled(true)
	cdpImageView.SetHorizontalCursorGuideEnabled(true)

	semblanceImageView = velocitywidgets.NewEnsembleImageView(velocitywidgets.EnsembleImageViewOptions{
		ShowTimeAxis:  true,
		AxisWidth:     64,
		ZoomFactor:    1.05,
		MinViewSpanMS: 20,
		MinSize:       fyne.NewSize(240, 220),
	})
	semblanceImageView.SetPickStrokeWidth(3)
	semblanceImageView.SetPickOverlayEnabled(true)
	semblanceImageView.SetHorizontalCursorGuideEnabled(true)
	semblanceImageView.SetPickEditEnabled(readPickModePreference())
	stackImageView = velocitywidgets.NewEnsembleImageView(velocitywidgets.EnsembleImageViewOptions{
		ShowTimeAxis:  true,
		AxisWidth:     64,
		ZoomFactor:    1.05,
		MinViewSpanMS: 20,
		MinSize:       fyne.NewSize(240, 180),
	})
	stackImageView.SetPickOverlayEnabled(false)
	stackImageView.SetCursorGuideEnabled(true)
	stackImageView.SetHorizontalCursorGuideEnabled(true)
	pickModeRequested = readPickModePreference()
	muteModeRequested = readCDPMuteModePreference()
	cdpNMOPreviewEnabled = readCDPNMOPreviewPreference()
	ensembleImageView = semblanceImageView

	ensembleViewSyncState.Register(cdpImageView)
	ensembleViewSyncState.Register(semblanceImageView)
	ensembleViewSyncState.Register(stackImageView)

	cdpImageView.SetOnViewChanged(func(minMS, maxMS float64) {
		ensembleViewSyncState.SyncFrom(cdpImageView, minMS, maxMS)
	})

	semblanceImageView.SetOnViewChanged(func(minMS, maxMS float64) {
		ensembleViewSyncState.SyncFrom(semblanceImageView, minMS, maxMS)
		pushStatus(fmt.Sprintf("View %.0fms - %.0fms", minMS, maxMS))
	})

	stackImageView.SetOnViewChanged(func(minMS, maxMS float64) {
		ensembleViewSyncState.SyncFrom(stackImageView, minMS, maxMS)
	})

	cdpImageView.SetOnCursorMoved(func(info velocitywidgets.EnsembleCursorInfo) {
		syncCursorTimeGuides(info)
		handleCDPCursorMove(info)
	})
	cdpImageView.SetOnPicksChanged(func(picks []velocitywidgets.EnsemblePick) {
		cdp := currentCDPForPickState()
		if cdp <= 0 {
			return
		}
		previousCount := len(topMutePicksByCDP[cdp])
		topMutePicksByCDP[cdp] = append([]velocitywidgets.EnsemblePick(nil), picks...)
		topMutePicksDirty = true
		markStackDirtyRangeForCDP(cdp, "mute")
		refreshStackPickMarkers()
		refreshCDPPreviewFromOriginalAsync()
		updateNMOIndicatorForCurrent()
		pushStatus(fmt.Sprintf("CDP %d top mute picks: %d", cdp, len(picks)))
		if autoStackEnabled && shouldAutoStackAfterPickEdit(previousCount, len(picks), false) {
			pushStatus(fmt.Sprintf("Auto Stack: rebuilding after mute pick deletion at CDP %d", cdp))
			buildWholeLineStackAsync()
		}
	})
	cdpImageView.SetOnDragEnded(func() {
		if !autoStackEnabled {
			return
		}
		pushStatus("Auto Stack: rebuilding after drag release")
		buildWholeLineStackAsync()
	})
	semblanceImageView.SetOnCursorMoved(func(info velocitywidgets.EnsembleCursorInfo) {
		syncCursorTimeGuides(info)
		handleSemblanceCursorMove(info)
	})
	stackImageView.SetOnCursorMoved(func(info velocitywidgets.EnsembleCursorInfo) {
		syncCursorTimeGuides(info)
		handleStackCursorMove(info)
	})
	stackImageView.SetOnCursorTapped(func(info velocitywidgets.EnsembleCursorInfo) {
		handleStackCursorTap(info)
	})
	semblanceImageView.SetOnPicksChanged(func(picks []velocitywidgets.EnsemblePick) {
		if displayedSemblanceGather == nil {
			return
		}
		cdp := displayedSemblanceGather.CDP
		previousCount := len(semblancePicksByCDP[cdp])
		semblancePicksByCDP[cdp] = append([]velocitywidgets.EnsemblePick(nil), picks...)
		semblancePicksDirty = true
		markStackDirtyRangeForCDP(cdp, "velocity")
		refreshStackPickMarkers()
		refreshCDPPreviewFromOriginalAsync()
		updateNMOIndicatorForCurrent()
		pushStatus(fmt.Sprintf("CDP %d picks: %d", cdp, len(picks)))

		if autoStackEnabled && shouldAutoStackAfterPickEdit(previousCount, len(picks), false) {
			pushStatus(fmt.Sprintf("Auto Stack: rebuilding after velocity pick deletion at CDP %d", cdp))
			buildWholeLineStackAsync()
		}
	})
	semblanceImageView.SetOnDragEnded(func() {
		if !autoStackEnabled {
			return
		}
		pushStatus("Auto Stack: rebuilding after drag release")
		buildWholeLineStackAsync()
	})

	applyCDPMutePickEditState()

	right := container.NewBorder(widget.NewLabel("CDP"), nil, nil, nil, cdpImageView)
	left := container.NewBorder(widget.NewLabel("Semblance"), nil, nil, nil, semblanceImageView)
	upperSplit := container.NewHSplit(left, right)
	upperSplit.Offset = readEnsembleSplitOffsetPreference(prefEnsembleUpperSplit, 0.5)
	stackPanel := container.NewBorder(widget.NewLabel("Stack Section"), nil, nil, nil, stackImageView)
	mainSplit := container.NewHSplit(upperSplit, stackPanel)
	mainSplit.Offset = readEnsembleSplitOffsetPreference(prefEnsembleMainSplit, 0.67)
	upperEnsembleSplit = upperSplit
	mainEnsembleSplit = mainSplit

	hint := widget.NewLabel("Load a SEG-Y/SU file and select a CDP")
	return container.NewStack(container.NewBorder(hint, nil, nil, nil, mainSplit))
}

func persistEnsembleSplitPreferences() {
	if upperEnsembleSplit != nil {
		saveEnsembleSplitOffsetPreference(prefEnsembleUpperSplit, upperEnsembleSplit.Offset)
	}
	if mainEnsembleSplit != nil {
		saveEnsembleSplitOffsetPreference(prefEnsembleMainSplit, mainEnsembleSplit.Offset)
	}
}

func readEnsembleSplitOffsetPreference(key string, fallback float64) float64 {
	app := fyne.CurrentApp()
	if app == nil {
		return fallback
	}
	text := strings.TrimSpace(app.Preferences().StringWithFallback(key, ""))
	if text == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fallback
	}
	return clampSplitOffset(v)
}

func saveEnsembleSplitOffsetPreference(key string, value float64) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	app.Preferences().SetString(key, fmt.Sprintf("%.6f", clampSplitOffset(value)))
}

func clampSplitOffset(v float64) float64 {
	if v < 0.05 {
		return 0.05
	}
	if v > 0.95 {
		return 0.95
	}
	return v
}

func markStackDirtyFull() {
	stackDirtyFull = true
	stackDirtyRangeSet = false
}

func markStackDirtyRangeForCDP(cdp int64, kind string) {
	if cdp <= 0 {
		markStackDirtyFull()
		return
	}
	left, right, ok := stackNeighborRangeForCDP(cdp, kind)
	if !ok {
		markStackDirtyFull()
		return
	}
	if !stackDirtyRangeSet {
		stackDirtyRangeMin = left
		stackDirtyRangeMax = right
		stackDirtyRangeSet = true
	} else {
		if left < stackDirtyRangeMin {
			stackDirtyRangeMin = left
		}
		if right > stackDirtyRangeMax {
			stackDirtyRangeMax = right
		}
	}
}

func stackNeighborRangeForCDP(cdp int64, kind string) (int64, int64, bool) {
	var anchors []int64
	switch kind {
	case "velocity":
		anchors = stackExplicitVelocityAnchorCDPs(cdp)
	case "mute":
		anchors = stackExplicitMuteAnchorCDPs(cdp)
	default:
		return 0, 0, false
	}
	if len(anchors) < 2 {
		return 0, 0, false
	}
	sort.Slice(anchors, func(i, j int) bool { return anchors[i] < anchors[j] })
	left := int64(0)
	right := int64(0)
	hasLeft := false
	hasRight := false
	for _, a := range anchors {
		if a < cdp {
			left = a
			hasLeft = true
			continue
		}
		if a > cdp {
			right = a
			hasRight = true
			break
		}
	}
	if !hasLeft || !hasRight {
		return 0, 0, false
	}
	return left, right, true
}

func stackExplicitVelocityAnchorCDPs(excludeCDP int64) []int64 {
	anchors := make([]int64, 0, len(semblancePicksByCDP))
	for cdp, picks := range semblancePicksByCDP {
		if cdp == excludeCDP {
			continue
		}
		if len(sanitizeSemblancePicks(picks)) == 0 {
			continue
		}
		anchors = append(anchors, cdp)
	}
	return anchors
}

func stackExplicitMuteAnchorCDPs(excludeCDP int64) []int64 {
	anchors := make([]int64, 0, len(topMutePicksByCDP))
	for cdp, picks := range topMutePicksByCDP {
		if cdp == excludeCDP {
			continue
		}
		if len(sanitizeTopMutePicks(picks)) == 0 {
			continue
		}
		anchors = append(anchors, cdp)
	}
	return anchors
}

func sameInt64Slice(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func currentStackProcessingSignature() string {
	entryText := func(e *widget.Entry) string {
		if e == nil {
			return ""
		}
		return strings.TrimSpace(e.Text)
	}
	selected := func(s *widget.Select) string {
		if s == nil {
			return ""
		}
		return strings.TrimSpace(s.Selected)
	}
	return fmt.Sprintf(
		"hcAuto=%t|hc=%s|gain=%s|ts=%s|agcw=%s|agcf=%s|smooth=%s|stretch=%s",
		highCutAuto,
		entryText(highCutEntry),
		selected(gainModeSelect),
		entryText(tSquaredScaleEntry),
		entryText(agcWindowEntry),
		entryText(agcFloorEntry),
		entryText(smoothingEntry),
		entryText(semblanceStretchMuteEntry),
	)
}

func clearEnsembleImageView() {
	if cdpImageView == nil && semblanceImageView == nil {
		return
	}
	atomic.AddUint64(&semblanceRenderSeq, 1)
	atomic.AddUint64(&cdpPreviewSeq, 1)
	atomic.AddUint64(&stackRenderSeq, 1)
	fyne.Do(func() {
		displayedCDPGather = nil
		originalCDPGatherForPreview = nil
		displayedSemblanceGather = nil
		lastCDPCursorStatus = ""
		lastSemblanceCursorStatus = ""
		lastStackCursorStatus = ""
		stackCDPAxis = nil
		stackTraceCacheByCDP = map[int64]stackTraceCacheEntry{}
		stackCacheCatalogCDPs = nil
		stackCacheProcessingSignature = ""
		stackCacheReady = false
		markStackDirtyFull()
		setStackExportAvailable(false)
		if cdpImageView != nil {
			cdpImageView.Clear()
		}
		if semblanceImageView != nil {
			semblanceImageView.Clear()
		}
		if stackImageView != nil {
			stackImageView.Clear()
			stackImageView.SetVerticalMarkerFractions(nil, nil, nil)
			stackImageView.SetActiveVerticalMarkerFraction(math.NaN())
		}
		updateNMOIndicatorForCurrent()
	})
}

func updateEnsembleImageViewFromGather(gather *segy.CDPGather) {
	if cdpImageView == nil || semblanceImageView == nil {
		return
	}
	if gather == nil || len(gather.Traces) == 0 {
		clearEnsembleImageView()
		return
	}
	refreshHighCutDefaultForSampleInterval(gather.SampleIntervalMS)

	requestID := atomic.AddUint64(&semblanceRenderSeq, 1)
	gatherCopy := *gather
	originalGatherCopy := cloneCDPGatherDeep(gatherCopy)
	originalCDPGatherForPreview = &originalGatherCopy

	go func(id uint64, g segy.CDPGather) {
		cdpOffsetAxis, _, _ := gatherOffsetAxisAndRange(g)
		picksForCDP := effectiveSemblancePicksForCDP(g.CDP)
		topMuteForCDP := effectiveTopMutePicksForCDP(g.CDP, cdpOffsetAxis)
		stretchMute, err := selectedSemblanceStretchMuteFraction()
		if err != nil {
			if id == atomic.LoadUint64(&semblanceRenderSeq) {
				pushStatus(err.Error())
				clearEnsembleImageView()
			}
			return
		}

		highCutHz, err := selectedHighCutHz(g.SampleIntervalMS)
		if err != nil {
			if id == atomic.LoadUint64(&semblanceRenderSeq) {
				pushStatus(err.Error())
				clearEnsembleImageView()
			}
			return
		}

		processed := g
		nyquist := 500.0 / g.SampleIntervalMS
		if highCutHz > 0 && highCutHz < nyquist {
			filtered, err := segy.ApplyHighCutToCDPGather(g, highCutHz)
			if err != nil {
				if id == atomic.LoadUint64(&semblanceRenderSeq) {
					pushStatus(fmt.Sprintf("CDP filtering failed: %v", err))
					clearEnsembleImageView()
				}
				return
			}
			processed = filtered
		}

		gain, err := selectedGainOptions(processed.SampleIntervalMS)
		if err != nil {
			if id == atomic.LoadUint64(&semblanceRenderSeq) {
				pushStatus(err.Error())
				clearEnsembleImageView()
			}
			return
		}
		processed = applyGainToGather(processed, gain)

		smoothing, err := selectedSmoothingWindow()
		if err != nil {
			if id == atomic.LoadUint64(&semblanceRenderSeq) {
				pushStatus(err.Error())
				clearEnsembleImageView()
			}
			return
		}
		processed, err = applySmoothingToGather(processed, smoothing)
		if err != nil {
			if id == atomic.LoadUint64(&semblanceRenderSeq) {
				pushStatus(fmt.Sprintf("CDP smoothing failed: %v", err))
				clearEnsembleImageView()
			}
			return
		}

		cdpPreviewGather := processed
		if cdpNMOPreviewEnabled {
			cdpPreviewGather, err = applyNMOPicksToGather(processed, picksForCDP, stretchMute)
			if err != nil {
				if id == atomic.LoadUint64(&semblanceRenderSeq) {
					pushStatus(fmt.Sprintf("NMO preview skipped for CDP %d: %v", processed.CDP, err))
				}
				cdpPreviewGather = processed
			}
		}
		cdpPreviewGather = applyTopMutePicksToGather(cdpPreviewGather, topMuteForCDP)

		cdpImg, cdpSampleCount, cdpErr := renderCDPGatherImage(cdpPreviewGather)
		if cdpErr != nil {
			if id == atomic.LoadUint64(&semblanceRenderSeq) {
				pushStatus(fmt.Sprintf("CDP render failed: %v", cdpErr))
				clearEnsembleImageView()
			}
			return
		}

		vMin, vMax, vStep, err := selectedSemblanceVelocityParams()
		if err != nil {
			if id == atomic.LoadUint64(&semblanceRenderSeq) {
				pushStatus(err.Error())
				clearEnsembleImageView()
			}
			return
		}
		semblancePower, err := selectedSemblancePower()
		if err != nil {
			if id == atomic.LoadUint64(&semblanceRenderSeq) {
				pushStatus(err.Error())
				clearEnsembleImageView()
			}
			return
		}
		semblanceClip, err := selectedSemblancePercentileClip()
		if err != nil {
			if id == atomic.LoadUint64(&semblanceRenderSeq) {
				pushStatus(err.Error())
				clearEnsembleImageView()
			}
			return
		}
		semblanceInput := applyTopMutePicksToGather(processed, topMuteForCDP)

		pushStatus(fmt.Sprintf(
			"Computing semblance for CDP %d (smooth=%d, power=%.3f, clip=%.2f%%, stretch=%.3f) ...",
			processed.CDP,
			smoothing,
			semblancePower,
			semblanceClip,
			stretchMute,
		))

		sem, err := segy.BuildSemblanceGatherFromCDPWithSmoothingPowerAndStretchMute(semblanceInput, vMin, vMax, vStep, smoothing, semblancePower, stretchMute)
		if err != nil {
			if id != atomic.LoadUint64(&semblanceRenderSeq) {
				return
			}
			pushStatus(fmt.Sprintf("Semblance failed for CDP %d: %v", processed.CDP, err))
			clearEnsembleImageView()
			return
		}

		if len(sem.Traces) == 0 {
			if id != atomic.LoadUint64(&semblanceRenderSeq) {
				return
			}
			pushStatus(fmt.Sprintf("Semblance empty for CDP %d", g.CDP))
			clearEnsembleImageView()
			return
		}

		sampleCount := len(sem.Traces[0].Samples)
		if sampleCount <= 0 {
			if id != atomic.LoadUint64(&semblanceRenderSeq) {
				return
			}
			pushStatus(fmt.Sprintf("Semblance sample count is zero for CDP %d", g.CDP))
			clearEnsembleImageView()
			return
		}

		traces := make([][]float32, len(sem.Traces))
		for i := range sem.Traces {
			traces[i] = sem.Traces[i].Samples
		}

		img, _, err := segy.RenderEnsembleImage(segy.EnsembleSamples{
			Traces:            traces,
			TraceCount:        len(traces),
			SamplesPerTrace:   sampleCount,
			TotalImageSamples: int64(len(traces)) * int64(sampleCount),
		}, segy.EnsembleImageOptions{
			Palette:        selectedSemblancePalette(),
			PercentileClip: semblanceClip,
			ZeroCentered:   false,
		})
		if err != nil {
			if id != atomic.LoadUint64(&semblanceRenderSeq) {
				return
			}
			pushStatus(fmt.Sprintf("Semblance render failed for CDP %d: %v", g.CDP, err))
			clearEnsembleImageView()
			return
		}

		if id != atomic.LoadUint64(&semblanceRenderSeq) {
			return
		}

		semblanceDTMS := sem.SampleIntervalMS
		if semblanceDTMS <= 0 {
			semblanceDTMS = 1
		}

		semblanceVMin, semblanceVMax := semblanceVelocityRange(sem)
		semblanceVelAxis := sem.VelocityAxis()
		cdpOffsetAxis, cdpOffsetMin, cdpOffsetMax := gatherOffsetAxisAndRange(cdpPreviewGather)
		effectiveSemblancePicks := effectiveSemblancePicksForCDP(sem.CDP)
		effectiveTopMutePicks := effectiveTopMutePicksForCDP(sem.CDP, cdpOffsetAxis)

		cdpDTMS := processed.SampleIntervalMS
		if cdpDTMS <= 0 {
			cdpDTMS = 1
		}

		fyne.Do(func() {
			cdpCopy := cdpPreviewGather
			displayedCDPGather = &cdpCopy
			semCopy := sem
			displayedSemblanceGather = &semCopy
			lastCDPCursorStatus = ""
			lastSemblanceCursorStatus = ""
			cdpImageView.SetImage(cdpImg, cdpDTMS, cdpSampleCount)
			cdpImageView.SetPickVelocityRange(cdpOffsetMin, cdpOffsetMax)
			cdpImageView.SetPickVelocityAxis(cdpOffsetAxis)
			cdpImageView.SetPicks(effectiveTopMutePicks)
			semblanceImageView.SetImage(img, semblanceDTMS, sampleCount)
			semblanceImageView.SetPickVelocityRange(semblanceVMin, semblanceVMax)
			semblanceImageView.SetPickVelocityAxis(semblanceVelAxis)
			semblanceImageView.SetPicks(effectiveSemblancePicks)
			syncPickLockCheck(lockedPicksByCDP[sem.CDP])
			applySemblancePickEditState()
			applyCDPMutePickEditState()
			refreshStackPickMarkers()
			updateNMOIndicatorForCurrent()
			ensembleViewSyncState.ApplyCurrent(cdpImageView)
			ensembleViewSyncState.ApplyCurrent(semblanceImageView)
		})
		pushStatus(fmt.Sprintf("Displayed CDP+semblance for CDP %d", processed.CDP))
	}(requestID, gatherCopy)
}

func refreshCDPPreviewFromOriginalAsync() {
	if cdpImageView == nil || originalCDPGatherForPreview == nil {
		return
	}

	base := cloneCDPGatherDeep(*originalCDPGatherForPreview)
	baseOffsetAxis, _, _ := gatherOffsetAxisAndRange(base)
	picks := effectiveSemblancePicksForCDP(base.CDP)
	mutePicks := effectiveTopMutePicksForCDP(base.CDP, baseOffsetAxis)
	requestID := atomic.AddUint64(&cdpPreviewSeq, 1)

	go func(id uint64, gather segy.CDPGather, pickList []velocitywidgets.EnsemblePick, topMuteList []velocitywidgets.EnsemblePick) {
		stretchMute, err := selectedSemblanceStretchMuteFraction()
		if err != nil {
			if id == atomic.LoadUint64(&cdpPreviewSeq) {
				pushStatus(err.Error())
			}
			return
		}

		processed, err := applyDisplayProcessingToCDPGather(gather)
		if err != nil {
			if id == atomic.LoadUint64(&cdpPreviewSeq) {
				pushStatus(fmt.Sprintf("CDP preview processing failed: %v", err))
			}
			return
		}

		preview := processed
		if cdpNMOPreviewEnabled {
			preview, err = applyNMOPicksToGather(processed, pickList, stretchMute)
			if err != nil {
				if id == atomic.LoadUint64(&cdpPreviewSeq) {
					pushStatus(fmt.Sprintf("NMO preview skipped for CDP %d: %v", gather.CDP, err))
				}
				preview = processed
			}
		}
		preview = applyTopMutePicksToGather(preview, topMuteList)

		img, sampleCount, err := renderCDPGatherImage(preview)
		if err != nil {
			if id == atomic.LoadUint64(&cdpPreviewSeq) {
				pushStatus(fmt.Sprintf("CDP preview render failed: %v", err))
			}
			return
		}

		dtMS := preview.SampleIntervalMS
		if dtMS <= 0 {
			dtMS = 1
		}

		if id != atomic.LoadUint64(&cdpPreviewSeq) {
			return
		}

		fyne.Do(func() {
			cdpCopy := preview
			displayedCDPGather = &cdpCopy
			cdpImageView.SetImage(img, dtMS, sampleCount)
			offsetAxis, minOffset, maxOffset := gatherOffsetAxisAndRange(preview)
			effectiveTopMute := effectiveTopMutePicksForCDP(cdpCopy.CDP, offsetAxis)
			cdpImageView.SetPickVelocityRange(minOffset, maxOffset)
			cdpImageView.SetPickVelocityAxis(offsetAxis)
			cdpImageView.SetPicks(effectiveTopMute)
			applyCDPMutePickEditState()
			updateNMOIndicatorForCurrent()
			ensembleViewSyncState.ApplyCurrent(cdpImageView)
		})
	}(requestID, base, picks, mutePicks)
}

func effectiveSemblancePicksForCDP(cdp int64) []velocitywidgets.EnsemblePick {
	explicit := sanitizeSemblancePicks(semblancePicksByCDP[cdp])
	if len(explicit) > 0 {
		return explicit
	}

	leftCDP, left, rightCDP, right, ok := bracketSemblanceAnchors(cdp)
	if !ok {
		return nil
	}
	if leftCDP == rightCDP {
		return append([]velocitywidgets.EnsemblePick(nil), left...)
	}

	alpha := float64(cdp-leftCDP) / float64(rightCDP-leftCDP)
	times := mergedSortedTimeNodes(left, right)
	out := make([]velocitywidgets.EnsemblePick, 0, len(times))
	for _, t := range times {
		vL := semblanceVelocityAtTime(left, t)
		vR := semblanceVelocityAtTime(right, t)
		v := vL + alpha*(vR-vL)
		if !isFinite(v) || v <= 0 {
			continue
		}
		out = append(out, velocitywidgets.EnsemblePick{TimeMS: t, Velocity: v})
	}
	return out
}

func effectiveTopMutePicksForCDP(cdp int64, targetOffsets []float64) []velocitywidgets.EnsemblePick {
	explicit := sanitizeTopMutePicks(topMutePicksByCDP[cdp])
	if len(explicit) > 0 {
		return explicit
	}

	leftCDP, left, rightCDP, right, ok := bracketTopMuteAnchors(cdp)
	if !ok {
		return nil
	}
	if leftCDP == rightCDP {
		return append([]velocitywidgets.EnsemblePick(nil), left...)
	}

	offsets := uniqueSortedFiniteFloat64(targetOffsets)
	if len(offsets) == 0 {
		offsets = mergedSortedOffsetNodes(left, right)
	}
	if len(offsets) == 0 {
		return nil
	}

	alpha := float64(cdp-leftCDP) / float64(rightCDP-leftCDP)
	out := make([]velocitywidgets.EnsemblePick, 0, len(offsets))
	for _, off := range offsets {
		tL := topMuteTimeAtOffset(left, off)
		tR := topMuteTimeAtOffset(right, off)
		t := tL + alpha*(tR-tL)
		if !isFinite(t) || t <= 0 {
			continue
		}
		out = append(out, velocitywidgets.EnsemblePick{Velocity: off, TimeMS: t})
	}
	return out
}

func bracketSemblanceAnchors(cdp int64) (int64, []velocitywidgets.EnsemblePick, int64, []velocitywidgets.EnsemblePick, bool) {
	anchors := make([]int64, 0, len(semblancePicksByCDP))
	curves := make(map[int64][]velocitywidgets.EnsemblePick, len(semblancePicksByCDP))
	for anchorCDP, picks := range semblancePicksByCDP {
		clean := sanitizeSemblancePicks(picks)
		if len(clean) == 0 {
			continue
		}
		anchors = append(anchors, anchorCDP)
		curves[anchorCDP] = clean
	}
	if len(anchors) == 0 {
		return 0, nil, 0, nil, false
	}
	sort.Slice(anchors, func(i, j int) bool { return anchors[i] < anchors[j] })
	if cdp <= anchors[0] {
		k := anchors[0]
		return k, curves[k], k, curves[k], true
	}
	last := anchors[len(anchors)-1]
	if cdp >= last {
		return last, curves[last], last, curves[last], true
	}
	for i := 0; i < len(anchors)-1; i++ {
		left := anchors[i]
		right := anchors[i+1]
		if cdp >= left && cdp <= right {
			return left, curves[left], right, curves[right], true
		}
	}
	return 0, nil, 0, nil, false
}

func bracketTopMuteAnchors(cdp int64) (int64, []velocitywidgets.EnsemblePick, int64, []velocitywidgets.EnsemblePick, bool) {
	anchors := make([]int64, 0, len(topMutePicksByCDP))
	curves := make(map[int64][]velocitywidgets.EnsemblePick, len(topMutePicksByCDP))
	for anchorCDP, picks := range topMutePicksByCDP {
		clean := sanitizeTopMutePicks(picks)
		if len(clean) == 0 {
			continue
		}
		anchors = append(anchors, anchorCDP)
		curves[anchorCDP] = clean
	}
	if len(anchors) == 0 {
		return 0, nil, 0, nil, false
	}
	sort.Slice(anchors, func(i, j int) bool { return anchors[i] < anchors[j] })
	if cdp <= anchors[0] {
		k := anchors[0]
		return k, curves[k], k, curves[k], true
	}
	last := anchors[len(anchors)-1]
	if cdp >= last {
		return last, curves[last], last, curves[last], true
	}
	for i := 0; i < len(anchors)-1; i++ {
		left := anchors[i]
		right := anchors[i+1]
		if cdp >= left && cdp <= right {
			return left, curves[left], right, curves[right], true
		}
	}
	return 0, nil, 0, nil, false
}

func sanitizeSemblancePicks(picks []velocitywidgets.EnsemblePick) []velocitywidgets.EnsemblePick {
	clean := make([]velocitywidgets.EnsemblePick, 0, len(picks))
	for _, p := range picks {
		if !isFinite(p.TimeMS) || !isFinite(p.Velocity) {
			continue
		}
		if p.Velocity <= 0 {
			continue
		}
		clean = append(clean, p)
	}
	sort.Slice(clean, func(i, j int) bool { return clean[i].TimeMS < clean[j].TimeMS })
	return clean
}

func sanitizeTopMutePicks(picks []velocitywidgets.EnsemblePick) []velocitywidgets.EnsemblePick {
	clean := make([]velocitywidgets.EnsemblePick, 0, len(picks))
	for _, p := range picks {
		if !isFinite(p.TimeMS) || !isFinite(p.Velocity) {
			continue
		}
		if p.TimeMS <= 0 {
			continue
		}
		clean = append(clean, p)
	}
	sort.Slice(clean, func(i, j int) bool { return clean[i].Velocity < clean[j].Velocity })
	return clean
}

func semblanceVelocityAtTime(picks []velocitywidgets.EnsemblePick, timeMS float64) float64 {
	if len(picks) == 0 {
		return 0
	}
	if len(picks) == 1 || timeMS <= picks[0].TimeMS {
		return picks[0].Velocity
	}
	last := len(picks) - 1
	if timeMS >= picks[last].TimeMS {
		return picks[last].Velocity
	}
	for i := 0; i < last; i++ {
		a := picks[i]
		b := picks[i+1]
		if timeMS < a.TimeMS || timeMS > b.TimeMS {
			continue
		}
		dt := b.TimeMS - a.TimeMS
		if dt == 0 {
			return b.Velocity
		}
		w := (timeMS - a.TimeMS) / dt
		return a.Velocity + w*(b.Velocity-a.Velocity)
	}
	return picks[last].Velocity
}

func topMuteTimeAtOffset(picks []velocitywidgets.EnsemblePick, offset float64) float64 {
	if len(picks) == 0 {
		return 0
	}
	if len(picks) == 1 || offset <= picks[0].Velocity {
		return picks[0].TimeMS
	}
	last := len(picks) - 1
	if offset >= picks[last].Velocity {
		return picks[last].TimeMS
	}
	for i := 0; i < last; i++ {
		a := picks[i]
		b := picks[i+1]
		if offset < a.Velocity || offset > b.Velocity {
			continue
		}
		dx := b.Velocity - a.Velocity
		if dx == 0 {
			return b.TimeMS
		}
		w := (offset - a.Velocity) / dx
		return a.TimeMS + w*(b.TimeMS-a.TimeMS)
	}
	return picks[last].TimeMS
}

func mergedSortedTimeNodes(a, b []velocitywidgets.EnsemblePick) []float64 {
	vals := make([]float64, 0, len(a)+len(b))
	for _, p := range a {
		if isFinite(p.TimeMS) {
			vals = append(vals, p.TimeMS)
		}
	}
	for _, p := range b {
		if isFinite(p.TimeMS) {
			vals = append(vals, p.TimeMS)
		}
	}
	return uniqueSortedFiniteFloat64(vals)
}

func mergedSortedOffsetNodes(a, b []velocitywidgets.EnsemblePick) []float64 {
	vals := make([]float64, 0, len(a)+len(b))
	for _, p := range a {
		if isFinite(p.Velocity) {
			vals = append(vals, p.Velocity)
		}
	}
	for _, p := range b {
		if isFinite(p.Velocity) {
			vals = append(vals, p.Velocity)
		}
	}
	return uniqueSortedFiniteFloat64(vals)
}

func uniqueSortedFiniteFloat64(in []float64) []float64 {
	if len(in) == 0 {
		return nil
	}
	vals := make([]float64, 0, len(in))
	for _, v := range in {
		if isFinite(v) {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return nil
	}
	sort.Float64s(vals)
	const eps = 1e-9
	out := make([]float64, 0, len(vals))
	for _, v := range vals {
		if len(out) == 0 || math.Abs(v-out[len(out)-1]) > eps {
			out = append(out, v)
		}
	}
	return out
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func applyDisplayProcessingToCDPGather(g segy.CDPGather) (segy.CDPGather, error) {
	highCutHz, err := selectedHighCutHz(g.SampleIntervalMS)
	if err != nil {
		return segy.CDPGather{}, err
	}

	processed := g
	nyquist := 500.0 / g.SampleIntervalMS
	if highCutHz > 0 && highCutHz < nyquist {
		filtered, err := segy.ApplyHighCutToCDPGather(g, highCutHz)
		if err != nil {
			return segy.CDPGather{}, err
		}
		processed = filtered
	}

	gain, err := selectedGainOptions(processed.SampleIntervalMS)
	if err != nil {
		return segy.CDPGather{}, err
	}
	processed = applyGainToGather(processed, gain)

	smoothing, err := selectedSmoothingWindow()
	if err != nil {
		return segy.CDPGather{}, err
	}
	processed, err = applySmoothingToGather(processed, smoothing)
	if err != nil {
		return segy.CDPGather{}, err
	}

	return processed, nil
}

func applyNMOPicksToGather(gather segy.CDPGather, picks []velocitywidgets.EnsemblePick, maxStretch float64) (segy.CDPGather, error) {
	if len(picks) == 0 {
		return gather, nil
	}

	samples := make([]segy.VelocitySample, 0, len(picks))
	for _, p := range picks {
		if p.Velocity <= 0 {
			continue
		}
		samples = append(samples, segy.VelocitySample{
			TimeMS:           p.TimeMS,
			StackingVelocity: p.Velocity,
		})
	}
	if len(samples) == 0 {
		return gather, fmt.Errorf("velocity picks are empty or invalid")
	}

	sort.Slice(samples, func(i, j int) bool { return samples[i].TimeMS < samples[j].TimeMS })
	velocity := segy.VelocityFunction{CDP: gather.CDP, Samples: samples}

	return segy.ApplyNMOToCDPGatherWithOptions(gather, velocity, segy.NMOOptions{
		ExtrapolationMode:  segy.VelocityExtrapolateHold,
		MaxStretchFraction: maxStretch,
	})
}

func gatherOffsetAxisAndRange(gather segy.CDPGather) ([]float64, float64, float64) {
	axis := make([]float64, len(gather.Traces))
	if len(gather.Traces) == 0 {
		return axis, 0, 0
	}
	minOffset := float64(gather.Traces[0].Header.SourceReceiverOffset)
	maxOffset := minOffset
	for i := range gather.Traces {
		off := float64(gather.Traces[i].Header.SourceReceiverOffset)
		axis[i] = off
		if off < minOffset {
			minOffset = off
		}
		if off > maxOffset {
			maxOffset = off
		}
	}
	return axis, minOffset, maxOffset
}

func applyTopMutePicksToGather(gather segy.CDPGather, picks []velocitywidgets.EnsemblePick) segy.CDPGather {
	if len(picks) == 0 || len(gather.Traces) == 0 || gather.SampleIntervalMS <= 0 {
		return gather
	}

	type muteSample struct {
		offset float64
		timeMS float64
	}

	curve := make([]muteSample, 0, len(picks))
	for _, p := range picks {
		if p.TimeMS <= 0 {
			continue
		}
		curve = append(curve, muteSample{offset: p.Velocity, timeMS: p.TimeMS})
	}
	if len(curve) == 0 {
		return gather
	}
	sort.Slice(curve, func(i, j int) bool { return curve[i].offset < curve[j].offset })

	out := cloneCDPGatherDeep(gather)

	topMuteAtOffset := func(offset float64) float64 {
		if offset <= curve[0].offset {
			return curve[0].timeMS
		}
		last := len(curve) - 1
		if offset >= curve[last].offset {
			return curve[last].timeMS
		}
		for i := 0; i < last; i++ {
			a := curve[i]
			b := curve[i+1]
			if offset < a.offset || offset > b.offset {
				continue
			}
			dx := b.offset - a.offset
			if dx == 0 {
				return a.timeMS
			}
			w := (offset - a.offset) / dx
			return a.timeMS + w*(b.timeMS-a.timeMS)
		}
		return curve[last].timeMS
	}

	for i := range out.Traces {
		offset := float64(out.Traces[i].Header.SourceReceiverOffset)
		muteMS := topMuteAtOffset(offset)
		if muteMS <= 0 {
			continue
		}
		muteSampleIndex := int(math.Ceil(muteMS / out.SampleIntervalMS))
		if muteSampleIndex < 0 {
			muteSampleIndex = 0
		}
		if muteSampleIndex > len(out.Traces[i].Samples) {
			muteSampleIndex = len(out.Traces[i].Samples)
		}
		for j := 0; j < muteSampleIndex; j++ {
			out.Traces[i].Samples[j] = 0
		}
	}

	return out
}

func cloneCDPGatherDeep(in segy.CDPGather) segy.CDPGather {
	out := segy.CDPGather{
		CDP:              in.CDP,
		SampleIntervalMS: in.SampleIntervalMS,
		Traces:           make([]segy.GatherTrace, len(in.Traces)),
	}
	for i := range in.Traces {
		out.Traces[i].Header = in.Traces[i].Header
		out.Traces[i].Samples = append([]float32(nil), in.Traces[i].Samples...)
	}
	return out
}

func handleCDPCursorMove(info velocitywidgets.EnsembleCursorInfo) {
	if !info.Inside {
		lastCDPCursorStatus = ""
		return
	}
	if displayedCDPGather == nil || len(displayedCDPGather.Traces) == 0 {
		return
	}

	traceIdx := fractionToIndex(info.XFrac, len(displayedCDPGather.Traces))
	offset := displayedCDPGather.Traces[traceIdx].Header.SourceReceiverOffset
	msg := fmt.Sprintf("CDP %d  offset=%d  time=%.1f ms", displayedCDPGather.CDP, offset, info.TimeMS)
	if msg == lastCDPCursorStatus {
		return
	}
	lastCDPCursorStatus = msg
	pushStatus(msg)
}

func handleSemblanceCursorMove(info velocitywidgets.EnsembleCursorInfo) {
	if !info.Inside {
		lastSemblanceCursorStatus = ""
		return
	}
	if displayedSemblanceGather == nil || len(displayedSemblanceGather.Traces) == 0 {
		return
	}

	traceIdx := fractionToIndex(info.XFrac, len(displayedSemblanceGather.Traces))
	velocity := displayedSemblanceGather.Traces[traceIdx].Velocity
	msg := fmt.Sprintf("CDP %d  velocity=%.1f m/s  time=%.1f ms", displayedSemblanceGather.CDP, velocity, info.TimeMS)
	if msg == lastSemblanceCursorStatus {
		return
	}
	lastSemblanceCursorStatus = msg
	pushStatus(msg)
}

func handleStackCursorMove(info velocitywidgets.EnsembleCursorInfo) {
	if !info.Inside {
		lastStackCursorStatus = ""
		return
	}
	if len(stackCDPAxis) == 0 {
		return
	}

	idx := fractionToIndex(info.XFrac, len(stackCDPAxis))
	cdp := stackCDPAxis[idx]
	marker := stackMarkerKindForCDP(cdp)
	if marker != "" {
		msg := fmt.Sprintf("Stack CDP %d (%s)  time=%.1f ms", cdp, marker, info.TimeMS)
		if msg == lastStackCursorStatus {
			return
		}
		lastStackCursorStatus = msg
		pushStatus(msg)
		return
	}
	msg := fmt.Sprintf("Stack CDP %d  time=%.1f ms", cdp, info.TimeMS)
	if msg == lastStackCursorStatus {
		return
	}
	lastStackCursorStatus = msg
	pushStatus(msg)
}

func handleStackCursorTap(info velocitywidgets.EnsembleCursorInfo) {
	if !info.Inside || len(stackCDPAxis) == 0 || cdpSelector == nil {
		return
	}

	if pickedCDP, marker, ok := nearestMarkedStackCDP(info.XFrac, 0.02); ok {
		if err := cdpSelector.Select(int(pickedCDP)); err != nil {
			pushStatus(fmt.Sprintf("Stack select failed for CDP %d: %v", pickedCDP, err))
			return
		}
		pushStatus(fmt.Sprintf("Selected CDP %d from stack marker (%s)", pickedCDP, marker))
		return
	}

	idx := fractionToIndex(info.XFrac, len(stackCDPAxis))
	cdp := int(stackCDPAxis[idx])
	if err := cdpSelector.Select(cdp); err != nil {
		pushStatus(fmt.Sprintf("Stack select failed for CDP %d: %v", cdp, err))
		return
	}
	pushStatus(fmt.Sprintf("Selected CDP %d from stack", cdp))
}

func refreshStackPickMarkers() {
	applyStackPickMarkers()
	if stackOverlayRefreshInProgress {
		return
	}
	refreshStackVelocityOverlay()
}

func applyStackPickMarkers() {
	if stackImageView == nil || len(stackCDPAxis) == 0 {
		if stackImageView != nil {
			stackImageView.SetVerticalMarkerFractions(nil, nil, nil)
			stackImageView.SetActiveVerticalMarkerFraction(math.NaN())
		}
		return
	}

	axisLen := len(stackCDPAxis)
	if axisLen == 0 {
		stackImageView.SetVerticalMarkerFractions(nil, nil, nil)
		stackImageView.SetActiveVerticalMarkerFraction(math.NaN())
		return
	}

	velocityFractions := make([]float64, 0)
	muteFractions := make([]float64, 0)
	bothFractions := make([]float64, 0)

	for i, cdp := range stackCDPAxis {
		hasVel := len(semblancePicksByCDP[cdp]) > 0
		hasMute := len(topMutePicksByCDP[cdp]) > 0
		if !hasVel && !hasMute {
			continue
		}

		frac := 0.0
		if axisLen > 1 {
			frac = float64(i) / float64(axisLen-1)
		}
		switch {
		case hasVel && hasMute:
			bothFractions = append(bothFractions, frac)
		case hasVel:
			velocityFractions = append(velocityFractions, frac)
		case hasMute:
			muteFractions = append(muteFractions, frac)
		}
	}

	stackImageView.SetVerticalMarkerFractions(velocityFractions, muteFractions, bothFractions)
	activeCDP := currentCDPForPickState()
	if frac, ok := stackFractionForCDP(activeCDP); ok {
		stackImageView.SetActiveVerticalMarkerFraction(frac)
	} else {
		stackImageView.SetActiveVerticalMarkerFraction(math.NaN())
	}
}

func refreshStackVelocityOverlay() {
	if stackImageView == nil || stackBaseImage == nil || stackBaseSampleCount <= 0 || stackBaseSampleIntervalMS <= 0 {
		return
	}
	if !stackOverlayEnabled() {
		stackOverlayRefreshInProgress = true
		fyne.Do(func() {
			stackImageView.SetImage(stackBaseImage, stackBaseSampleIntervalMS, stackBaseSampleCount)
			applyStackPickMarkers()
			ensembleViewSyncState.ApplyCurrent(stackImageView)
		})
		stackOverlayRefreshInProgress = false
		return
	}

	opacity := stackOverlayOpacityFraction()
	base := stackBaseImage
	if opacity <= 0 || len(stackCDPAxis) == 0 {
		stackOverlayRefreshInProgress = true
		fyne.Do(func() {
			stackImageView.SetImage(base, stackBaseSampleIntervalMS, stackBaseSampleCount)
			applyStackPickMarkers()
			ensembleViewSyncState.ApplyCurrent(stackImageView)
		})
		stackOverlayRefreshInProgress = false
		return
	}

	overlayTraces := make([][]float32, len(stackCDPAxis))
	hasOverlay := false
	for i, cdp := range stackCDPAxis {
		picks := effectiveSemblancePicksForCDP(cdp)
		trace := make([]float32, stackBaseSampleCount)
		for sample := 0; sample < stackBaseSampleCount; sample++ {
			velocity := semblanceVelocityAtTime(picks, float64(sample)*stackBaseSampleIntervalMS)
			if !isFiniteFloat64(velocity) || velocity <= 0 {
				trace[sample] = float32(math.NaN())
				continue
			}
			trace[sample] = float32(velocity)
			hasOverlay = true
		}
		overlayTraces[i] = trace
	}
	if !hasOverlay {
		stackOverlayRefreshInProgress = true
		fyne.Do(func() {
			stackImageView.SetImage(base, stackBaseSampleIntervalMS, stackBaseSampleCount)
			applyStackPickMarkers()
			ensembleViewSyncState.ApplyCurrent(stackImageView)
		})
		stackOverlayRefreshInProgress = false
		return
	}

	overlayImg, _, err := segy.RenderEnsembleImage(segy.EnsembleSamples{
		Traces:            overlayTraces,
		TraceCount:        len(overlayTraces),
		SamplesPerTrace:   stackBaseSampleCount,
		TotalImageSamples: int64(len(overlayTraces)) * int64(stackBaseSampleCount),
	}, segy.EnsembleImageOptions{
		Palette:        blueWhiteRedPalette(false),
		PercentileClip: 2,
		ZeroCentered:   false,
	})
	if err != nil || overlayImg == nil {
		stackOverlayRefreshInProgress = true
		fyne.Do(func() {
			stackImageView.SetImage(base, stackBaseSampleIntervalMS, stackBaseSampleCount)
			applyStackPickMarkers()
			ensembleViewSyncState.ApplyCurrent(stackImageView)
		})
		stackOverlayRefreshInProgress = false
		return
	}

	composited := compositeStackVelocityOverlay(base, overlayImg, overlayTraces, opacity)
	stackOverlayRefreshInProgress = true
	fyne.Do(func() {
		stackImageView.SetImage(composited, stackBaseSampleIntervalMS, stackBaseSampleCount)
		applyStackPickMarkers()
		ensembleViewSyncState.ApplyCurrent(stackImageView)
	})
	stackOverlayRefreshInProgress = false
}

func stackOverlayOpacityFraction() float64 {
	if stackOverlayOpacitySlider == nil {
		return 0
	}
	if stackOverlayOpacitySlider.Value <= 0 {
		return 0
	}
	if stackOverlayOpacitySlider.Value >= 60 {
		return 1
	}
	return stackOverlayOpacitySlider.Value / 60.0
}

func stackOverlayEnabled() bool {
	return stackOverlayEnabledCheck != nil && stackOverlayEnabledCheck.Checked
}

func compositeStackVelocityOverlay(base, overlay *image.Paletted, overlayTraces [][]float32, opacity float64) *image.NRGBA {
	bounds := base.Bounds()
	out := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			baseColor := color.NRGBAModel.Convert(base.At(x, y)).(color.NRGBA)
			trace := overlayTraces[x-bounds.Min.X]
			v := trace[y-bounds.Min.Y]
			if !isFiniteFloat64(float64(v)) {
				out.SetNRGBA(x, y, baseColor)
				continue
			}
			overlayColor := color.NRGBAModel.Convert(overlay.At(x, y)).(color.NRGBA)
			out.SetNRGBA(x, y, blendNRGBA(baseColor, overlayColor, opacity))
		}
	}
	return out
}

func blendNRGBA(base, overlay color.NRGBA, opacity float64) color.NRGBA {
	if opacity <= 0 {
		return base
	}
	if opacity > 1 {
		opacity = 1
	}
	alpha := opacity * (float64(overlay.A) / 255.0)
	if alpha <= 0 {
		return base
	}
	inv := 1.0 - alpha
	return color.NRGBA{
		R: uint8(math.Round(float64(base.R)*inv + float64(overlay.R)*alpha)),
		G: uint8(math.Round(float64(base.G)*inv + float64(overlay.G)*alpha)),
		B: uint8(math.Round(float64(base.B)*inv + float64(overlay.B)*alpha)),
		A: 255,
	}
}

func isFiniteFloat64(v float64) bool {
	f := v
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func stackFractionForCDP(cdp int64) (float64, bool) {
	if cdp <= 0 || len(stackCDPAxis) == 0 {
		return 0, false
	}
	if len(stackCDPAxis) == 1 {
		if stackCDPAxis[0] == cdp {
			return 0, true
		}
		return 0, false
	}
	for i, axisCDP := range stackCDPAxis {
		if axisCDP == cdp {
			return float64(i) / float64(len(stackCDPAxis)-1), true
		}
	}
	return 0, false
}

func stackMarkerKindForCDP(cdp int64) string {
	hasVel := len(semblancePicksByCDP[cdp]) > 0
	hasMute := len(topMutePicksByCDP[cdp]) > 0
	switch {
	case hasVel && hasMute:
		return "v+m"
	case hasVel:
		return "v"
	case hasMute:
		return "mute"
	default:
		return ""
	}
}

func nearestMarkedStackCDP(xFrac, maxDist float64) (int64, string, bool) {
	if len(stackCDPAxis) == 0 {
		return 0, "", false
	}
	if xFrac < 0 {
		xFrac = 0
	}
	if xFrac > 1 {
		xFrac = 1
	}
	if maxDist <= 0 {
		maxDist = 0.02
	}

	bestDist := maxDist
	bestCDP := int64(0)
	bestKind := ""
	for i, cdp := range stackCDPAxis {
		kind := stackMarkerKindForCDP(cdp)
		if kind == "" {
			continue
		}
		frac := 0.0
		if len(stackCDPAxis) > 1 {
			frac = float64(i) / float64(len(stackCDPAxis)-1)
		}
		d := math.Abs(frac - xFrac)
		if d > bestDist {
			continue
		}
		bestDist = d
		bestCDP = cdp
		bestKind = kind
	}

	if bestKind == "" {
		return 0, "", false
	}
	return bestCDP, bestKind, true
}

func syncCursorTimeGuides(info velocitywidgets.EnsembleCursorInfo) {
	if cdpImageView == nil || semblanceImageView == nil || stackImageView == nil {
		return
	}
	if !info.Inside {
		cdpImageView.ClearCursorGuideTime()
		semblanceImageView.ClearCursorGuideTime()
		stackImageView.ClearCursorGuideTime()
		return
	}
	cdpImageView.SetCursorGuideTimeMS(info.TimeMS)
	semblanceImageView.SetCursorGuideTimeMS(info.TimeMS)
	stackImageView.SetCursorGuideTimeMS(info.TimeMS)
}

func fractionToIndex(frac float64, n int) int {
	if n <= 1 {
		return 0
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	idx := int(math.Round(frac * float64(n-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return idx
}

func semblanceVelocityRange(gather segy.SemblanceGather) (float64, float64) {
	if len(gather.Traces) == 0 {
		return 0, 0
	}
	minV := gather.Traces[0].Velocity
	maxV := gather.Traces[0].Velocity
	for i := 1; i < len(gather.Traces); i++ {
		v := gather.Traces[i].Velocity
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return minV, maxV
}

func setSemblancePickEditMode(enabled bool) {
	pickModeRequested = enabled
	applySemblancePickEditState()
	updateNMOIndicatorForCurrent()
	if enabled {
		pushStatus("Semblance pick mode enabled")
		return
	}
	pushStatus("Semblance pick mode disabled")
}

func setCDPMutePickMode(enabled bool) {
	muteModeRequested = enabled
	applyCDPMutePickEditState()
	updateNMOIndicatorForCurrent()
	if enabled {
		pushStatus("CDP top mute mode enabled")
		return
	}
	pushStatus("CDP top mute mode disabled")
}

func setCDPNMOPreviewEnabled(enabled bool) {
	cdpNMOPreviewEnabled = enabled
	updateNMOIndicatorForCurrent()
	refreshCDPPreviewFromOriginalAsync()
	if enabled {
		pushStatus("CDP NMO preview enabled")
		return
	}
	pushStatus("CDP NMO preview disabled")
}

func setCurrentCDPPickLocked(locked bool) {
	cdp := currentCDPForPickState()
	if cdp <= 0 {
		return
	}
	lockedPicksByCDP[cdp] = locked
	applySemblancePickEditState()
	updateNMOIndicatorForCurrent()
	if locked {
		pushStatus(fmt.Sprintf("CDP %d picks locked", cdp))
		return
	}
	pushStatus(fmt.Sprintf("CDP %d picks unlocked", cdp))
}

func clearCurrentCDPVelocityPicks() {
	cdp := currentCDPForPickState()
	if cdp <= 0 {
		pushStatus("No active CDP for clearing velocity picks")
		return
	}
	picks, had := semblancePicksByCDP[cdp]
	if !had || len(picks) == 0 {
		pushStatus(fmt.Sprintf("CDP %d has no explicit velocity picks to clear", cdp))
		return
	}

	delete(semblancePicksByCDP, cdp)
	semblancePicksDirty = true
	markStackDirtyRangeForCDP(cdp, "velocity")
	refreshStackPickMarkers()

	if displayedSemblanceGather != nil && displayedSemblanceGather.CDP == cdp && semblanceImageView != nil {
		semblanceImageView.SetPicks(effectiveSemblancePicksForCDP(cdp))
	}
	refreshCDPPreviewFromOriginalAsync()
	updateNMOIndicatorForCurrent()
	pushStatus(fmt.Sprintf("Cleared %d velocity picks for CDP %d", len(picks), cdp))
}

func clearCurrentCDPMutePicks() {
	cdp := currentCDPForPickState()
	if cdp <= 0 {
		pushStatus("No active CDP for clearing top mute picks")
		return
	}
	picks, had := topMutePicksByCDP[cdp]
	if !had || len(picks) == 0 {
		pushStatus(fmt.Sprintf("CDP %d has no explicit top mute picks to clear", cdp))
		return
	}

	delete(topMutePicksByCDP, cdp)
	topMutePicksDirty = true
	markStackDirtyRangeForCDP(cdp, "mute")
	refreshStackPickMarkers()

	if displayedCDPGather != nil && displayedCDPGather.CDP == cdp && cdpImageView != nil {
		offsetAxis, _, _ := gatherOffsetAxisAndRange(*displayedCDPGather)
		cdpImageView.SetPicks(effectiveTopMutePicksForCDP(cdp, offsetAxis))
	}
	refreshCDPPreviewFromOriginalAsync()
	updateNMOIndicatorForCurrent()
	pushStatus(fmt.Sprintf("Cleared %d top mute picks for CDP %d", len(picks), cdp))
}

func clearAllVelocityPicks() {
	if len(semblancePicksByCDP) == 0 {
		pushStatus("No velocity picks to clear")
		return
	}

	total := 0
	for _, picks := range semblancePicksByCDP {
		total += len(picks)
	}
	semblancePicksByCDP = map[int64][]velocitywidgets.EnsemblePick{}
	semblancePicksDirty = true
	markStackDirtyFull()
	refreshStackPickMarkers()

	if displayedSemblanceGather != nil && semblanceImageView != nil {
		semblanceImageView.SetPicks(nil)
	}
	refreshCDPPreviewFromOriginalAsync()
	updateNMOIndicatorForCurrent()
	pushStatus(fmt.Sprintf("Cleared all velocity picks (%d picks)", total))
}

func clearAllMutePicks() {
	if len(topMutePicksByCDP) == 0 {
		pushStatus("No top mute picks to clear")
		return
	}

	total := 0
	for _, picks := range topMutePicksByCDP {
		total += len(picks)
	}
	topMutePicksByCDP = map[int64][]velocitywidgets.EnsemblePick{}
	topMutePicksDirty = true
	markStackDirtyFull()
	refreshStackPickMarkers()

	if displayedCDPGather != nil && cdpImageView != nil {
		cdpImageView.SetPicks(nil)
	}
	refreshCDPPreviewFromOriginalAsync()
	updateNMOIndicatorForCurrent()
	pushStatus(fmt.Sprintf("Cleared all top mute picks (%d picks)", total))
}

func applySemblancePickEditState() {
	if semblanceImageView == nil {
		return
	}
	cdp := currentCDPForPickState()
	locked := lockedPicksByCDP[cdp]
	semblanceImageView.SetPickEditEnabled(pickModeRequested && !locked)
}

func applyCDPMutePickEditState() {
	if cdpImageView == nil {
		return
	}
	cdpImageView.SetPickEditEnabled(muteModeRequested)
}

func currentCDPForPickState() int64 {
	if displayedSemblanceGather != nil {
		return displayedSemblanceGather.CDP
	}
	if currentCDPGather != nil {
		return currentCDPGather.CDP
	}
	return 0
}

func updateNMOIndicatorForCurrent() {
	if displayedSemblanceGather == nil {
		setNMOStateLabel("NMO: idle")
		return
	}

	cdp := displayedSemblanceGather.CDP
	pickCount := len(semblancePicksByCDP[cdp])
	mutePickCount := len(topMutePicksByCDP[cdp])
	stretch := 0.0
	if s, err := selectedSemblanceStretchMuteFraction(); err == nil {
		stretch = s
	}

	mode := "raw"
	if cdpNMOPreviewEnabled {
		mode = "nmo"
	}
	lock := "unlocked"
	if lockedPicksByCDP[cdp] {
		lock = "locked"
	}
	muteMode := "off"
	if muteModeRequested {
		muteMode = "on"
	}

	setNMOStateLabel(fmt.Sprintf("NMO %s | CDP %d | v-picks %d | mute-picks %d | stretch %.3f | v-lock %s | mute-mode %s", mode, cdp, pickCount, mutePickCount, stretch, lock, muteMode))
}

func renderCDPGatherImage(gather segy.CDPGather) (image.Image, int, error) {
	if len(gather.Traces) == 0 {
		return nil, 0, fmt.Errorf("gather has no traces")
	}

	sampleCount := 0
	for i := range gather.Traces {
		if len(gather.Traces[i].Samples) > sampleCount {
			sampleCount = len(gather.Traces[i].Samples)
		}
	}
	if sampleCount <= 0 {
		return nil, 0, fmt.Errorf("gather traces have zero samples")
	}

	traces := make([][]float32, len(gather.Traces))
	for i := range gather.Traces {
		in := gather.Traces[i].Samples
		if len(in) == sampleCount {
			traces[i] = in
			continue
		}
		padded := make([]float32, sampleCount)
		copy(padded, in)
		traces[i] = padded
	}

	img, _, err := segy.RenderEnsembleImage(segy.EnsembleSamples{
		Traces:            traces,
		TraceCount:        len(traces),
		SamplesPerTrace:   sampleCount,
		TotalImageSamples: int64(len(traces)) * int64(sampleCount),
	}, segy.EnsembleImageOptions{
		Palette:        grayscalePalette(false),
		PercentileClip: 2,
		ZeroCentered:   true,
		ZeroIndex:      127,
	})
	if err != nil {
		return nil, 0, err
	}

	return img, sampleCount, nil
}

func buildWholeLineStackAsync() {
	if loadedFile == nil || catalog == nil {
		pushStatus("Load a file before building stack section")
		setStackStateLabel("Stack: no file")
		return
	}
	if stackImageView == nil {
		pushStatus("Stack view is not ready")
		setStackStateLabel("Stack: view unavailable")
		return
	}

	requestID := atomic.AddUint64(&stackRenderSeq, 1)
	cat := catalog
	file := loadedFile

	go func(id uint64, f *segy.File, cat *segy.TraceCatalog) {
		if f == nil || cat == nil {
			return
		}
		pushStatus("Building stack section ...")
		setStackStateLabel("Stack: running ...")

		stretchMute, err := selectedSemblanceStretchMuteFraction()
		if err != nil {
			if id == atomic.LoadUint64(&stackRenderSeq) {
				pushStatus(err.Error())
				setStackStateLabel("Stack: invalid stretch")
			}
			return
		}

		cdps := allCatalogCDPs(cat)
		if len(cdps) == 0 {
			if id == atomic.LoadUint64(&stackRenderSeq) {
				pushStatus("No CDPs found in catalog")
				setStackStateLabel("Stack: no CDPs")
			}
			return
		}

		processingSig := currentStackProcessingSignature()
		catalogMatches := sameInt64Slice(stackCacheCatalogCDPs, cdps)
		fullRebuild := !stackCacheReady || stackDirtyFull || processingSig != stackCacheProcessingSignature || !catalogMatches

		type targetCDP struct {
			idx int
			cdp int64
		}

		targets := make([]targetCDP, 0, len(cdps))
		if fullRebuild {
			for i, cdp := range cdps {
				targets = append(targets, targetCDP{idx: i, cdp: cdp})
			}
			pushStatus("Building full stack section across all CDPs ...")
		} else if stackDirtyRangeSet {
			for i, cdp := range cdps {
				if cdp < stackDirtyRangeMin || cdp > stackDirtyRangeMax {
					continue
				}
				targets = append(targets, targetCDP{idx: i, cdp: cdp})
			}
			pushStatus(fmt.Sprintf("Updating stack for CDP range %d-%d ...", stackDirtyRangeMin, stackDirtyRangeMax))
		} else {
			pushStatus("Stack already up to date; reusing cached traces")
		}

		workingCache := make(map[int64]stackTraceCacheEntry)
		if !fullRebuild {
			workingCache = make(map[int64]stackTraceCacheEntry, len(stackTraceCacheByCDP))
			for cdp, entry := range stackTraceCacheByCDP {
				workingCache[cdp] = stackTraceCacheEntry{trace: append([]float32(nil), entry.trace...), dtMS: entry.dtMS}
			}
		}

		workerCount := runtime.NumCPU()
		if workerCount < 1 {
			workerCount = 1
		}

		type loadedCDPJob struct {
			idx    int
			cdp    int64
			gather segy.CDPGather
			err    error
		}
		type stackResult struct {
			idx   int
			cdp   int64
			trace []float32
			dtMS  float64
			skip  string
		}

		processedCount := len(targets)
		skipGather := 0
		skipProcessing := 0
		skipNMO := 0
		skipStack := 0
		skipDTMismatch := 0

		if len(targets) > 0 {
			jobs := make(chan loadedCDPJob, workerCount)
			results := make(chan stackResult, workerCount)

			var workers sync.WaitGroup
			for w := 0; w < workerCount; w++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					for job := range jobs {
						if id != atomic.LoadUint64(&stackRenderSeq) {
							return
						}
						if job.err != nil {
							results <- stackResult{idx: job.idx, cdp: job.cdp, skip: "gather"}
							continue
						}

						processed, err := applyStackProcessingToCDPGather(job.gather)
						if err != nil {
							results <- stackResult{idx: job.idx, cdp: job.cdp, skip: "processing"}
							continue
						}

						offsetAxis, _, _ := gatherOffsetAxisAndRange(processed)
						topMutePicks := effectiveTopMutePicksForCDP(job.cdp, offsetAxis)
						processed = applyTopMutePicksToGather(processed, topMutePicks)

						velocityPicks := effectiveSemblancePicksForCDP(job.cdp)
						processed, err = applyNMOPicksToGather(processed, velocityPicks, stretchMute)
						if err != nil {
							results <- stackResult{idx: job.idx, cdp: job.cdp, skip: "nmo"}
							continue
						}

						stacked, err := segy.StackCDPGatherMean(processed, nil)
						if err != nil {
							results <- stackResult{idx: job.idx, cdp: job.cdp, skip: "stack"}
							continue
						}

						results <- stackResult{
							idx:   job.idx,
							cdp:   job.cdp,
							trace: append([]float32(nil), stacked.Samples...),
							dtMS:  stacked.SampleIntervalMS,
						}
					}
				}()
			}

			go func() {
				for i, target := range targets {
					if id != atomic.LoadUint64(&stackRenderSeq) {
						break
					}
					if i > 0 && i%25 == 0 {
						pushStatus(fmt.Sprintf("Stacking CDPs: %d/%d", i, len(targets)))
						setStackStateLabel(fmt.Sprintf("Stack: %d/%d", i, len(targets)))
					}
					gather, err := f.BuildCDPGatherByCDP(target.cdp, true)
					jobs <- loadedCDPJob{idx: target.idx, cdp: target.cdp, gather: gather, err: err}
				}
				close(jobs)
			}()

			go func() {
				workers.Wait()
				close(results)
			}()

			for res := range results {
				if res.skip != "" {
					switch res.skip {
					case "gather":
						skipGather++
					case "processing":
						skipProcessing++
					case "nmo":
						skipNMO++
					case "stack":
						skipStack++
					}
					delete(workingCache, res.cdp)
					continue
				}
				workingCache[res.cdp] = stackTraceCacheEntry{trace: res.trace, dtMS: res.dtMS}
			}
		}

		if id != atomic.LoadUint64(&stackRenderSeq) {
			setStackStateLabel("Stack: canceled")
			return
		}

		stackTraces := make([][]float32, 0, len(workingCache))
		keptCDPs := make([]int64, 0, len(workingCache))
		referenceDT := 0.0
		maxSamples := 0
		for i := 0; i < len(cdps); i++ {
			cdp := cdps[i]
			entry, ok := workingCache[cdp]
			if !ok {
				continue
			}
			if referenceDT <= 0 {
				referenceDT = entry.dtMS
			}
			if math.Abs(entry.dtMS-referenceDT) > 1e-9 {
				skipDTMismatch++
				delete(workingCache, cdp)
				continue
			}
			if len(entry.trace) > maxSamples {
				maxSamples = len(entry.trace)
			}
			stackTraces = append(stackTraces, entry.trace)
			keptCDPs = append(keptCDPs, cdp)
		}

		if len(stackTraces) == 0 || maxSamples <= 0 {
			if id == atomic.LoadUint64(&stackRenderSeq) {
				setStackExportAvailable(false)
				fyne.Do(func() {
					if stackImageView != nil {
						stackImageView.Clear()
						stackImageView.SetVerticalMarkerFractions(nil, nil, nil)
						stackImageView.SetActiveVerticalMarkerFraction(math.NaN())
					}
					stackBaseImage = nil
					stackBaseSampleIntervalMS = 0
					stackBaseSampleCount = 0
					stackCDPAxis = nil
				})
				pushStatus("No stack traces could be built")
				setStackStateLabel(fmt.Sprintf("Stack: 0/%d kept (g:%d p:%d n:%d s:%d dt:%d)", processedCount, skipGather, skipProcessing, skipNMO, skipStack, skipDTMismatch))
			}
			return
		}

		for i := range stackTraces {
			if len(stackTraces[i]) == maxSamples {
				continue
			}
			padded := make([]float32, maxSamples)
			copy(padded, stackTraces[i])
			stackTraces[i] = padded
		}

		img, _, err := segy.RenderEnsembleImage(segy.EnsembleSamples{
			Traces:            stackTraces,
			TraceCount:        len(stackTraces),
			SamplesPerTrace:   maxSamples,
			TotalImageSamples: int64(len(stackTraces)) * int64(maxSamples),
		}, segy.EnsembleImageOptions{
			Palette:        grayscalePalette(false),
			PercentileClip: 2,
			ZeroCentered:   true,
			ZeroIndex:      127,
		})
		if err != nil {
			if id == atomic.LoadUint64(&stackRenderSeq) {
				pushStatus(fmt.Sprintf("Stack section render failed: %v", err))
				setStackStateLabel("Stack: render failed")
			}
			return
		}

		if referenceDT <= 0 {
			referenceDT = 1
		}

		if id != atomic.LoadUint64(&stackRenderSeq) {
			setStackStateLabel("Stack: canceled")
			return
		}

		stackTraceCacheByCDP = workingCache
		stackCacheCatalogCDPs = append([]int64(nil), cdps...)
		stackCacheProcessingSignature = processingSig
		stackCacheReady = true
		stackDirtyFull = false
		stackDirtyRangeSet = false
		stackDirtyRangeMin = 0
		stackDirtyRangeMax = 0

		fyne.Do(func() {
			stackBaseImage = img
			stackBaseSampleIntervalMS = referenceDT
			stackBaseSampleCount = maxSamples
			stackImageView.SetImage(img, referenceDT, maxSamples)
			stackCDPAxis = append([]int64(nil), keptCDPs...)
			refreshStackPickMarkers()
			lastStackCursorStatus = ""
			ensembleViewSyncState.ApplyCurrent(stackImageView)
		})
		pushStatus(fmt.Sprintf("Stack section ready: %d CDPs (%d to %d)", len(keptCDPs), keptCDPs[0], keptCDPs[len(keptCDPs)-1]))
		setStackExportAvailable(true)
		mode := "full"
		if !fullRebuild {
			mode = "partial"
		}
		setStackStateLabel(fmt.Sprintf("Stack %s: %d/%d kept | g:%d p:%d n:%d s:%d dt:%d | %d-%d", mode, len(keptCDPs), processedCount, skipGather, skipProcessing, skipNMO, skipStack, skipDTMismatch, keptCDPs[0], keptCDPs[len(keptCDPs)-1]))
	}(requestID, file, cat)
}

func allCatalogCDPs(cat *segy.TraceCatalog) []int64 {
	if cat == nil {
		return nil
	}
	seen := make(map[int64]struct{}, len(cat.Entries))
	cdps := make([]int64, 0, len(cat.Entries))
	for i := range cat.Entries {
		cdp := int64(cat.Entries[i].Header.CDP)
		if cdp <= 0 {
			continue
		}
		if _, ok := seen[cdp]; ok {
			continue
		}
		seen[cdp] = struct{}{}
		cdps = append(cdps, cdp)
	}
	sort.Slice(cdps, func(i, j int) bool { return cdps[i] < cdps[j] })
	return cdps
}

func applyStackProcessingToCDPGather(g segy.CDPGather) (segy.CDPGather, error) {
	highCutHz, err := selectedHighCutHz(g.SampleIntervalMS)
	if err != nil {
		return segy.CDPGather{}, err
	}

	processed := g
	nyquist := 500.0 / g.SampleIntervalMS
	if highCutHz > 0 && highCutHz < nyquist {
		filtered, err := segy.ApplyHighCutToCDPGather(g, highCutHz)
		if err != nil {
			return segy.CDPGather{}, err
		}
		processed = filtered
	}

	gain, err := selectedGainOptions(processed.SampleIntervalMS)
	if err != nil {
		return segy.CDPGather{}, err
	}
	processed = applyGainToGather(processed, gain)

	return processed, nil
}

func applyGainToGather(gather segy.CDPGather, gain segy.GainOptions) segy.CDPGather {
	out := segy.CDPGather{
		CDP:              gather.CDP,
		SampleIntervalMS: gather.SampleIntervalMS,
		Traces:           make([]segy.GatherTrace, len(gather.Traces)),
	}

	for i := range gather.Traces {
		out.Traces[i].Header = gather.Traces[i].Header
		out.Traces[i].Samples = append([]float32(nil), gather.Traces[i].Samples...)
	}

	switch gain.Mode {
	case segy.GainModeTSquared:
		scale := gain.TSquaredScale
		if scale <= 0 {
			scale = 1
		}
		dtSec := gain.SampleIntervalSec
		if dtSec <= 0 {
			dtSec = out.SampleIntervalMS / 1000.0
		}
		for i := range out.Traces {
			for j := range out.Traces[i].Samples {
				t := float64(j) * dtSec
				factor := 1.0 + scale*t*t
				out.Traces[i].Samples[j] = float32(float64(out.Traces[i].Samples[j]) * factor)
			}
		}
	case segy.GainModeAGC:
		window := gain.AGCWindowSamples
		if window <= 0 {
			window = 101
		}
		if window%2 == 0 {
			window++
		}
		half := window / 2
		floor := gain.AGCFloor
		if floor <= 0 {
			floor = 1e-6
		}

		for i := range out.Traces {
			s := out.Traces[i].Samples
			n := len(s)
			if n == 0 {
				continue
			}
			prefix := make([]float64, n+1)
			for j := 0; j < n; j++ {
				v := float64(s[j])
				prefix[j+1] = prefix[j] + v*v
			}
			for j := 0; j < n; j++ {
				start := j - half
				if start < 0 {
					start = 0
				}
				end := j + half + 1
				if end > n {
					end = n
				}
				count := end - start
				if count <= 0 {
					continue
				}
				sumSq := prefix[end] - prefix[start]
				rms := math.Sqrt(sumSq / float64(count))
				if rms < floor {
					rms = floor
				}
				s[j] = float32(float64(s[j]) / rms)
			}
		}
	}

	return out
}

func applySmoothingToGather(gather segy.CDPGather, windowLength int) (segy.CDPGather, error) {
	if windowLength <= 1 {
		out := segy.CDPGather{
			CDP:              gather.CDP,
			SampleIntervalMS: gather.SampleIntervalMS,
			Traces:           make([]segy.GatherTrace, len(gather.Traces)),
		}
		for i := range gather.Traces {
			out.Traces[i].Header = gather.Traces[i].Header
			out.Traces[i].Samples = append([]float32(nil), gather.Traces[i].Samples...)
		}
		return out, nil
	}

	out := segy.CDPGather{
		CDP:              gather.CDP,
		SampleIntervalMS: gather.SampleIntervalMS,
		Traces:           make([]segy.GatherTrace, len(gather.Traces)),
	}

	for i := range gather.Traces {
		smoothed, err := segy.SmoothTraceTimeRunningAverage(gather.Traces[i].Samples, windowLength)
		if err != nil {
			return segy.CDPGather{}, err
		}
		out.Traces[i] = segy.GatherTrace{
			Header:  gather.Traces[i].Header,
			Samples: smoothed,
		}
	}

	return out, nil
}
