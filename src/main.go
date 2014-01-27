package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"fmt"
	"gomgen"
	"os"
	"io/ioutil"
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

	// Analyze
	if err := mgen.Analyse(); err != nil {
		panic(err)
	}

	// generate
	if err := mgen.Generate(); err != nil {
		panic(err)
	}

	// working path
	wp, _ := os.Getwd()

	// write the file
	ioutil.WriteFile(wp + "/model/model.go", mgen.Output.Bytes(), 0644)

	// done
	fmt.Printf("done\n")
}
