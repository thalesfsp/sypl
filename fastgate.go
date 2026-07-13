// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package sypl

import (
	"os"

	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/shared"
	"github.com/thalesfsp/sypl/v2/status"
)

//////
// Fast level gate.
//
// An OPT-IN optimization: when enabled, Print-family calls whose level cannot
// possibly be written by any output return before ANY message, or options
// construction - skipping content formatting, message allocation, and the
// whole pipeline.
//
// Semantics (same contract as slog/zap level gating):
//   - With the gate on, processors CANNOT resurrect a gated-out message - it
//     never enters the pipeline.
//   - Calls carrying options (e.g. `WithFlag(flag.Force)`) always take the
//     slow path - options can alter the message's flag, so the gate cannot
//     know the effective flag without constructing the message.
//   - Fatal is never gated - it must run the full pipeline, and exit.
//   - When the SYPL_LEVEL, or SYPL_FILTER env vars are set - they can raise
//     levels, and filter at runtime - the gate defers to the slow path.
//
// The max effective level is recomputed from the LIVE outputs on every call
// (a handful of uncontended atomic lock operations), so ANY reconfiguration -
// via the Sypl-level setters, or by mutating an output directly - is observed
// immediately. There is no cache to go stale.
//////

// SetFastGate toggles the opt-in fast level gate. Default: disabled - zero
// behavior change.
func (sypl *Sypl) SetFastGate(enabled bool) *Sypl {
	sypl.lock()
	defer sypl.unlock()

	sypl.fastGate = enabled

	return sypl
}

// FastGateEnabled returns whether the fast level gate is enabled.
func (sypl *Sypl) FastGateEnabled() bool {
	sypl.rLock()
	defer sypl.rUnlock()

	return sypl.fastGate
}

// fastGated determines - without constructing a message - whether a
// Print-family call at level `l`, carrying no options, can be dropped
// entirely. See the semantics above.
func (sypl *Sypl) fastGated(l level.Level) bool {
	// A nil logger takes the slow path: `process` owns the nil-receiver
	// contract (log.Fatalf).
	if sypl == nil || !sypl.FastGateEnabled() {
		return false
	}

	// Fatal must always run the full pipeline - it exits the process.
	if l == level.Fatal {
		return false
	}

	// SYPL_LEVEL raises levels, and SYPL_FILTER filters by component name,
	// at runtime - defer to the slow path, which honors both.
	if os.Getenv(shared.LevelEnvVar) != "" || os.Getenv(shared.FilterEnvVar) != "" {
		return false
	}

	// Highest effective max level across the ENABLED outputs, read live.
	maxEnabledLevel := level.None

	for _, o := range sypl.GetOutputs() {
		if o.GetStatus() != status.Enabled {
			continue
		}

		if ml := o.GetMaxLevel(); ml > maxEnabledLevel {
			maxEnabledLevel = ml
		}
	}

	return l > maxEnabledLevel
}
