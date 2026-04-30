package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// RDSCreateReadReplicaProposal is the typed proposal for creating a
// new read replica off an existing primary RDS instance.
//
// Reversibility: reversible — rollback deletes the just-created
// replica. Note: replicas accumulate replication lag over time, so a
// rollback that runs days later might leave the replica in a state
// that's been actively used (read traffic, etc.) — but for v1 the
// assumption is rollback runs immediately after a failed create, not
// long after.
type RDSCreateReadReplicaProposal struct {
	SourceDBInstanceIdentifier  string `json:"source_db_instance_identifier"`
	ReplicaDBInstanceIdentifier string `json:"replica_db_instance_identifier"`
	Region                      string `json:"region"`
	ReplicaInstanceClass        string `json:"replica_instance_class"`
	Reasoning                   string `json:"reasoning"`
}

//go:embed rds_create_read_replica.json
var rdsCreateReadReplicaSchemaJSON []byte

const rdsCreateReadReplicaSchemaURL = "https://casper.dev/schemas/rds_create_read_replica.schema.json"

var rdsCreateReadReplicaSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rdsCreateReadReplicaSchemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_create_read_replica schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(rdsCreateReadReplicaSchemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_create_read_replica schema: %w", err))
	}
	s, err := c.Compile(rdsCreateReadReplicaSchemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_create_read_replica schema: %w", err))
	}
	rdsCreateReadReplicaSchema = s
}

// ValidateRDSCreateReadReplica validates raw JSON against the action's schema.
func ValidateRDSCreateReadReplica(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsCreateReadReplicaSchema.Validate(doc)
}
