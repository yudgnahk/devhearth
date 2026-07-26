package recommend

import (
	"fmt"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/bytesize"
)

// obsoleteWorktreeRule flags linked Git worktrees whose source has been idle for
// a long time. Git state that has not been verified is high risk by the product
// risk model: a worktree may hold the only copy of unpushed work, so this rule
// proposes verification first and never presents removal as safe.
type obsoleteWorktreeRule struct{}

func (r *obsoleteWorktreeRule) ID() string     { return "rule.remove_obsolete_worktree" }
func (r *obsoleteWorktreeRule) Version() int   { return 1 }
func (r *obsoleteWorktreeRule) Family() Family { return FamilyObsoleteWorktree }

func (r *obsoleteWorktreeRule) Evaluate(input Input) []Recommendation {
	var out []Recommendation
	for _, worktree := range ofKind(input.Graph, assets.KindGitWorktree) {
		dormantFor, dormant := dormancy(worktree.LastActivityAt, input)
		if !dormant {
			continue
		}
		out = append(out, r.recommend(worktree, dormantFor.Hours()/24))
	}
	return out
}

func (r *obsoleteWorktreeRule) recommend(worktree assets.Asset, dormantDays float64) Recommendation {
	reclaimable := worktree.Size.ExclusiveAllocatedBytes
	return Recommendation{
		ID:        newID(FamilyObsoleteWorktree, worktree.Path),
		Family:    FamilyObsoleteWorktree,
		Ecosystem: worktree.Ecosystem,
		Title:     fmt.Sprintf("Check whether worktree %s is still needed", worktree.DisplayName),
		Explanation: fmt.Sprintf(
			"The linked worktree %s has shown no source changes for %d days and holds %s. Agent-driven work often leaves "+
				"worktrees behind, but this scan cannot see whether its branch is merged or whether it carries commits that exist "+
				"nowhere else, so nothing about it is safe to remove yet.",
			worktree.DisplayName, int(dormantDays), bytesize.Format(reclaimable)),
		// Unverified Git state is High in the product risk model, and an
		// explanation layer may never lower that.
		Risk:       assets.RiskHigh,
		Confidence: clampConfidence(0.45),
		Savings: Savings{
			// Nothing is claimed until Git verification runs; the upper bound is
			// what the worktree currently occupies.
			LowBytes:  0,
			HighBytes: reclaimable,
			Uncertain: true,
		},
		RestorationCost:     "recreating a worktree is one Git command plus a dependency install, provided the branch still exists somewhere",
		CompatibilityImpact: "none for other worktrees; the shared repository keeps its history",
		Preconditions: []string{
			"the worktree has no uncommitted changes",
			"every commit reachable only from this worktree exists on a configured remote",
			"the branch is merged, or is intentionally being abandoned",
		},
		ProposedActions: []string{
			"inspect the worktree's status and branch state with Git",
			"push or archive any commits that exist only here",
			"remove the worktree through Git rather than by deleting the directory",
		},
		Verification: []string{
			"the repository still lists every other worktree",
			"no branch lost its last reachable commit",
		},
		Rollback: "recreate the worktree from the branch; if commits were only local, they cannot be recovered after removal, which is why verification comes first",
		Blockers: []string{
			"Git working-tree, branch, and unpushed-commit verification is not available yet",
		},
		AffectedAssetIDs: assetIDs([]assets.Asset{worktree}),
		Evidence: mergeEvidence(worktree.Evidence, []assets.Evidence{{
			Kind:       "last_source_activity_days",
			Value:      fmt.Sprintf("%d", int(dormantDays)),
			Confidence: 0.8,
		}}),
		DominantFactors: []string{"project_activity"},
	}
}
