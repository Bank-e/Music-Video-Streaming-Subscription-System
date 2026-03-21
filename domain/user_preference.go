package domain

type UserPreference struct {
	PrefID    string `json:"prefId"`
	UserID    string `json:"userId"`
	PrefKey   string `json:"prefKey"`
	PrefValue string `json:"prefValue"`
}