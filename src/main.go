package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"gomgen"
	"fmt"
)



func main() {
	// database connection
	schema := "gomgen"
	db, err := sql.Open("mysql", "gomgen:dAXthbfKTzNenMRE@tcp(localhost:3306)/gomgen")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Gomgen
	mgen := gomgen.NewGenerator(db, schema)

	// find all available tables
	if err := mgen.Process(); err != nil {
		panic(err)
	}

	src := mgen.Generate()
	fmt.Printf("%v\n", src)
}

