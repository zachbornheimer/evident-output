//go:build !unix

package publish

// sameVolume is false where volume identity is not portable, so stages
// stay beside their destinations.
func sameVolume(string, string) bool { return false }
