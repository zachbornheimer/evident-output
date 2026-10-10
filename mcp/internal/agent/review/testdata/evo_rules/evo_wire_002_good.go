// Fixture: EVO-WIRE-002 must stay silent. The version constant documents
// what changed beside the json-tagged wire struct.
package wire002

// JSONSchemaVersion is the final JSON document schema version.
// Bumped from 0.3 to 0.4: JSONTask gained warnings.
const JSONSchemaVersion = "0.4"

type JSONDocument struct {
	SchemaVersion string   `json:"schema_version"`
	Tasks         []string `json:"tasks"`
}
