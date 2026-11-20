//go:build !windows

package desktop

// Viva owns native dialog history on non-Windows platforms.
func pickAppExportDestination(initialDir, title, filename string, single, native bool) (string, error) {
	return basicAppExportPicker(title, filename, single)
}
