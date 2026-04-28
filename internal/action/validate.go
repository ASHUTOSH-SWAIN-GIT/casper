package action

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema.json
var schemaJSON []byte

const schemaURL = "https://casper.dev/schemas/rds_resize.schema.json"

var rdsResizeSchema *jsonschema.Schema

func init() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		panic(fmt.Errorf("casper: parse embedded rds_resize schema: %w", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaURL, doc); err != nil {
		panic(fmt.Errorf("casper: register rds_resize schema: %w", err))
	}
	s, err := c.Compile(schemaURL)
	if err != nil {
		panic(fmt.Errorf("casper: compile rds_resize schema: %w", err))
	}
	rdsResizeSchema = s
}

// Validate checks raw JSON bytes against the RDSResizeProposal schema.
// Returns nil if valid; otherwise a jsonschema.ValidationError describing
// every violation (path, keyword, expected vs. actual).
func Validate(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse proposal: %w", err)
	}
	return rdsResizeSchema.Validate(doc)
}
