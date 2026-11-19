// Package export_context is the body of a Cloud SQL Admin API export: what to export, how, and
// where to.
package export_context

const (
	Kind = "sql#exportContext"

	FileTypeSql = "SQL"
	FileTypeCsv = "CSV"
)

// PostgresExportOptions are the options of a PostgreSQL SQL export.
type PostgresExportOptions struct {
	// Clean writes DROP statements ahead of the CREATE statements.
	Clean bool `json:"clean,omitzero"`
	// IfExists writes those DROP statements as DROP ... IF EXISTS.
	IfExists bool `json:"ifExists,omitzero"`
}

type SqlExportOptions struct {
	Tables     []string `json:"tables,omitzero"`
	SchemaOnly bool     `json:"schemaOnly,omitzero"`
	// Parallel exports in pg_dump's directory format, a file per table and a toc.dat beside
	// them, under the export's uri as a folder.
	Parallel bool `json:"parallel,omitzero"`
	// Threads is how many tables a parallel export writes at a time; a serverless export takes
	// at most two.
	Threads               int                    `json:"threads,omitzero"`
	PostgresExportOptions *PostgresExportOptions `json:"postgresExportOptions,omitzero"`
}

// ExportContext is the exportContext of an export request.
type ExportContext struct {
	Kind string `json:"kind,omitzero"`
	// Uri is where the export is written: gs://bucket/object for one file, or gs://bucket/folder
	// for a parallel export.
	Uri string `json:"uri"`
	// Databases names the databases to export. Left empty in a parallel export, it is every user
	// database of the instance, each in a folder of its own name.
	Databases []string `json:"databases,omitzero"`
	FileType  string   `json:"fileType,omitzero"`
	// Offload runs the export on a temporary instance of its own, a serverless export, sparing
	// the instance's resources.
	Offload          bool              `json:"offload,omitzero"`
	SqlExportOptions *SqlExportOptions `json:"sqlExportOptions,omitzero"`
}

// Request is the body of an export call.
type Request struct {
	ExportContext *ExportContext `json:"exportContext"`
}
