package snapshot

import (
	"encoding/json"
	"io"

	"github.com/cpcloud/gh-pulse/internal/pulse"
)

func WriteJSON(writer io.Writer, value pulse.Snapshot) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
