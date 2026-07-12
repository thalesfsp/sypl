package debug

import (
	"fmt"
	"testing"

	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/shared"
)

// The matchers cache is keyed by component+output name, and child loggers
// can be dynamically named - the cache must be bounded (1024 entries), never
// growing forever. Above the cap, matchers are compiled fresh - never
// evicted - and results stay correct.
func TestAudit_DebugCacheBounded(t *testing.T) {
	const bound = 1024

	t.Setenv(shared.LevelEnvVar, "trace")

	cacheLen := func() int {
		n := 0

		matchersCache.Range(func(_, _ any) bool {
			n++

			return true
		})

		return n
	}

	// Simulates dynamically named child loggers, well past the bound.
	for i := 0; i < bound+64; i++ {
		d := New(fmt.Sprintf("audit-bounded-%d", i), "out")

		// Results stay correct whether cached or compiled fresh.
		if l, m, ok := d.Level(); !ok || m != L || l != level.Trace {
			t.Fatalf("entry %d: Level() = (%s, %s, %v), expected (trace, %s, true)", i, l, m, ok, L)
		}
	}

	if got := cacheLen(); got > bound {
		t.Fatalf("cache grew to %d entries, expected a bound of %d", got, bound)
	}

	// Beyond the cap nothing new is stored.
	New("audit-bounded-extra", "out")

	if _, ok := matchersCache.Load("audit-bounded-extra\x00out"); ok {
		t.Fatal("an entry was cached beyond the cap")
	}

	// An over-cap component still matches correctly - including the most
	// specific component:output:level form - via freshly compiled matchers.
	t.Setenv(shared.LevelEnvVar, "audit-bounded-fresh:freshout:debug")

	d := New("audit-bounded-fresh", "freshout")

	if l, m, ok := d.Level(); !ok || m != COL || l != level.Debug {
		t.Fatalf("over-cap Level() = (%s, %s, %v), expected (debug, %s, true)", l, m, ok, COL)
	}

	// Cached entries keep being served (hit path).
	d2 := New("audit-bounded-0", "out")

	if l, _, ok := d2.Level(); ok || l != level.None {
		t.Fatalf("cached-entry Level() = (%s, %v), expected (none, false) for a non-matching env var", l, ok)
	}
}
