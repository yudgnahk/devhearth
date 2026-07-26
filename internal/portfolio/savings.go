package portfolio

// sharedStoreCapability is how much of a shared-store ceiling a tool can
// realistically reach, from what the tool's install model does on disk:
//
//   - pnpm and uv keep one content-addressed copy and hard-link into projects.
//   - bun keeps a global install cache but still materializes many files.
//   - Yarn Berry caches archives; PnP avoids per-project trees, the classic
//     layout does not, and the scan cannot tell which a migration would adopt.
//   - npm, pip, Poetry, and Pipenv install a per-project copy, so adopting them
//     removes no duplication.
//   - Conda shares packages through its own pkgs cache, but environments still
//     hold binaries the cache cannot deduplicate.
//
// These are documented modelling assumptions, not measurements: no content
// hashing has run, so every figure derived from them is a range.
var sharedStoreCapability = map[string]float64{
	"pnpm":   1.0,
	"uv":     1.0,
	"bun":    0.8,
	"yarn":   0.6,
	"conda":  0.5,
	"npm":    0.0,
	"pip":    0.0,
	"poetry": 0.0,
	"pipenv": 0.0,
}

// Savings-range bounds around the deduplication ceiling. Overlap between
// project-local installs is unknown without hashing, so the low bound assumes
// little sharing and the high bound stops short of perfect sharing.
const (
	savingsLowRatio  = 0.25
	savingsHighRatio = 0.80
)

// savingsEstimate is the modelled result of adopting one option.
type savingsEstimate struct {
	lowBytes          int64
	highBytes         int64
	futureGrowthBytes int64
	// share is the midpoint expressed as a fraction of project-local bytes,
	// used as the normalized duplication-cost factor score.
	share float64
}

// estimateSavings models moving installCount project-local installs holding
// localBytes behind the option's store. With N identical installs a shared
// store would keep one copy, so the ceiling is localBytes*(N-1)/N; the tool's
// capability and the uncertainty band narrow it from there.
func estimateSavings(tool string, localBytes int64, installCount int) savingsEstimate {
	capability := sharedStoreCapability[tool]
	if capability <= 0 || installCount < 2 || localBytes <= 0 {
		return savingsEstimate{}
	}
	ceiling := float64(localBytes) * float64(installCount-1) / float64(installCount) * capability
	estimate := savingsEstimate{
		lowBytes:  int64(ceiling * savingsLowRatio),
		highBytes: int64(ceiling * savingsHighRatio),
	}
	// Future projects would otherwise duplicate the same material again, so the
	// avoided growth is modelled at the midpoint of the immediate range.
	estimate.futureGrowthBytes = (estimate.lowBytes + estimate.highBytes) / 2
	if localBytes > 0 {
		estimate.share = float64(estimate.futureGrowthBytes) / float64(localBytes)
	}
	return estimate
}
