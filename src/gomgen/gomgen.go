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
			"errors":       true,
			"fmt":          true,
		},
		Output: &bytes.Buffer{},
	}
}

// find table by name
func (this *Generator) GetTable(name string) *Table {
	for _, table := range this.Tables {
		if table.Name == name {
			return table
		}
	}
	return nil
}

// Investigate the database
func (this *Generator) Analyse() error {
	mysql := &Mysql{}
	return mysql.Analyze(this)
}

// Generate the model source code
func (this *Generator) Generate() error {
	// entities
	for _, table := range this.Tables {
		this.genStruct(table)
		this.genScanFn(table)
		this.genFindFn(table)
		this.genSaveFn(table)
		this.genRelFn(table)
	}

	// generate the header
	var header = bytes.Buffer{}
	var t = template.Must(template.New("headerTpl").Parse(headerTpl))
	if err := t.Execute(&header, this); err != nil {
		return err
	}

	header.Write(this.Output.Bytes())
	this.Output = &header

	// format the code
	if true {
		c, err := format.Source(this.Output.Bytes())
		if err != nil {
			return err
		}
		this.Output.Reset()
		this.Output.Write(c)
	}

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
		Params string            // params for the Scan method
		Inits  []string          // value loads for the variables
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
			params = append(params, "&"+field.Name)
			init := "this." + field.Name + ", _ = time.Parse(\"" + field.Format + "\", " + field.Name + ")"
			p.Inits = append(p.Inits, init)
		} else {
			params = append(params, "&this."+field.Name)
		}
	}
	p.Params = strings.Join(params, ", ")

	// process
	return t.Execute(this.Output, p)
}

// find function
func (this *Generator) genFindFn(table *Table) error {
	type params struct {
		*Table
		IdentityField *Field
	}
	p := params{Table: table}

	// singly identifiable table
	if len(table.Identity) == 1 {
		id := table.Identity[0]
		if id.Type == GoInt {
			p.IdentityField = id
			this.Imports["strconv"] = true
		}
	}

	// render
	var t = template.Must(template.New("findEntityTpl").Parse(findEntityTpl))
	return t.Execute(this.Output, p)
	return nil
}

// generate the table entity
func (this *Generator) genSaveFn(table *Table) error {
	type params struct {
		*Table
		IdCheck      string
		InsertCols   string
		InsertVals   string
		UpdateVals   string
		Where        string
		UpdateParams string
		InsertParams string
		AutoIncField *Field
	}
	p := &params{Table: table}

	// identity check to know if insert or update
	for _, field := range table.Identity {
		if len(p.IdCheck) > 0 {
			p.IdCheck += " && "
		}
		p.IdCheck += "this." + field.Name + " == "
		if field.Type == GoInt || field.Type == GoFloat64 {
			p.IdCheck += "0"
		} else if field.Type == GoString {
			p.IdCheck += "\"\""
		}

		if len(p.Where) > 0 {
			p.Where += " AND "
		}
		p.Where += field.EscapedName + " = ?"

		if len(p.UpdateParams) > 0 {
			p.UpdateParams += ", "
		}
		p.UpdateParams += "this." + field.Name

		if field.AutoInc {
			p.AutoIncField = field
		}
	}

	// insert / update cols
	first := true
	for _, field := range table.Fields {
		// skip primary key columns
		if field.Primary {
			continue
		}
		// separate
		if !first {
			p.InsertCols += ", "
			p.InsertVals += ", "
			p.UpdateVals += ", "
			p.InsertParams += ", "
		}
		first = false

		p.InsertCols += field.EscapedName
		p.InsertVals += "?"
		p.UpdateVals += field.EscapedName + " = ?"

		p.InsertParams += "this." + field.Name
		if field.Type == GoTime {
			p.InsertParams += ".Format(\"" + field.Format + "\")"
		}
	}

	// update params
	p.UpdateParams = p.InsertParams + ", " + p.UpdateParams

	// render the template
	var t = template.Must(template.New("entitySaveTpl").Parse(entitySaveTpl))
	return t.Execute(this.Output, p)
}

const entityOneToOneTpl = `
// find related {{ .TargetEntity.EntitySingular }}
func (this *{{ .Table.EntitySingular }}) Find{{ .Name }}() (*{{ .TargetEntity.EntitySingular }}, error) {
	sql := "WHERE {{ .TargetEntity.EscapedName }}.{{ .TargetColumn.EscapedName }} = ?"
	return Find{{ .TargetEntity.EntitySingular }}(sql, this.{{ .Column.Name }})
}
`

// generate relations
func (this *Generator) genRelFn(table *Table) error {
	var t = template.Must(template.New("entityOneToOneTpl").Parse(entityOneToOneTpl))
	for _, rel := range table.Relations {
		return t.Execute(this.Output, rel)
	}
	return nil
}

// specify the relation type between the entities
type RelationType int

const (
	OneToOne RelationType = iota
	OneToMany
	ManyToMany
)

// represent a relation between the tables
type Relation struct {
	Name            string
	Table 			*Table
	Column          *Field // null for many-to-many
	TargetEntity    *Table
	TargetColumn    *Field // null for many-to-many
	MiddleEntity    *Table // connecting table
	MiddleSrcColumn *Field // point to this entity
	MiddleDstColumn *Field // point to target entity
}

// Create new relation object
func NewRelation(name string) *Relation {
	return &Relation{
		Name:  strings.Title(strings.ToLower(name)),
	}
}

// represent a database table
type Table struct {
	Name           string
	EscapedName    string
	EntitySingular string
	EntityPlural   string
	Comment        string
	Fields         []*Field
	Identity       []*Field
	Relations      []*Relation
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

// get field by name
func (this *Table) GetField(name string) *Field {
	for _, field := range this.Fields {
		if field.RealName == name {
			return field
		}
	}
	return nil
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
	GoInt:         "int64",
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
	Name        string
	EscapedName string
	RealName	string
	Default     sql.NullString
	Nullable    bool
	Type        GoType
	GoType      string
	Primary     bool
	AutoInc     bool
	Comment     string
	Format      string
}

// the name of the field
func NewField(rawName string) *Field {
	parts := strings.Split(strings.ToLower(rawName), "_")
	for i := 0; i < len(parts); i++ {
		parts[i] = strings.Title(parts[i])
	}
	return &Field{
		Name: strings.Join(parts, ""),
		RealName: rawName,
	}
}
