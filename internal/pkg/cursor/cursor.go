// internal/pkg/cursor/cursor.go
//
// Opaque cursor encoding for keyset (cursor) pagination.
//
// Strategy: (created_at DESC, id DESC)
//
// Why two columns?
//   - created_at alone is not unique (two rows can share a timestamp).
//   - Adding id as a tiebreaker makes the cursor stable and unique.
//
// Wire format: base64url( RFC3339Nano(created_at) + "|" + uuid(id) )
// The base64 encoding makes it opaque to clients — they must not parse it.

package cursor

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const sep = "|"

// Encode builds an opaque cursor string from a row's created_at and id.
// Call this on the LAST item returned in a page.
func Encode(createdAt time.Time, id uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + sep + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// Decode parses a cursor string back into its components.
// Returns an error if the cursor is malformed or tampered with.
func Decode(c string) (createdAt time.Time, id uuid.UUID, err error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor")
	}

	parts := strings.SplitN(string(b), sep, 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor")
	}

	createdAt, err = time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor")
	}

	id, err = uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor")
	}

	return createdAt, id, nil
}
