package main

import (
	"database/sql"
	"fmt"

	"github.com/gin-gonic/gin"
	"workflow-example.com/handler"
	"workflow-example.com/repository"
	"workflow-example.com/service"
)

func main() {
	fmt.Println("Starting server")

	var db *sql.DB

	studentRepo := repository.Repository{DB: db}
	studentSvc := service.New(studentRepo)
	studentHandler := &handler.StudentHandler{Service: studentSvc}

	r := gin.Default()
	r.GET("/students", studentHandler.GetStudents)

	err := r.Run(":8080")
	if err != nil {
		return
	}
}
