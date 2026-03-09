// internal/service/location/service.go
package location

import (
	_ "embed"
	"encoding/json"
	"log"
	"strings"
)

//go:embed nigeria_lga.json
var nigeriaLGAData []byte

type stateEntry struct {
	State string   `json:"state"`
	LGAs  []string `json:"lgas"`
}

// Service holds the pre-loaded Nigeria location data
type Service struct {
	// stateIndex maps lowercase state name → original entry
	stateIndex map[string]*stateEntry
	// stateNames is the ordered list of state names for the /states endpoint
	stateNames []string
}

// NewService loads and indexes the embedded Nigeria LGA data.
// Call once at startup and reuse the returned *Service.
func NewService() *Service {
	var entries []stateEntry
	if err := json.Unmarshal(nigeriaLGAData, &entries); err != nil {
		log.Fatalf("location service: failed to parse nigeria_lga.json: %v", err)
	}

	index := make(map[string]*stateEntry, len(entries))
	names := make([]string, 0, len(entries))

	for i := range entries {
		e := &entries[i]
		index[strings.ToLower(e.State)] = e
		names = append(names, e.State)
	}

	return &Service{
		stateIndex: index,
		stateNames: names,
	}
}

// GetStates returns all Nigerian state names in alphabetical order.
func (s *Service) GetStates() []string {
	return s.stateNames
}

// GetLGAs returns the list of LGAs for the given state name (case-insensitive).
// The second return value is false when the state is not found.
func (s *Service) GetLGAs(state string) ([]string, bool) {
	entry, ok := s.stateIndex[strings.ToLower(strings.TrimSpace(state))]
	if !ok {
		return nil, false
	}
	return entry.LGAs, true
}
