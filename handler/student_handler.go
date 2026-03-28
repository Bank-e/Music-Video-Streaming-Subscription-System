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

func (h *StudentHandler) GetStudentByID(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
        return
    }

    student, err := h.Service.GetStudentByID(uint(id))
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "student not found"})
        return
    }
    c.JSON(http.StatusOK, student)
}