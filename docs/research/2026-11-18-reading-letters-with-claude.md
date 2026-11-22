# Reading a letter with Claude: cost and quality, first look

2026-11-18. A spike, not a decision. One letter, one run per cell.

## Question

What does it cost to have a Claude model transcribe a scanned letter and
propose its people, dates and events, and do the models differ enough in
quality to matter when there are many letters to do?

## Method

Item `2j` (record `k`): a one-penny letter card, one sheet, two sides,
scanned at 600 ppi on the FF-680W. Handwritten in ink on the front;
address, imprinted stamp and two postmarks on the back.

Each model was given both sides as JPEGs (quality 85), each side
labelled, and asked for the text of each side line for line and a list
of claims (sender, recipient, dates, people, places, events) each with
its source and a confidence. The matrix:

- four models: Haiku 5.5, Sonnet 5.5, Opus 5.5, Fable 5.2;
- two image sizes: long edge 1568 px and 2576 px;
- with and without two lines of hints from the archivist: that the
  papers are the Evans family's of Lovesgrove, and that "Jim Jack" is
  John James Evans.

Effort was `medium` throughout. Scores:

- tokens as the API reported them, and cost at list prices of
  2026-11-03 (dollars per million tokens in/out: Haiku 0.10/0.50,
  Sonnet 2/10, Opus 4/20, Fable 8/40); thinking is billed as output;
- word error rate of the front's text against a reference reading,
  ignoring case, punctuation and editorial marks;
- how many of eight facts a regular expression found anywhere in the
  reading: the year, the posting date, "funeral", Victoria, Windsor,
  Roberts, the Kaiser by name, Edward VII by name.

## Results

| Model | Long edge | Hints | Tokens in | Tokens out | Cost $ | Seconds | Front WER | Facts found |
|---|---|---|---|---|---|---|---|---|
| Haiku 5.5 | 1568 | no | 5639 | 3087 | 0.0021 | 14 | 5.0% | 6/8 |
| Haiku 5.5 | 1568 | yes | 5705 | 2991 | 0.0021 | 14 | 2.5% | 6/8 |
| Haiku 5.5 | 2576 | no | 9799 | 3651 | 0.0028 | 19 | 3.7% | 4/8 |
| Haiku 5.5 | 2576 | yes | 9865 | 2844 | 0.0024 | 14 | 1.9% | 5/8 |
| Sonnet 5.5 | 1568 | no | 5639 | 880 | 0.0201 | 8 | 3.1% | 5/8 |
| Sonnet 5.5 | 1568 | yes | 5705 | 968 | 0.0211 | 7 | 1.9% | 5/8 |
| Sonnet 5.5 | 2576 | no | 9799 | 902 | 0.0286 | 7 | 1.9% | 6/8 |
| Sonnet 5.5 | 2576 | yes | 9865 | 930 | 0.0290 | 10 | 1.9% | 5/8 |
| Opus 5.5 | 1568 | no | 5639 | 2291 | 0.0684 | 22 | 0.6% | 7/8 |
| Opus 5.5 | 1568 | yes | 5705 | 2726 | 0.0773 | 25 | 1.9% | 8/8 |
| Opus 5.5 | 2576 | no | 9799 | 2586 | 0.0909 | 26 | 2.5% | 8/8 |
| Opus 5.5 | 2576 | yes | 9865 | 2549 | 0.0904 | 24 | 0.6% | 8/8 |
| Fable 5.2 | 1568 | no | 5639 | 2476 | 0.1442 | 23 | 1.9% | 6/8 |
| Fable 5.2 | 1568 | yes | 5705 | 2141 | 0.1313 | 19 | 2.5% | 8/8 |
| Fable 5.2 | 2576 | no | 9799 | 2178 | 0.1655 | 21 | 0.6% | 8/8 |
| Fable 5.2 | 2576 | yes | 9865 | 2409 | 0.1753 | 26 | 1.9% | 8/8 |

The sixteen calls cost about $1.05.

## What it shows

**The text does not separate the models.** Every reading was within one
to eight words of the reference over about 160. That is inside the
reference's own error (below), so the word error rate ranks nothing
here. Of the four readings compared line by line, Haiku's was the
weakest in form: it ran the lines together, dropped an "&" and restored
a struck word.

**The postmarks do.** The card was posted at Windsor on 5 February 1901
and received at Llanbadarn Fawr on the 6th. The posting date was found
in all 4 Opus runs and all 4 Fable runs, and in no Sonnet run. One
Haiku run matched, falsely: it gave the 5th as the day received. The
Haiku and Sonnet readings opened gave dates two or three days out, with
medium or low confidence stated.

**Every run named the event**, Queen Victoria's funeral procession,
without help.

**The larger image cost about 4,000 more input tokens and bought
nothing visible** on this letter.

**The hints changed the sender, not the text.** Without them the sender
was "a son, signing Jim Jack" or just "Jack"; with them, John James
Evans in the readings checked.

**Cost per one-sheet letter**, roughly: Haiku $0.002, Sonnet $0.02,
Opus $0.08, Fable $0.15. The Batch API is priced at half these for work
that can wait; it was not tried.

## Limits

- One letter, in one clear hand, and one run per cell: differences of a
  word or two, or of one fact, are noise.
- The reference was read by Fable 5.2 and not corrected by a person, so
  it favours Fable. It has at least one error: "the same way to see the
  Procession" where the writer wrote "was".
- The facts check is a regular expression over the whole reading; it
  does not tell a right date from the same digits in a wrong claim, and
  the dates above were confirmed by reading only four of the sixteen.
- Effort was not varied, and nothing harder was tried: no faded ink, no
  crossed writing, no second language.

## Where it leaves us

Good enough to build on. Any of the four can transcribe a letter like
this one; the choice of model turns on the claims, the postmark dates
above all, and on price. Opus 5.5 at 1568 px is the working candidate:
it read the dates at about half Fable's price. That is a starting point
to be checked against harder letters once readings are being made in
earnest, not a finding.

The spike's code and reference are in `spike/reading/` (its own module);
its readings were written under `staging/spike-reading/2j/`.
