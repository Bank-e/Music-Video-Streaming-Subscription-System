// service/student_service.go
package service

import (
	"errors"
	"fmt"

	"workflow-example.com/model"
	"workflow-example.com/repository"
)

// StudentService Interface 
type StudentService interface {
	Create(student model.Student) error
	GetByID(id int) (*model.Student, error)
	Update(student model.Student) error
	Delete(id int) error
	List() ([]model.Student, error)
}

// studentServiceImpl implementation
type studentServiceImpl struct {
	repo repository.Repository
}

// NewStudentService constructor
func NewStudentService(repo repository.Repository) StudentService {
	return &studentServiceImpl{
		repo: repo,
	}
}

// Create new student
func (s *studentServiceImpl) Create(student model.Student) error {
	if student.ID == 0 {
		return errors.New("invalid student id")
	}

	err := s.repo.Create(student)
	if err != nil {
		return err
	}

	fmt.Println("Student created:", student.Name)
	return nil
}

// Get student by ID
func (s *studentServiceImpl) GetByID(id int) (*model.Student, error) {
	student, err := s.repo.GetByID(id)
	if err != nil {
		return nil, errors.New("student not found")
	}
	return student, nil
}

// Update student
func (s *studentServiceImpl) Update(student model.Student) error {
	err := s.repo.Update(student)
	if err != nil {
		return errors.New("student not found")
	}

	fmt.Println("Student updated:", student.Name)
	return nil
}

// Delete student
func (s *studentServiceImpl) Delete(id int) error {
	err := s.repo.Delete(id)
	if err != nil {
		return errors.New("student not found")
	}

	fmt.Println("Student deleted with ID:", id)
	return nil
}

// List all students
func (s *studentServiceImpl) List() ([]model.Student, error) {
	return s.repo.GetAll()
}