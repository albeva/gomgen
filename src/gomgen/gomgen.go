package gomgen

import (
	"bitbucket.org/pkg/inflect"
	"bytes"
	"database/sql"
	"go/format"
	"strings"
	"text/template"
)

// Gomgen generator is the primary interface for scanning,
// analyzing and generating models with gomgen
type Generator struct {
	Db      *sql.DB
	Schema  string
	Tables  []*Table
	Imports map[string]bool
	Output  *bytes.Buffer
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
		Output: &bytes.Buffer{},
	}
}

// Investigate the database
func (this *Generator) Analyse() error {
	mysql := &Mysql{}
	return mysql.Analyze(this)
}

// Generate the model source code
func (this *Generator) Generate() error {
	var t = template.Must(template.New("headerTpl").Parse(headerTpl))
	if err := t.Execute(this.Output, this); err != nil {
		return err
	}

	// entities
	for _, table := range this.Tables {
		this.genStruct(table)
		this.genScanFn(table)
	}

	// format the code
	c, err := format.Source(this.Output.Bytes())
	if err != nil {
		// return err
	}
	this.Output.Reset()
	this.Output.Write(c)

	// done :)
	return nil
}

// generate the table entity
func (this *Generator) genStruct(table *Table) error {
	var t = template.Must(template.New("entityStructTpl").Parse(entityStructTpl))
	return t.Execute(this.Output, table)
}

// Generate scan function
func (this *Generator) genScanFn(table *Table) error {
	// the template
	var t = template.Must(template.New("scanEntity").Parse(scanEntityTpl))

	// template params
	type templateParams struct {
		*Table
		Vars   map[string]string // declared extra variables
		Params string // params for the Scan method
		Inits  []string // value loads for the variables
	}
	p := &templateParams{}
	p.Table = table
	p.Vars = make(map[string]string)

	// process fields
	var params []string
	for _, field := range table.Fields {
		if field.Type == GoTime {
			if _, ok := p.Vars["string"]; ok {
				p.Vars["string"] += ", " + field.Name
			} else {
				p.Vars["string"] = field.Name
			}
			params = append(params, "&" + field.Name)
			init := "if t, err := time.Parse(\"" + field.Format + "\", " + field.Name + "); err != nil {\n"
			init += "	return err\n"
			init += "} else {\n"
			init += "	this." + field.Name + " = t\n"
			init += "}"
			p.Inits = append(p.Inits, init)
		} else {
			params = append(params, "&this." + field.Name)
		}
	}
	p.Params = strings.Join(params, ", ")

	// process
	return t.Execute(this.Output, p)
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
	SqlName	string
	Default  sql.NullString
	Nullable bool
	Type     GoType
	GoType   string
	Primary  bool
	Comment  string
	Format 	 string
}

// the name of the field
func NewField(rawName string) *Field {
	parts := strings.Split(strings.ToLower(rawName), "_")
	for i := 0; i < len(parts); i++ {
		parts[i] = strings.Title(parts[i])
	}
	return &Field{
		Name: strings.Join(parts, ""),
		SqlName: rawName,
	}
}
