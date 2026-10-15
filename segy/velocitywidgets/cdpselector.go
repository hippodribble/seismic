package velocitywidgets

import (
	"fmt"
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// CDPSelectorOptions controls default behavior and appearance.
type CDPSelectorOptions struct {
	StepValues       []int
	DefaultStep      int
	MaxEntryOptions  int
	MinInputWidth    float32
	ShowPreviewLabel bool
}

// CDPSelector is a reusable control for selecting CDP values using
// step entry, slider, and direct numeric entry.
type CDPSelector struct {
	Root         *fyne.Container
	StepEntry    *widget.SelectEntry
	Slider       *widget.Slider
	CDPEntry     *widget.SelectEntry
	PreviewLabel *widget.Label

	rangeMin int
	rangeMax int
	ready    bool

	maxEntryOptions int
	suppressEvents  bool

	onPreview func(cdp int)
	onCommit  func(cdp int)
}

// NewCDPSelector creates a new CDP selector widget bundle.
func NewCDPSelector(opts CDPSelectorOptions) *CDPSelector {
	defaults := normalizeOptions(opts)
	s := &CDPSelector{maxEntryOptions: defaults.MaxEntryOptions}

	stepStrings := make([]string, 0, len(defaults.StepValues))
	for _, v := range defaults.StepValues {
		stepStrings = append(stepStrings, strconv.Itoa(v))
	}

	s.StepEntry = widget.NewSelectEntry(stepStrings)
	s.StepEntry.SetText(strconv.Itoa(defaults.DefaultStep))
	s.StepEntry.Validator = validatePositiveInteger
	s.StepEntry.OnChanged = func(_ string) {
		if s.suppressEvents {
			return
		}
		s.applySliderStepFromEntry()
		s.rebuildCDPEntryOptions()
		if s.ready {
			s.commitSelection(s.currentSliderCDP())
		}
	}

	s.Slider = widget.NewSlider(0, 1)
	s.Slider.Step = float64(defaults.DefaultStep)
	s.Slider.Disable()
	s.Slider.OnChanged = func(value float64) {
		if s.suppressEvents || !s.ready {
			return
		}
		cdp := s.snapCDPToStep(int(math.Round(value)))
		s.setPreview(cdp, defaults.ShowPreviewLabel)
		if s.onPreview != nil {
			s.onPreview(cdp)
		}
	}
	s.Slider.OnChangeEnded = func(value float64) {
		if s.suppressEvents || !s.ready {
			return
		}
		cdp := s.snapCDPToStep(int(math.Round(value)))
		s.withSuppressedEvents(func() {
			s.Slider.SetValue(float64(cdp))
			s.CDPEntry.SetText(strconv.Itoa(cdp))
		})
		s.setPreview(cdp, false)
		s.commitSelection(cdp)
	}

	s.CDPEntry = widget.NewSelectEntry(nil)
	s.CDPEntry.Disable()
	s.CDPEntry.Validator = s.validateCDPInRange
	s.CDPEntry.OnChanged = func(text string) {
		if s.suppressEvents || !s.ready {
			return
		}
		v, err := strconv.Atoi(text)
		if err != nil {
			return
		}
		if v < s.rangeMin || v > s.rangeMax {
			return
		}
		s.withSuppressedEvents(func() {
			s.Slider.SetValue(float64(v))
		})
		s.commitSelection(v)
	}

	s.PreviewLabel = widget.NewLabel("")
	s.PreviewLabel.Alignment = fyne.TextAlignCenter
	s.PreviewLabel.Hide()

	stepSized := StackOverMinSizeRect(s.StepEntry, defaults.MinInputWidth, 2)
	cdpSized := StackOverMinSizeRect(s.CDPEntry, defaults.MinInputWidth, 2)
	sliderOverlay := container.NewCenter(s.PreviewLabel)
	sliderMinRect := canvas.NewRectangle(color.Transparent)
	sliderMinRect.SetMinSize(fyne.NewSize(300, 10))
	sliderWithOverlay := container.NewStack(sliderMinRect, s.Slider, sliderOverlay)
	sliderBox := container.NewBorder(nil, nil, widget.NewLabel("CDP"), nil, sliderWithOverlay)

	s.Root = container.NewBorder(
		nil,
		nil,
		container.NewHBox(widget.NewLabel("Step"), stepSized),
		cdpSized,
		sliderBox,
	)

	return s
}

// CanvasObject returns the root widget for placement in containers.
func (s *CDPSelector) CanvasObject() fyne.CanvasObject {
	return s.Root
}

// SetRange applies a valid CDP range and enables controls.
func (s *CDPSelector) SetRange(minCDP, maxCDP int) {
	if minCDP > maxCDP {
		minCDP, maxCDP = maxCDP, minCDP
	}
	s.rangeMin = minCDP
	s.rangeMax = maxCDP

	step := s.currentStepValue()
	s.withSuppressedEvents(func() {
		s.Slider.Min = float64(s.rangeMin)
		s.Slider.Max = float64(s.rangeMax)
		s.Slider.Step = float64(step)
		s.Slider.SetValue(float64(s.rangeMin))
		s.Slider.Enable()
		s.CDPEntry.SetText(strconv.Itoa(s.rangeMin))
		s.CDPEntry.Enable()
	})
	if s.PreviewLabel != nil {
		s.setPreview(s.rangeMin, false)
	}

	s.ready = true
	s.rebuildCDPEntryOptions()
	s.commitSelection(s.rangeMin)
}

// Disable clears readiness and disables controls.
func (s *CDPSelector) Disable() {
	s.ready = false
	s.withSuppressedEvents(func() {
		s.Slider.Disable()
		s.CDPEntry.Disable()
	})
	s.setPreview(0, false)
}

// SetOnPreview registers a callback for slider preview updates.
func (s *CDPSelector) SetOnPreview(fn func(cdp int)) {
	s.onPreview = fn
}

// SetOnCommit registers a callback for committed CDP changes.
func (s *CDPSelector) SetOnCommit(fn func(cdp int)) {
	s.onCommit = fn
}

// Value returns the current CDP value as shown in the CDP entry.
func (s *CDPSelector) Value() (int, error) {
	if s.CDPEntry == nil {
		return 0, fmt.Errorf("cdp entry is nil")
	}
	v, err := strconv.Atoi(s.CDPEntry.Text)
	if err != nil {
		return 0, fmt.Errorf("invalid cdp entry: %w", err)
	}
	return v, nil
}

// Select programmatically selects a CDP and commits it.
func (s *CDPSelector) Select(cdp int) error {
	if !s.ready {
		return fmt.Errorf("cdp selector is not ready")
	}
	if cdp < s.rangeMin || cdp > s.rangeMax {
		return fmt.Errorf("cdp %d is out of range [%d, %d]", cdp, s.rangeMin, s.rangeMax)
	}
	s.withSuppressedEvents(func() {
		s.Slider.SetValue(float64(cdp))
		s.CDPEntry.SetText(strconv.Itoa(cdp))
	})
	s.setPreview(cdp, false)
	s.commitSelection(cdp)
	return nil
}

func (s *CDPSelector) commitSelection(cdp int) {
	if s.onCommit != nil {
		s.onCommit(cdp)
	}
}

func (s *CDPSelector) currentSliderCDP() int {
	if s.Slider == nil {
		return 0
	}
	return s.snapCDPToStep(int(math.Round(s.Slider.Value)))
}

func (s *CDPSelector) applySliderStepFromEntry() {
	if s.Slider == nil {
		return
	}
	step := s.currentStepValue()
	if step <= 0 {
		return
	}
	s.withSuppressedEvents(func() {
		s.Slider.Step = float64(step)
		if s.ready {
			cdp := s.snapCDPToStep(int(math.Round(s.Slider.Value)))
			s.Slider.SetValue(float64(cdp))
			s.CDPEntry.SetText(strconv.Itoa(cdp))
		}
	})
}

func (s *CDPSelector) rebuildCDPEntryOptions() {
	if s.CDPEntry == nil || !s.ready {
		return
	}
	step := s.currentStepValue()
	if step <= 0 {
		step = 10
	}

	count := ((s.rangeMax - s.rangeMin) / step) + 1
	if count < 0 {
		count = 0
	}
	if count > s.maxEntryOptions {
		s.CDPEntry.SetOptions(nil)
		return
	}

	options := make([]string, 0, count)
	for cdp := s.rangeMin; cdp <= s.rangeMax; cdp += step {
		options = append(options, strconv.Itoa(cdp))
	}
	if len(options) == 0 || options[len(options)-1] != strconv.Itoa(s.rangeMax) {
		options = append(options, strconv.Itoa(s.rangeMax))
	}
	s.CDPEntry.SetOptions(options)
}

func (s *CDPSelector) currentStepValue() int {
	if s.StepEntry == nil {
		return 10
	}
	v, err := strconv.Atoi(s.StepEntry.Text)
	if err != nil || v <= 0 {
		return 10
	}
	return v
}

func (s *CDPSelector) snapCDPToStep(v int) int {
	if !s.ready {
		return v
	}
	if v < s.rangeMin {
		v = s.rangeMin
	}
	if v > s.rangeMax {
		v = s.rangeMax
	}
	step := s.currentStepValue()
	if step <= 1 {
		return v
	}
	offset := v - s.rangeMin
	snapped := s.rangeMin + int(math.Round(float64(offset)/float64(step)))*step
	if snapped < s.rangeMin {
		snapped = s.rangeMin
	}
	if snapped > s.rangeMax {
		snapped = s.rangeMax
	}
	return snapped
}

func (s *CDPSelector) withSuppressedEvents(fn func()) {
	s.suppressEvents = true
	defer func() { s.suppressEvents = false }()
	fn()
}

func (s *CDPSelector) setPreview(cdp int, visible bool) {
	if s.PreviewLabel == nil {
		return
	}
	if cdp > 0 {
		s.PreviewLabel.SetText(fmt.Sprintf("CDP %d", cdp))
	} else {
		s.PreviewLabel.SetText("")
	}
	if visible {
		s.PreviewLabel.Show()
	} else {
		s.PreviewLabel.Hide()
	}
}

func (s *CDPSelector) validateCDPInRange(text string) error {
	if text == "" {
		return fmt.Errorf("required")
	}
	v, err := strconv.Atoi(text)
	if err != nil {
		return fmt.Errorf("must be an integer")
	}
	if !s.ready {
		return nil
	}
	if v < s.rangeMin || v > s.rangeMax {
		return fmt.Errorf("must be between %d and %d", s.rangeMin, s.rangeMax)
	}
	return nil
}

func validatePositiveInteger(text string) error {
	if text == "" {
		return fmt.Errorf("required")
	}
	v, err := strconv.Atoi(text)
	if err != nil {
		return fmt.Errorf("must be an integer")
	}
	if v <= 0 {
		return fmt.Errorf("must be > 0")
	}
	return nil
}

func normalizeOptions(opts CDPSelectorOptions) CDPSelectorOptions {
	out := opts
	if len(out.StepValues) == 0 {
		out.StepValues = []int{10, 20, 50, 100}
	}
	if out.DefaultStep <= 0 {
		out.DefaultStep = 10
	}
	if out.MaxEntryOptions <= 0 {
		out.MaxEntryOptions = 5000
	}
	if out.MinInputWidth <= 0 {
		out.MinInputWidth = 100
	}
	if !out.ShowPreviewLabel {
		// false is intentionally allowed; no default override.
	}
	return out
}

// StackOverMinSizeRect stacks an object over a transparent rectangle used as
// a layout width/height anchor.
func StackOverMinSizeRect(obj fyne.CanvasObject, minWidth, minHeight float32) *fyne.Container {
	pad := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 0})
	pad.SetMinSize(fyne.NewSize(minWidth, minHeight))
	return container.NewStack(pad, obj)
}
