package manifest

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// FetchYAMLDocuments fetches YAML content from a URL and splits it into separate documents
func FetchYAMLDocuments(url string) ([][]byte, error) {
	// Fetch content
	resp, err := http.Get(url) //nolint:gosec // URL is from trusted internal config (Kwok manifest URLs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Split YAML documents by ---
	documents := strings.Split(string(data), "\n---\n")
	var result [][]byte

	for _, doc := range documents {
		trimmed := strings.TrimSpace(doc)
		if trimmed == "" {
			continue
		}
		result = append(result, []byte(trimmed))
	}

	return result, nil
}
