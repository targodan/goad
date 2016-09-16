package goad

import "fmt"

func codeToError(code int) error {
	if code == 0 {
		return nil
	}
	return fmt.Errorf("Error %d", code)
}
