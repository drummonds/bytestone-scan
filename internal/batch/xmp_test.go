package batch

import (
	"encoding/xml"
	"math"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

type xmpBox struct {
	Angle   float64 `xml:"https://bytestone.uk/ns/scan/1.0/ angle"`
	CentreX float64 `xml:"https://bytestone.uk/ns/scan/1.0/ centreX"`
	CentreY float64 `xml:"https://bytestone.uk/ns/scan/1.0/ centreY"`
	Width   float64 `xml:"https://bytestone.uk/ns/scan/1.0/ width"`
	Height  float64 `xml:"https://bytestone.uk/ns/scan/1.0/ height"`
}

// readXMPBoxes reads the print and region from a master's XMP packet.
func readXMPBoxes(t *testing.T, packet []byte) (print, region *xmpBox) {
	t.Helper()
	var doc struct {
		Desc struct {
			Print  *xmpBox `xml:"https://bytestone.uk/ns/scan/1.0/ print"`
			Region *xmpBox `xml:"https://bytestone.uk/ns/scan/1.0/ region"`
		} `xml:"RDF>Description"`
	}
	if err := xml.Unmarshal(packet, &doc); err != nil {
		t.Fatalf("XMP does not parse: %v\n%s", err, packet)
	}
	return doc.Desc.Print, doc.Desc.Region
}

// Story 009: the master records where the print and its margin lie in it,
// in its own pixels, so it can be straightened without the sidecar.
func TestTheMastersXMPSaysWhereThePrintLies(t *testing.T) {
	region := find.Box{CX: 100, CY: 80, W: 131, H: 105.6, Angle: -3}
	print := find.Box{CX: 100, CY: 80, W: 127, H: 101.6, Angle: -3}
	bounds := region.Bounds()
	cases := []struct {
		name          string
		print, region find.Box
		bounds        find.Rect
	}{
		{"turned", print, region, bounds},
		{"square", find.Box{CX: 50, CY: 40, W: 40, H: 30}, find.Box{CX: 50, CY: 40, W: 44, H: 34}, find.Rect{X0: 28, Y0: 23, X1: 72, Y1: 57}},
	}
	for _, tc := range cases {
		side := Side{Print: tc.print, Region: tc.region, Bounds: tc.bounds, MasterPPI: 600}
		gotPrint, gotRegion := readXMPBoxes(t, xmp(Photo{Front: side}, "front"))
		for _, c := range []struct {
			what string
			got  *xmpBox
			box  find.Box
		}{{"print", gotPrint, tc.print}, {"region", gotRegion, tc.region}} {
			if c.got == nil {
				t.Errorf("%s: XMP has no %s", tc.name, c.what)
				continue
			}
			want := c.box.InImage(tc.bounds, 600)
			if math.Abs(c.got.Angle-want.Angle) > 0.005 || math.Abs(c.got.CentreX-want.CX) > 0.05 || math.Abs(c.got.CentreY-want.CY) > 0.05 ||
				math.Abs(c.got.Width-want.W) > 0.05 || math.Abs(c.got.Height-want.H) > 0.05 {
				t.Errorf("%s: XMP %s = %+v, want %+v in master pixels", tc.name, c.what, *c.got, want)
			}
		}
	}
}
