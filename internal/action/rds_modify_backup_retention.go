package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// RDSModifyBackupRetentionProposal is the typed proposal for changing
// an RDS instance's automated backup retention period.
//
// Reversibility: reversible — backup retention is a metadata setting;
// flipping it back returns the instance to its prior state. There is
// a subtle caveat: shortening retention deletes any automated backups
// older than the new window, which cannot be undone. The policy
// engine should treat large reductions as needs_approval.
type RDSModifyBackupRetentionProposal struct {
	DBInstanceIdentifier string `json:"db_instance_identifier"`
	Region               string `json:"region"`
	CurrentRetentionDays int    `json:"current_retention_days"`
	TargetRetentionDays  int    `json:"target_retention_days"`
	ApplyImmediately     bool   `json:"apply_immediately"`
	Reasoning            string `json:"reasoning"`
}

//go:embed rds_modify_backup_retention.json
var rdsModifyBackupRetentionSchemaJSON []byte

const rdsModifyBackupRetentionSchemaURL = "https://casper.dev/schemas/rds_modify_backup_retention.schema.json"

var rdsModifyBackupRetentionSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rdsModifyBackupRetentionSchemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_modify_backup_retention schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(rdsModifyBackupRetentionSchemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_modify_backup_retention schema: %w", err))
	}
	s, err := c.Compile(rdsModifyBackupRetentionSchemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_modify_backup_retention schema: %w", err))
	}
	rdsModifyBackupRetentionSchema = s
}

// ValidateRDSModifyBackupRetention validates raw JSON against the
// action's schema.
func ValidateRDSModifyBackupRetention(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsModifyBackupRetentionSchema.Validate(doc)
}
