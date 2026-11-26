// Package schema embeds the AISLO v1alpha1 JSON Schema.
package schema

import _ "embed"

// JSON is the AISLO v1alpha1 JSON Schema (Draft 2020-12).
//
//go:embed aislo.schema.json
var JSON []byte

// ID is the schema's $id.
const ID = "https://sloeval.dev/schemas/aislo/v1alpha1.json"
