package flexera

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func Write(w io.Writer, format string, value any) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return WriteJSON(w, value)
	case "table":
		return WriteTable(w, value)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func WriteJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func WriteErrorJSON(w io.Writer, err error) error {
	return WriteJSON(w, map[string]any{
		"error": err.Error(),
	})
}
