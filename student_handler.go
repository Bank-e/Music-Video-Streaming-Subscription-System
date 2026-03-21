package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"workflow-example.com/service"
)

type StudentHandler struct {
	Service *service.StudentService
}

func (h *StudentHandler) GetStudents(c *gin.Context) {
	students, err := h.Service.GetStudents()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, students)
}
