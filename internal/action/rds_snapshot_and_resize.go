package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// RDSSnapshotAndResizeProposal is the typed proposal for the compound
// action: snapshot the instance first (safety checkpoint), then resize
// it. The snapshot stays regardless of whether the resize succeeds —
// it is the recovery artefact. Rollback only reverts the resize.
type RDSSnapshotAndResizeProposal struct {
	ActionType           string `json:"action_type"`
	DBInstanceIdentifier string `json:"db_instance_identifier"`
	Region               string `json:"region"`
	SnapshotIdentifier   string `json:"snapshot_identifier"`
	CurrentInstanceClass string `json:"current_instance_class"`
	TargetInstanceClass  string `json:"target_instance_class"`
	Reasoning            string `json:"reasoning"`
}

//go:embed rds_snapshot_and_resize.json
var rdsSnapshotAndResizeSchemaJSON []byte

const rdsSnapshotAndResizeSchemaURL = "https://casper.dev/schemas/rds_snapshot_and_resize.schema.json"

var rdsSnapshotAndResizeSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rdsSnapshotAndResizeSchemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_snapshot_and_resize schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(rdsSnapshotAndResizeSchemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_snapshot_and_resize schema: %w", err))
	}
	s, err := c.Compile(rdsSnapshotAndResizeSchemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_snapshot_and_resize schema: %w", err))
	}
	rdsSnapshotAndResizeSchema = s
}

// ValidateRDSSnapshotAndResize validates raw JSON against the action schema.
func ValidateRDSSnapshotAndResize(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsSnapshotAndResizeSchema.Validate(doc)
}
