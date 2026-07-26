package protocol

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yudgnahk/devhearth/internal/policy"
	"github.com/yudgnahk/devhearth/internal/store"
	"github.com/yudgnahk/devhearth/internal/trend"
)

// Monitoring is the durable Phase 4 state the server reads and writes: the
// portable policy, local feedback, and trend history. The engine binary wires a
// SQLite-backed implementation; tests supply their own. A nil Monitoring leaves
// the policy, feedback, and trend methods reporting "not available" rather than
// pretending to store anything.
type Monitoring interface {
	// EnsureActivePolicy returns the active policy, creating a default one on
	// first use so later writes always have a row to update.
	EnsureActivePolicy(ctx context.Context, defaultID string) (policy.Document, error)
	// PolicyByID returns one stored policy.
	PolicyByID(ctx context.Context, id string) (policy.Document, error)
	// SavePolicy validates and stores a document.
	SavePolicy(ctx context.Context, document policy.Document, activate bool) error
	// RecordFeedback stores one local verdict. Feedback never leaves the machine.
	RecordFeedback(ctx context.Context, feedback store.Feedback) (store.Feedback, error)
	// Snapshots returns recent trend history, oldest first.
	Snapshots(ctx context.Context, limit int) ([]trend.Snapshot, error)
}

// DefaultPolicyID names the policy the engine creates on first run.
const DefaultPolicyID = "local-machine"

// activePolicy loads the policy a request should act on. An empty id means the
// active policy.
func (s *Server) activePolicy(ctx context.Context, id string) (policy.Document, *Error) {
	if s.options.Monitoring == nil {
		return policy.Document{}, &Error{Code: -32010, Message: "policy storage is not available in this engine session"}
	}
	if id != "" {
		document, err := s.options.Monitoring.PolicyByID(ctx, id)
		if err != nil {
			return policy.Document{}, &Error{Code: -32011, Message: "policy not found"}
		}
		return document, nil
	}
	document, err := s.options.Monitoring.EnsureActivePolicy(ctx, DefaultPolicyID)
	if err != nil {
		s.options.Logger.Error("load active policy", "error", err)
		return policy.Document{}, &Error{Code: -32012, Message: "the stored policy could not be loaded"}
	}
	return document, nil
}

// resolvePolicy returns the effective policy for this machine, falling back to
// the defaults when policy storage is unavailable. Analysis must never fail
// because a preference could not be read.
func (s *Server) resolvePolicy(ctx context.Context, id string) policy.Effective {
	document, failure := s.activePolicy(ctx, id)
	if failure != nil {
		return policy.Resolve(policy.DefaultDocument(DefaultPolicyID), s.options.Machine)
	}
	return policy.Resolve(document, s.options.Machine)
}

// policyResult renders a document plus how it resolves on this machine.
//
// Only portable forms cross the wire: roots appear as "~/Projects", never as
// the absolute path they resolve to here. The document itself is emitted as the
// canonical export bytes, so what a client displays and what a user would share
// are the same content.
func policyResult(document policy.Document, machine policy.Machine, home string) (PolicyResult, error) {
	encoded, err := policy.Marshal(document)
	if err != nil {
		return PolicyResult{}, fmt.Errorf("encode policy: %w", err)
	}
	effective := policy.Resolve(document, machine)

	result := PolicyResult{
		Document: json.RawMessage(encoded),
		Effective: EffectivePolicy{
			PolicyID:          effective.PolicyID,
			Name:              effective.Name,
			FitMode:           effective.FitMode,
			RiskThreshold:     effective.RiskThreshold,
			PreferredTools:    effective.PreferredTools,
			Exclusions:        effective.Exclusions,
			ActiveWithinDays:  int(effective.ActiveWithin.Hours() / 24),
			InactiveAfterDays: int(effective.InactiveAfter.Hours() / 24),
			ScanHistoryCount:  effective.ScanHistoryCount,
			SuppressionCount:  len(effective.Suppressions),
		},
		Machine: PolicyMachine{
			Architecture: machine.Architecture,
			DiskClass:    machine.DiskClass,
			Role:         machine.Role,
		},
		Choices: PolicyChoices{
			FitModes:       policy.FitModes(),
			RiskThresholds: []string{policy.RiskInformational, policy.RiskLow, policy.RiskMedium, policy.RiskHigh},
			Verdicts:       store.Verdicts(),
		},
	}
	for _, applied := range effective.AppliedOverlays {
		result.Effective.AppliedOverlays = append(result.Effective.AppliedOverlays, PolicyMachine{
			Architecture: applied.Architecture,
			DiskClass:    applied.DiskClass,
			Role:         applied.Role,
		})
	}
	for _, root := range effective.Roots {
		entry := PolicyRootStatus{Display: root.Display(), Alias: string(root.Alias)}
		if _, err := root.Resolve(home); err == nil {
			entry.Resolvable = true
		}
		result.Effective.Roots = append(result.Effective.Roots, entry)
		if !entry.Resolvable {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("this machine cannot resolve the root %q from the policy", root.Display()))
		}
	}
	return result, nil
}

// policyRoots resolves the policy's roots into absolute paths for a scan. The
// paths stay inside the engine: they are used to walk the filesystem and are
// redacted before any result travels back.
func (s *Server) policyRoots(ctx context.Context, policyID string) ([]string, *Error) {
	document, failure := s.activePolicy(ctx, policyID)
	if failure != nil {
		return nil, failure
	}
	effective := policy.Resolve(document, s.options.Machine)
	resolved, unresolved := effective.ResolveRoots(s.options.HomeDir)
	if len(resolved) == 0 {
		if len(unresolved) > 0 {
			return nil, &Error{Code: -32013, Message: "no policy root could be resolved on this machine"}
		}
		return nil, &Error{Code: -32013, Message: "the policy declares no scan roots"}
	}
	return resolved, nil
}

// reapplyPolicy re-partitions every held scan under a newly saved policy.
//
// Without this, hiding a recommendation would appear to do nothing until the
// next scan: the partition is computed during analysis, and a policy edit
// afterwards would leave the inbox describing the policy as it was. The rules
// are not re-run — hiding is a display decision, and re-running analysis to
// apply one could produce different advice than the user was looking at.
func (s *Server) reapplyPolicy(effective policy.Effective) {
	s.scansMu.Lock()
	defer s.scansMu.Unlock()
	for _, current := range s.scans {
		if len(current.advice.All) == 0 {
			continue
		}
		current.advice = current.advice.Refilter(effective)
		current.policy = effective
	}
}

// suppress applies or reverses a suppression on the stored policy. Suppression
// hides advice; it never marks work as done and never authorizes anything.
func (s *Server) suppress(ctx context.Context, params RecommendationsSuppressParams) (any, *Error) {
	document, failure := s.activePolicy(ctx, params.PolicyID)
	if failure != nil {
		return nil, failure
	}
	entry := policy.Suppression{
		RecommendationID: params.RecommendationID,
		Family:           params.Family,
		Ecosystem:        params.Ecosystem,
		Reason:           params.Reason,
		CreatedAt:        policy.Today(s.now()),
	}
	updated := document.WithSuppression(entry)
	if params.Undo {
		updated = document.WithoutSuppression(entry)
	}
	// Validation runs on the whole document rather than the one entry, so a
	// suppression that would hide everything is refused here and not stored.
	validated, err := policy.Validate(updated)
	if err != nil {
		return nil, &Error{Code: -32602, Message: err.Error()}
	}
	if err := s.options.Monitoring.SavePolicy(ctx, validated, true); err != nil {
		s.options.Logger.Error("save policy suppression", "error", err)
		return nil, &Error{Code: -32014, Message: "the suppression could not be stored"}
	}
	s.reapplyPolicy(policy.Resolve(validated, s.options.Machine))
	result, err := policyResult(validated, s.options.Machine, s.options.HomeDir)
	if err != nil {
		return nil, &Error{Code: -32012, Message: "the stored policy could not be rendered"}
	}
	return result, nil
}

// importPolicy validates an untrusted document and stores it. The bytes come
// from a file the user chose, so nothing is trusted before policy.Parse runs.
func (s *Server) importPolicy(ctx context.Context, params PolicyImportParams) (any, *Error) {
	if s.options.Monitoring == nil {
		return nil, &Error{Code: -32010, Message: "policy storage is not available in this engine session"}
	}
	document, err := policy.Parse([]byte(params.Document))
	if err != nil {
		return nil, &Error{Code: -32602, Message: fmt.Sprintf("the policy file was rejected: %v", err)}
	}
	activate := true
	if params.Activate != nil {
		activate = *params.Activate
	}
	if err := s.options.Monitoring.SavePolicy(ctx, document, activate); err != nil {
		s.options.Logger.Error("import policy", "error", err)
		return nil, &Error{Code: -32014, Message: "the policy could not be stored"}
	}
	if activate {
		s.reapplyPolicy(policy.Resolve(document, s.options.Machine))
	}
	result, err := policyResult(document, s.options.Machine, s.options.HomeDir)
	if err != nil {
		return nil, &Error{Code: -32012, Message: "the imported policy could not be rendered"}
	}
	result.Warnings = append(result.Warnings,
		"imported policies carry preferences only: scan history, inventory, and recommendation feedback stay on the machine that produced them")
	return result, nil
}

// setPolicy replaces the stored policy from client-supplied fields. The client
// is not trusted either: the same validation runs as for an imported file.
func (s *Server) setPolicy(ctx context.Context, params PolicySetParams) (any, *Error) {
	if s.options.Monitoring == nil {
		return nil, &Error{Code: -32010, Message: "policy storage is not available in this engine session"}
	}
	// The same parser as policy.import, so a document assembled by a client gets
	// exactly the treatment a file from a stranger gets — including rejection of
	// unknown fields, which would otherwise let a client appear to set something
	// this build does not read.
	validated, err := policy.Parse(params.Document)
	if err != nil {
		return nil, &Error{Code: -32602, Message: err.Error()}
	}
	if err := s.options.Monitoring.SavePolicy(ctx, validated, true); err != nil {
		s.options.Logger.Error("save policy", "error", err)
		return nil, &Error{Code: -32014, Message: "the policy could not be stored"}
	}
	s.reapplyPolicy(policy.Resolve(validated, s.options.Machine))
	result, renderErr := policyResult(validated, s.options.Machine, s.options.HomeDir)
	if renderErr != nil {
		return nil, &Error{Code: -32012, Message: "the stored policy could not be rendered"}
	}
	return result, nil
}

// recordFeedback stores a local verdict on a recommendation.
func (s *Server) recordFeedback(ctx context.Context, params RecommendationsFeedbackParams) (any, *Error) {
	if s.options.Monitoring == nil {
		return nil, &Error{Code: -32010, Message: "feedback storage is not available in this engine session"}
	}
	stored, err := s.options.Monitoring.RecordFeedback(ctx, store.Feedback{
		RecommendationID: params.RecommendationID,
		Family:           params.Family,
		Ecosystem:        params.Ecosystem,
		Verdict:          params.Verdict,
		Note:             params.Note,
		ScanID:           params.ScanID,
	})
	if err != nil {
		return nil, &Error{Code: -32602, Message: err.Error()}
	}
	return RecommendationsFeedbackResult{
		RecommendationID: stored.RecommendationID,
		Verdict:          stored.Verdict,
		RecordedAt:       stored.CreatedAt.UTC().Format(rfc3339),
		LocalOnly:        true,
	}, nil
}

// trends analyses stored history. Snapshots hold only counts and byte totals,
// so nothing here needs redaction.
func (s *Server) trends(ctx context.Context, params TrendsListParams) (any, *Error) {
	if s.options.Monitoring == nil {
		return nil, &Error{Code: -32010, Message: "trend history is not available in this engine session"}
	}
	snapshots, err := s.options.Monitoring.Snapshots(ctx, params.Limit)
	if err != nil {
		s.options.Logger.Error("read trend history", "error", err)
		return nil, &Error{Code: -32015, Message: "trend history could not be read"}
	}
	return TrendsListResult{Trends: trend.Analyze(snapshots, trend.Options{Now: s.now()})}, nil
}
