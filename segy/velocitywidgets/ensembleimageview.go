package velocitywidgets

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/hippodribble/drawing/layermanager"
)

const pickHitRadiusPixels = 100.0
const defaultPickStrokeWidth = float32(6)
const defaultHighlightDiameterFactor = float32(4)

// EnsembleImageViewOptions controls appearance and interaction defaults.
type EnsembleImageViewOptions struct {
	ShowTimeAxis  bool
	AxisWidth     float32
	ZoomFactor    float64
	MinViewSpanMS float64
	MinSize       fyne.Size
}

// EnsembleImageView displays a 2D time-oriented image with optional vertical
// time axis and vertical zoom/pan interactions.
type EnsembleImageView struct {
	widget.BaseWidget

	imgWidget         *canvas.Image
	axisLayer         *fyne.Container
	pickLayer         *layermanager.GeometryLayer
	highlightLayer    *layermanager.GeometryLayer
	velMarkerLayer    *layermanager.GeometryLayer
	muteMarkerLayer   *layermanager.GeometryLayer
	bothMarkerLayer   *layermanager.GeometryLayer
	activeMarkerLayer *layermanager.GeometryLayer
	showTimeAxis      bool
	axisWidth         float32
	zoomFactor        float64
	cursorGuide       *layermanager.GeometryLayer
	minViewSpanMS     float64
	fullMinMS         float64
	fullMaxMS         float64
	viewMinMS         float64
	viewMaxMS         float64
	fullImage         image.Image
	minSize           fyne.Size
	onViewChanged     func(minMS, maxMS float64)
	onCursorMoved     func(info EnsembleCursorInfo)
	onCursorTapped    func(info EnsembleCursorInfo)
	onPicksChanged    func(picks []EnsemblePick)
	onDragEnded       func()

	picksVisible           bool
	pickEditEnabled        bool
	pickVelMin             float64
	pickVelMax             float64
	pickVelAxis            []float64
	picks                  []EnsemblePick
	visiblePickIndices     []int
	highlightedPickIdx     int
	pickStrokeWidth        float32
	cursorGuideEnabled     bool
	horizontalGuideEnabled bool
	cursorXFrac            float64
	cursorTimeMS           float64
	velMarkerXFracs        []float64
	muteMarkerXFracs       []float64
	bothMarkerXFracs       []float64
	activeMarkerXFrac      float64

	middleButtonDown  bool
	primaryButtonDown bool
	primaryDragMoved  bool
	activePickIndex   int
}

const keyboardPanFraction = 0.1

// EnsembleCursorInfo reports cursor position in image/domain coordinates.
type EnsembleCursorInfo struct {
	Inside bool
	XFrac  float64
	TimeMS float64
}

// EnsemblePick stores one velocity pick in world/domain coordinates.
type EnsemblePick struct {
	Velocity float64
	TimeMS   float64
}

// NewEnsembleImageView creates a new image view widget.
func NewEnsembleImageView(opts EnsembleImageViewOptions) *EnsembleImageView {
	opts = normalizeEnsembleImageViewOptions(opts)

	img := canvas.NewImageFromImage(nil)
	img.FillMode = canvas.ImageFillStretch
	img.ScaleMode = canvas.ImageScaleFastest

	v := &EnsembleImageView{
		imgWidget:          img,
		axisLayer:          container.NewWithoutLayout(),
		pickLayer:          layermanager.NewGeometryLayer(),
		highlightLayer:     layermanager.NewGeometryLayer(),
		velMarkerLayer:     layermanager.NewGeometryLayer(),
		muteMarkerLayer:    layermanager.NewGeometryLayer(),
		bothMarkerLayer:    layermanager.NewGeometryLayer(),
		activeMarkerLayer:  layermanager.NewGeometryLayer(),
		showTimeAxis:       opts.ShowTimeAxis,
		axisWidth:          opts.AxisWidth,
		zoomFactor:         opts.ZoomFactor,
		cursorGuide:        layermanager.NewGeometryLayer(),
		minViewSpanMS:      opts.MinViewSpanMS,
		minSize:            opts.MinSize,
		activePickIndex:    -1,
		highlightedPickIdx: -1,
		picksVisible:       true,
		pickStrokeWidth:    defaultPickStrokeWidth,
		cursorXFrac:        -1,
		cursorTimeMS:       math.NaN(),
		activeMarkerXFrac:  math.NaN(),
	}
	v.pickLayer.SetStrokeColor(color.NRGBA{R: 0, G: 0, B: 0, A: 128})
	v.pickLayer.SetStrokeWidth(v.pickStrokeWidth)
	v.highlightLayer.SetStrokeColor(color.NRGBA{R: 0, G: 0, B: 0, A: 255})
	v.highlightLayer.SetStrokeWidth(defaultHighlightDiameterFactor * v.pickStrokeWidth)
	v.velMarkerLayer.SetStrokeColor(color.NRGBA{R: 30, G: 120, B: 220, A: 210})
	v.velMarkerLayer.SetStrokeWidth(2)
	v.muteMarkerLayer.SetStrokeColor(color.NRGBA{R: 220, G: 90, B: 20, A: 210})
	v.muteMarkerLayer.SetStrokeWidth(2)
	v.bothMarkerLayer.SetStrokeColor(color.NRGBA{R: 20, G: 20, B: 20, A: 230})
	v.bothMarkerLayer.SetStrokeWidth(3)
	v.activeMarkerLayer.SetStrokeColor(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	v.activeMarkerLayer.SetStrokeWidth(3)
	v.ExtendBaseWidget(v)
	return v
}

// SetVerticalMarkerFractions sets full-height vertical marker lines by category.
// Fractions are in [0,1], where 0 is left edge of the image area and 1 is right edge.
func (v *EnsembleImageView) SetVerticalMarkerFractions(velocityXFracs, muteXFracs, bothXFracs []float64) {
	v.velMarkerXFracs = append(v.velMarkerXFracs[:0], velocityXFracs...)
	v.muteMarkerXFracs = append(v.muteMarkerXFracs[:0], muteXFracs...)
	v.bothMarkerXFracs = append(v.bothMarkerXFracs[:0], bothXFracs...)
	v.refreshVerticalMarkerLayers()
}

// SetActiveVerticalMarkerFraction sets one prominent full-height marker line.
// Use NaN to hide it.
func (v *EnsembleImageView) SetActiveVerticalMarkerFraction(xFrac float64) {
	v.activeMarkerXFrac = xFrac
	v.refreshVerticalMarkerLayers()
}

// SetCursorGuideEnabled toggles the vertical cursor guide overlay.
func (v *EnsembleImageView) SetCursorGuideEnabled(enabled bool) {
	v.cursorGuideEnabled = enabled
	if !enabled {
		v.cursorXFrac = -1
	}
	v.refreshCursorGuideLayer()
}

// SetHorizontalCursorGuideEnabled toggles the horizontal time guide overlay.
func (v *EnsembleImageView) SetHorizontalCursorGuideEnabled(enabled bool) {
	v.horizontalGuideEnabled = enabled
	if !enabled {
		v.cursorTimeMS = math.NaN()
	}
	v.refreshCursorGuideLayer()
}

// SetCursorGuideTimeMS sets the horizontal guide to a specific time in ms.
func (v *EnsembleImageView) SetCursorGuideTimeMS(timeMS float64) {
	v.cursorTimeMS = timeMS
	v.refreshCursorGuideLayer()
}

// ClearCursorGuideTime hides the horizontal guide line.
func (v *EnsembleImageView) ClearCursorGuideTime() {
	v.cursorTimeMS = math.NaN()
	v.refreshCursorGuideLayer()
}

// SetImage configures the image using sample interval and sample count.
func (v *EnsembleImageView) SetImage(img image.Image, sampleIntervalMS float64, sampleCount int) {
	if sampleIntervalMS <= 0 || sampleCount <= 0 {
		v.Clear()
		return
	}
	v.SetImageWithTime(img, 0, float64(sampleCount-1)*sampleIntervalMS)
}

// SetImageWithTime configures image and explicit full time range.
func (v *EnsembleImageView) SetImageWithTime(img image.Image, minMS, maxMS float64) {
	if img == nil {
		v.Clear()
		return
	}
	if maxMS < minMS {
		minMS, maxMS = maxMS, minMS
	}

	v.fullImage = img
	v.fullMinMS = minMS
	v.fullMaxMS = maxMS
	v.viewMinMS = minMS
	v.viewMaxMS = maxMS
	v.refreshView()
}

// Clear removes image and axis content.
func (v *EnsembleImageView) Clear() {
	v.fullImage = nil
	v.fullMinMS = 0
	v.fullMaxMS = 0
	v.viewMinMS = 0
	v.viewMaxMS = 0
	v.imgWidget.Image = nil
	v.imgWidget.Refresh()
	v.axisLayer.Objects = nil
	v.axisLayer.Refresh()
	v.pickLayer.ClearPolylines()
	v.pickLayer.Points = nil
	v.pickLayer.Refresh()
	v.highlightedPickIdx = -1
	v.highlightLayer.ClearPolylines()
	v.highlightLayer.Points = nil
	v.highlightLayer.Refresh()
	v.velMarkerLayer.ClearPolylines()
	v.velMarkerLayer.Points = nil
	v.velMarkerLayer.Refresh()
	v.muteMarkerLayer.ClearPolylines()
	v.muteMarkerLayer.Points = nil
	v.muteMarkerLayer.Refresh()
	v.bothMarkerLayer.ClearPolylines()
	v.bothMarkerLayer.Points = nil
	v.bothMarkerLayer.Refresh()
	v.activeMarkerLayer.ClearPolylines()
	v.activeMarkerLayer.Points = nil
	v.activeMarkerLayer.Refresh()
	v.activeMarkerXFrac = math.NaN()
	v.cursorXFrac = -1
	v.cursorTimeMS = math.NaN()
	v.cursorGuide.ClearPolylines()
	v.cursorGuide.Points = nil
	v.cursorGuide.Refresh()
	v.Refresh()
}

// SetPickOverlayEnabled toggles overlay display and interaction for picks.
func (v *EnsembleImageView) SetPickOverlayEnabled(enabled bool) {
	v.picksVisible = enabled
	v.refreshPickLayer()
	v.Refresh()
}

// SetPickEditEnabled toggles edit operations while leaving pick rendering intact.
func (v *EnsembleImageView) SetPickEditEnabled(enabled bool) {
	v.pickEditEnabled = enabled
	if !enabled {
		v.primaryButtonDown = false
		v.primaryDragMoved = false
		v.activePickIndex = -1
		v.highlightedPickIdx = -1
		v.refreshHighlightLayer()
	}
}

// SetPickStrokeWidth sets the base stroke width used for pick lines/markers.
func (v *EnsembleImageView) SetPickStrokeWidth(width float32) {
	if width <= 0 {
		width = defaultPickStrokeWidth
	}
	v.pickStrokeWidth = width
	v.pickLayer.SetStrokeWidth(v.pickStrokeWidth)
	v.highlightLayer.SetStrokeWidth(defaultHighlightDiameterFactor * v.pickStrokeWidth)
	v.refreshPickLayer()
}

// SetPickVelocityRange sets the horizontal velocity range used by pick mapping.
func (v *EnsembleImageView) SetPickVelocityRange(minVelocity, maxVelocity float64) {
	if maxVelocity < minVelocity {
		minVelocity, maxVelocity = maxVelocity, minVelocity
	}
	v.pickVelMin = minVelocity
	v.pickVelMax = maxVelocity
	v.refreshPickLayer()
}

// SetPickVelocityAxis sets the per-column velocity axis used for pick mapping.
// This should match the horizontal sampling used to render the image.
func (v *EnsembleImageView) SetPickVelocityAxis(axis []float64) {
	v.pickVelAxis = append(v.pickVelAxis[:0], axis...)
	v.refreshPickLayer()
}

// SetOnPicksChanged sets a callback invoked when picks are edited.
func (v *EnsembleImageView) SetOnPicksChanged(fn func(picks []EnsemblePick)) {
	v.onPicksChanged = fn
}

// SetOnDragEnded sets a callback invoked after a primary drag-edit completes.
func (v *EnsembleImageView) SetOnDragEnded(fn func()) {
	v.onDragEnded = fn
}

// SetPicks replaces the current pick set.
func (v *EnsembleImageView) SetPicks(picks []EnsemblePick) {
	v.picks = append([]EnsemblePick(nil), picks...)
	v.refreshPickLayer()
}

// Picks returns a copy of current picks.
func (v *EnsembleImageView) Picks() []EnsemblePick {
	return append([]EnsemblePick(nil), v.picks...)
}

// SetShowTimeAxis toggles the left vertical time axis.
func (v *EnsembleImageView) SetShowTimeAxis(show bool) {
	v.showTimeAxis = show
	v.Refresh()
	v.refreshAxis()
}

// SetOnViewChanged sets a callback invoked after zoom/pan/reset updates.
func (v *EnsembleImageView) SetOnViewChanged(fn func(minMS, maxMS float64)) {
	v.onViewChanged = fn
}

// SetOnCursorMoved sets a callback invoked when the mouse moves over the image area.
func (v *EnsembleImageView) SetOnCursorMoved(fn func(info EnsembleCursorInfo)) {
	v.onCursorMoved = fn
}

// SetOnCursorTapped sets a callback invoked when the widget is primary-clicked.
func (v *EnsembleImageView) SetOnCursorTapped(fn func(info EnsembleCursorInfo)) {
	v.onCursorTapped = fn
}

// ResetView resets the visible range to the full time range.
func (v *EnsembleImageView) ResetView() {
	if v.fullImage == nil {
		return
	}
	v.viewMinMS = v.fullMinMS
	v.viewMaxMS = v.fullMaxMS
	v.refreshView()
	v.notifyViewChanged()
}

// SetViewRange applies a clamped visible time range.
func (v *EnsembleImageView) SetViewRange(minMS, maxMS float64) {
	v.setViewRange(minMS, maxMS, true)
}

// ApplyViewRange applies a clamped visible time range without notifying listeners.
func (v *EnsembleImageView) ApplyViewRange(minMS, maxMS float64) {
	v.setViewRange(minMS, maxMS, false)
}

func (v *EnsembleImageView) setViewRange(minMS, maxMS float64, notify bool) {
	if v.fullImage == nil {
		return
	}
	if maxMS < minMS {
		minMS, maxMS = maxMS, minMS
	}
	span := maxMS - minMS
	if span < v.minViewSpanMS {
		center := 0.5 * (minMS + maxMS)
		half := 0.5 * v.minViewSpanMS
		minMS = center - half
		maxMS = center + half
	}

	v.viewMinMS, v.viewMaxMS = clampRange(minMS, maxMS, v.fullMinMS, v.fullMaxMS)
	v.refreshView()
	if notify {
		v.notifyViewChanged()
	}
}

// ViewRange returns the current visible time range in milliseconds.
func (v *EnsembleImageView) ViewRange() (float64, float64) {
	return v.viewMinMS, v.viewMaxMS
}

// Scrolled applies mouse-centered vertical zoom.
func (v *EnsembleImageView) Scrolled(ev *fyne.ScrollEvent) {
	if v.fullImage == nil || ev == nil {
		return
	}
	if v.zoomFactor <= 1 {
		return
	}

	delta := ev.Scrolled.DY
	if delta == 0 {
		return
	}

	factor := v.zoomFactor
	if delta < 0 {
		factor = 1.0 / factor
	}
	if factor <= 0 {
		return
	}

	height := float64(v.Size().Height)
	if height <= 0 {
		height = 1
	}
	cursorFrac := float64(ev.Position.Y) / height
	if cursorFrac < 0 {
		cursorFrac = 0
	}
	if cursorFrac > 1 {
		cursorFrac = 1
	}

	oldSpan := v.viewMaxMS - v.viewMinMS
	if oldSpan <= 0 {
		oldSpan = v.fullMaxMS - v.fullMinMS
	}
	newSpan := oldSpan / factor
	if newSpan < v.minViewSpanMS {
		newSpan = v.minViewSpanMS
	}
	fullSpan := v.fullMaxMS - v.fullMinMS
	if newSpan > fullSpan {
		newSpan = fullSpan
	}

	tCursor := v.viewMinMS + cursorFrac*oldSpan
	newMin := tCursor - cursorFrac*newSpan
	newMax := newMin + newSpan
	v.viewMinMS, v.viewMaxMS = clampRange(newMin, newMax, v.fullMinMS, v.fullMaxMS)
	v.refreshView()
	v.notifyViewChanged()
}

// Dragged applies vertical panning.
func (v *EnsembleImageView) Dragged(ev *fyne.DragEvent) {
	if v.primaryButtonDown && v.activePickIndex >= 0 && ev != nil && v.activePickIndex < len(v.picks) {
		pick, ok := v.pickAtScreen(ev.Position)
		if !ok {
			return
		}
		v.primaryDragMoved = true
		v.picks[v.activePickIndex] = pick
		sort.SliceStable(v.picks, func(i, j int) bool {
			return v.picks[i].TimeMS < v.picks[j].TimeMS
		})
		v.activePickIndex = v.nearestPickIndexAt(ev.Position, pickHitRadiusPixels*2)
		v.refreshPickLayer()
		v.notifyPicksChanged()
		return
	}

	if v.fullImage == nil || ev == nil || !v.middleButtonDown {
		return
	}
	height := float64(v.Size().Height)
	if height <= 0 {
		return
	}
	span := v.viewMaxMS - v.viewMinMS
	if span <= 0 {
		return
	}

	shift := float64(ev.Dragged.DY) * span / height
	newMin := v.viewMinMS + shift
	newMax := v.viewMaxMS + shift
	v.viewMinMS, v.viewMaxMS = clampRange(newMin, newMax, v.fullMinMS, v.fullMaxMS)
	v.refreshView()
	v.notifyViewChanged()
}

// DragEnd completes a drag gesture.
func (v *EnsembleImageView) DragEnd() {
	v.middleButtonDown = false
	if v.primaryDragMoved {
		v.primaryDragMoved = false
		if v.onDragEnded != nil {
			v.onDragEnded()
		}
	}
	v.primaryButtonDown = false
	v.activePickIndex = -1
}

// Tapped gives the widget keyboard focus for arrow-key panning.
func (v *EnsembleImageView) Tapped(ev *fyne.PointEvent) {
	v.handleTapForPicks(ev)
	if ev != nil && v.onCursorTapped != nil {
		v.onCursorTapped(v.cursorInfoAt(ev.Position))
	}

	if app := fyne.CurrentApp(); app != nil {
		if canvas := app.Driver().CanvasForObject(v); canvas != nil {
			canvas.Focus(v)
		}
	}
}

// TappedSecondary resets the view to full-range.
func (v *EnsembleImageView) TappedSecondary(ev *fyne.PointEvent) {
	if ev != nil && v.tryDeleteNearestPick(ev.Position) {
		return
	}
	v.ResetView()
}

// MouseDown tracks middle-button state for panning.
func (v *EnsembleImageView) MouseDown(ev *desktop.MouseEvent) {
	if ev == nil {
		return
	}
	if ev.Button == desktop.MouseButtonPrimary {
		if !v.pickEditEnabled {
			return
		}
		v.primaryDragMoved = false
		v.primaryButtonDown = true
		v.activePickIndex = v.nearestPickIndexAt(ev.Position, pickHitRadiusPixels)
	}
	if ev.Button == desktop.MouseButtonTertiary {
		v.middleButtonDown = true
	}
}

// MouseUp clears middle-button pan state.
func (v *EnsembleImageView) MouseUp(ev *desktop.MouseEvent) {
	if ev == nil {
		return
	}
	if ev.Button == desktop.MouseButtonPrimary {
		if v.primaryDragMoved {
			v.primaryDragMoved = false
			if v.onDragEnded != nil {
				v.onDragEnded()
			}
		}
		v.primaryButtonDown = false
		v.activePickIndex = -1
	}
	if ev.Button == desktop.MouseButtonTertiary {
		v.middleButtonDown = false
	}
}

// MouseMoved emits cursor updates for hover interactions.
func (v *EnsembleImageView) MouseMoved(ev *desktop.MouseEvent) {
	if ev == nil {
		return
	}
	v.updateCursorGuide(ev.Position)
	if v.onCursorMoved != nil {
		v.onCursorMoved(v.cursorInfoAt(ev.Position))
	}
	v.updatePickHighlight(ev.Position)
}

// MouseIn emits an initial cursor update when entering the widget.
func (v *EnsembleImageView) MouseIn(ev *desktop.MouseEvent) {
	if ev == nil {
		return
	}
	v.updateCursorGuide(ev.Position)
	if v.onCursorMoved != nil {
		v.onCursorMoved(v.cursorInfoAt(ev.Position))
	}
	v.updatePickHighlight(ev.Position)
}

// MouseOut notifies listeners that the cursor left the image area.
func (v *EnsembleImageView) MouseOut() {
	v.cursorXFrac = -1
	v.cursorTimeMS = math.NaN()
	v.refreshCursorGuideLayer()
	if v.onCursorMoved != nil {
		v.onCursorMoved(EnsembleCursorInfo{Inside: false})
	}
	v.highlightedPickIdx = -1
	v.refreshHighlightLayer()
}

// FocusGained is required for keyboard focus support.
func (v *EnsembleImageView) FocusGained() {}

// FocusLost is required for keyboard focus support.
func (v *EnsembleImageView) FocusLost() {}

// TypedRune is required for keyboard focus support.
func (v *EnsembleImageView) TypedRune(_ rune) {}

// TypedKey pans vertically with arrow keys.
func (v *EnsembleImageView) TypedKey(ev *fyne.KeyEvent) {
	if v.fullImage == nil || ev == nil {
		return
	}

	span := v.viewMaxMS - v.viewMinMS
	if span <= 0 {
		return
	}

	shift := span * keyboardPanFraction
	switch ev.Name {
	case fyne.KeyUp:
		shift = -shift
	case fyne.KeyDown:
		// keep positive shift
	default:
		return
	}

	newMin := v.viewMinMS + shift
	newMax := v.viewMaxMS + shift
	v.viewMinMS, v.viewMaxMS = clampRange(newMin, newMax, v.fullMinMS, v.fullMaxMS)
	v.refreshView()
	v.notifyViewChanged()
}

func (v *EnsembleImageView) cursorInfoAt(pos fyne.Position) EnsembleCursorInfo {
	if v.fullImage == nil {
		return EnsembleCursorInfo{Inside: false}
	}

	size := v.Size()
	imgX := float32(0)
	imgW := size.Width
	if v.showTimeAxis {
		imgX = v.axisWidth
		if imgX <= 0 {
			imgX = 64
		}
		if imgX > size.Width {
			imgX = size.Width
		}
		imgW = size.Width - imgX
	}

	if imgW <= 0 || size.Height <= 0 {
		return EnsembleCursorInfo{Inside: false}
	}

	x := pos.X - imgX
	y := pos.Y
	if x < 0 || x >= imgW || y < 0 || y >= size.Height {
		return EnsembleCursorInfo{Inside: false}
	}

	xFrac := float64(x) / float64(imgW)
	if xFrac < 0 {
		xFrac = 0
	}
	if xFrac > 1 {
		xFrac = 1
	}

	yFrac := float64(y) / float64(size.Height)
	if yFrac < 0 {
		yFrac = 0
	}
	if yFrac > 1 {
		yFrac = 1
	}

	timeMS := v.viewMinMS + yFrac*(v.viewMaxMS-v.viewMinMS)
	return EnsembleCursorInfo{Inside: true, XFrac: xFrac, TimeMS: timeMS}
}

func (v *EnsembleImageView) refreshView() {
	if v.fullImage == nil {
		return
	}
	v.imgWidget.Image = cropToView(v.fullImage, v.fullMinMS, v.fullMaxMS, v.viewMinMS, v.viewMaxMS)
	v.imgWidget.Refresh()
	v.refreshAxis()
	v.refreshVerticalMarkerLayers()
	v.refreshPickLayer()
	v.Refresh()
}

func (v *EnsembleImageView) refreshAxis() {
	if v.axisLayer == nil {
		return
	}
	if !v.showTimeAxis || v.fullImage == nil {
		v.axisLayer.Objects = nil
		v.axisLayer.Refresh()
		return
	}

	size := v.Size()
	if size.Height <= 0 {
		size.Height = 200
	}
	w := v.axisWidth
	if w <= 0 {
		w = 64
	}
	if w > size.Width {
		w = size.Width
	}

	targetTicks := chooseTickCount(size.Height)
	spanMS := v.viewMaxMS - v.viewMinMS
	stepMS := niceTickStep(spanMS, targetTicks)
	firstTick := math.Ceil(v.viewMinMS/stepMS) * stepMS
	lastTick := math.Floor(v.viewMaxMS/stepMS) * stepMS
	objects := make([]fyne.CanvasObject, 0, targetTicks*2+2)

	axisLine := canvas.NewLine(color.NRGBA{R: 150, G: 150, B: 150, A: 255})
	axisLine.Position1 = fyne.NewPos(w-1, 0)
	axisLine.Position2 = fyne.NewPos(w-1, size.Height)
	objects = append(objects, axisLine)

	for tickMS := firstTick; tickMS <= lastTick+stepMS*0.5; tickMS += stepMS {
		if spanMS <= 0 {
			break
		}
		frac := (tickMS - v.viewMinMS) / spanMS
		if frac < 0 || frac > 1 {
			continue
		}
		y := float32(frac) * size.Height

		tick := canvas.NewLine(color.NRGBA{R: 150, G: 150, B: 150, A: 255})
		tick.Position1 = fyne.NewPos(w-8, y)
		tick.Position2 = fyne.NewPos(w-1, y)
		objects = append(objects, tick)

		label := canvas.NewText(formatTimeTickLabel(tickMS, stepMS), color.NRGBA{R: 220, G: 220, B: 220, A: 255})
		label.TextSize = 10
		label.Move(fyne.NewPos(2, y-7))
		objects = append(objects, label)
	}

	unit := canvas.NewText(timeAxisUnitLabel(stepMS), color.NRGBA{R: 180, G: 180, B: 180, A: 255})
	unit.TextSize = 10
	unit.Move(fyne.NewPos(2, 2))
	objects = append(objects, unit)

	v.axisLayer.Objects = objects
	v.axisLayer.Refresh()
}

func (v *EnsembleImageView) notifyViewChanged() {
	if v.onViewChanged != nil {
		v.onViewChanged(v.viewMinMS, v.viewMaxMS)
	}
}

func (v *EnsembleImageView) notifyPicksChanged() {
	if v.onPicksChanged != nil {
		v.onPicksChanged(v.Picks())
	}
}

func (v *EnsembleImageView) CreateRenderer() fyne.WidgetRenderer {
	objects := []fyne.CanvasObject{v.axisLayer, v.imgWidget, v.velMarkerLayer, v.muteMarkerLayer, v.bothMarkerLayer, v.activeMarkerLayer, v.cursorGuide, v.pickLayer, v.highlightLayer}
	return &ensembleImageViewRenderer{view: v, objects: objects}
}

type ensembleImageViewRenderer struct {
	view    *EnsembleImageView
	objects []fyne.CanvasObject
}

func (r *ensembleImageViewRenderer) Layout(size fyne.Size) {
	axisWidth := float32(0)
	if r.view.showTimeAxis {
		axisWidth = r.view.axisWidth
		if axisWidth <= 0 {
			axisWidth = 64
		}
		if axisWidth > size.Width {
			axisWidth = size.Width
		}
	}

	r.view.axisLayer.Move(fyne.NewPos(0, 0))
	r.view.axisLayer.Resize(fyne.NewSize(axisWidth, size.Height))

	imgX := axisWidth
	imgW := size.Width - axisWidth
	if imgW < 0 {
		imgW = 0
	}
	r.view.imgWidget.Move(fyne.NewPos(imgX, 0))
	r.view.imgWidget.Resize(fyne.NewSize(imgW, size.Height))
	r.view.pickLayer.Move(fyne.NewPos(imgX, 0))
	r.view.pickLayer.Resize(fyne.NewSize(imgW, size.Height))
	r.view.highlightLayer.Move(fyne.NewPos(imgX, 0))
	r.view.highlightLayer.Resize(fyne.NewSize(imgW, size.Height))
	r.view.velMarkerLayer.Move(fyne.NewPos(imgX, 0))
	r.view.velMarkerLayer.Resize(fyne.NewSize(imgW, size.Height))
	r.view.muteMarkerLayer.Move(fyne.NewPos(imgX, 0))
	r.view.muteMarkerLayer.Resize(fyne.NewSize(imgW, size.Height))
	r.view.bothMarkerLayer.Move(fyne.NewPos(imgX, 0))
	r.view.bothMarkerLayer.Resize(fyne.NewSize(imgW, size.Height))
	r.view.activeMarkerLayer.Move(fyne.NewPos(imgX, 0))
	r.view.activeMarkerLayer.Resize(fyne.NewSize(imgW, size.Height))
	r.view.cursorGuide.Move(fyne.NewPos(imgX, 0))
	r.view.cursorGuide.Resize(fyne.NewSize(imgW, size.Height))

	r.view.refreshAxis()
	r.view.refreshVerticalMarkerLayers()
	r.view.refreshCursorGuideLayer()
	r.view.refreshPickLayer()
}

func (r *ensembleImageViewRenderer) MinSize() fyne.Size {
	return r.view.minSize
}

func (r *ensembleImageViewRenderer) Refresh() {
	r.view.imgWidget.Refresh()
	r.view.axisLayer.Refresh()
	r.view.pickLayer.Refresh()
	r.view.highlightLayer.Refresh()
	r.view.velMarkerLayer.Refresh()
	r.view.muteMarkerLayer.Refresh()
	r.view.bothMarkerLayer.Refresh()
	r.view.activeMarkerLayer.Refresh()
	r.view.cursorGuide.Refresh()
}

func (v *EnsembleImageView) refreshVerticalMarkerLayers() {
	v.refreshVerticalMarkerLayer(v.velMarkerLayer, v.velMarkerXFracs)
	v.refreshVerticalMarkerLayer(v.muteMarkerLayer, v.muteMarkerXFracs)
	v.refreshVerticalMarkerLayer(v.bothMarkerLayer, v.bothMarkerXFracs)
	if math.IsNaN(v.activeMarkerXFrac) || math.IsInf(v.activeMarkerXFrac, 0) {
		v.refreshVerticalMarkerLayer(v.activeMarkerLayer, nil)
	} else {
		v.refreshVerticalMarkerLayer(v.activeMarkerLayer, []float64{v.activeMarkerXFrac})
	}
}

func (v *EnsembleImageView) refreshVerticalMarkerLayer(layer *layermanager.GeometryLayer, fracs []float64) {
	if layer == nil {
		return
	}
	layer.ClearPolylines()
	layer.Points = layer.Points[:0]
	_, imgW, imgH, ok := v.plotRect()
	if !ok || len(fracs) == 0 {
		layer.Refresh()
		return
	}

	layer.SetProjection(layermanager.NewProjection(fyne.NewPos(0, 0), fyne.NewPos(0, 0), 1))
	for _, frac := range fracs {
		if math.IsNaN(frac) || math.IsInf(frac, 0) || frac < 0 || frac > 1 {
			continue
		}
		x := frac * float64(imgW)
		line := []layermanager.Point{
			layermanager.NewPoint(x, 0),
			layermanager.NewPoint(x, -float64(imgH)),
		}
		layer.Points = append(layer.Points, line...)
		layer.AddPolyline(layermanager.NewPolyline(line))
	}
	layer.Refresh()
}
func (v *EnsembleImageView) updateCursorGuide(pos fyne.Position) {
	if !v.cursorGuideEnabled {
		if !v.horizontalGuideEnabled {
			return
		}
	}
	info := v.cursorInfoAt(pos)
	if !info.Inside {
		v.cursorXFrac = -1
		v.cursorTimeMS = math.NaN()
		v.refreshCursorGuideLayer()
		return
	}
	if v.cursorGuideEnabled {
		v.cursorXFrac = info.XFrac
	}
	if v.horizontalGuideEnabled {
		v.cursorTimeMS = info.TimeMS
	}
	v.refreshCursorGuideLayer()
}

func (v *EnsembleImageView) refreshCursorGuideLayer() {
	if v.cursorGuide == nil {
		return
	}
	v.cursorGuide.ClearPolylines()
	v.cursorGuide.Points = v.cursorGuide.Points[:0]
	if !v.cursorGuideEnabled && !v.horizontalGuideEnabled {
		v.cursorGuide.Refresh()
		return
	}

	_, imgW, imgH, ok := v.plotRect()
	if !ok {
		v.cursorGuide.Refresh()
		return
	}

	v.cursorGuide.SetProjection(layermanager.NewProjection(fyne.NewPos(0, 0), fyne.NewPos(0, 0), 1))
	if v.cursorGuideEnabled && v.cursorXFrac >= 0 && v.cursorXFrac <= 1 {
		x := float64(v.cursorXFrac) * float64(imgW)
		vertical := []layermanager.Point{
			layermanager.NewPoint(x, 0),
			layermanager.NewPoint(x, -float64(imgH)),
		}
		v.cursorGuide.Points = append(v.cursorGuide.Points, vertical...)
		v.cursorGuide.AddPolyline(layermanager.NewPolyline(vertical))
	}
	if v.horizontalGuideEnabled && !math.IsNaN(v.cursorTimeMS) && !math.IsInf(v.cursorTimeMS, 0) && v.viewMaxMS > v.viewMinMS {
		yFrac := (v.cursorTimeMS - v.viewMinMS) / (v.viewMaxMS - v.viewMinMS)
		if yFrac >= 0 && yFrac <= 1 {
			y := yFrac * float64(imgH)
			horizontal := []layermanager.Point{
				layermanager.NewPoint(0, -y),
				layermanager.NewPoint(float64(imgW), -y),
			}
			v.cursorGuide.Points = append(v.cursorGuide.Points, horizontal...)
			v.cursorGuide.AddPolyline(layermanager.NewPolyline(horizontal))
		}
	}
	v.cursorGuide.Refresh()
}

func (r *ensembleImageViewRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *ensembleImageViewRenderer) Destroy() {}

func (r *ensembleImageViewRenderer) BackgroundColor() color.Color {
	return color.Transparent
}

func normalizeEnsembleImageViewOptions(opts EnsembleImageViewOptions) EnsembleImageViewOptions {
	out := opts
	if out.AxisWidth <= 0 {
		out.AxisWidth = 64
	}
	if out.ZoomFactor <= 1 {
		out.ZoomFactor = 1.01
	}
	if out.MinViewSpanMS <= 0 {
		out.MinViewSpanMS = 20
	}
	if out.MinSize.Width <= 0 || out.MinSize.Height <= 0 {
		out.MinSize = fyne.NewSize(220, 180)
	}
	return out
}

func chooseTickCount(height float32) int {
	switch {
	case height < 140:
		return 4
	case height < 240:
		return 5
	case height < 360:
		return 6
	default:
		return 8
	}
}

func niceTickStep(spanMS float64, targetTicks int) float64 {
	if spanMS <= 0 {
		return 1
	}
	if targetTicks < 2 {
		targetTicks = 2
	}
	rawStep := spanMS / float64(targetTicks-1)
	if rawStep <= 0 {
		return 1
	}
	return niceNumber(rawStep)
}

func niceNumber(value float64) float64 {
	if value <= 0 {
		return 1
	}
	exponent := math.Floor(math.Log10(value))
	pow10 := math.Pow(10, exponent)
	fraction := value / pow10

	var niceFraction float64
	switch {
	case fraction <= 1:
		niceFraction = 1
	case fraction <= 2:
		niceFraction = 2
	case fraction <= 5:
		niceFraction = 5
	default:
		niceFraction = 10
	}

	return niceFraction * pow10
}

func formatTimeTickLabel(timeMS, stepMS float64) string {
	if stepMS >= 2000 || math.Abs(timeMS) >= 2000 {
		return fmt.Sprintf("%.1f s", timeMS/1000.0)
	}
	return fmt.Sprintf("%.0f ms", math.Round(timeMS))
}

func timeAxisUnitLabel(stepMS float64) string {
	if stepMS >= 2000 {
		return "s"
	}
	return "ms"
}

func clampRange(minV, maxV, fullMin, fullMax float64) (float64, float64) {
	if maxV < minV {
		minV, maxV = maxV, minV
	}
	fullSpan := fullMax - fullMin
	span := maxV - minV
	if span > fullSpan {
		return fullMin, fullMax
	}
	if minV < fullMin {
		maxV += fullMin - minV
		minV = fullMin
	}
	if maxV > fullMax {
		minV -= maxV - fullMax
		maxV = fullMax
	}
	if minV < fullMin {
		minV = fullMin
	}
	if maxV > fullMax {
		maxV = fullMax
	}
	if maxV < minV {
		maxV = minV
	}
	return minV, maxV
}

func (v *EnsembleImageView) handleTapForPicks(ev *fyne.PointEvent) {
	if ev == nil || !v.picksVisible || !v.pickEditEnabled || v.fullImage == nil {
		return
	}
	pick, ok := v.pickAtScreen(ev.Position)
	if !ok {
		return
	}
	v.picks = append(v.picks, pick)
	sort.SliceStable(v.picks, func(i, j int) bool {
		return v.picks[i].TimeMS < v.picks[j].TimeMS
	})
	v.refreshPickLayer()
	v.notifyPicksChanged()
}

func (v *EnsembleImageView) tryDeleteNearestPick(pos fyne.Position) bool {
	if !v.pickEditEnabled {
		return false
	}
	idx := v.nearestPickIndexAt(pos, pickHitRadiusPixels)
	if idx < 0 {
		return false
	}
	v.picks = append(v.picks[:idx], v.picks[idx+1:]...)
	v.refreshPickLayer()
	v.notifyPicksChanged()
	return true
}

func (v *EnsembleImageView) nearestPickIndexAt(pos fyne.Position, maxDist float64) int {
	if !v.picksVisible || len(v.picks) == 0 {
		return -1
	}
	best := -1
	bestSq := maxDist * maxDist
	for i := range v.picks {
		sx, sy, ok := v.pickToScreen(v.picks[i])
		if !ok {
			continue
		}
		dx := float64(pos.X - sx)
		dy := float64(pos.Y - sy)
		d2 := dx*dx + dy*dy
		if d2 <= bestSq {
			bestSq = d2
			best = i
		}
	}
	return best
}

func (v *EnsembleImageView) pickAtScreen(pos fyne.Position) (EnsemblePick, bool) {
	imgX, imgW, imgH, ok := v.plotRect()
	if !ok || v.viewMaxMS <= v.viewMinMS {
		return EnsemblePick{}, false
	}
	if pos.X < imgX || pos.X >= imgX+imgW || pos.Y < 0 || pos.Y >= imgH {
		return EnsemblePick{}, false
	}
	xFrac := float64(pos.X-imgX) / float64(imgW)
	yFrac := float64(pos.Y) / float64(imgH)
	if xFrac < 0 {
		xFrac = 0
	}
	if xFrac > 1 {
		xFrac = 1
	}
	if yFrac < 0 {
		yFrac = 0
	}
	if yFrac > 1 {
		yFrac = 1
	}
	return EnsemblePick{Velocity: v.velocityAtFrac(xFrac), TimeMS: v.viewMinMS + yFrac*(v.viewMaxMS-v.viewMinMS)}, true
}

func (v *EnsembleImageView) pickToScreen(p EnsemblePick) (float32, float32, bool) {
	imgX, imgW, imgH, ok := v.plotRect()
	if !ok || v.viewMaxMS <= v.viewMinMS {
		return 0, 0, false
	}
	xFrac := v.fracAtVelocity(p.Velocity)
	yFrac := (p.TimeMS - v.viewMinMS) / (v.viewMaxMS - v.viewMinMS)
	if xFrac < 0 || xFrac > 1 || yFrac < 0 || yFrac > 1 {
		return 0, 0, false
	}
	sx := imgX + float32(xFrac)*imgW
	sy := float32(yFrac) * imgH
	return sx, sy, true
}

func (v *EnsembleImageView) refreshPickLayer() {
	if v.pickLayer == nil {
		return
	}
	v.pickLayer.ClearPolylines()
	v.pickLayer.Points = v.pickLayer.Points[:0]
	v.visiblePickIndices = v.visiblePickIndices[:0]

	if !v.picksVisible || len(v.picks) == 0 {
		v.pickLayer.Refresh()
		v.highlightedPickIdx = -1
		v.refreshHighlightLayer()
		return
	}

	imgX, _, _, ok := v.plotRect()
	if !ok {
		v.pickLayer.Refresh()
		v.highlightedPickIdx = -1
		v.refreshHighlightLayer()
		return
	}
	// The overlay layer is already positioned over the image area in Layout,
	// so its projection must be local to that layer (origin at 0,0).
	v.pickLayer.SetProjection(layermanager.NewProjection(fyne.NewPos(0, 0), fyne.NewPos(0, 0), 1))

	polyPoints := make([]layermanager.Point, 0, len(v.picks))
	for i, pick := range v.picks {
		sx, sy, ok := v.pickToScreen(pick)
		if !ok {
			continue
		}
		p := layermanager.NewPoint(float64(sx-imgX), -float64(sy))
		v.pickLayer.Points = append(v.pickLayer.Points, p)
		v.visiblePickIndices = append(v.visiblePickIndices, i)
		polyPoints = append(polyPoints, p)
	}

	if len(polyPoints) >= 2 {
		v.pickLayer.AddPolyline(layermanager.NewPolyline(polyPoints))
	}
	v.pickLayer.Refresh()
	v.refreshHighlightLayer()
}

func (v *EnsembleImageView) updatePickHighlight(pos fyne.Position) {
	if !v.picksVisible || !v.pickEditEnabled {
		v.highlightedPickIdx = -1
		v.refreshHighlightLayer()
		return
	}
	pickIdx := v.nearestPickIndexAt(pos, pickHitRadiusPixels)
	if pickIdx < 0 {
		v.highlightedPickIdx = -1
		v.refreshHighlightLayer()
		return
	}
	v.highlightedPickIdx = pickIdx
	v.refreshHighlightLayer()
}

func (v *EnsembleImageView) refreshHighlightLayer() {
	if v.highlightLayer == nil {
		return
	}
	v.highlightLayer.ClearPolylines()
	v.highlightLayer.Points = v.highlightLayer.Points[:0]
	if !v.picksVisible || v.highlightedPickIdx < 0 || v.highlightedPickIdx >= len(v.picks) {
		v.highlightLayer.Refresh()
		return
	}

	imgX, _, _, ok := v.plotRect()
	if !ok {
		v.highlightLayer.Refresh()
		return
	}
	v.highlightLayer.SetProjection(layermanager.NewProjection(fyne.NewPos(0, 0), fyne.NewPos(0, 0), 1))
	sx, sy, ok := v.pickToScreen(v.picks[v.highlightedPickIdx])
	if !ok {
		v.highlightLayer.Refresh()
		return
	}
	v.highlightLayer.Points = append(v.highlightLayer.Points, layermanager.NewPoint(float64(sx-imgX), -float64(sy)))
	v.highlightLayer.Refresh()
}

func (v *EnsembleImageView) plotRect() (imgX, imgW, imgH float32, ok bool) {
	size := v.Size()
	imgX = 0
	imgW = size.Width
	if v.showTimeAxis {
		imgX = v.axisWidth
		if imgX <= 0 {
			imgX = 64
		}
		if imgX > size.Width {
			imgX = size.Width
		}
		imgW = size.Width - imgX
	}
	imgH = size.Height
	ok = imgW > 0 && imgH > 0
	return
}

func (v *EnsembleImageView) velocityAtFrac(frac float64) float64 {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}

	axis := v.pickVelAxis
	if len(axis) >= 2 {
		pos := frac * float64(len(axis)-1)
		i := int(math.Floor(pos))
		if i < 0 {
			i = 0
		}
		if i >= len(axis)-1 {
			return axis[len(axis)-1]
		}
		w := pos - float64(i)
		return axis[i]*(1-w) + axis[i+1]*w
	}

	if v.pickVelMax <= v.pickVelMin {
		return 0
	}
	return v.pickVelMin + frac*(v.pickVelMax-v.pickVelMin)
}

func (v *EnsembleImageView) fracAtVelocity(velocity float64) float64 {
	axis := v.pickVelAxis
	if len(axis) >= 2 {
		if velocity <= axis[0] {
			return 0
		}
		last := len(axis) - 1
		if velocity >= axis[last] {
			return 1
		}
		for i := 0; i < last; i++ {
			v0 := axis[i]
			v1 := axis[i+1]
			if (velocity >= v0 && velocity <= v1) || (velocity <= v0 && velocity >= v1) {
				dv := v1 - v0
				if dv == 0 {
					return float64(i) / float64(last)
				}
				w := (velocity - v0) / dv
				return (float64(i) + w) / float64(last)
			}
		}
	}

	if v.pickVelMax <= v.pickVelMin {
		return 0
	}
	return (velocity - v.pickVelMin) / (v.pickVelMax - v.pickVelMin)
}

func cropToView(src image.Image, fullMinMS, fullMaxMS, viewMinMS, viewMaxMS float64) image.Image {
	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}

	fullSpan := fullMaxMS - fullMinMS
	if fullSpan <= 0 {
		return src
	}
	f0 := (viewMinMS - fullMinMS) / fullSpan
	f1 := (viewMaxMS - fullMinMS) / fullSpan
	if f0 < 0 {
		f0 = 0
	}
	if f1 > 1 {
		f1 = 1
	}
	if f1 < f0 {
		f0, f1 = f1, f0
	}

	y0 := b.Min.Y + int(math.Floor(f0*float64(h)))
	y1 := b.Min.Y + int(math.Ceil(f1*float64(h)))
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	if y1 <= y0 {
		y1 = y0 + 1
		if y1 > b.Max.Y {
			y0 = b.Max.Y - 1
			y1 = b.Max.Y
		}
	}

	if sub, ok := src.(interface {
		SubImage(r image.Rectangle) image.Image
	}); ok {
		return sub.SubImage(image.Rect(b.Min.X, y0, b.Max.X, y1))
	}

	out := image.NewRGBA(image.Rect(0, 0, w, y1-y0))
	for y := y0; y < y1; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x-b.Min.X, y-y0, src.At(x, y))
		}
	}
	return out
}
