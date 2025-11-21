package apperrors

const (
	UsernameNickTaken           = "Username or nickname is already taken."
	InternalServer              = "Something went wrong on our side."
	MissingRequiredFields       = "Some required fields are missing."
	UserNameEmpty               = "Username can not be empty."
	PasswordEmpty               = "Password can not be empty."
	InvalidUsernameOrPassword   = "Invalid username or password."
	NickAlreadyExists           = "Nick already exists."
	AuthenticationRequired      = "Authentication required."
	MissingOrInvalidCredentials = "Missing or invalid credentials."
	MissingMetadata             = "Missing metadata."
	MissingAuthorization        = "Missing authorization header."
)
