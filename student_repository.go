//branch feat-A/repository from 6609650749
package repository

import (
	"database/sql"

	"workflow-example.com/model"
)

type Repository struct {
	DB *sql.DB
}

// StudentRepository Repository Interface for student repository
type StudentRepository interface {
	GetAll() ([]model.Student, error)
}

// GetAll Implementation of Repository Interface
func (r *Repository) GetAll() ([]model.Student, error) {
	// todo actual implementation should be fetched from a database
	students := []model.Student{
		{
			Id:    "S001",
			Name:  "Emmy",
			Major: "Computer Science",
			GPA:   3.00,
		},
		{
			Id:    "S002",
			Name:  "Tammy",
			Major: "Computer Science",
			GPA:   3.50,
		},
	}
	return students, nil
}
