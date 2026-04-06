package fits

import (
	"bytes"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func TestReadBintable(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "TFIELDS", Value: "2"})
	h.Append(Card{Keyword: "NAXIS2", Value: "1"}) // 1 row
	h.Append(Card{Keyword: "NAXIS1", Value: "8"}) // 8 bytes per row
	h.Append(Card{Keyword: "TTYPE1", Value: "id"})
	h.Append(Card{Keyword: "TFORM1", Value: "1J"}) // int32
	h.Append(Card{Keyword: "TTYPE2", Value: "flux"})
	h.Append(Card{Keyword: "TFORM2", Value: "1E"}) // float32

	payload := []byte{0, 0, 0, 1, 0, 0, 0, 2} // 8 bytes total
	padded := make([]byte, 2880)
	copy(padded, payload)

	r := bytes.NewReader(padded)
	bt, err := ReadBintable(h, r)
	testutil.AssertNoError(t, err)

	if bt.Cols != 2 {
		t.Errorf("expected 2 columns, got %d", bt.Cols)
	}

	// Read errors
	h2 := NewHeader() // missing TFIELDS
	_, err = ReadBintable(h2, bytes.NewReader(nil))
	if err == nil {
		t.Errorf("expected error for missing TFIELDS")
	}

	// Test Column Extractor with manual dummy batch setup since reader builds empty batches currently:
	mem := memory.NewGoAllocator()
	schema := bt.Batch.Schema()

	bldr := array.NewRecordBuilder(mem, schema)
	defer bldr.Release()

	bldr.Field(0).(*array.Int32Builder).Append(42)
	bldr.Field(1).(*array.Float32Builder).Append(3.14)

	bt.Batch = bldr.NewRecordBatch()

	// Test Extractor
	strCol, err := bt.GetStringColumn("id")
	testutil.AssertNoError(t, err)
	if len(strCol) != 1 || strCol[0] != "42" {
		t.Errorf("expected string '42', got %v", strCol)
	}

	fltCol, err := bt.GetFloatColumn("flux")
	testutil.AssertNoError(t, err)
	if len(fltCol) != 1 || math.Abs(fltCol[0]-float64(float32(3.14))) > 1e-4 {
		t.Errorf("expected float 3.14, got %v", fltCol)
	}

	_, err = bt.GetStringColumn("invalid")
	if err == nil {
		t.Errorf("expected error on invalid column")
	}
	_, err = bt.GetFloatColumn("invalid")
	if err == nil {
		t.Errorf("expected error on invalid column")
	}
}
