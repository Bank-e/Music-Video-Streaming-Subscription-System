package service

import (
	"workflow-example.com/model"
	"workflow-example.com/repository"
)

type StudentService struct {
	repo repository.Repository
}

// New Constructor for StudentService
func New(repo repository.Repository) *StudentService {
	return &StudentService{
		repo: repo,
	}
}

// Service Interface for student service
type Service interface {
	GetStudents() ([]model.Student, error)
}

// GetStudents Implementation of Service Interface
func (s *StudentService) GetStudents() ([]model.Student, error) {
	return s.repo.GetAll()
}
