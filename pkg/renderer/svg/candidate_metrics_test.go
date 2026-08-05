package svg

import (
	"context"
	"io"
	"testing"

	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestMeasureCandidateReportsSerializedStructure(t *testing.T) {
	r := New(renderer.DefaultConfig(), WithAnimation(AnimationSMIL))
	metrics, err := r.MeasureCandidate(context.Background(), experimentalRecording())
	if err != nil {
		t.Fatal(err)
	}
	if metrics.FinalBytes == 0 || metrics.XMLNodes == 0 || metrics.TextNodes == 0 ||
		metrics.GroupNodes == 0 || metrics.AnimationNodes == 0 || metrics.AnimatedElements == 0 {
		t.Fatalf("incomplete candidate metrics: %#v", metrics)
	}
	if metrics.XMLNodes != metrics.DefinitionNodes+metrics.ActiveNodes {
		t.Fatalf("nodes = %d, definitions %d + active %d", metrics.XMLNodes,
			metrics.DefinitionNodes, metrics.ActiveNodes)
	}
	if metrics.MaxTranslatedWidth <= 0 || metrics.MaxTranslatedArea <= 0 {
		t.Fatalf("translated surface metrics: %#v", metrics)
	}
}

func TestCandidateWriterCountsTagsSplitAcrossWrites(t *testing.T) {
	metrics := CandidateMetrics{}
	w := candidateWriter{w: io.Discard, metrics: &metrics}
	for _, part := range [][]byte{[]byte("<svg><de"), []byte("fs><g/>"), []byte("</defs><text>x</text></svg>")} {
		if _, err := w.Write(part); err != nil {
			t.Fatal(err)
		}
	}
	if metrics.XMLNodes != 4 || metrics.DefinitionNodes != 2 || metrics.ActiveNodes != 2 ||
		metrics.GroupNodes != 1 || metrics.TextNodes != 1 {
		t.Fatalf("split metrics = %#v", metrics)
	}
	if metrics.FinalBytes != int64(len("<svg><defs><g/></defs><text>x</text></svg>")) {
		t.Fatalf("bytes = %d", metrics.FinalBytes)
	}
}

func TestCandidateWriterCountsAnimatedElementsOnce(t *testing.T) {
	metrics := CandidateMetrics{}
	w := candidateWriter{w: io.Discard, metrics: &metrics}
	_, _ = w.Write([]byte(`<svg><g><animate/><animateTransform/></g></svg>`))
	w.finish()
	if metrics.AnimationNodes != 2 || metrics.AnimatedElements != 1 {
		t.Fatalf("animation metrics = %#v", metrics)
	}
}

func TestCandidateWriterDistinguishesAnimatedSiblings(t *testing.T) {
	metrics := CandidateMetrics{}
	w := candidateWriter{w: io.Discard, metrics: &metrics}
	_, _ = w.Write([]byte(`<svg><g><animate/></g><g><animate/></g></svg>`))
	w.finish()
	if metrics.AnimatedElements != 2 {
		t.Fatalf("animated elements = %d", metrics.AnimatedElements)
	}
}
