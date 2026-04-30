package action

import (
	"embed"
	"fmt"
)

// schemaFiles re-embeds the per-action JSON schemas as a single FS so
// the HTTP catalog endpoint can serve them by action_type without each
// action having to register its raw bytes individually.
//
// The per-action go:embed declarations in rds_*.go files remain — they
// drive runtime validation. This is purely a catalog accessor.
//
//go:embed *.json
var schemaFiles embed.FS

// schemaFilenames maps each registered action type to its schema file.
// Adding a new action means adding the action to Registry and an entry
// here with the matching filename. The mismatch case (action without
// schema file) returns an error from SchemaJSON, surfaced as 500 from
// the API — easier to debug than silent empty bodies.
var schemaFilenames = map[string]string{
	"rds_resize":                   "schema.json",
	"rds_create_snapshot":          "rds_create_snapshot.json",
	"rds_modify_backup_retention":  "rds_modify_backup_retention.json",
	"rds_reboot_instance":          "rds_reboot_instance.json",
	"rds_modify_multi_az":          "rds_modify_multi_az.json",
	"rds_storage_grow":             "rds_storage_grow.json",
	"rds_delete_snapshot":          "rds_delete_snapshot.json",
	"rds_create_read_replica":      "rds_create_read_replica.json",
	"rds_modify_engine_version":    "rds_modify_engine_version.json",
	"rds_restore_from_snapshot":    "rds_restore_from_snapshot.json",
}

// SchemaJSON returns the raw JSON Schema bytes for an action type.
// Returns an error when the action is unregistered or its schema file
// is missing from the embedded FS.
func SchemaJSON(actionType string) ([]byte, error) {
	if _, ok := Registry[actionType]; !ok {
		return nil, fmt.Errorf("unknown action_type %q", actionType)
	}
	name, ok := schemaFilenames[actionType]
	if !ok {
		return nil, fmt.Errorf("no schema file registered for %q", actionType)
	}
	b, err := schemaFiles.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read schema for %q: %w", actionType, err)
	}
	return b, nil
}
