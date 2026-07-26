package portfolio

import (
	"fmt"

	"github.com/yudgnahk/devhearth/internal/bytesize"
)

// Plain-language factor details. They must stay path-free: assessments travel
// into reports where only redacted paths are allowed.

func presenceDetail(installed bool, inUse int) string {
	switch {
	case inUse > 0:
		return "already used by at least one project on this machine"
	case installed:
		return "installed on this machine but not referenced by a detected project"
	default:
		return "no installation evidence found under the scanned roots"
	}
}

func duplicationDetail(current *signals, estimate savingsEstimate) string {
	if current.localInstallCount < 2 {
		return "fewer than two project-local installs, so there is no duplication to share"
	}
	if estimate.highBytes == 0 {
		return fmt.Sprintf("%d project-local installs, but this tool installs a per-project copy so sharing them is not possible",
			current.localInstallCount)
	}
	return fmt.Sprintf("%d project-local installs holding %s could share one store",
		current.localInstallCount, bytesize.Format(current.localInstallBytes))
}

func frictionDetail(stayPut bool, friction float64) string {
	if stayPut {
		return "no migration: this is the tool the machine already leans on"
	}
	return fmt.Sprintf("modelled migration friction %.0f%% from observed blockers", friction*100)
}

func policyDetail(preferred string) string {
	if preferred == "" {
		return "no preferred tool configured, so policy does not affect this ranking"
	}
	return fmt.Sprintf("policy prefers %s", preferred)
}

func workflowImpact(stayPut bool, tool string, friction float64) string {
	if stayPut {
		return "no workflow change; existing commands and lockfiles keep working"
	}
	if friction >= 0.6 {
		return fmt.Sprintf("substantial workflow change: %s alters install layout and command habits, and blockers remain unresolved", tool)
	}
	return fmt.Sprintf("moderate workflow change: %s replaces install commands and lockfiles for migrated projects", tool)
}
