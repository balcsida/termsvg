package main

import "testing"

func TestMeasure(t *testing.T) {
	svg := []byte(`<svg><style>@keyframes k{0%,50%{transform:translateX(0px)}50%,100%{transform:translateX(-24px)}}</style><defs><filter id="f"/></defs><g style="animation:k 1s"><rect filter="url(#f)"/><use href="#r"/></g></svg>`)

	m, err := measure(svg)
	if err != nil {
		t.Fatal(err)
	}
	if m.RawBytes != len(svg) || m.MinifiedBytes <= 0 || m.GzipBytes <= 0 {
		t.Fatalf("byte sizes = raw %d, minified %d, gzip %d", m.RawBytes, m.MinifiedBytes, m.GzipBytes)
	}
	if m.Elements != 7 || m.Tags["rect"] != 1 || m.Tags["use"] != 1 {
		t.Fatalf("elements/tags = %d %#v", m.Elements, m.Tags)
	}
	if m.FilterAttrs != 1 || m.Keyframes != 1 || m.KeyframeSelectors != 4 || m.DuplicateSelectors != 1 {
		t.Fatalf("filter/keyframes/selectors/duplicates = %d/%d/%d/%d", m.FilterAttrs, m.Keyframes, m.KeyframeSelectors, m.DuplicateSelectors)
	}
	if m.MaxTranslate != 24 || m.AnimatedGroups != 1 {
		t.Fatalf("max translate/animated groups = %g/%d", m.MaxTranslate, m.AnimatedGroups)
	}
}
