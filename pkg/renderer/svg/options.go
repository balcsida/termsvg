package svg

import (
	"fmt"
	"time"
)

// LayoutMode selects how changing terminal content is arranged in the SVG.
type LayoutMode string

// AnimationMode selects the SVG animation mechanism.
type AnimationMode string

// Options contains SVG-specific renderer settings.
type Options struct {
	Layout    LayoutMode
	Animation AnimationMode
	// MaxFPS limits SVG timeline samples. Zero preserves every source state.
	MaxFPS int
}

// Option updates SVG renderer options.
type Option func(*Options)

const (
	// LayoutFrames retains the compatibility layout: complete dynamic states are
	// placed in one horizontal strip.
	LayoutFrames LayoutMode = "frames"
	// LayoutBands builds independently animated horizontal strips for adjacent
	// rows that share a change schedule.
	LayoutBands LayoutMode = "bands"

	// AnimationCSS emits CSS keyframes.
	AnimationCSS AnimationMode = "css"
	// AnimationSMIL emits discrete SVG animation elements.
	AnimationSMIL AnimationMode = "smil"
)

// DefaultOptions returns the compatibility-preserving SVG defaults.
func DefaultOptions() Options {
	return Options{
		Layout:    LayoutFrames,
		Animation: AnimationCSS,
	}
}

// WithLayout selects the SVG content layout.
func WithLayout(layout LayoutMode) Option {
	return func(options *Options) { options.Layout = layout }
}

// WithAnimation selects the SVG animation backend.
func WithAnimation(animation AnimationMode) Option {
	return func(options *Options) { options.Animation = animation }
}

// WithMaxFPS enables opt-in lossy timeline sampling. Zero keeps all states.
func WithMaxFPS(maxFPS int) Option {
	return func(options *Options) { options.MaxFPS = maxFPS }
}

func (o Options) normalized() Options {
	if o.Layout == "" {
		o.Layout = LayoutFrames
	}
	if o.Animation == "" {
		o.Animation = AnimationCSS
	}
	return o
}

// Validate checks SVG-specific options.
func (o Options) Validate() error {
	switch o.Layout {
	case LayoutFrames, LayoutBands:
	default:
		return fmt.Errorf("unsupported SVG layout %q", o.Layout)
	}
	switch o.Animation {
	case AnimationCSS, AnimationSMIL:
	default:
		return fmt.Errorf("unsupported SVG animation mode %q", o.Animation)
	}
	if o.MaxFPS < 0 {
		return fmt.Errorf("max SVG FPS must not be negative: %d", o.MaxFPS)
	}
	if o.MaxFPS > int(time.Second) {
		return fmt.Errorf("max SVG FPS is too large: %d", o.MaxFPS)
	}
	return nil
}
