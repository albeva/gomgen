package main

import (
	"fmt"
	"model"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// database connection
	db, err := sql.Open("mysql", "gomgen:dAXthbfKTzNenMRE@tcp(localhost:3306)/gomgen")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	model.Register(db);

	articles, err := model.FindArticles("ORDER BY create_date DESC")
	for _, article := range articles {
		fmt.Printf("%v\n", article)
	}

	fmt.Printf("Done\n")
}