package graceful

import (
	"encoding/gob"
	"io"
)

// Meta is a helper type that is marshaled/unmarshaled by `endoding/gob`
// package and could be used as an additional information for a descriptor.
type Meta map[string]interface{}

func MetaFrom(r io.Reader) (Meta, error) {
	m := make(Meta)
	_, err := m.ReadFrom(r)
	return m, err
}

func (m *Meta) WriteTo(w io.Writer) (int64, error) {
	wc := &writeCounter{W: w}
	enc := gob.NewEncoder(wc)
	return wc.N, enc.Encode(m)
}

func (m *Meta) ReadFrom(r io.Reader) (int64, error) {
	rc := &readCounter{R: r}
	dec := gob.NewDecoder(rc)
	return rc.N, dec.Decode(m)
}

type readCounter struct {
	R io.Reader
	N int64
}

func (r *readCounter) Read(p []byte) (int, error) {
	n, err := r.R.Read(p)
	r.N += int64(n)
	return n, err
}

type writeCounter struct {
	W io.Writer
	N int64
}

func (w *writeCounter) Write(p []byte) (int, error) {
	n, err := w.W.Write(p)
	w.N += int64(n)
	return n, err
}
