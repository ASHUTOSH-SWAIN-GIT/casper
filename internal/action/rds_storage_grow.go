package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// RDSStorageGrowProposal is the typed proposal for increasing an RDS
// instance's allocated storage. This action is **irreversible**: AWS
// does not support shrinking allocated storage on an existing
// instance. The policy engine treats this asymmetry as a first-class
// input and defaults to needs_approval (or deny for large jumps).
//
// Reversibility: irreversible. There is no rollback plan — once
// storage grows, the only way to "shrink" is to dump → create new
// instance with smaller storage → restore. That recovery is out of
// scope for v1 and explicitly not what Casper's rollback covers.
type RDSStorageGrowProposal struct {
	DBInstanceIdentifier      string `json:"db_instance_identifier"`
	Region                    string `json:"region"`
	CurrentAllocatedStorageGB int    `json:"current_allocated_storage_gb"`
	TargetAllocatedStorageGB  int    `json:"target_allocated_storage_gb"`
	ApplyImmediately          bool   `json:"apply_immediately"`
	Reasoning                 string `json:"reasoning"`
}

//go:embed rds_storage_grow.json
var rdsStorageGrowSchemaJSON []byte

const rdsStorageGrowSchemaURL = "https://casper.dev/schemas/rds_storage_grow.schema.json"

var rdsStorageGrowSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rdsStorageGrowSchemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_storage_grow schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(rdsStorageGrowSchemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_storage_grow schema: %w", err))
	}
	s, err := c.Compile(rdsStorageGrowSchemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_storage_grow schema: %w", err))
	}
	rdsStorageGrowSchema = s
}

// ValidateRDSStorageGrow validates raw JSON against the action's schema.
func ValidateRDSStorageGrow(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsStorageGrowSchema.Validate(doc)
}
