package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/plosson/agentio-go/internal/testbox"
)

// withHelpWidth makes every help print at width, terminal or not.
func withHelpWidth(t *testing.T, width int) {
	saved := helpWidth
	helpWidth = func(io.Writer) int { return width }
	t.Cleanup(func() { helpWidth = saved })
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
