package batch

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

// xmp is the packet embedded in the master of one side of p: only what is
// fixed at capture, since a master never changes after it.
func xmp(p Photo, side string) []byte {
	sd := p.side(side)
	esc := func(s string) string {
		var b strings.Builder
		xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	var settings strings.Builder
	for _, s := range sd.Settings {
		fmt.Fprintf(&settings, "<rdf:li>%s</rdf:li>", esc(s))
	}
	// box is where b lies in the master, in its pixels.
	box := func(name string, b find.Box) string {
		m := b.InImage(sd.Bounds, sd.MasterPPI)
		return fmt.Sprintf(`   <bs:%s rdf:parseType="Resource">
    <bs:angle>%.2f</bs:angle>
    <bs:centreX>%.1f</bs:centreX>
    <bs:centreY>%.1f</bs:centreY>
    <bs:width>%.1f</bs:width>
    <bs:height>%.1f</bs:height>
   </bs:%[1]s>
`, name, m.Angle, m.CX, m.CY, m.W, m.H)
	}
	return []byte(`<?xpacket begin="` + "\ufeff" + `" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:xmp="http://ns.adobe.com/xap/1.0/"
    xmlns:bs="https://bytestone.uk/ns/scan/1.0/">
   <dc:identifier>` + p.ID.String() + `</dc:identifier>
   <dc:rights>` + esc(p.Credit) + `</dc:rights>
   <xmp:CreateDate>` + sd.Captured.UTC().Format(time.RFC3339) + `</xmp:CreateDate>
   <xmp:CreatorTool>bytestone-scan</xmp:CreatorTool>
   <bs:side>` + side + `</bs:side>
   <bs:batch>` + esc(p.Batch) + `</bs:batch>
   <bs:load>` + fmt.Sprint(p.Load) + `</bs:load>
   <bs:scanner>` + esc(sd.Scanner) + `</bs:scanner>
   <bs:scanPPI>` + fmt.Sprint(sd.ScanPPI) + `</bs:scanPPI>
   <bs:masterPPI>` + fmt.Sprint(sd.MasterPPI) + `</bs:masterPPI>
   <bs:averaging>` + fmt.Sprint(sd.Averaging) + `</bs:averaging>
   <bs:settings><rdf:Bag>` + settings.String() + `</rdf:Bag></bs:settings>
` + box("print", sd.Print) + box("region", sd.Region) + `  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="r"?>`)
}
