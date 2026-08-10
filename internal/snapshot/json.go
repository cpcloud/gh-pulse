// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

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
