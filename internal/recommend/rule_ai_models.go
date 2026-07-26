package recommend

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/bytesize"
)

// duplicateModelRule groups downloadable model files that share a name and an
// exact size. Similar filenames alone are not evidence of duplication
// (SPECS §7.7), so matching sizes are required, content hashing is still listed
// as the missing step, and user-created fine-tunes are never included.
type duplicateModelRule struct{}

func (r *duplicateModelRule) ID() string     { return "rule.unify_duplicate_ai_models" }
func (r *duplicateModelRule) Version() int   { return 1 }
func (r *duplicateModelRule) Family() Family { return FamilyDuplicateAIModel }

// minDuplicateModelBytes keeps trivial or placeholder files out of the inbox.
const minDuplicateModelBytes = 16 * 1024 * 1024

func (r *duplicateModelRule) Evaluate(input Input) []Recommendation {
	groups := map[string][]assets.Asset{}
	for _, asset := range ofKind(input.Graph, assets.KindAIAsset) {
		if !downloadableModelFile(asset) {
			continue
		}
		if asset.Size.LogicalBytes < minDuplicateModelBytes {
			continue
		}
		key := fmt.Sprintf("%s|%d", strings.ToLower(filepath.Base(asset.Path)), asset.Size.LogicalBytes)
		groups[key] = append(groups[key], asset)
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var out []Recommendation
	for _, key := range keys {
		copies := groups[key]
		if len(copies) < 2 {
			continue
		}
		out = append(out, r.recommend(copies))
	}
	return out
}

// downloadableModelFile excludes fine-tunes, adapters, and anything the risk
// model treats as user-created: those are never duplication candidates.
func downloadableModelFile(asset assets.Asset) bool {
	if asset.Attributes["ai_kind"] != "model_file" {
		return false
	}
	return asset.Risk != assets.RiskHigh && asset.Risk != assets.RiskProhibited
}

func (r *duplicateModelRule) recommend(copies []assets.Asset) Recommendation {
	sorted := make([]assets.Asset, len(copies))
	copy(sorted, copies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	perCopy := sorted[0].Size.AllocatedBytes
	reclaimable := perCopy * int64(len(sorted)-1)
	name := filepath.Base(sorted[0].Path)
	evidence := make([][]assets.Evidence, 0, len(sorted)+1)
	for _, asset := range sorted {
		evidence = append(evidence, asset.Evidence)
	}
	evidence = append(evidence, []assets.Evidence{{
		Kind:       "matching_name_and_size",
		Value:      fmt.Sprintf("%d copies of %s at %s each", len(sorted), name, bytesize.Format(perCopy)),
		Confidence: 0.6,
	}})

	return Recommendation{
		ID:        newID(FamilyDuplicateAIModel, name, fmt.Sprint(perCopy)),
		Family:    FamilyDuplicateAIModel,
		Ecosystem: "ai",
		Title:     fmt.Sprintf("Unify %d copies of model %s", len(sorted), name),
		Explanation: fmt.Sprintf(
			"%d files named %s have the same size (%s each) in different locations, which usually means several tools each "+
				"downloaded the same base model. Keeping one copy would release up to %s. The files have not been compared "+
				"byte for byte, so this is a candidate rather than a confirmed duplicate.",
			len(sorted), name, bytesize.Format(perCopy), bytesize.Format(reclaimable)),
		Risk:       assets.RiskMedium,
		Confidence: clampConfidence(0.4),
		Savings: Savings{
			// Zero until the copies are actually compared.
			LowBytes:  0,
			HighBytes: reclaimable,
			Uncertain: true,
		},
		RestorationCost:     "re-downloading a base model is bandwidth and time, often minutes to hours depending on size",
		CompatibilityImpact: "each tool must be pointed at the surviving copy through supported configuration; fragile symbolic links are not a substitute",
		Preconditions: []string{
			"the copies are confirmed identical by content, not by name and size",
			"none of the copies is a fine-tune, adapter, or otherwise user-created artifact",
			"every tool that uses a removed copy supports a configurable model location",
		},
		ProposedActions: []string{
			"compare the candidates by content hash before acting",
			"point each tool at one shared location through its own configuration",
		},
		Verification: []string{
			"every tool still loads the model after reconfiguration",
			"a follow-up scan reports a single copy",
		},
		Rollback: "re-download the model from its original source",
		Blockers: []string{
			"no content comparison has run: name and size matching is not proof of duplication",
		},
		AffectedAssetIDs: assetIDs(sorted),
		Evidence:         mergeEvidence(evidence...),
		DominantFactors:  []string{"duplication_cost"},
	}
}
