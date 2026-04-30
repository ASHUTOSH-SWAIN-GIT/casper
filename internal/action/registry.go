package action

import "fmt"

// Spec is the metadata Casper carries about a registered action type.
// It is intentionally narrow — typed structs, schemas, plan compilers,
// and policy queries live in their per-action packages. Spec is a
// directory: a reader can grep this file to see every action Casper
// supports without spelunking through subpackages.
//
// Adding a new action is a four-step exercise:
//
//  1. Define the typed proposal struct + JSON Schema in this package
//     (e.g. rds_create_snapshot.go + rds_create_snapshot.json).
//  2. Implement plan.Compile<Action> that takes the typed struct and
//     emits forward/rollback ExecutionPlans.
//  3. Implement identity.Build<Action>Policy that emits the per-action
//     IAM session policy scoped to the proposal's resource.
//  4. Add a Rego rules file (rules_<action>.rego) and register an
//     entry in this map. The CLI dispatch then knows how to route.
type Spec struct {
	// Type is the canonical action identifier used in proposal JSON
	// (the action_type field) and as the discriminator in NL routing.
	Type string

	// Service is the AWS service this action targets ("rds", "ecs", ...).
	// Used for grouping in docs and per-service policy bundles.
	Service string

	// Description is a one-line human summary of what the action does.
	// Surfaced in `casperctl actions`, embedded in the NL router's
	// prompt as a discriminator hint.
	Description string

	// Reversibility classifies the recovery story for failed runs.
	// "reversible" — rollback can return to prior state.
	// "partially_reversible" — rollback covers most cases but not all
	//   (e.g. an engine version upgrade leaves data file format changed).
	// "irreversible" — no automatic undo possible (delete, storage grow).
	// Policy rules treat this as a first-class input.
	Reversibility string

	// PolicyDefault is the policy verdict that fires when no allow/deny
	// rule matches in the action's Rego file. Reversible actions
	// typically default to "needs_approval"; irreversible to "deny".
	PolicyDefault string

	// PolicyQuery is the Rego data path to read the verdict from.
	// E.g. "data.casper.rds_resize.result".
	PolicyQuery string
}

// Registry holds every action Casper currently supports. The map is
// the source of truth — CLI dispatching, the NL router's prompt
// generation, and the docs all read from here.
var Registry = map[string]Spec{
	"rds_resize": {
		Type:          "rds_resize",
		Service:       "rds",
		Description:   "Resize an RDS instance to a different DBInstanceClass (e.g. db.t4g.micro → db.t4g.small)",
		Reversibility: "reversible",
		PolicyDefault: "needs_approval",
		PolicyQuery:   "data.casper.rds_resize.result",
	},
	"rds_create_snapshot": {
		Type:          "rds_create_snapshot",
		Service:       "rds",
		Description:   "Create a manual DB snapshot of an RDS instance (additive — the source instance is unchanged)",
		Reversibility: "reversible", // rollback deletes the just-created snapshot
		PolicyDefault: "needs_approval",
		PolicyQuery:   "data.casper.rds_create_snapshot.result",
	},
	"rds_modify_backup_retention": {
		Type:          "rds_modify_backup_retention",
		Service:       "rds",
		Description:   "Change the automated backup retention period (in days) for an RDS instance",
		Reversibility: "reversible",
		PolicyDefault: "needs_approval",
		PolicyQuery:   "data.casper.rds_modify_backup_retention.result",
	},
	"rds_reboot_instance": {
		Type:          "rds_reboot_instance",
		Service:       "rds",
		Description:   "Reboot an RDS instance (or, with force_failover, fail over Multi-AZ to the standby)",
		Reversibility: "reversible", // a reboot is transient — there is nothing to undo
		PolicyDefault: "needs_approval",
		PolicyQuery:   "data.casper.rds_reboot_instance.result",
	},
}

// Lookup returns the Spec for an action type, or false if unregistered.
// Callers should use this rather than indexing the map directly so the
// false case is handled explicitly.
func Lookup(actionType string) (Spec, bool) {
	s, ok := Registry[actionType]
	return s, ok
}

// Types returns the list of registered action types in insertion order
// (Go maps don't preserve order, so this sorts alphabetically — stable
// across runs, useful for help output and the NL router's prompt).
func Types() []string {
	out := make([]string, 0, len(Registry))
	for k := range Registry {
		out = append(out, k)
	}
	// Stable sort — actions appear in alphabetical order.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// MustLookup is the panicking version, for use in CLI dispatch where
// the action type was already validated upstream and a missing entry
// is a programmer error.
func MustLookup(actionType string) Spec {
	s, ok := Lookup(actionType)
	if !ok {
		panic(fmt.Sprintf("action.MustLookup: unregistered action %q", actionType))
	}
	return s
}
