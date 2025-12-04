package viewmodel

// User represents the authenticated user context exposed to templates.
type User struct {
	Email string
	Role  string
}

// Layout captures shared chrome metadata (titles, navigation state, auth flags).
type Layout struct {
	Title              string
	PageTitle          string
	CurrentPage        string
	CSRFToken          string
	IsAuthenticated    bool
	CanManageAllowlist bool
	CanManageJobs      bool
	User               *User
}

// LayoutProvider exposes layout metadata for renderer utilities.
type LayoutProvider interface {
	LayoutData() *Layout
}
