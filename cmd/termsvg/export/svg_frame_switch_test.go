package export

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/mrmarble/termsvg/pkg/renderer/svg"
)

func TestNormalizedSVGOptionsAcceptsSMILHref(t *testing.T) {
	cmd := Cmd{SVGLayout: "bands", SVGAnimation: "smil", SVGFrameSwitch: "href"}

	options, err := cmd.normalizedSVGOptions("svg")
	if err != nil {
		t.Fatalf("normalizedSVGOptions() error = %v", err)
	}
	if options.Layout != svg.LayoutBands || options.Animation != svg.AnimationSMIL ||
		options.FrameSwitch != svg.FrameSwitchHref {
		t.Fatalf("normalized options = %#v", options)
	}
}

func TestNormalizedSVGOptionsRejectsHrefForNonSVG(t *testing.T) {
	cmd := Cmd{SVGAnimation: "smil", SVGFrameSwitch: "href"}
	if _, err := cmd.normalizedSVGOptions("gif"); err == nil {
		t.Fatal("SVG href option unexpectedly accepted for GIF")
	}
}

func TestNormalizedSVGOptionsAcceptsAutomaticSwitching(t *testing.T) {
	cmd := Cmd{SVGLayout: "bands", SVGAnimation: "smil", SVGFrameSwitch: "auto", SVGAutoObjective: "runtime"}
	options, err := cmd.normalizedSVGOptions("svg")
	if err != nil {
		t.Fatal(err)
	}
	if options.Layout != svg.LayoutBands || options.FrameSwitch != svg.FrameSwitchAuto ||
		options.AutoObjective != svg.AutoObjectiveRuntime {
		t.Fatalf("options = %+v", options)
	}
	cmd.SVGAnimation = "css"
	if _, err := cmd.normalizedSVGOptions("svg"); err == nil {
		t.Fatal("CSS accepted automatic switching")
	}
	cmd.SVGAnimation = "smil"
	if _, err := cmd.normalizedSVGOptions("gif"); err == nil {
		t.Fatal("GIF accepted automatic switching")
	}
}

func TestCLIParsesAutomaticSwitching(t *testing.T) {
	file := filepath.Join(t.TempDir(), "recording.cast")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var cmd Cmd
	parser, err := kong.New(&cmd)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{file, "--svg-layout=bands", "--svg-animation=smil", "--svg-frame-switch=auto"}
	if _, err := parser.Parse(args); err != nil {
		t.Fatal(err)
	}
	options, err := cmd.normalizedSVGOptions("svg")
	if err != nil {
		t.Fatal(err)
	}
	if options.Layout != svg.LayoutBands || options.FrameSwitch != svg.FrameSwitchAuto {
		t.Fatalf("parsed options = %+v", options)
	}
}
