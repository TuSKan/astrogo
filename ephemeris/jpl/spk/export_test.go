package spk

// WithRecordCache is r with the record cache NewReader gives every Reader it
// builds — for the package's tests, whose fixtures are Reader literals and so
// evaluate uncached, to compare the two paths on the same bytes.
func WithRecordCache(r *Reader) *Reader {
	return &Reader{F: r.F, FileRec: r.FileRec, records: new(recordCache)}
}
