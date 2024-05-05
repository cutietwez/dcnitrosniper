//go:build windows
// +build windows

package filelimit

// on windows it is NOT needed
func SetFileLimit() bool {
	return true
}
