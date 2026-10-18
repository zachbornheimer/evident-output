// Fixture: EVO-WIRE-002 must fire. The schema version constant records no
// history beside json-tagged wire structs.
package fixture

const ReportSchemaVersion = "0.4"

type Report struct {
	SchemaVersion string   `json:"schema_version"`
	Tasks         []string `json:"tasks"`
}
