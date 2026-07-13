package debug

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/shared"
)

type Matcher string

const (
	// L matches against any valid level, specified at the beginning of the
	// debug env var, examples:
	// - SYPL_DEBUG="info,componentX:outputY:debug,outputZ:trace" -> `info`
	// - SYPL_DEBUG="componentX:outputY:debug,outputZ:trace,info" -> ``.
	//
	// NOTE: For this matcher, the order matter!
	L Matcher = "Level"

	// OL matches against a specific output, and any valid level specified in
	// the debug env var, example:
	// - SYPL_DEBUG="info,componentX:outputY:debug,outputZ:trace" -> `trace`.
	OL Matcher = "OutputLevel"

	// COL matches against a specific component and output, and any valid level
	// specified in the debug env var, example:
	// - SYPL_DEBUG="info,componentX:outputY:debug,outputZ:trace" -> `debug`.
	COL Matcher = "ComponentOutputLevel"

	// None means no Matcher matched against the debug env var.
	None Matcher = "None"
)

// Matchers' regexes.
//
// NOTE: The env var is a comma-separated list of entries. Each mask matches
// only a COMPLETE entry - anchored at start-or-comma before, and comma-or-end
// after. Otherwise an entry such as "infosvc:console:trace" would leak as the
// bare level "info" (a prefix), or an output named "es" would match an entry
// scoped to "es-backup" (a substring).
const (
	lReMask   = `(?i)^(?:%s)(?:,|$)`
	oLReMask  = `(?i)(?:^|,)(?:%s):(?:%s)(?:,|$)`
	cOLReMask = `(?i)(?:^|,)(?:%s):(?:%s):(?:%s)(?:,|$)`
)

// Debug definition.
type Debug struct {
	// ComponentName is the component name.
	ComponentName string

	// OutputName is the output name.
	OutputName string

	// Content of the debug env var.
	Content string

	// Levels matcher regex matches against any valid level, specified at the
	// beginning of the debug env var, examples:
	// - SYPL_DEBUG="info,componentX:outputY:debug,outputZ:trace" -> `info`
	// - SYPL_DEBUG="componentX:outputY:debug,outputZ:trace,info" -> ``.
	//
	// NOTE: For this matcher, the order matter!
	Levels *regexp.Regexp

	// Output, and levels matcher regex matches against a specific output, and
	// any valid level specified in the debug env var, example:
	// - SYPL_DEBUG="info,componentX:outputY:debug,outputZ:trace" -> `trace`
	OutputLevels *regexp.Regexp

	// COL matches against a specific component and output, and any valid level
	// specified in the debug env var, example:
	// - SYPL_DEBUG="info,componentX:outputY:debug,outputZ:trace" -> `debug`.
	ComponentOutputLevels *regexp.Regexp
}

// MatchL uses the `Levels` matcher against any valid level, specified at the
// beginning of the debug env var, examples:
// - SYPL_DEBUG="info,componentX:outputY:debug,outputZ:trace" -> `info`
// - SYPL_DEBUG="componentX:outputY:debug,outputZ:trace,info" -> ` `.
//
// Notes:
// - For this matcher, the order matter!
// - Prefer to use the `Level` method.
func (d *Debug) MatchL() string {
	return strings.Trim(d.Levels.FindString(d.Content), ",")
}

// MatchOL uses the `OutputLevels` matcher against a specific output, and
// any valid level specified in the debug env var, example:
// - SYPL_DEBUG="info,componentX:outputY:debug,outputZ:trace" -> `trace`
//
// NOTE: Prefer to use the `Level` method.
func (d *Debug) MatchOL() string {
	return strings.Trim(d.OutputLevels.FindString(d.Content), ",")
}

// MatchCOL uses the `ComponentOutputLevels` matcher against a specific
// component and output, and any valid level specified in the debug env var,
// example:
// - SYPL_DEBUG="info,componentX:outputY:debug,outputZ:trace" -> `debug`.
//
// NOTE: Prefer to use the `Level` method.
func (d *Debug) MatchCOL() string {
	return strings.Trim(d.ComponentOutputLevels.FindString(d.Content), ",")
}

// Level checks the content of the debug env var against all matchers returning:
// - The level extracted from the last Matcher
// - The last `Matcher` that matched
// - If any matcher succeeded on matching
//
// Matchers:
// - {componentName:outputName:level} -> forwarder:console:trace
// - {outputName:level} -> console:trace
// - {level}, e.g.: trace
//
// NOTE: Don't use the returned level to check if `Level` succeeded because
// `level.None` is a valid, and usable level.
func (d *Debug) Level() (level.Level, Matcher, bool) {
	// Shouldn't' do anything if the debug env var isn't set.
	if d.Content == "" {
		return level.None, None, false
	}

	var (
		finalMatcher       Matcher
		finalLevelAsString string
	)

	// Matches' order matters.
	if lReMatch := d.MatchL(); lReMatch != "" {
		finalLevelAsString = strings.Split(lReMatch, ":")[0]
		finalMatcher = L
	}

	if oLReMatch := d.MatchOL(); oLReMatch != "" {
		finalLevelAsString = strings.Split(oLReMatch, ":")[1]
		finalMatcher = OL
	}

	if nOLReMatch := d.MatchCOL(); nOLReMatch != "" {
		finalLevelAsString = strings.Split(nOLReMatch, ":")[2]
		finalMatcher = COL
	}

	finalLevel, err := level.FromString(finalLevelAsString)
	if err != nil {
		return level.None, None, false
	}

	return finalLevel, finalMatcher, true
}

//////
// Factory.
//////

// matchers groups the compiled regexes for a component, and output pair.
type matchers struct {
	levels                *regexp.Regexp
	outputLevels          *regexp.Regexp
	componentOutputLevels *regexp.Regexp
}

// maxCachedMatchers bounds `matchersCache`. Component names can be
// dynamically generated (e.g. per-request child loggers), and the cache is
// keyed by them - without a bound it would grow forever. Above the cap,
// matchers are compiled fresh per call - never evicted - which is the
// simplest correct bound.
const maxCachedMatchers = 1024

// matchersCache caches compiled matchers, keyed by component, and output
// names. `New` is called per-message-per-output; the regexes depend only on
// the names - not on the env var content, which is read fresh on every call -
// so they're safe to reuse.
var matchersCache sync.Map

// matchersCacheSize tracks the number of entries in `matchersCache` -
// `sync.Map` has no O(1) length. Concurrent first-seen misses may overshoot
// the cap by a handful of entries; the point is that growth stops.
var matchersCacheSize atomic.Int64

// newMatchers compiles the matchers for a component, and output pair.
func newMatchers(componentName, outputName string) *matchers {
	levels := strings.Join(level.LevelsNames(), "|")

	return &matchers{
		levels:                regexp.MustCompile(fmt.Sprintf(lReMask, levels)),
		outputLevels:          regexp.MustCompile(fmt.Sprintf(oLReMask, outputName, levels)),
		componentOutputLevels: regexp.MustCompile(fmt.Sprintf(cOLReMask, componentName, outputName, levels)),
	}
}

// New is the Debug factory.
func New(componentName, outputName string) *Debug {
	key := componentName + "\x00" + outputName

	cached, ok := matchersCache.Load(key)
	if !ok {
		cached = newMatchers(componentName, outputName)

		// Bounded: above the cap, skip caching, and use the freshly
		// compiled matchers.
		if matchersCacheSize.Load() < maxCachedMatchers {
			actual, loaded := matchersCache.LoadOrStore(key, cached)

			cached = actual

			if !loaded {
				matchersCacheSize.Add(1)
			}
		}
	}

	m, _ := cached.(*matchers)

	return &Debug{
		ComponentName: componentName,
		OutputName:    outputName,

		Content: os.Getenv(shared.LevelEnvVar),

		Levels:                m.levels,
		OutputLevels:          m.outputLevels,
		ComponentOutputLevels: m.componentOutputLevels,
	}
}
