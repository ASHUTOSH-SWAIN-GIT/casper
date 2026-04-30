package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// RDSRestoreFromSnapshotProposal is the typed proposal for creating a
// new RDS instance from an existing snapshot.
//
// Reversibility: reversible — rollback deletes the just-restored
// instance. The original snapshot is unchanged. Like create_read_replica,
// this is additive: nothing existing is modified.
type RDSRestoreFromSnapshotProposal struct {
	SnapshotIdentifier         string `json:"snapshot_identifier"`
	TargetDBInstanceIdentifier string `json:"target_db_instance_identifier"`
	Region                     string `json:"region"`
	TargetInstanceClass        string `json:"target_instance_class"`
	Reasoning                  string `json:"reasoning"`
}

//go:embed rds_restore_from_snapshot.json
var rdsRestoreFromSnapshotSchemaJSON []byte

const rdsRestoreFromSnapshotSchemaURL = "https://casper.dev/schemas/rds_restore_from_snapshot.schema.json"

var rdsRestoreFromSnapshotSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rdsRestoreFromSnapshotSchemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_restore_from_snapshot schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(rdsRestoreFromSnapshotSchemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_restore_from_snapshot schema: %w", err))
	}
	s, err := c.Compile(rdsRestoreFromSnapshotSchemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_restore_from_snapshot schema: %w", err))
	}
	rdsRestoreFromSnapshotSchema = s
}

// ValidateRDSRestoreFromSnapshot validates raw JSON against the action's schema.
func ValidateRDSRestoreFromSnapshot(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsRestoreFromSnapshotSchema.Validate(doc)
}
