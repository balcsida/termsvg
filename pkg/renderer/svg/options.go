package svg

import (
	"fmt"
	"time"
)

// LayoutMode selects how changing terminal content is arranged in the SVG.
type LayoutMode string

// AnimationMode selects the SVG animation mechanism.
type AnimationMode string

// FrameSwitchMode selects how discrete content states are activated.
type FrameSwitchMode string

// AutoObjective selects how automatic layout or switching candidates are compared.
type AutoObjective string

// StyleMode selects the SVG paint encoding.
type StyleMode string

// PrimitiveMode selects snapshot or retained SVG primitive emission.
type PrimitiveMode string

// Options contains SVG-specific renderer settings.
type Options struct {
	Layout        LayoutMode
	Animation     AnimationMode
	FrameSwitch   FrameSwitchMode
	AutoObjective AutoObjective
	Style         StyleMode
	Primitives    PrimitiveMode
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
	// LayoutRegions builds independently animated two-dimensional viewports.
	LayoutRegions LayoutMode = "regions"
	// LayoutScroll converts strictly proven upward-scrolling bands to clipped
	// vertical tapes and leaves every other band on the lossless snapshot path.
	LayoutScroll LayoutMode = "scroll"
	// LayoutAuto compares frames, bands, regions, and semantically eligible scroll
	// candidates. It is opt-in because candidate preparation has an extra cost.
	LayoutAuto LayoutMode = "auto"

	// AnimationCSS emits CSS keyframes.
	AnimationCSS AnimationMode = "css"
	// AnimationSMIL emits discrete SVG animation elements.
	AnimationSMIL AnimationMode = "smil"

	// FrameSwitchTranslate places states in a translated strip.
	FrameSwitchTranslate FrameSwitchMode = "translate"
	// FrameSwitchHref animates one use element between state definitions.
	FrameSwitchHref FrameSwitchMode = "href"
	// FrameSwitchAuto compares translation and href under SMIL.
	FrameSwitchAuto FrameSwitchMode = "auto"

	AutoObjectiveSize    AutoObjective = "size"
	AutoObjectiveRuntime AutoObjective = "runtime"

	StyleLegacy StyleMode = "legacy"
	StyleAuto   StyleMode = "auto"

	PrimitiveSnapshots  PrimitiveMode = "snapshots"
	PrimitiveRectTracks PrimitiveMode = "rect-tracks"
)

func (o Options) usesLocalViewports() bool {
	return o.Layout == LayoutBands || o.Layout == LayoutRegions || o.Layout == LayoutScroll
}

// DefaultOptions returns the compatibility-preserving SVG defaults.
func DefaultOptions() Options {
	return Options{
		Layout:        LayoutFrames,
		Animation:     AnimationCSS,
		FrameSwitch:   FrameSwitchTranslate,
		AutoObjective: AutoObjectiveSize,
		Style:         StyleLegacy,
		Primitives:    PrimitiveSnapshots,
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

// WithFrameSwitch selects translated strips, animated hrefs, or SMIL comparison.
func WithFrameSwitch(frameSwitch FrameSwitchMode) Option {
	return func(options *Options) { options.FrameSwitch = frameSwitch }
}

// WithMaxFPS enables opt-in lossy timeline sampling. Zero keeps all states.
func WithMaxFPS(maxFPS int) Option {
	return func(options *Options) { options.MaxFPS = maxFPS }
}

// WithAutoObjective selects size or structural runtime proxy comparison for automatic layout or switching.
func WithAutoObjective(objective AutoObjective) Option {
	return func(options *Options) { options.AutoObjective = objective }
}

// WithStyleMode selects the compatibility or profitability-driven paint encoding.
func WithStyleMode(style StyleMode) Option {
	return func(options *Options) { options.Style = style }
}

// WithPrimitiveMode selects snapshot or retained rectangle emission.
func WithPrimitiveMode(primitives PrimitiveMode) Option {
	return func(options *Options) { options.Primitives = primitives }
}

func (o Options) normalized() Options {
	if o.Layout == "" {
		o.Layout = LayoutFrames
	}
	if o.Animation == "" {
		o.Animation = AnimationCSS
	}
	if o.FrameSwitch == "" {
		o.FrameSwitch = FrameSwitchTranslate
	}
	if o.AutoObjective == "" {
		o.AutoObjective = AutoObjectiveSize
	}
	if o.Style == "" {
		o.Style = StyleLegacy
	}
	if o.Primitives == "" {
		o.Primitives = PrimitiveSnapshots
	}
	return o
}

// Validate checks SVG-specific options.
func (o Options) Validate() error {
	o = o.normalized()
	switch o.Layout {
	case LayoutFrames, LayoutBands, LayoutRegions, LayoutScroll, LayoutAuto:
	default:
		return fmt.Errorf("unsupported SVG layout %q", o.Layout)
	}
	switch o.Animation {
	case AnimationCSS, AnimationSMIL:
	default:
		return fmt.Errorf("unsupported SVG animation mode %q", o.Animation)
	}
	switch o.FrameSwitch {
	case FrameSwitchTranslate, FrameSwitchHref, FrameSwitchAuto:
	default:
		return fmt.Errorf("unsupported SVG frame switch mode %q", o.FrameSwitch)
	}
	switch o.AutoObjective {
	case AutoObjectiveSize, AutoObjectiveRuntime:
	default:
		return fmt.Errorf("unsupported SVG auto objective %q", o.AutoObjective)
	}
	switch o.Style {
	case StyleLegacy, StyleAuto:
	default:
		return fmt.Errorf("unsupported SVG style mode %q", o.Style)
	}
	switch o.Primitives {
	case PrimitiveSnapshots, PrimitiveRectTracks:
	default:
		return fmt.Errorf("unsupported SVG primitive mode %q", o.Primitives)
	}
	if o.Primitives == PrimitiveRectTracks && o.Animation != AnimationSMIL {
		return fmt.Errorf("SVG rectangle tracks require SMIL animation")
	}
	if o.Primitives == PrimitiveRectTracks && o.Layout != LayoutRegions && o.Layout != LayoutScroll {
		return fmt.Errorf("SVG rectangle tracks require regions or scroll layout")
	}
	if o.AutoObjective != AutoObjectiveSize && o.Layout != LayoutAuto && o.FrameSwitch != FrameSwitchAuto {
		return fmt.Errorf("SVG auto objective %q requires auto layout or frame switching", o.AutoObjective)
	}
	if o.FrameSwitch == FrameSwitchHref && o.Animation != AnimationSMIL {
		return fmt.Errorf("SVG href frame switching requires SMIL animation")
	}
	if o.FrameSwitch == FrameSwitchAuto && o.Animation != AnimationSMIL {
		return fmt.Errorf("SVG automatic frame switching requires SMIL animation")
	}
	if o.MaxFPS < 0 {
		return fmt.Errorf("max SVG FPS must not be negative: %d", o.MaxFPS)
	}
	if o.MaxFPS > int(time.Second) {
		return fmt.Errorf("max SVG FPS is too large: %d", o.MaxFPS)
	}
	return nil
}
