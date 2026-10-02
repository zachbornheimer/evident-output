// Fixture: EVO-WIRE-002 must fire. The schema version constant records no
// history beside json-tagged wire structs.
package wire002

const JSONSchemaVersion = "0.4"

type JSONDocument struct {
	SchemaVersion string   `json:"schema_version"`
	Tasks         []string `json:"tasks"`
}
