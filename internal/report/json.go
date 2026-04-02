package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DraftOps1/lgtm-link-scan/internal/model"
)

func Render(data []byte, format string) ([]byte, error) {
	var result model.ScanResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode scan result: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "markdown", "md":
		return renderMarkdown(result), nil
	case "json":
		rendered, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("render json report: %w", err)
		}
		return rendered, nil
	default:
		return nil, fmt.Errorf("unsupported report format %q", format)
	}
}
