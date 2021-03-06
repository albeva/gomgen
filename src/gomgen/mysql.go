package gomgen

import (
	"database/sql"
	"regexp"
	"fmt"
	"strconv"
)

// mysql analyzer
type Mysql struct {
	gen *Generator
}

// analyze mysql table
func (this *Mysql) Analyze(gen *Generator) error {
	this.gen = gen

	// fetch the tables
	if err := this.fetchTables(); err != nil {
		return err
	}

	// fetch the columns
	for _, table := range this.gen.Tables {
		if err := this.fetchColumns(table); err != nil {
			return err
		}
	}

	// fetch the references
	for _, table := range this.gen.Tables {
		if err := this.fetchRelations(table); err != nil {
			return err
		}
	}

	// done
	return nil
}

// get list of available tables
func (this *Mysql) fetchTables() error {
	// get the information from the information_schema
	SQL := `
		SELECT   Tables.TABLE_NAME,
				 Tables.TABLE_COMMENT
		FROM     information_schema.TABLES AS Tables
		WHERE    Tables.TABLE_SCHEMA = ? AND Tables.TABLE_TYPE = "BASE TABLE"
		ORDER BY Tables.TABLE_NAME
	`
	rows, err := this.gen.Db.Query(SQL, this.gen.Schema)
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
		table := NewTable(name, comment)
		table.EscapedName = "`" + name + "`"
		this.gen.Tables = append(this.gen.Tables, table)
	}

	return nil
}

// mathc foo_id, article_id field names for relations
var sqlTableIdFieldMatch = regexp.MustCompile(`^([a-zA-Z0-9_]+)_id$`)


// fetch table relations
// support for:
// One to Many - A can relate to many B
// One to One  - A can relate to one B
func (this *Mysql) fetchRelations(table *Table) error {
	// fetch info
	SQL := `
		SELECT 	Relations.CONSTRAINT_NAME,
				Relations.COLUMN_NAME,
				Relations.REFERENCED_TABLE_NAME,
				Relations.REFERENCED_COLUMN_NAME
		FROM    information_schema.KEY_COLUMN_USAGE AS Relations
		WHERE	Relations.CONSTRAINT_SCHEMA = ? AND
				Relations.TABLE_SCHEMA = ? AND
				Relations.REFERENCED_TABLE_SCHEMA = ? AND
				Relations.TABLE_NAME = ? AND 
				Relations.REFERENCED_TABLE_NAME IS NOT NULL AND Relations.REFERENCED_COLUMN_NAME IS NOT NULL
	`
	schema := this.gen.Schema
	rows, err := this.gen.Db.Query(SQL, schema, schema, schema, table.Name)
	if err != nil {
		return err
	}
	defer rows.Close()

	// process rows
	for rows.Next() {
		var name, srcColumn, dstTable, dstColumn string
		if err := rows.Scan(&name, &srcColumn, &dstTable, &dstColumn); err != nil {
			return err
		}

		fmt.Printf("%v: %v -> %v.%v\n", name, srcColumn, dstTable, dstColumn)

		// relation name
		t := sqlTableIdFieldMatch.FindStringSubmatch(srcColumn)
		if len(t) > 1 {
			name = t[1]
		}

		// add relation to the source
		srcRelation := NewRelation(name)
		srcRelation.Table = table
		srcRelation.Column = table.GetField(srcColumn)
		srcRelation.TargetEntity = this.gen.GetTable(dstTable)
		srcRelation.TargetColumn = srcRelation.TargetEntity.GetField(dstColumn)
		table.Relations = append(table.Relations, srcRelation)
	}
	return nil
}


// Fetch table columns
func (this *Mysql) fetchColumns(table *Table) error {
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
	rows, err := this.gen.Db.Query(SQL, this.gen.Schema, table.Name)
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
		field.EscapedName = "`" + name + "`"
		field.Default = def
		field.Nullable = nullable == "YES"
		field.Comment = comment
		field.Type = this.detetcType(typ, field.Nullable)
		field.GoType = GoTypeMap[field.Type]
		field.Primary = key == "PRI"
		field.AutoInc = field.Primary && extra == "auto_increment"
		field.Comment = comment

		// add to table identity
		if field.Primary {
			table.Identity = append(table.Identity, field)
		}

		// need to import time?
		if field.Type == GoTime {
			this.gen.Imports["time"] = true
			field.Format = sqlTimeFormats[typ]
		}

		table.Fields = append(table.Fields, field)
	}

	// done
	return nil
}

// use this to decode sql types. int(11), ...
var sqlTypeMatch = regexp.MustCompile(`^([a-zA-Z_]+)\(([0-9]+)(,[0-9]+)?\)$`)

// sql time formats
var sqlTimeFormats = map[string]string{
	"datetime": "2006-01-02 15:04:05",
	"date":		"2006-01-02",
	"time":		"15:04:05",
}

// convert sql data type to go type
func (this *Mysql) detetcType(sqlType string, nullable bool) GoType {

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
