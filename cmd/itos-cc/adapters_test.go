package main

import (
	"io"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/features"
)

// TestEveryPortIsBoundOnDiskAndInProcess holds both providers of adapters
// to every port (decision 53): each field of kind interface, func, pointer,
// map or slice is bound by diskAdapters and by inProcessAdapters. It reads
// the fields, not the methods, so a port a method answers from a field, as
// SelectionExclusion answers from Selection, is held too, and a new field
// fails it until both providers bind it. inProcessAdapters alone leaves
// SnapshotReaders unbound: only serve reads it, and the in-process driver
// refuses serve until serve's work.
func TestEveryPortIsBoundOnDiskAndInProcess(t *testing.T) {
	disk := diskAdapters(io.Discard, func() func() { return func() {} })
	m := newMemory(t)
	folder, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	inProcess, _ := m.inProcessAdapters(features.Run{Dir: folder}, io.Discard)
	for _, provider := range []struct {
		name   string
		a      adapters
		exempt map[string]bool
	}{
		{"diskAdapters", disk, nil},
		{"inProcessAdapters", inProcess, map[string]bool{"SnapshotReaders": true}},
	} {
		v := reflect.ValueOf(provider.a)
		for i := range v.NumField() {
			field := v.Type().Field(i)
			switch field.Type.Kind() {
			case reflect.Interface, reflect.Func, reflect.Pointer, reflect.Map, reflect.Slice:
			default:
				continue
			}
			if v.Field(i).IsNil() && !provider.exempt[field.Name] {
				t.Errorf("%s leaves adapters.%s unbound: bind its port there", provider.name, field.Name)
			}
		}
	}
}
