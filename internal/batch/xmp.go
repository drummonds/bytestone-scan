package batch

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

// xmp is the packet embedded in a master: only what is fixed at capture,
// since a master never changes after it.
func xmp(p Photo, side string) []byte {
	esc := func(s string) string {
		var b strings.Builder
		xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	var settings strings.Builder
	for _, s := range p.Front.Settings {
		fmt.Fprintf(&settings, "<rdf:li>%s</rdf:li>", esc(s))
	}
	// box is where b lies in the master, in its pixels.
	box := func(name string, b find.Box) string {
		m := b.InImage(p.Front.Bounds, p.Front.MasterPPI)
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
   <xmp:CreateDate>` + p.Front.Captured.UTC().Format(time.RFC3339) + `</xmp:CreateDate>
   <xmp:CreatorTool>bytestone-scan</xmp:CreatorTool>
   <bs:side>` + side + `</bs:side>
   <bs:batch>` + esc(p.Batch) + `</bs:batch>
   <bs:scanner>` + esc(p.Front.Scanner) + `</bs:scanner>
   <bs:scanPPI>` + fmt.Sprint(p.Front.ScanPPI) + `</bs:scanPPI>
   <bs:masterPPI>` + fmt.Sprint(p.Front.MasterPPI) + `</bs:masterPPI>
   <bs:averaging>` + fmt.Sprint(p.Front.Averaging) + `</bs:averaging>
   <bs:settings><rdf:Bag>` + settings.String() + `</rdf:Bag></bs:settings>
` + box("print", p.Front.Print) + box("region", p.Front.Region) + `  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="r"?>`)
}
