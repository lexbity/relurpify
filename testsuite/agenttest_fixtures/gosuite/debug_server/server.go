//go:build live
// +build live

package debug_server

// GetUser returns the user entry if present.
func GetUser(id string) string {
	users := map[string]string{}
	return users[id]
}
