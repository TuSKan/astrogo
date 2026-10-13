package simbad

import (
	"bytes"
	"testing"
)

// simbadCSVSeeds are this package's CSV fixtures, inlined so the fuzz seeds
// are literals: SIMBAD's TAP answers in the shape ParseCSV and ParseBrightCSV
// read, plus the empty and malformed answers the ordinary tests pin.
var simbadCSVSeeds = []string{
	"oid,main_id,ra,dec,otype,otype_path,id\n" +
		"1,NAME M  31,10.68470833,41.26875,AGN,G > AGN,M31\n" +
		"1,NAME M  31,10.68470833,41.26875,AGN,G > AGN,Andromeda Galaxy\n",
	"oid,main_id,ra,dec,otype,otype_path,pmra,pmdec,plx_value,rvz_radvel,vmag\n" +
		"1,* alf CMa,101.28715,-16.71611,SB*,* > ** > SB*,-546.01,-1223.07,379.21,-5.5,-1.46\n" +
		"2,* alf Car,95.98787,-52.69566,*,*,19.99,23.67,10.43,20.5,-0.74\n",
	"oid,main_id,ra,dec,otype,otype_path,pmra,pmdec,plx_value,rvz_radvel,id,V\n" +
		"2,* zero RV test,10.0,20.0,V*,* > V*,0.0,0.0,10.0,0.0,ZeroRVStar,8.0\n",
	"oid,main_id,ra,dec,otype,id\n",
	"not a real header\n1,2,3,4,foo,bar\n\"unclosed quote\n",
	"",
}

// FuzzParseCSV holds SIMBAD's two CSV readers to never panicking or hanging
// on whatever a TAP response holds (#746). The bytes are CDS's, and a reader
// that indexes a row by a column it found in the header is one short row away
// from reading past it.
func FuzzParseCSV(f *testing.F) {
	for _, s := range simbadCSVSeeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(_ *testing.T, b []byte) {
		_, _ = ParseCSV(bytes.NewReader(b))
		_, _ = ParseBrightCSV(bytes.NewReader(b))
	})
}
