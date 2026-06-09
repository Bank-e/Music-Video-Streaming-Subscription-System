// service/admin_service.go
package service

import (
	"errors"
	"fmt"
)

// Admin represents an admin user
type Admin struct {
	ID       int
	Name     string
	Email    string
	Password string
}

// AdminService defines methods to manage admins
type AdminService interface {
	Create(admin Admin) error
	GetByID(id int) (*Admin, error)
	Update(admin Admin) error
	Delete(id int) error
	List() ([]Admin, error)
}

// adminServiceImpl is the concrete implementation of AdminService
type adminServiceImpl struct {
	admins map[int]Admin // simple in-memory store
}

// NewAdminService returns a new AdminService instance
func NewAdminService() AdminService {
	return &adminServiceImpl{
		admins: make(map[int]Admin),
	}
}

// Create adds a new admin
func (s *adminServiceImpl) Create(admin Admin) error {
	if _, exists := s.admins[admin.ID]; exists {
		return errors.New("admin already exists")
	}
	s.admins[admin.ID] = admin
	fmt.Println("Admin created:", admin.Name)
	return nil
}

// GetByID returns an admin by ID
func (s *adminServiceImpl) GetByID(id int) (*Admin, error) {
	admin, exists := s.admins[id]
	if !exists {
		return nil, errors.New("admin not found")
	}
	return &admin, nil
}

// Update modifies an existing admin
func (s *adminServiceImpl) Update(admin Admin) error {
	if _, exists := s.admins[admin.ID]; !exists {
		return errors.New("admin not found")
	}
	s.admins[admin.ID] = admin
	fmt.Println("Admin updated:", admin.Name)
	return nil
}

// Delete removes an admin by ID
func (s *adminServiceImpl) Delete(id int) error {
	if _, exists := s.admins[id]; !exists {
		return errors.New("admin not found")
	}
	delete(s.admins, id)
	fmt.Println("Admin deleted with ID:", id)
	return nil
}

// List returns all admins
func (s *adminServiceImpl) List() ([]Admin, error) {
	var list []Admin
	for _, a := range s.admins {
		list = append(list, a)
	}
	return list, nil
}
