package tools

import "github.com/go-scopy/go-scopy/internal/domain"

// The management data shapes live in domain so the backend contract can use
// them; tools works with the same types through aliases.
type (
	PhoneInfo = domain.PhoneInfo
	FileEntry = domain.FileEntry
	AppInfo   = domain.AppInfo
)
