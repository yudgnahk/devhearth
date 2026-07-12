package detect

import "fmt"

type Registry struct {
	detectors map[string]Detector
}

func NewRegistry() *Registry { return &Registry{detectors: make(map[string]Detector)} }

func (r *Registry) Register(detector Detector) error {
	descriptor := detector.Descriptor()
	if descriptor.ID == "" || descriptor.Version < 1 {
		return fmt.Errorf("detector id and positive version are required")
	}
	if _, exists := r.detectors[descriptor.ID]; exists {
		return fmt.Errorf("detector %q is already registered", descriptor.ID)
	}
	r.detectors[descriptor.ID] = detector
	return nil
}

func (r *Registry) All() []Detector {
	result := make([]Detector, 0, len(r.detectors))
	for _, detector := range r.detectors {
		result = append(result, detector)
	}
	return result
}
