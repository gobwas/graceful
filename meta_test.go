package graceful

import (
	"bytes"
	"reflect"
	"testing"
)

func TestMeta(t *testing.T) {
	m1 := Meta{
		"foo": "bar",
		"baz": 1,
	}
	buf := new(bytes.Buffer)
	n, err := m1.WriteTo(buf)
	if err != nil {
		t.Fatal(err)
	}
	if act, exp := int(n), buf.Len(); act != exp {
		t.Fatalf("unexpected number of wrote bytes: %d; want %d", act, exp)
	}

	m2, err := MetaFrom(buf)
	if err != nil {
		t.Fatalf("MetaFrom() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(m1, m2) {
		t.Fatalf("unequal results:\n%+v\n%+v", m1, m2)
	}
}
