package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// RDSModifyEngineVersionProposal is the typed proposal for upgrading
// an RDS instance's engine version.
//
// Reversibility: irreversible. AWS does not support downgrading the
// engine version of an existing RDS instance. Once data files are
// migrated to the new version's format, the only way "back" is
// dump → restore-from-pre-upgrade-snapshot → cutover, which is
// out of Casper's scope. Treat all engine upgrades — minor or major
// — as irreversible from the trust layer's perspective.
type RDSModifyEngineVersionProposal struct {
	DBInstanceIdentifier     string `json:"db_instance_identifier"`
	Region                   string `json:"region"`
	CurrentEngineVersion     string `json:"current_engine_version"`
	TargetEngineVersion      string `json:"target_engine_version"`
	AllowMajorVersionUpgrade bool   `json:"allow_major_version_upgrade"`
	ApplyImmediately         bool   `json:"apply_immediately"`
	Reasoning                string `json:"reasoning"`
}

//go:embed rds_modify_engine_version.json
var rdsModifyEngineVersionSchemaJSON []byte

const rdsModifyEngineVersionSchemaURL = "https://casper.dev/schemas/rds_modify_engine_version.schema.json"

var rdsModifyEngineVersionSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rdsModifyEngineVersionSchemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_modify_engine_version schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(rdsModifyEngineVersionSchemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_modify_engine_version schema: %w", err))
	}
	s, err := c.Compile(rdsModifyEngineVersionSchemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_modify_engine_version schema: %w", err))
	}
	rdsModifyEngineVersionSchema = s
}

// ValidateRDSModifyEngineVersion validates raw JSON against the action's schema.
func ValidateRDSModifyEngineVersion(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsModifyEngineVersionSchema.Validate(doc)
}
