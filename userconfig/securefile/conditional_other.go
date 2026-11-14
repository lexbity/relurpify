//go:build !linux && !darwin

package securefile

import (
	"fmt"
	"os"
)

func openRegularWithin(*os.Root, string) (*os.File, error) {
	return nil, fmt.Errorf("confined policy file operations require Linux or macOS")
}
