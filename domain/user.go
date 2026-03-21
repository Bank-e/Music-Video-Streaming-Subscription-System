package domain

type User struct {
    UserID       string `json:"userId"`
    Email        string `json:"email"`
    PasswordHash string `json:"-"`
    CreatedAt    string `json:"createdAt"`
}