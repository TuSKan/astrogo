package cams

import (
	"context"
	"testing"
)

// TestAttributeReadersSeparateAbsenceFromFailure is the fix for #172.
//
// # What was wrong
//
// The three attribute readers treated every error from ReadAttribute as
// "attribute absent":
//
//	v, err := ds.ReadAttribute(name)
//	if err != nil {
//	    return 0, false, nil // absence is expected, not an error
//	}
//
// That is true of one of the three paths ReadAttribute can fail on. It also
// fails when the object header cannot be read at all — a corrupt or truncated
// file — and when the attribute is present but undecodable. Both arrive as a
// dynamic fmt.Errorf with no sentinel, so the readers reported a broken file as
// a file that simply omits its optional attributes, and the reader carried on
// with defaults.
//
// # How the failure is provoked
//
// By closing the file and reading through the still-live dataset handle, which
// makes the header read fail for real rather than through a mock. The hdf5
// package returns []*core.Attribute from an internal package, so the dependency
// cannot be faked behind an interface — the type cannot be named here.
func TestAttributeReadersSeparateAbsenceFromFailure(t *testing.T) {
	bucket, key := synthFixture(t)

	f, err := Open(context.Background(), bucket, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	ds, ok := f.vars["aermr01"]
	if !ok {
		t.Fatal("the fixture no longer carries an aermr01 variable")
	}

	// Present: the value comes back.
	if got, err := readStringAttribute(ds, "units"); err != nil || got != "kg kg**-1" {
		t.Fatalf(`readStringAttribute("units") = %q, %v; want "kg kg**-1", nil`, got, err)
	}

	// Absent: empty and no error, which is the case the old code was right
	// about and must keep being right about.
	if got, err := readStringAttribute(ds, "no_such_attribute"); err != nil || got != "" {
		t.Errorf(`readStringAttribute(absent) = %q, %v; want "", nil`, got, err)
	}

	if _, ok, err := readInt32Attribute(ds, "no_such_attribute"); err != nil || ok {
		t.Errorf("readInt32Attribute(absent) = ok %v, err %v; want false, nil", ok, err)
	}

	if _, ok, err := readFloat64Attribute(ds, "no_such_attribute"); err != nil || ok {
		t.Errorf("readFloat64Attribute(absent) = ok %v, err %v; want false, nil", ok, err)
	}

	// Now make the header unreadable. Every reader must report a failure, not
	// absence — this is the assertion the old code fails.
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := readStringAttribute(ds, "units"); err == nil {
		t.Error("readStringAttribute reported an unreadable header as an absent " +
			"attribute. A corrupt file then reads as one that merely omits its " +
			"optional attributes, and the reader carries on with defaults.")
	}

	if _, ok, err := readInt32Attribute(ds, "units"); err == nil {
		t.Errorf("readInt32Attribute reported an unreadable header as absence (ok=%v)", ok)
	}

	if _, ok, err := readFloat64Attribute(ds, "units"); err == nil {
		t.Errorf("readFloat64Attribute reported an unreadable header as absence (ok=%v)", ok)
	}
}
