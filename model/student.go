package model

type Student struct {
	Id    string  `json:"id"`
	Name  string  `json:"name"`
	Major string  `json:"major"`
	GPA   float64 `json:"gpa"`
}
