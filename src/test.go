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

	// find
	article, _ := model.FindArticle(1)
	category, _ := article.FindCategory();
	fmt.Printf("category: %v\n", category.Name)

	fmt.Printf("Done\n")
}
