package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func decodeJSON(resp *http.Response, target any) error {
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return PackageNotFoundError{}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode registry response: %w", err)
	}
	return nil
}
