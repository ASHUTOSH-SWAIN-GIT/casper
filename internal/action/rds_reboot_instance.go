package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// RDSRebootInstanceProposal is the typed proposal for rebooting an RDS
// instance. ForceFailover is meaningful only on Multi-AZ instances:
// when true, AWS fails over to the standby (testing failover), when
// false the primary just reboots in place.
//
// Reversibility: there is nothing to "undo" — a reboot is a transient
// state change. There is no rollback plan; failure modes are limited
// to "reboot didn't bring the instance back to available," which is a
// hard incident that needs human attention rather than another reboot.
type RDSRebootInstanceProposal struct {
	DBInstanceIdentifier string `json:"db_instance_identifier"`
	Region               string `json:"region"`
	ForceFailover        bool   `json:"force_failover"`
	Reasoning            string `json:"reasoning"`
}

//go:embed rds_reboot_instance.json
var rdsRebootInstanceSchemaJSON []byte

const rdsRebootInstanceSchemaURL = "https://casper.dev/schemas/rds_reboot_instance.schema.json"

var rdsRebootInstanceSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rdsRebootInstanceSchemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_reboot_instance schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(rdsRebootInstanceSchemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_reboot_instance schema: %w", err))
	}
	s, err := c.Compile(rdsRebootInstanceSchemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_reboot_instance schema: %w", err))
	}
	rdsRebootInstanceSchema = s
}

// ValidateRDSRebootInstance validates raw JSON against the action's schema.
func ValidateRDSRebootInstance(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsRebootInstanceSchema.Validate(doc)
}
