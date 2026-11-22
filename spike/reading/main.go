// Spike: what does it cost, and how well does it go, to have each Claude
// model read one archived letter? Throwaway; not part of bytestone-scan.
//
// It fetches a letter's masters from its page on the archive site, scales
// them to each long edge asked for, has each model read them with and
// without the operator's suggestions, and scores each reading: tokens and
// cost as the API reports them, and the word error rate of the front's
// text against a reference reading.
//
//	tp secrets go run . -item https://i.bytestone.org/k/items/2j/ -name 2j
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// Dollars per million tokens, input and output: the Claude API's list
// prices as of 2026-11-03. Thinking is billed as output.
var prices = map[string][2]float64{
	"claude-haiku-5-5":  {0.10, 0.50},
	"claude-sonnet-5-5": {2, 10},
	"claude-opus-5-5":   {4, 20},
	"claude-fable-5-2":  {8, 40},
}

const system = `You transcribe scanned family letters for an archive. You are given every side of every sheet of one letter, each image labelled.

Write the reading in exactly this form:

## <the side's label>
The text of that side, line for line as written, keeping the writer's spelling and punctuation. One such section per side, in the order given.

## Claims
One per line, as "what: value (source; confidence high|medium|low)": the sender, the recipient, where and when it was written or posted, when it was received, and the people, places and events it mentions. Give a date as YYYY-MM-DD when the letter or a postmark supports it. Say what an event is when the text and date identify it. Do not guess a full name the letter does not give.

Conventions: [struck: word] for a deletion, [struck: illegible] when it cannot be read, ^word^ for an insertion above the line, [?] after a doubtful word, [illegible] for one that cannot be read, [printed] or [postmark] before printed or stamped text. Nothing else in square brackets. No commentary outside these sections.`

type side struct {
	label string // "Sheet 1, front"
	tif   string // path of the master
}

type run struct {
	model string
	edge  int
	hints bool

	in, out  int64
	cost     float64
	took     time.Duration
	stop     string
	wer      float64
	facts    string
	textPath string
	err      error
}

func main() {
	item := flag.String("item", "https://i.bytestone.org/k/items/2j/", "the letter's page on the archive site")
	name := flag.String("name", "2j", "reference/<name>.txt, .hints and .facts are read for it")
	models := flag.String("models", "claude-haiku-5-5,claude-sonnet-5-5,claude-opus-5-5,claude-fable-5-2", "models, comma separated")
	edges := flag.String("edges", "1568,2576", "long edges in pixels to scale each side to, comma separated")
	effort := flag.String("effort", "medium", "effort for every model: low, medium, high, xhigh or max")
	work := flag.String("work", "../../staging/spike-reading", "where masters, scaled sides and readings are kept")
	flag.Parse()

	dir := filepath.Join(*work, *name)
	must(os.MkdirAll(dir, 0o755))
	sides := fetch(*item, dir)
	ref := frontText(string(mustRead(filepath.Join("reference", *name+".txt"))), sides[0].label)
	hints := strings.TrimSpace(string(mustRead(filepath.Join("reference", *name+".hints"))))
	facts := readFacts(filepath.Join("reference", *name+".facts"))

	var runs []*run
	for _, m := range strings.Split(*models, ",") {
		if _, ok := prices[m]; !ok {
			log.Fatalf("no price for %s", m)
		}
		for _, e := range strings.Split(*edges, ",") {
			var edge int
			fmt.Sscan(e, &edge)
			for _, h := range []bool{false, true} {
				runs = append(runs, &run{model: m, edge: edge, hints: h})
			}
		}
	}

	client := anthropic.NewClient()
	var wg sync.WaitGroup
	gate := make(chan struct{}, 4)
	for _, r := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			read(client, r, sides, dir, hints, *effort)
			if r.err != nil {
				return
			}
			text := string(mustRead(r.textPath))
			r.wer = wer(words(ref), words(frontText(text, sides[0].label)))
			r.facts = found(facts, text)
		}()
	}
	wg.Wait()

	var b strings.Builder
	fmt.Fprintf(&b, "Letter %s, %d sides, effort %s, %s\n\n", *name, len(sides), *effort, time.Now().Format("2006-01-02 15:04"))
	fmt.Fprintln(&b, "| Model | Long edge | Hints | Tokens in | Tokens out | Cost $ | Seconds | Front WER | Facts found | Stop |")
	fmt.Fprintln(&b, "|---|---|---|---|---|---|---|---|---|---|")
	for _, r := range runs {
		if r.err != nil {
			fmt.Fprintf(&b, "| %s | %d | %v | failed: %v |\n", r.model, r.edge, r.hints, r.err)
			continue
		}
		fmt.Fprintf(&b, "| %s | %d | %v | %d | %d | %.4f | %.0f | %.1f%% | %s | %s |\n",
			r.model, r.edge, r.hints, r.in, r.out, r.cost, r.took.Seconds(), 100*r.wer, r.facts, r.stop)
	}
	must(os.WriteFile(filepath.Join(dir, "results.md"), []byte(b.String()), 0o644))
	fmt.Print(b.String())
}

// fetch downloads the masters the letter's page links to, in page order.
func fetch(page, dir string) []side {
	html := string(get(page))
	re := regexp.MustCompile(`href="(sheet-(\d+)-(front|back)\.tif)"`)
	var sides []side
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		var n int
		fmt.Sscan(m[2], &n)
		s := side{label: fmt.Sprintf("Sheet %d, %s", n, m[3]), tif: filepath.Join(dir, m[1])}
		if _, err := os.Stat(s.tif); err != nil {
			u, err := url.JoinPath(page, m[1])
			must(err)
			must(os.WriteFile(s.tif, get(u), 0o644))
		}
		sides = append(sides, s)
	}
	if len(sides) == 0 {
		log.Fatalf("no masters linked from %s", page)
	}
	return sides
}

// scaled is the side as a JPEG whose long edge is at most edge pixels.
func scaled(s side, edge int) []byte {
	jpg := fmt.Sprintf("%s-%d.jpg", strings.TrimSuffix(s.tif, ".tif"), edge)
	if _, err := os.Stat(jpg); err != nil {
		size := fmt.Sprintf("%dx%d>", edge, edge)
		out, err := exec.Command("convert", s.tif, "-resize", size, "-quality", "85", jpg).CombinedOutput()
		if err != nil {
			log.Fatalf("convert %s: %v: %s", s.tif, err, out)
		}
	}
	return mustRead(jpg)
}

func read(client anthropic.Client, r *run, sides []side, dir, hints, effort string) {
	var content []anthropic.ContentBlockParamUnion
	for _, s := range sides {
		content = append(content,
			anthropic.NewTextBlock(s.label+":"),
			anthropic.NewImageBlockBase64("image/jpeg", base64.StdEncoding.EncodeToString(scaled(s, r.edge))))
	}
	ask := "Read this letter."
	if r.hints {
		ask += "\n\nWhat the archivist already knows:\n" + hints
	}
	content = append(content, anthropic.NewTextBlock(ask))

	start := time.Now()
	// No refusal fallback: another model answering would spoil the score.
	resp, err := client.Messages.New(context.Background(), anthropic.MessageNewParams{
		Model:        r.model,
		MaxTokens:    16000,
		System:       []anthropic.TextBlockParam{{Text: system}},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(effort)},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(content...)},
	})
	r.took = time.Since(start)
	if err != nil {
		r.err = err
		return
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	r.stop = string(resp.StopReason)
	r.in = resp.Usage.InputTokens + resp.Usage.CacheCreationInputTokens + resp.Usage.CacheReadInputTokens
	r.out = resp.Usage.OutputTokens
	p := prices[r.model]
	r.cost = (float64(r.in)*p[0] + float64(r.out)*p[1]) / 1e6
	h := "plain"
	if r.hints {
		h = "hints"
	}
	r.textPath = filepath.Join(dir, fmt.Sprintf("reading-%s-%d-%s.md", r.model, r.edge, h))
	r.err = os.WriteFile(r.textPath, []byte(text.String()), 0o644)
}

// frontText is the body of the section headed by label in a reading.
func frontText(reading, label string) string {
	var body []string
	in := false
	for _, line := range strings.Split(reading, "\n") {
		if strings.HasPrefix(line, "## ") {
			in = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "## ")), label)
			continue
		}
		if in {
			body = append(body, line)
		}
	}
	return strings.Join(body, "\n")
}

var (
	bracketed = regexp.MustCompile(`\[[^\]]*\]`)
	notWord   = regexp.MustCompile(`[^a-z0-9&']+`)
)

// words is the text as the writer left it, for comparison: deletions and
// editorial marks dropped, insertions kept, case and punctuation ignored.
func words(text string) []string {
	text = bracketed.ReplaceAllString(text, " ")
	text = strings.NewReplacer("’", "'", "^", " ").Replace(strings.ToLower(text))
	return strings.Fields(notWord.ReplaceAllString(text, " "))
}

// wer is the word error rate of got against want: edits over want's length.
func wer(want, got []string) float64 {
	if len(want) == 0 {
		return 1
	}
	prev := make([]int, len(got)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(want); i++ {
		cur := make([]int, len(got)+1)
		cur[0] = i
		for j := 1; j <= len(got); j++ {
			sub := prev[j-1]
			if want[i-1] != got[j-1] {
				sub++
			}
			cur[j] = min(sub, prev[j]+1, cur[j-1]+1)
		}
		prev = cur
	}
	return float64(prev[len(got)]) / float64(len(want))
}

// readFacts reads "name<TAB>regexp" lines: things a good reading says.
func readFacts(path string) map[string]*regexp.Regexp {
	facts := map[string]*regexp.Regexp{}
	for _, line := range strings.Split(string(mustRead(path)), "\n") {
		name, re, ok := strings.Cut(line, "\t")
		if ok {
			facts[name] = regexp.MustCompile("(?i)" + re)
		}
	}
	return facts
}

func found(facts map[string]*regexp.Regexp, text string) string {
	var names []string
	for name, re := range facts {
		if re.MatchString(text) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return fmt.Sprintf("%d/%d %s", len(names), len(facts), strings.Join(names, " "))
}

func get(u string) []byte {
	resp, err := http.Get(u)
	must(err)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("%s: %s", u, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	must(err)
	return b
}

func mustRead(path string) []byte {
	b, err := os.ReadFile(path)
	must(err)
	return b
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
