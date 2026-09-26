package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/plosson/agentio-go/internal/golden"
	"github.com/plosson/agentio-go/internal/plugins"
	"github.com/plosson/agentio-go/internal/testbox"
	"github.com/spf13/cobra"
)

// helpWidths are the widths the help is compared at: 80 (no terminal), a
// narrow one where some descriptions no longer wrap, and a wide terminal.
var helpWidths = []int{80, 50, 132}

// notYetPorted are Bun command groups Go does not have yet. The goldens are
// captured without those Go still lacks when they are regenerated, and a test
// fails once Go has one of them, until they are regenerated with it.
var notYetPorted = []string{"update"}

// missingGroups are the notYetPorted groups the Go tree lacks.
func missingGroups() []string {
	have := map[string]bool{}
	for _, c := range NewRoot(plugins.Default).Commands() {
		have[c.Name()] = true
	}
	missing := []string{}
	for _, name := range notYetPorted {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

// wantNoExcludedGroup fails t when Go now has a group the goldens left out.
func wantNoExcludedGroup(t *testing.T, excluded []string) {
	t.Helper()
	have := map[string]bool{}
	for _, c := range NewRoot(plugins.Default).Commands() {
		have[c.Name()] = true
	}
	for _, name := range excluded {
		if have[name] {
			t.Errorf("Go now has %q, which the goldens leave out: regenerate them (make golden BUN_REPO=…)", name)
		}
	}
}

// bunProgram builds Bun's command tree: its catalog minus telegram (out of
// scope), without the groups named in $SKIP.
const bunProgram = `
import { createProgram } from './src/cli';
import { PluginRegistry } from './src/plugins/plugin-registry';
import { SERVICE_PLUGINS } from './src/plugins/registry';
const program = createProgram(new PluginRegistry(SERVICE_PLUGINS.filter((p) => p.id !== 'telegram')));
for (const name of JSON.parse(process.env.SKIP)) {
  const i = program.commands.findIndex((c) => c.name() === name);
  if (i < 0) throw new Error('Bun has no ' + name);
  program.commands.splice(i, 1);
}
`

// Bun's help, as --help prints it, for every command of its tree at each
// width. The external plugin tooling (plugin) is out of scope: only the
// root's line for it is kept.
const bunHelpTree = bunProgram + `
const widths = JSON.parse(process.env.WIDTHS);
const out = [];
const walk = (cmd, path) => {
  const help = {};
  for (const w of widths) {
    let text = '';
    cmd.configureOutput({ writeOut: (s) => { text += s; }, writeErr: (s) => { text += s; }, getOutHelpWidth: () => w, getErrHelpWidth: () => w });
    cmd.outputHelp();
    help[w] = text;
  }
  out.push({
    path,
    helpCommand: !!cmd._getHelpCommand(),
    errorHelp: cmd.commands.length > 0 && !cmd._actionHandler && !cmd._defaultCommandName,
    help,
  });
  for (const sub of cmd.commands) if (!(path.length === 0 && sub.name() === 'plugin')) walk(sub, [...path, sub.name()]);
};
walk(program, []);
await Bun.write(process.env.OUT, JSON.stringify(out));
`

type bunHelpEntry struct {
	Path        []string `json:"path"`
	HelpCommand bool     `json:"helpCommand"`
	ErrorHelp   bool     `json:"errorHelp"`
	// Help is keyed by width; a width whose help is the one at 80 is left out.
	Help map[string]string `json:"help"`
}

// bunHelpGolden is testdata/bun/help.json.
type bunHelpGolden struct {
	// Excluded are the notYetPorted groups left out of the capture.
	Excluded []string       `json:"excluded"`
	Tree     []bunHelpEntry `json:"tree"`
}

// runBun runs a script in the Bun checkout and returns what it wrote to $OUT
// (a large write to a stdout pipe can come back garbled from Bun). Only the
// golden regeneration calls it.
func runBun(t *testing.T, script string, env ...string) []byte {
	t.Helper()
	outFile := filepath.Join(t.TempDir(), "out.json")
	cmd := golden.Bun(t, append([]string{"OUT=" + outFile}, env...), "-e", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("bun: %v\n%s", err, stderr.String())
	}
	out, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// withHelpWidth makes every help print at width, terminal or not.
func withHelpWidth(t *testing.T, width int) {
	saved := helpWidth
	helpWidth = func(io.Writer) int { return width }
	t.Cleanup(func() { helpWidth = saved })
}

// withoutPluginLine drops the root's `plugin` entry: Bun's is the external
// plugin tooling (out of scope), Go's its own catalog listing.
func withoutPluginLine(text string) string {
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "  plugin ") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// Every command's help, however it is asked for, is Bun's byte for byte:
// `<path> --help` and `-h` on stdout with exit 0, `<parent> help <name>` on
// stdout, and a group given no subcommand on stderr with exit 1.
func TestHelpIsBunsForTheWholeTree(t *testing.T) {
	var ref bunHelpGolden
	golden.JSON(t, "help.json", &ref, func(t *testing.T) any {
		widths, _ := json.Marshal(helpWidths)
		skip, _ := json.Marshal(missingGroups())
		var tree []bunHelpEntry
		if err := json.Unmarshal(runBun(t, bunHelpTree, "WIDTHS="+string(widths), "SKIP="+string(skip)), &tree); err != nil {
			t.Fatal(err)
		}
		// A help the same at every width is kept once, at 80.
		for _, e := range tree {
			for w, text := range e.Help {
				if w != "80" && text == e.Help["80"] {
					delete(e.Help, w)
				}
			}
		}
		return bunHelpGolden{Excluded: missingGroups(), Tree: tree}
	})
	tree := ref.Tree
	testbox.Isolate(t)
	wantNoExcludedGroup(t, ref.Excluded)
	byPath := map[string]bunHelpEntry{}
	for _, e := range tree {
		byPath[strings.Join(e.Path, " ")] = e
	}
	compared, identical := 0, 0
	for _, width := range helpWidths {
		withHelpWidth(t, width)
		for _, e := range tree {
			path := strings.Join(e.Path, " ")
			if len(e.Path) > 0 && e.Path[0] == "plugin" {
				continue // external plugins are out of scope
			}
			want, ok := e.Help[strconv.Itoa(width)]
			if !ok {
				want = e.Help["80"]
			}
			type form struct {
				args        []string
				toErr, fail bool
			}
			forms := []form{{args: append(append([]string{}, e.Path...), "--help")}, {args: append(append([]string{}, e.Path...), "-h")}}
			if len(e.Path) > 0 && byPath[strings.Join(e.Path[:len(e.Path)-1], " ")].HelpCommand {
				parent := e.Path[:len(e.Path)-1]
				forms = append(forms, form{args: append(append(append([]string{}, parent...), "help"), e.Path[len(e.Path)-1])})
			}
			if e.ErrorHelp {
				forms = append(forms, form{args: e.Path, toErr: true, fail: true})
			}
			for _, f := range forms {
				compared++
				var out, errOut bytes.Buffer
				code := Execute(plugins.Default, f.args, &out, &errOut, strings.NewReader(""))
				got, other := out.String(), errOut.String()
				if f.toErr {
					got, other = other, got
				}
				wantCode := 0
				if f.fail {
					wantCode = 1
				}
				if path == "" {
					got, want = withoutPluginLine(got), withoutPluginLine(want)
				}
				if code != wantCode || other != "" || got != want {
					t.Errorf("width %d %q: exit %d, other stream %q\n%s", width, f.args, code, other, firstDifference(got, want))
					continue
				}
				identical++
			}
		}
	}
	// And Go has no command Bun lacks, hidden or not (its `plugin list` aside).
	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		if _, ok := byPath[strings.Join(path, " ")]; !ok && !(len(path) > 0 && path[0] == "plugin") {
			t.Errorf("Go has %q, Bun does not", path)
		}
		for _, sub := range c.Commands() {
			walk(sub, append(append([]string{}, path...), sub.Name()))
		}
	}
	walk(NewRoot(plugins.Default), nil)
	t.Logf("%d help outputs compared, %d identical", compared, identical)
	if compared < 1000 {
		t.Fatalf("only %d help outputs compared", compared)
	}
}

// The root shows its help groups in Bun's order, and the footer after it.
func TestRootHelpGroupsAndFooter(t *testing.T) {
	testbox.Isolate(t)
	withHelpWidth(t, 80)
	code, out, errOut := productionRun("--help")
	if code != 0 || errOut != "" {
		t.Fatalf("%d %q", code, errOut)
	}
	services, advanced, setup := strings.Index(out, "\nServices\n"), strings.Index(out, "\nAdvanced\n"), strings.Index(out, "\nSetup\n")
	if services < 0 || !(services < advanced && advanced < setup) || strings.Contains(out, "Commands:") {
		t.Fatalf("groups:\n%s", out)
	}
	if !strings.HasSuffix(out, "\n\nFor agent/LLM usage: run `agentio skill <service>` to dump a full SKILL.md, or `agentio docs` for the machine-readable command index.\n\n") {
		t.Fatalf("footer:\n%q", out[len(out)-200:])
	}
	for _, hidden := range []string{"\n  docs", "\n  skill", "\n  reauth", "\n  setup", "\n  config", "--json"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("root help shows %q", hidden)
		}
	}
}

// The host's --json is a Go extra: hidden from help, so help stays Bun's, and
// not counted as an option in the parent's `[options]` marker.
func TestHostJSONIsHiddenFromHelp(t *testing.T) {
	testbox.Isolate(t)
	for _, args := range [][]string{{"rss", "articles", "--help"}, {"gmail", "list", "--help"}, {"rss", "--help"}} {
		code, out, _ := productionRun(args...)
		if code != 0 || strings.Contains(out, "--json") || strings.Contains(out, "Output structured JSON") {
			t.Fatalf("%v:\n%s", args, out)
		}
	}
	// gchat send declares its own --json [file]: that one is Bun's and shows.
	if _, out, _ := productionRun("gchat", "send", "--help"); !strings.Contains(out, "\n  --json [file]") {
		t.Fatalf("gchat send help lost its own --json:\n%s", out)
	}
}

// boxWrap and formatItem are Commander's, on inputs that stress them: runs of
// spaces at a break, words wider than the column, Unicode and astral text,
// JavaScript-only whitespace, CRLF, blank lines, preformatted text, colour
// escapes, and widths either side of minWidthToWrap.
func TestWrapIsCommanders(t *testing.T) {
	texts := []string{
		"", " ", "   ", "a", "word ", "  leading words here", "a  b   c    d",
		"Recipient (repeatable, required unless --reply-to) (default: [])",
		strings.Repeat("x", 90) + " tail", "short " + strings.Repeat("y", 60) + " z",
		"line one\r\nline two\n\nline four", "keep\n  preformatted\n  lines as they are",
		"tab\tseparated\twords and more words to go past the column width here",
		"日本語のテキスト 日本語のテキスト 日本語のテキスト 日本語のテキスト 日本語のテキスト",
		"wide\u3000ideographic\u3000space and\u00a0no-break\u00a0space in a long enough sentence to wrap",
		strings.Repeat("😀 ", 40), "next\u2028line separator and\u0085NEL and more words to wrap at forty",
		"\x1b[31mred\x1b[0m words " + strings.Repeat("colour ", 12), "trailing spaces at the end of a long line of words   ",
		"a\nb", "\n", "\n\n",
	}
	widths := []int{10, 39, 40, 41, 47, 80}
	terms := []string{"", "-h, --help", "--profile <name>", strings.Repeat("t", 60), "\x1b[1mbold\x1b[0m"}
	type wrapCase struct {
		Text  string `json:"text"`
		Width int    `json:"width"`
		Want  string `json:"want"`
	}
	type itemCase struct {
		Term      string `json:"term"`
		TermWidth int    `json:"termWidth"`
		Text      string `json:"text"`
		Width     int    `json:"width"`
		Want      string `json:"want"`
	}
	var wraps []wrapCase
	var items []itemCase
	for _, text := range texts {
		for _, w := range widths {
			wraps = append(wraps, wrapCase{Text: text, Width: w})
		}
		for _, term := range terms {
			for _, w := range []int{50, 80, 100} {
				items = append(items, itemCase{Term: term, TermWidth: displayWidth(term) + 3, Text: text, Width: w})
			}
		}
	}
	type cases struct {
		Wraps []wrapCase `json:"wraps"`
		Items []itemCase `json:"items"`
	}
	var ref cases
	golden.JSON(t, "wrap.json", &ref, func(t *testing.T) any {
		input, _ := json.Marshal(map[string]any{"wraps": wraps, "items": items})
		script := `
import { Help } from 'commander';
const { wraps, items } = JSON.parse(process.env.CASES);
const help = new Help();
const wrapped = wraps.map((c) => help.boxWrap(c.text, c.width));
const formatted = items.map((c) => { const h = new Help(); h.prepareContext({ helpWidth: c.width }); return h.formatItem(c.term, c.termWidth, c.text, h); });
await Bun.write(process.env.OUT, JSON.stringify({ wrapped, formatted }));
`
		var out struct {
			Wrapped   []string `json:"wrapped"`
			Formatted []string `json:"formatted"`
		}
		if err := json.Unmarshal(runBun(t, script, "CASES="+string(input)), &out); err != nil {
			t.Fatal(err)
		}
		c := cases{Wraps: append([]wrapCase(nil), wraps...), Items: append([]itemCase(nil), items...)}
		for i := range c.Wraps {
			c.Wraps[i].Want = out.Wrapped[i]
		}
		for i := range c.Items {
			c.Items[i].Want = out.Formatted[i]
		}
		return c
	})
	// The golden holds Bun's answer for exactly these inputs.
	if len(ref.Wraps) != len(wraps) || len(ref.Items) != len(items) {
		t.Fatalf("wrap.json has %d+%d cases, the test %d+%d: regenerate the goldens", len(ref.Wraps), len(ref.Items), len(wraps), len(items))
	}
	for i, c := range ref.Wraps {
		if c.Text != wraps[i].Text || c.Width != wraps[i].Width {
			t.Fatalf("wrap.json case %d is for other input: regenerate the goldens", i)
		}
		if got := boxWrap(c.Text, c.Width); got != c.Want {
			t.Errorf("boxWrap(%q, %d)\n got %q\nwant %q", c.Text, c.Width, got, c.Want)
		}
	}
	for i, c := range ref.Items {
		if c.Term != items[i].Term || c.TermWidth != items[i].TermWidth || c.Text != items[i].Text || c.Width != items[i].Width {
			t.Fatalf("wrap.json item %d is for other input: regenerate the goldens", i)
		}
		if got := formatItem(helpItem{c.Term, c.Text}, c.TermWidth, c.Width); got != c.Want {
			t.Errorf("formatItem(%q, %d, %q, %d)\n got %q\nwant %q", c.Term, c.TermWidth, c.Text, c.Width, got, c.Want)
		}
	}
}

// Help goes to the terminal's width, and to 80 columns when it is not one.
func TestHelpWidthIsEightyOffATerminal(t *testing.T) {
	var buf bytes.Buffer
	if got := helpWidth(&buf); got != 80 {
		t.Fatalf("buffer: %d", got)
	}
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if got := helpWidth(f); got != 80 {
		t.Fatalf("file: %d", got)
	}
}
