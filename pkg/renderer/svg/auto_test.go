package svg

import "testing"

func TestLayoutAutoValidatesWithoutChangingDefault(t *testing.T) {
	if DefaultOptions().Layout != LayoutFrames {
		t.Fatalf("default layout = %q; want frames", DefaultOptions().Layout)
	}
	options := DefaultOptions()
	options.Layout = LayoutAuto
	if err := options.Validate(); err != nil {
		t.Fatalf("auto layout failed validation: %v", err)
	}
}

func TestCountingWriterCountsAllBytes(t *testing.T) {
	counter := &countingWriter{}
	for _, value := range []string{"abc", "", "defgh"} {
		if _, err := counter.Write([]byte(value)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if counter.size() != 8 {
		t.Fatalf("counted bytes = %d; want 8", counter.size())
	}
}

func TestCountingWriterUsesPostMinifyNBSPWidth(t *testing.T) {
	counter := &countingWriter{collapseNBSP: true}
	if _, err := counter.Write([]byte{'a', 0xc2}); err != nil {
		t.Fatal(err)
	}
	if _, err := counter.Write([]byte{0xa0, 'b'}); err != nil {
		t.Fatal(err)
	}
	if counter.size() != int64(len("a b")) {
		t.Fatalf("counted transformed bytes = %d; want %d", counter.size(), len("a b"))
	}
}
