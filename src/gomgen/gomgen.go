package gomgen

import (
	"bitbucket.org/pkg/inflect"
	"database/sql"
	"regexp"
	"strconv"
	"strings"
	"go/format"
)

// Gomgen generator is the primary interface for scanning,
// analyzing and generating models with gomgen
type Generator struct {
	Db      *sql.DB
	Schema  string
	Tables  []*Table
	Imports map[string]bool
}

// create and initialize new Gomgen object
func NewGenerator(db *sql.DB, schema string) *Generator {
	return &Generator{
		Db:     db,
		Schema: schema,
		Tables: nil,
		Imports: map[string]bool{
			"database/sql": true,
		},
	}
}

// Investigate the database
func (this *Generator) Process() error {
	// fetch the existing tables from the database
	if err := this.FetchTables(); err != nil {
		return err
	}

	// fetch the table columns
	for _, table := range this.Tables {
		if err := this.FetchColumns(table); err != nil {
			return err
		}
	}

	return nil
}

// get list of available tables
func (this *Generator) FetchTables() error {
	// get the information from the information_schema
	SQL := `
		SELECT   Tables.TABLE_NAME,
				 Tables.TABLE_COMMENT
		FROM     information_schema.TABLES AS Tables
		WHERE    Tables.TABLE_SCHEMA = ? AND Tables.TABLE_TYPE = "BASE TABLE"
		ORDER BY Tables.TABLE_NAME
	`
	rows, err := this.Db.Query(SQL, this.Schema)
	if err != nil {
		return err
	}
	defer rows.Close()

	// process the result
	for rows.Next() {
		var name, comment string
		if err := rows.Scan(&name, &comment); err != nil {
			return err
		}
		this.Tables = append(this.Tables, NewTable(name, comment))
	}

	return nil
}

// Fetch table columns
func (this *Generator) FetchColumns(table *Table) error {
	// get information about table columns. Do not use DESCRIBE
	// because this provides more information
	SQL := `
		SELECT		Columns.COLUMN_NAME,
					Columns.COLUMN_DEFAULT,
					Columns.IS_NULLABLE,
					Columns.COLUMN_TYPE,
					Columns.COLUMN_KEY,
					Columns.EXTRA,
					Columns.COLUMN_COMMENT
		FROM		information_schema.COLUMNS AS Columns
		WHERE		Columns.TABLE_SCHEMA = ? AND Columns.TABLE_NAME = ?
		ORDER BY	Columns.ORDINAL_POSITION
	`
	rows, err := this.Db.Query(SQL, this.Schema, table.Name)
	if err != nil {
		return err
	}
	defer rows.Close()

	// process rows
	for rows.Next() {
		var name, nullable, typ, key, extra, comment string
		var def sql.NullString
		if err := rows.Scan(&name, &def, &nullable, &typ, &key, &extra, &comment); err != nil {
			return err
		}

		// add field to the table
		field := NewField(name)
		field.Default = def
		field.Nullable = nullable == "YES"
		field.Comment = comment
		field.Type = sqlToGoType(typ, field.Nullable)
		field.Primary = key == "PRI"
		field.Comment = comment

		// add to table identity
		if field.Primary {
			table.Identity = append(table.Identity, name)
		}

		// need to import time?
		if field.Type == GoTime {
			this.Imports["time"] = true
		}

		table.Fields = append(table.Fields, field)
	}

	// done
	return nil
}

// use this to decode sql types. int(11), ...
var sqlTypeMatch = regexp.MustCompile(`^([a-zA-Z_]+)\(([0-9]+)(,[0-9]+)?\)$`)

// convert sql data type to go type
func sqlToGoType(sqlType string, nullable bool) GoType {

	t := sqlTypeMatch.FindStringSubmatch(sqlType)
	size := int64(-1)
	if len(t) > 0 {
		sqlType = t[1]
		size, _ = strconv.ParseInt(t[2], 10, 32)
	}

	switch sqlType {
	case "int", "smallint", "tinyint", "bool":
		if size == 1 || sqlType == "bool" {
			if nullable {
				return GoNullBool
			}
			return GoBool
		}
		if nullable {
			return GoNullInt
		}
		return GoInt
	case "timestamp":
		if nullable {
			return GoNullInt
		}
		return GoInt
	case "float", "double", "decimal":
		if nullable {
			return GoNullFloat64
		}
		return GoFloat64
	case "text", "enum", "set":
		if nullable {
			return GoNullString
		}
		return GoString
	case "datetime", "time", "date":
		if nullable {
			panic("Nuulable datetime not implemented yet")
		}
		return GoTime
	}

	// default to string
	return GoString
}

// Generate the model source code
func (this *Generator) Generate() string {
	// header
	code := "// Autogeneratoed by gomgen\n"

	// imports
	code += "import (\n"
	for k, _ := range this.Imports {
		code += "\"" + k + "\"\n"
	}
	code += ")\n"

	code += `
		// database connection
		var theDb *sql.Db

		// register db object for use with models
		func Register(db *sql.DB) error {
			theDb = db
			return nil
		}
	`

	// entities
	for _, table := range this.Tables {
		code += generateTable(table)
	}

	// format the code
	c, err := format.Source([]byte(code));
	if err != nil {
		panic(err)
	}

	// done :)
	return string(c)
}

// generate the table entity
func generateTable(table *Table) string {
	// declare
	code := "\ntype " + table.EntitySingular + " struct {\n"

	// field
	for _, field := range table.Fields {
		code += field.Name + " " + GoTypeMap[field.Type] + "\n"
	}

	// done
	code += "}\n"
	return code
}

// represent a database table
type Table struct {
	Name           string
	EntitySingular string
	EntityPlural   string
	Comment        string
	Fields         []*Field
	Identity       []string
}

// create new table
func NewTable(sqlName, comment string) *Table {
	name := strings.ToLower(sqlName)
	singular := inflect.Singularize(name)
	plural := inflect.Pluralize(name)
	return &Table{
		Name:           sqlName,
		Comment:        comment,
		EntitySingular: strings.Title(singular),
		EntityPlural:   strings.Title(plural),
	}
}

// Field data type mapping to Go
type GoType int

const (
	GoInt GoType = iota
	GoFloat64
	GoBool
	GoString
	GoTime
	GoNullInt
	GoNullFloat64
	GoNullBool
	GoNullString
)

// map GoType constants to strings of actual types
var GoTypeMap = map[GoType]string{
	GoInt:         "int",
	GoFloat64:     "float64",
	GoBool:        "bool",
	GoString:      "string",
	GoTime:        "time.Time",
	GoNullInt:     "sql.NullInt64",
	GoNullFloat64: "sql.NullFloat64",
	GoNullBool:    "sql.NullBool",
	GoNullString:  "sql.NullString",
}

// represent individual field in the table
type Field struct {
	Name     string
	Default  sql.NullString
	Nullable bool
	Type     GoType
	Primary  bool
	Comment  string
}

// the name of the field
func NewField(name string) *Field {
	return &Field{Name: name}
}
