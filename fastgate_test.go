// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/flag"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/message"
	"github.com/thalesfsp/sypl/output"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/shared"
	"github.com/thalesfsp/sypl/status"
)

// clearSyplEnvVars isolates the test from ambient SYPL_LEVEL, and SYPL_FILTER
// values - the gate defers to the slow path when either is set.
func clearSyplEnvVars(t *testing.T) {
	t.Helper()

	t.Setenv(shared.LevelEnvVar, "")
	t.Setenv(shared.FilterEnvVar, "")
}

// recordingProcessor returns a processor that counts its invocations. The
// processor chain runs BEFORE the output's level check, so it observes
// messages the level filter would silently drop - making it the perfect probe
// for "did the message enter the pipeline at all?".
func recordingProcessor(counter *int, mu *sync.Mutex) processor.IProcessor {
	return processor.New("Recorder", func(_ message.IMessage) error {
		mu.Lock()
		defer mu.Unlock()

		*counter++

		return nil
	})
}

// The gate is opt-in: disabled by default, toggleable, chainable.
func TestFastGate_DefaultOffAndToggle(t *testing.T) {
	l := sypl.New("fastgate-toggle")

	if l.FastGateEnabled() {
		t.Fatal("FastGateEnabled() = true on a fresh logger, want false (opt-in)")
	}

	if got := l.SetFastGate(true); got != l {
		t.Fatal("SetFastGate must return the same *Sypl for chaining")
	}

	if !l.FastGateEnabled() {
		t.Fatal("FastGateEnabled() = false after SetFastGate(true)")
	}

	l.SetFastGate(false)

	if l.FastGateEnabled() {
		t.Fatal("FastGateEnabled() = true after SetFastGate(false)")
	}
}

// A gated call must produce zero output AND never enter the pipeline -
// processors cannot resurrect a gated-out message. The negative control is
// the gate-off half: the same call DOES reach the processor chain (and is
// then dropped by the level check).
func TestFastGate_GatedCallSkipsPipeline(t *testing.T) {
	clearSyplEnvVars(t)

	var (
		mu    sync.Mutex
		count int
	)

	buf, o := output.SafeBuffer(level.Info, recordingProcessor(&count, &mu))

	l := sypl.New("fastgate-pipeline", o)

	// Gate OFF (default): Debug against an Info-capped output produces no
	// output, but the processor chain RUNS.
	l.Print(level.Debug, "gate off")

	mu.Lock()
	offCount := count
	mu.Unlock()

	if offCount != 1 {
		t.Fatalf("gate off: processor ran %d times, want 1", offCount)
	}

	if buf.String() != "" {
		t.Fatalf("gate off: unexpected output %q", buf.String())
	}

	// Gate ON: the same call returns before message construction - the
	// processor must NOT run.
	l.SetFastGate(true)

	l.Print(level.Debug, "gate on")
	l.Printf(level.Debug, "gate %s", "on")
	l.Println(level.Debug, "gate on")
	l.Printlnf(level.Debug, "gate %s", "on")
	l.Debug("gate on")

	mu.Lock()
	onCount := count
	mu.Unlock()

	if onCount != offCount {
		t.Fatalf("gate on: processor ran %d more times, want 0", onCount-offCount)
	}

	if buf.String() != "" {
		t.Fatalf("gate on: unexpected output %q", buf.String())
	}

	// Messages at, or below the max level must still flow.
	l.Print(level.Info, "allowed\n")

	if buf.String() != "allowed\n" {
		t.Fatalf("gate on: allowed message produced %q, want %q", buf.String(), "allowed\n")
	}
}

// A gated call must be (nearly) allocation-free.
func TestFastGate_GatedCallAllocs(t *testing.T) {
	clearSyplEnvVars(t)

	_, o := output.SafeBuffer(level.Info)

	l := sypl.New("fastgate-allocs", o)
	l.SetFastGate(true)

	// PrintWithOptions with no options: zero allocations.
	allocs := testing.AllocsPerRun(100, func() {
		l.PrintWithOptions(level.Debug, "gated")
	})

	if allocs != 0 {
		t.Errorf("gated PrintWithOptions allocated %v times per run, want 0", allocs)
	}

	// Print: at most the variadic args slice (escape analysis pins it to the
	// heap because of the non-gated branch).
	allocs = testing.AllocsPerRun(100, func() {
		l.Print(level.Debug, "gated")
	})

	if allocs > 1 {
		t.Errorf("gated Print allocated %v times per run, want <= 1", allocs)
	}
}

// The Force flag must bypass the gate: options can alter the flag, so any
// call carrying options takes the slow path.
// forcedMsg is the payload used by the force-bypass probe.
const forcedMsg = "forced\n"

func TestFastGate_ForceBypassesGate(t *testing.T) {
	clearSyplEnvVars(t)

	buf, o := output.SafeBuffer(level.Info)

	l := sypl.New("fastgate-force", o)
	l.SetFastGate(true)

	l.PrintWithOptions(level.Debug, forcedMsg, sypl.WithFlag(flag.Force))

	if buf.String() != forcedMsg {
		t.Fatalf("forced message produced %q, want %q", buf.String(), forcedMsg)
	}
}

// SYPL_LEVEL can raise levels at runtime: when set, the gate must defer to
// the slow path so the debug capability still works.
func TestFastGate_LevelEnvVarDefers(t *testing.T) {
	t.Setenv(shared.FilterEnvVar, "")
	t.Setenv(shared.LevelEnvVar, "trace")

	buf, o := output.SafeBuffer(level.Info)

	l := sypl.New("fastgate-env", o)
	l.SetFastGate(true)

	l.Println(level.Debug, "resurrected by env var")

	if !strings.Contains(buf.String(), "resurrected by env var") {
		t.Fatalf("SYPL_LEVEL set: message was gated, output %q", buf.String())
	}
}

// SYPL_FILTER filters by component name at runtime: when set, the gate must
// defer to the slow path.
func TestFastGate_FilterEnvVarDefers(t *testing.T) {
	t.Setenv(shared.LevelEnvVar, "")
	t.Setenv(shared.FilterEnvVar, "fastgate-filter")

	var (
		mu    sync.Mutex
		count int
	)

	_, o := output.SafeBuffer(level.Info, recordingProcessor(&count, &mu))

	l := sypl.New("fastgate-filter", o)
	l.SetFastGate(true)

	l.Print(level.Debug, "filter env var set")

	mu.Lock()
	defer mu.Unlock()

	if count != 1 {
		t.Fatalf("SYPL_FILTER set: pipeline ran %d times, want 1 (gate must defer)", count)
	}
}

// Reconfiguration must be observed by the gate: the max effective level is
// recomputed from the live outputs on every call.
func TestFastGate_Reconfiguration(t *testing.T) {
	clearSyplEnvVars(t)

	buf, o := output.SafeBuffer(level.Info)

	l := sypl.New("fastgate-reconf", o)
	l.SetFastGate(true)

	l.Println(level.Debug, "gated")

	if buf.String() != "" {
		t.Fatalf("pre-reconfiguration: unexpected output %q", buf.String())
	}

	// Sypl-level reconfiguration.
	l.SetMaxLevel(level.Debug)

	l.Println(level.Debug, "after SetMaxLevel")

	if !strings.Contains(buf.String(), "after SetMaxLevel") {
		t.Fatalf("after SetMaxLevel: message still gated, output %q", buf.String())
	}

	// Direct output mutation is ALSO observed - the gate reads the live
	// output state on every call.
	o.SetMaxLevel(level.Info)

	l.Println(level.Debug, "re-gated")

	if strings.Contains(buf.String(), "re-gated") {
		t.Fatalf("after direct SetMaxLevel(Info): message not gated, output %q", buf.String())
	}

	o.SetMaxLevel(level.Trace)

	l.Println(level.Trace, "direct mutation")

	if !strings.Contains(buf.String(), "direct mutation") {
		t.Fatalf("after direct SetMaxLevel(Trace): message still gated, output %q", buf.String())
	}

	// AddOutputs raises the effective max level.
	l.SetOutputs(output.New("SafeBuffer", level.None, os.Stderr))

	bufHigh, oHigh := output.SafeBuffer(level.Trace)
	oHigh.SetName("High")

	l.AddOutputs(oHigh)

	l.Println(level.Trace, "via added output")

	if !strings.Contains(bufHigh.String(), "via added output") {
		t.Fatalf("after AddOutputs: message still gated, output %q", bufHigh.String())
	}
}

// A disabled output must not contribute to the gate's max effective level.
func TestFastGate_DisabledOutputIgnored(t *testing.T) {
	clearSyplEnvVars(t)

	bufTrace, oTrace := output.SafeBuffer(level.Trace)
	oTrace.SetName("TraceCapped")
	oTrace.SetStatus(status.Disabled)

	bufInfo, oInfo := output.SafeBuffer(level.Info)
	oInfo.SetName("InfoCapped")

	l := sypl.New("fastgate-disabled", oTrace, oInfo)
	l.SetFastGate(true)

	l.Println(level.Debug, "should be gated")

	if bufTrace.String() != "" || bufInfo.String() != "" {
		t.Fatalf(
			"debug message against disabled-trace + enabled-info outputs printed: trace=%q info=%q",
			bufTrace.String(), bufInfo.String(),
		)
	}

	// Re-enabling the trace output must lift the gate.
	oTrace.SetStatus(status.Enabled)

	l.Println(level.Debug, "not gated anymore")

	if !strings.Contains(bufTrace.String(), "not gated anymore") {
		t.Fatalf("after re-enabling: message still gated, output %q", bufTrace.String())
	}
}

// Fatal is never gated: the full pipeline (and os.Exit(1)) must run even when
// every output would drop the message. Asserted via subprocess re-exec.
func TestFastGate_FatalNotGated(t *testing.T) {
	if os.Getenv("SYPL_TEST_FASTGATE_FATAL") == "1" {
		l := sypl.New("fastgate-fatal", output.New("Discard", level.None, os.Stderr))
		l.SetFastGate(true)

		l.Fatal("fatal is never gated")

		os.Exit(42) // Sentinel: Fatal did not exit.
	}

	//nolint:gosec // Re-running the test binary itself.
	cmd := exec.Command(os.Args[0], "-test.run=TestFastGate_FatalNotGated$")

	cmd.Env = append(os.Environ(),
		"SYPL_TEST_FASTGATE_FATAL=1",
		shared.LevelEnvVar+"=",
		shared.FilterEnvVar+"=",
	)

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError

	if !errors.As(err, &exitErr) {
		t.Fatalf("expected subprocess to exit with an error, got %v (stderr: %s)", err, stderr.String())
	}

	if code := exitErr.ExitCode(); code != 1 {
		t.Errorf("expected exit code 1 (Fatal), got %d (stderr: %s)", code, stderr.String())
	}
}

// Toggling the gate, logging, and reconfiguring concurrently must be
// race-clean.
func TestFastGate_Race(t *testing.T) {
	clearSyplEnvVars(t)

	_, o := output.SafeBuffer(level.Info)

	l := sypl.New("fastgate-race", o)

	const goroutines = 8

	var wg sync.WaitGroup

	wg.Add(goroutines * 3)

	for g := range goroutines {
		go func() {
			defer wg.Done()

			for range 50 {
				l.SetFastGate(true)
				_ = l.FastGateEnabled()
				l.SetFastGate(false)
			}
		}()

		go func() {
			defer wg.Done()

			for j := range 50 {
				l.Printlnf(level.Debug, "g%d-m%d", g, j)
				l.Printlnf(level.Info, "g%d-m%d", g, j)
			}
		}()

		go func() {
			defer wg.Done()

			for range 50 {
				l.SetMaxLevel(level.Debug)
				l.SetMaxLevel(level.Info)
			}
		}()
	}

	wg.Wait()
}
