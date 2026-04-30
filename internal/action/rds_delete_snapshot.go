package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// RDSDeleteSnapshotProposal is the typed proposal for permanently
// deleting a manual RDS snapshot.
//
// Reversibility: irreversible. Once a snapshot is deleted, the data
// is gone. AWS does not retain a tombstone or grace period for manual
// snapshots. The policy engine treats this as an irreversible action
// with deny-by-default semantics.
type RDSDeleteSnapshotProposal struct {
	SnapshotIdentifier string `json:"snapshot_identifier"`
	Region             string `json:"region"`
	Reasoning          string `json:"reasoning"`
}

//go:embed rds_delete_snapshot.json
var rdsDeleteSnapshotSchemaJSON []byte

const rdsDeleteSnapshotSchemaURL = "https://casper.dev/schemas/rds_delete_snapshot.schema.json"

var rdsDeleteSnapshotSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rdsDeleteSnapshotSchemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_delete_snapshot schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(rdsDeleteSnapshotSchemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_delete_snapshot schema: %w", err))
	}
	s, err := c.Compile(rdsDeleteSnapshotSchemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_delete_snapshot schema: %w", err))
	}
	rdsDeleteSnapshotSchema = s
}

// ValidateRDSDeleteSnapshot validates raw JSON against the action's schema.
func ValidateRDSDeleteSnapshot(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsDeleteSnapshotSchema.Validate(doc)
}
