//go:build live
// +build live

package greet

// Greet returns a friendly greeting.
func Greet(name string) string {
	return "Hi, " + name
}
