package portfolio

import (
	"fmt"
	"sort"
)

// Balanced factor weights. They sum to 1.0 so a score is directly comparable
// between options, and disk savings deliberately never dominate on their own
// (SPECS §7.5: fit is multi-factor, not disk-first). A policy may select a
// different named tradeoff or override individual weights; see weights.go.
const (
	weightInstalledPresence = 0.10
	weightInUseShare        = 0.30
	weightDuplicationCost   = 0.20
	weightReproducibility   = 0.10
	weightMigrationFriction = 0.20
	weightProjectActivity   = 0.05
	weightPolicyPreference  = 0.05
)

// Friction penalties applied to a migration option. Each has an observable
// trigger in the scan; nothing is charged for a tool being new or unfamiliar.
const (
	frictionBaseMigration    = 0.30
	frictionToolNotInstalled = 0.20
	frictionPnPInUse         = 0.40
	frictionLargePortfolio   = 0.10
	frictionCondaTarget      = 0.30
	frictionCondaInUse       = 0.20
	// largePortfolio is where a migration stops being a single afternoon.
	largePortfolio = 6
)

// scoreOption builds one ranked option from ecosystem signals under the
// resolved policy weights.
func scoreOption(current *signals, tool string, preferred string, weights Weights) Option {
	dominant := current.dominantTool()
	stayPut := tool == dominant && dominant != ""
	_, installed := current.installed[tool]
	inUse := current.inUse[tool]
	if inUse > 0 {
		installed = true
	}

	option := Option{
		Tool: tool, StayPut: stayPut, Installed: installed, ProjectsUsing: inUse,
	}
	estimate := savingsEstimate{}
	if !stayPut {
		estimate = estimateSavings(tool, current.localInstallBytes, current.localInstallCount)
	}
	option.ImmediateSavingsLowBytes = estimate.lowBytes
	option.ImmediateSavingsHighBytes = estimate.highBytes
	option.FutureGrowthReductionBytes = estimate.futureGrowthBytes
	option.SavingsUncertain = estimate.highBytes > 0 || current.uncertainSizes

	friction, blockers := migrationFriction(current, tool, stayPut)
	option.Blockers = blockers
	option.WorkflowImpact = workflowImpact(stayPut, tool, friction)
	option.Factors = factors(current, tool, preferred, stayPut, installed, inUse, estimate, friction, weights)
	option.Score = weightedScore(option.Factors)
	option.DominantFactors = dominantFactors(option.Factors)
	option.Confidence = confidence(current, installed, inUse)
	return option
}

func factors(
	current *signals,
	tool, preferred string,
	stayPut, installed bool,
	inUse int,
	estimate savingsEstimate,
	friction float64,
	weights Weights,
) []Factor {
	projects := current.projectCount()
	inUseShare := share(inUse, projects)
	activeShare := share(current.activeProjects, projects)
	reproducible := share(current.lockfileProjects, projects)
	// Dormant portfolios favour staying put: migrating cold projects spends
	// effort on work nobody is doing. Hot portfolios favour a change paying off.
	activityScore := activeShare
	if stayPut {
		activityScore = 1 - activeShare
	}
	policyScore := 0.0
	if preferred != "" && preferred == tool {
		policyScore = 1
	}
	presence := 0.0
	if installed {
		presence = 1
	}

	return []Factor{
		{Kind: FactorInstalledPresence, Score: presence, Weight: weights.weightFor(FactorInstalledPresence),
			Detail: presenceDetail(installed, inUse)},
		{Kind: FactorInUseShare, Score: inUseShare, Weight: weights.weightFor(FactorInUseShare),
			Detail: fmt.Sprintf("%d of %d detected projects already use %s", inUse, projects, tool)},
		{Kind: FactorDuplicationCost, Score: clamp(estimate.share), Weight: weights.weightFor(FactorDuplicationCost),
			Detail: duplicationDetail(current, estimate)},
		{Kind: FactorReproducibility, Score: reproducible, Weight: weights.weightFor(FactorReproducibility),
			Detail: fmt.Sprintf("%d of %d projects carry a lockfile", current.lockfileProjects, projects)},
		{Kind: FactorMigrationFriction, Score: clamp(1 - friction), Weight: weights.weightFor(FactorMigrationFriction),
			Detail: frictionDetail(stayPut, friction)},
		{Kind: FactorProjectActivity, Score: activityScore, Weight: weights.weightFor(FactorProjectActivity),
			Detail: fmt.Sprintf("%d of %d projects changed recently", current.activeProjects, projects)},
		{Kind: FactorPolicyPreference, Score: policyScore, Weight: weights.weightFor(FactorPolicyPreference),
			Detail: policyDetail(preferred)},
	}
}

// migrationFriction returns a 0..1 penalty plus the blockers that produced it.
func migrationFriction(current *signals, tool string, stayPut bool) (float64, []string) {
	if stayPut {
		return 0, nil
	}
	friction := frictionBaseMigration
	var blockers []string
	if _, installed := current.installed[tool]; !installed && current.inUse[tool] == 0 {
		friction += frictionToolNotInstalled
		blockers = append(blockers, fmt.Sprintf("%s is not installed on this machine; adopting it starts with a new install", tool))
	}
	if current.pnpDetected && tool != "yarn" {
		friction += frictionPnPInUse
		blockers = append(blockers, "Yarn Plug'n'Play resolution is in use; moving away from it requires replacing PnP-aware tooling")
	}
	if current.projectCount() >= largePortfolio {
		friction += frictionLargePortfolio
		blockers = append(blockers, fmt.Sprintf("%d projects would need migrating, so a staged rollout is likely", current.projectCount()))
	}
	if tool == "conda" {
		friction += frictionCondaTarget
		blockers = append(blockers, "Conda environments are not reproducible from a PyPI lockfile alone")
	}
	if current.inUse["conda"] > 0 && tool != "conda" {
		friction += frictionCondaInUse
		blockers = append(blockers, "projects on Conda may depend on non-PyPI binary packages that another tool cannot install")
	}
	return clamp(friction), blockers
}

func weightedScore(items []Factor) float64 {
	total := 0.0
	for _, factor := range items {
		total += clamp(factor.Score) * factor.Weight
	}
	return round4(total)
}

// dominantFactors names the weighted contributions that carried the score, so
// the UI can explain which signals decided the ranking.
func dominantFactors(items []Factor) []FactorKind {
	type contribution struct {
		kind  FactorKind
		value float64
	}
	ranked := make([]contribution, 0, len(items))
	for _, factor := range items {
		ranked = append(ranked, contribution{kind: factor.Kind, value: clamp(factor.Score) * factor.Weight})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].value == ranked[j].value {
			return ranked[i].kind < ranked[j].kind
		}
		return ranked[i].value > ranked[j].value
	})
	out := make([]FactorKind, 0, 2)
	for _, item := range ranked {
		if item.value <= 0 || len(out) == 2 {
			break
		}
		out = append(out, item.kind)
	}
	return out
}

// confidence stays conservative: evidence comes from path signatures and
// lockfiles, never from content comparison or version verification.
func confidence(current *signals, installed bool, inUse int) float64 {
	value := 0.5
	if inUse > 0 {
		value += 0.2
	}
	if installed {
		value += 0.1
	}
	if !installed && inUse == 0 {
		value -= 0.2
	}
	if current.uncertainSizes {
		value -= 0.1
	}
	if current.projectCount() < 2 {
		value -= 0.1
	}
	return round4(clampRange(value, 0.1, 0.9))
}

func share(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return clamp(float64(part) / float64(total))
}

func clamp(value float64) float64 { return clampRange(value, 0, 1) }

func clampRange(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// round4 keeps scores stable across runs and readable in reports.
func round4(value float64) float64 {
	return float64(int64(value*10000+0.5)) / 10000
}
