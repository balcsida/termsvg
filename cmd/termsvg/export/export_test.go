package export

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mrmarble/termsvg/pkg/ir"
)

type rendererFunc func(io.Writer) error

func (f rendererFunc) Render(_ context.Context, _ *ir.Recording, w io.Writer) error {
	return f(w)
}

func (rendererFunc) Format() string        { return "svg" }
func (rendererFunc) FileExtension() string { return ".svg" }

func TestWriteOutputMinifiesValidSVGAndRestoresSpaces(t *testing.T) {
	var dst bytes.Buffer
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"> <text>a  </text> </svg>`)
		return err
	})

	if err := writeOutput(context.Background(), rdr, nil, &dst, true); err != nil {
		t.Fatalf("writeOutput() error = %v", err)
	}
	if strings.Contains(dst.String(), " ") {
		t.Fatalf("writeOutput() retained NBSP: %q", dst.String())
	}
	if !strings.Contains(dst.String(), ">a  </text>") {
		t.Fatalf("writeOutput() lost terminal spaces: %q", dst.String())
	}
	if err := xml.Unmarshal(dst.Bytes(), new(any)); err != nil {
		t.Fatalf("writeOutput() produced invalid XML: %v\n%s", err, dst.String())
	}
}

func TestNBSPWriterHandlesSplitSequence(t *testing.T) {
	var dst bytes.Buffer
	w := newNBSPWriter(&dst)

	for _, chunk := range [][]byte{{'a', 0xc2}, {0xa0, 'b'}, {0xc2}, {'x'}} {
		if _, err := w.Write(chunk); err != nil {
			t.Fatalf("Write(%q) error = %v", chunk, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got, want := dst.String(), "a b\xc2x"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestWriteOutputPropagatesDestinationError(t *testing.T) {
	wantErr := errors.New("destination failed")
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"><text>`+strings.Repeat("x", 8192)+`</text></svg>`)
		return err
	})

	err := writeOutput(context.Background(), rdr, nil, errorWriter{err: wantErr}, true)
	if !errors.Is(err, wantErr) {
		t.Fatalf("writeOutput() error = %v, want %v", err, wantErr)
	}
}

func TestWriteOutputPropagatesRenderError(t *testing.T) {
	wantErr := errors.New("render failed")
	rdr := rendererFunc(func(io.Writer) error { return wantErr })

	err := writeOutput(context.Background(), rdr, nil, io.Discard, true)
	if !errors.Is(err, wantErr) {
		t.Fatalf("writeOutput() error = %v, want %v", err, wantErr)
	}
}

func TestWriteOutputStreamsLargeSVG(t *testing.T) {
	wantErr := errors.New("renderer received whole-file buffer")
	rdr := rendererFunc(func(w io.Writer) error {
		if _, ok := w.(*bytes.Buffer); ok {
			return wantErr
		}
		if _, err := io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg">`); err != nil {
			return err
		}
		for range 100_000 {
			if _, err := io.WriteString(w, `<text>x</text>`); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</svg>`)
		return err
	})

	if err := writeOutput(context.Background(), rdr, nil, io.Discard, true); err != nil {
		t.Fatalf("writeOutput() error = %v", err)
	}
}

func TestNBSPWriterPropagatesShortWrite(t *testing.T) {
	w := newNBSPWriter(shortWriter{})

	if _, err := w.Write([]byte("x")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Write() error = %v, want %v", err, io.ErrShortWrite)
	}
}

func TestWriteOutputLeavesNonMinifiedBytesUnchanged(t *testing.T) {
	want := []byte(" <svg>\n\t<text>a  b</text>\n</svg> ")
	var dst bytes.Buffer
	rdr := rendererFunc(func(w io.Writer) error {
		_, err := w.Write(want)
		return err
	})

	if err := writeOutput(context.Background(), rdr, nil, &dst, false); err != nil {
		t.Fatalf("writeOutput() error = %v", err)
	}
	if !bytes.Equal(dst.Bytes(), want) {
		t.Fatalf("writeOutput() = %q, want %q", dst.Bytes(), want)
	}
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }

type shortWriter struct{}

func (shortWriter) Write([]byte) (int, error) { return 0, nil }
