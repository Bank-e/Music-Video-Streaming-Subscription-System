package domain

type Profile struct {
	ProfileID   string `json:"profileId"`
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}