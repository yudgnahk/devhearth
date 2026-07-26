package recommend

import (
	"fmt"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/bytesize"
)

// hibernationRule finds dormant projects whose dependencies are restorable from
// a lockfile. Hibernation itself is a Phase 5 operation: this rule only proposes
// the plan, and it always carries the Git-verification gap as a blocker because
// uncommitted or unpushed work must stop the operation (SPECS §7.9).
type hibernationRule struct{}

func (r *hibernationRule) ID() string     { return "rule.hibernate_inactive_project" }
func (r *hibernationRule) Version() int   { return 1 }
func (r *hibernationRule) Family() Family { return FamilyHibernateProject }

func (r *hibernationRule) Evaluate(input Input) []Recommendation {
	installs := installsByProject(input.Graph)
	reproducible := reproducibleProjects(input.Graph)

	var out []Recommendation
	for _, project := range ofKind(input.Graph, assets.KindProject) {
		owned := installs[project.Path]
		managers := reproducible[project.Path]
		if len(owned) == 0 || len(managers) == 0 {
			continue
		}
		dormantFor, dormant := dormancy(project.LastActivityAt, input)
		if !dormant {
			continue
		}
		out = append(out, r.recommend(project, owned, managers, dormantFor))
	}
	return out
}

// dormancy reports how long a project has been untouched. An unknown activity
// time never counts as dormant: absent evidence is not evidence of disuse.
func dormancy(last time.Time, input Input) (time.Duration, bool) {
	if last.IsZero() {
		return 0, false
	}
	elapsed := input.Now.Sub(last)
	return elapsed, elapsed > input.InactiveAfter
}

func (r *hibernationRule) recommend(project assets.Asset, installs, managers []assets.Asset, dormantFor time.Duration) Recommendation {
	var reclaimable int64
	uncertain := false
	evidence := [][]assets.Evidence{project.Evidence}
	for _, install := range installs {
		reclaimable += install.Size.ExclusiveAllocatedBytes
		uncertain = uncertain || install.Size.Uncertain
		evidence = append(evidence, install.Evidence)
	}
	tools := make([]string, 0, len(managers))
	for _, manager := range managers {
		tools = append(tools, toolOf(manager))
		evidence = append(evidence, manager.Evidence)
	}

	confidence := 0.6
	if uncertain {
		confidence -= 0.1
	}
	if len(installs) > 1 {
		confidence += 0.05
	}

	return Recommendation{
		ID:        newID(FamilyHibernateProject, project.Ecosystem, project.Path),
		Family:    FamilyHibernateProject,
		Ecosystem: project.Ecosystem,
		Title:     fmt.Sprintf("Hibernate %s (%d days dormant)", project.DisplayName, int(dormantFor.Hours()/24)),
		Explanation: fmt.Sprintf(
			"Source files under %s have not changed for %d days, and its dependencies are declared by %v with a lockfile present. "+
				"Removing the generated dependency material would release about %s and the project could be restored by reinstalling "+
				"from that lockfile. Generated output is measured; nothing here counts source files or user data.",
			project.DisplayName, int(dormantFor.Hours()/24), tools, bytesize.Format(reclaimable)),
		// Low risk applies to reproducible, project-local generated data only,
		// and only once the preconditions below are verified.
		Risk:       assets.RiskLow,
		Confidence: clampConfidence(confidence),
		Savings: Savings{
			// Measured bytes, so both bounds match; hard links make it a floor.
			LowBytes:  reclaimable,
			HighBytes: reclaimable,
			Uncertain: uncertain,
		},
		RestorationCost:     fmt.Sprintf("one dependency install with %v: a download plus build, typically minutes", tools),
		CompatibilityImpact: "none while dormant; the project must be reinstalled before work resumes",
		Preconditions: []string{
			"the repository has no uncommitted changes",
			"the repository has no commits missing from its configured remotes",
			"a manifest and lockfile are present and current",
			"no unique user data lives inside the generated directories",
		},
		ProposedActions: []string{
			"record the exact paths, sizes, tool versions, and restore command as a hibernation manifest",
			"move the generated dependency directories to the Trash after a dry run reports the same paths",
		},
		Verification: []string{
			"source files and declared persistent data are still present",
			"a restore run reinstalls dependencies and the project builds",
		},
		Rollback: "reinstall from the lockfile, or recover the directories from the Trash before it is emptied",
		Blockers: []string{
			"Git working-tree and unpushed-commit verification is not available yet, so this candidate cannot be cleared for execution",
		},
		AffectedAssetIDs: assetIDs([]assets.Asset{project}, installs),
		Evidence: mergeEvidence(append(evidence, []assets.Evidence{{
			Kind:       "last_source_activity_days",
			Value:      fmt.Sprintf("%d", int(dormantFor.Hours()/24)),
			Confidence: 0.8,
		}})...),
		DominantFactors: []string{"project_activity", "reproducibility"},
	}
}
