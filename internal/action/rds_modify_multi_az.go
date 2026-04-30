package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// RDSModifyMultiAZProposal is the typed proposal for toggling an RDS
// instance's Multi-AZ deployment status.
//
// Reversibility: reversible — toggling back returns the topology to
// its prior state. Operationally heavy: the modification takes 5–15
// minutes (the instance reprovisions a standby) and incurs cost
// changes (Multi-AZ is roughly 2x the price).
type RDSModifyMultiAZProposal struct {
	DBInstanceIdentifier string `json:"db_instance_identifier"`
	Region               string `json:"region"`
	CurrentMultiAZ       bool   `json:"current_multi_az"`
	TargetMultiAZ        bool   `json:"target_multi_az"`
	ApplyImmediately     bool   `json:"apply_immediately"`
	Reasoning            string `json:"reasoning"`
}

//go:embed rds_modify_multi_az.json
var rdsModifyMultiAZSchemaJSON []byte

const rdsModifyMultiAZSchemaURL = "https://casper.dev/schemas/rds_modify_multi_az.schema.json"

var rdsModifyMultiAZSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rdsModifyMultiAZSchemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_modify_multi_az schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(rdsModifyMultiAZSchemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_modify_multi_az schema: %w", err))
	}
	s, err := c.Compile(rdsModifyMultiAZSchemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_modify_multi_az schema: %w", err))
	}
	rdsModifyMultiAZSchema = s
}

// ValidateRDSModifyMultiAZ validates raw JSON against the action's schema.
func ValidateRDSModifyMultiAZ(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsModifyMultiAZSchema.Validate(doc)
}
