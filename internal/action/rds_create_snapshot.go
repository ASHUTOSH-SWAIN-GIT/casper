package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// RDSCreateSnapshotProposal is the typed v1 action proposal for taking
// a manual snapshot of an RDS instance. The action is purely additive
// — the source instance is unchanged. Rollback deletes the snapshot.
type RDSCreateSnapshotProposal struct {
	DBInstanceIdentifier string `json:"db_instance_identifier"`
	Region               string `json:"region"`
	SnapshotIdentifier   string `json:"snapshot_identifier"`
	Reasoning            string `json:"reasoning"`
}

//go:embed rds_create_snapshot.json
var rdsCreateSnapshotSchemaJSON []byte

const rdsCreateSnapshotSchemaURL = "https://casper.dev/schemas/rds_create_snapshot.schema.json"

var rdsCreateSnapshotSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rdsCreateSnapshotSchemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_create_snapshot schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(rdsCreateSnapshotSchemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_create_snapshot schema: %w", err))
	}
	s, err := c.Compile(rdsCreateSnapshotSchemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_create_snapshot schema: %w", err))
	}
	rdsCreateSnapshotSchema = s
}

// ValidateRDSCreateSnapshot validates raw JSON against the action's
// schema. Same shape as Validate (which is the rds_resize-specific
// validator); the v1 split exists because we don't yet have a generic
// "validate by action type" entry point.
func ValidateRDSCreateSnapshot(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsCreateSnapshotSchema.Validate(doc)
}
