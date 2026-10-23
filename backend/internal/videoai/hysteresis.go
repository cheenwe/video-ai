package videoai

import "time"

// hysteresis turns per-frame rule verdicts into trigger/recover decisions
// using the deployment's trigger/recover hold windows. State is in-memory:
// a process restart re-arms it, which is acceptable because active events
// live in the database and are merged by event_key.
type hysteresis struct {
	triggerHold time.Duration
	recoverHold time.Duration

	active      bool
	activeRule  string
	activeLevel string // near | cross

	pendingRule  string
	pendingLevel string
	pendingSince time.Time

	normalSince time.Time
	hasNormal   bool
}

func newHysteresis(triggerHoldSec, recoverHoldSec float64) *hysteresis {
	if triggerHoldSec < 0 {
		triggerHoldSec = 0
	}
	if recoverHoldSec < 0 {
		recoverHoldSec = 0
	}
	return &hysteresis{
		triggerHold: time.Duration(triggerHoldSec * float64(time.Second)),
		recoverHold: time.Duration(recoverHoldSec * float64(time.Second)),
	}
}

// hysteresisDecision tells the runtime what to do with the current verdict.
type hysteresisDecision struct {
	// Trigger: the violation held long enough (or escalated) — create/merge event.
	Trigger bool
	// Recover: a previously active violation stayed normal long enough — close events.
	Recover bool
	// Escalated: active violation worsened (near → cross); old event must be
	// closed before merging the new rule code.
	Escalated bool
	Level     string
	Rule      string
}

func levelRank(level string) int {
	if level == ResultCross {
		return 2
	}
	return 1
}

// Update feeds one verdict. Unknown/offline frames advance nothing: they
// neither confirm a pending trigger nor count toward recovery.
func (h *hysteresis) Update(result, ruleCode string, now time.Time) hysteresisDecision {
	switch result {
	case ResultNear, ResultCross:
		h.hasNormal = false
		if h.active {
			if ruleCode != h.activeRule || levelRank(result) > levelRank(h.activeLevel) {
				old := hysteresisDecision{Trigger: true, Recover: true, Escalated: true, Level: result, Rule: ruleCode}
				h.activeRule, h.activeLevel = ruleCode, result
				return old
			}
			// Still violating with the same rule: keep refreshing the active event.
			return hysteresisDecision{Trigger: true, Level: h.activeLevel, Rule: h.activeRule}
		}
		if h.pendingLevel == "" {
			h.pendingLevel, h.pendingRule, h.pendingSince = result, ruleCode, now
		} else if levelRank(result) > levelRank(h.pendingLevel) {
			// keep original pendingSince; worst level wins when it fires
			h.pendingLevel, h.pendingRule = result, ruleCode
		}
		if now.Sub(h.pendingSince) >= h.triggerHold {
			h.active, h.activeRule, h.activeLevel = true, h.pendingRule, h.pendingLevel
			h.pendingLevel, h.pendingRule = "", ""
			return hysteresisDecision{Trigger: true, Level: h.activeLevel, Rule: h.activeRule}
		}
		return hysteresisDecision{}
	case ResultNormal:
		h.pendingLevel, h.pendingRule = "", ""
		if !h.active {
			h.hasNormal, h.normalSince = false, time.Time{}
			return hysteresisDecision{}
		}
		if !h.hasNormal {
			h.hasNormal, h.normalSince = true, now
		}
		if now.Sub(h.normalSince) >= h.recoverHold {
			h.active = false
			h.activeRule, h.activeLevel = "", ""
			h.hasNormal, h.normalSince = false, time.Time{}
			return hysteresisDecision{Recover: true}
		}
		return hysteresisDecision{Level: "", Rule: ""}
	default: // unknown / offline
		h.pendingLevel, h.pendingRule = "", ""
		h.hasNormal, h.normalSince = false, time.Time{}
		return hysteresisDecision{}
	}
}
