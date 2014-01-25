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

	// find 1
	article := &model.Article{}
	article.Active = true
	article.Title = "New Article"
	article.Content = "Some random content"
	article.CategoryId = 4
	error := article.Save()
	if error != nil {
		panic(error)
	}

	fmt.Printf("Done\n")
}
