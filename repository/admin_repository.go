package repository

import (
	"database/sql"

	"workflow-example.com/model"
)

type Repository struct {
	DB *sql.DB
}

// AdminRepository Repository Interface for admin repository
type AdminRepository interface {
	GetAll() ([]model.Admin, error)
}

// GetAll Implementation of Repository Interface
func (r *Repository) GetAll() ([]model.Admin, error) {
	// todo actual implementation should be fetched from a database
	admins := []model.Admin{
		{
			Id:   "A001",
			Name: "Emmy",
			Role: "Super Admin",
		},
		{
			Id:   "A002",
			Name: "Tammy",
			Role: "Moderator",
		},
	}
	return admins, nil
}