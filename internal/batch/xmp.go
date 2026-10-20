package batch

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
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
	return []byte(`<?xpacket begin="` + "\ufeff" + `" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:xmp="http://ns.adobe.com/xap/1.0/"
    xmlns:bs="https://bytestone.uk/ns/scan/1.0/">
   <dc:identifier>` + p.ID.String() + `</dc:identifier>
   <xmp:CreateDate>` + p.Front.Captured.UTC().Format(time.RFC3339) + `</xmp:CreateDate>
   <xmp:CreatorTool>bytestone-scan</xmp:CreatorTool>
   <bs:side>` + side + `</bs:side>
   <bs:batch>` + esc(p.Batch) + `</bs:batch>
   <bs:scanner>` + esc(p.Front.Scanner) + `</bs:scanner>
   <bs:scanPPI>` + fmt.Sprint(p.Front.ScanPPI) + `</bs:scanPPI>
   <bs:masterPPI>` + fmt.Sprint(p.Front.MasterPPI) + `</bs:masterPPI>
   <bs:averaging>` + fmt.Sprint(p.Front.Averaging) + `</bs:averaging>
   <bs:settings><rdf:Bag>` + settings.String() + `</rdf:Bag></bs:settings>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="r"?>`)
}
