package identity

import "streaming-subscription-system/identity/domain"

/****************************************  API  ***********************************************/
// UserService defines the contract for user-related operations
type UserService interface {
	// User
	Register(email string, password string) (*User, error)
	GetUserByID(userID string) (*User, error)
	ChangeEmail(userID string, newEmail string) (bool, error)
	ChangePassword(userID string, oldPassword string, newPassword string) (bool, error)
 
	// Profile
	GetProfileByUserID(userID string) (*Profile, error)
	UpdateProfile(userID string, displayName string, avatarURL string) (bool, error)
 
	// UserPreference
	GetAllUserPreferences(userID string) ([]*UserPreference, error)
	GetUserPreference(userID string, prefKey string) (string, error)
	SetUserPreference(userID string, prefKey string, prefValue string) (bool, error)
	RemoveUserPreference(userID string, prefKey string) (bool, error)
}
 
/****************************************  Method for API  ***********************************************/
// UserRepository defines the contract for user data access
type UserRepository interface {
	Save(user *User) error
	FindByID(userID string) (*User, error)
	FindByEmail(email string) (*User, error)
	UpdateEmail(userID string, newEmail string) error
	UpdatePasswordHash(userID string, passwordHash string) error
}
 
// ProfileRepository defines the contract for profile data access
type ProfileRepository interface {
	Save(profile *Profile) error
	FindByUserID(userID string) (*Profile, error)
	Update(profile *Profile) error
}
 
// UserPreferenceRepository defines the contract for user preference data access
type UserPreferenceRepository interface {
	Save(pref *UserPreference) error
	FindAllByUserID(userID string) ([]*UserPreference, error)
	FindByUserIDAndKey(userID string, prefKey string) (*UserPreference, error)
	Upsert(pref *UserPreference) error
	DeleteByUserIDAndKey(userID string, prefKey string) error
}