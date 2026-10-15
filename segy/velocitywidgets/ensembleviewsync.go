package velocitywidgets

// EnsembleViewSync keeps multiple EnsembleImageView instances on the same visible time range.
// Apps can use one sync group per logical set of linked ensemble views.
type EnsembleViewSync struct {
	hasRange bool
	minMS    float64
	maxMS    float64
	views    map[*EnsembleImageView]struct{}
}

// NewEnsembleViewSync creates a new empty view sync group.
func NewEnsembleViewSync() *EnsembleViewSync {
	return &EnsembleViewSync{
		views: make(map[*EnsembleImageView]struct{}),
	}
}

// Register adds a view to the sync group and applies the current range if one exists.
func (s *EnsembleViewSync) Register(view *EnsembleImageView) {
	if s == nil || view == nil {
		return
	}
	s.views[view] = struct{}{}
	if s.hasRange {
		view.ApplyViewRange(s.minMS, s.maxMS)
	}
}

// SyncFrom stores the visible range from source and applies it to the other registered views.
func (s *EnsembleViewSync) SyncFrom(source *EnsembleImageView, minMS, maxMS float64) {
	if s == nil {
		return
	}
	s.hasRange = true
	s.minMS = minMS
	s.maxMS = maxMS
	for view := range s.views {
		if view == nil || view == source {
			continue
		}
		view.ApplyViewRange(minMS, maxMS)
	}
}

// ApplyCurrent reapplies the stored range to one view.
func (s *EnsembleViewSync) ApplyCurrent(view *EnsembleImageView) {
	if s == nil || view == nil || !s.hasRange {
		return
	}
	view.ApplyViewRange(s.minMS, s.maxMS)
}