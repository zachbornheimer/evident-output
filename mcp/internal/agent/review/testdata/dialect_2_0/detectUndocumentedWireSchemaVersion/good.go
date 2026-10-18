package fixture

// ReportSchemaVersion is 0.4: tasks became a list of names (0.3 held objects).
const ReportSchemaVersion = "0.4"

type Report struct {
	SchemaVersion string   `json:"schema_version"`
	Tasks         []string `json:"tasks"`
}
