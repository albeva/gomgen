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

	// fetch rows
	rows, err := db.Query("SELECT * FROM article ORDER BY create_date DESC")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	// process
	for rows.Next() {
		article := &model.Article{}
		if err := article.Scan(rows); err != nil {
			panic(err)
		}
		fmt.Printf("Article: %v\n", article.CreateDate)
	}

	fmt.Printf("Done\n")
}