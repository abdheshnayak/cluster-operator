package raw_json

import (
	"encoding/json"

	"github.com/kloudlite/cluster-operator/lib/errors"
)

// +kubebuilder:pruning:PreserveUnknownFields
// +kubebuilder:validation:Schemaless
// +kubebuilder:validation:Type=object

type RawJson struct {
	items map[string]any `json:"-"`
	// RawJson[string, json.RawMessage] `json:",inline"`
	json.RawMessage `json:",inline,omitempty"`
}

// MarshalJSON returns m as the JSON encoding of m.
func (m RawJson) MarshalJSON() ([]byte, error) {
	if m.RawMessage == nil {
		return []byte("null"), nil
	}
	return m.RawMessage, nil
}

// UnmarshalJSON sets *m to a copy of data.
func (m *RawJson) UnmarshalJSON(data []byte) error {
	m = m.EnsureJson()
	if m == nil {
		return errors.New("json.RawMessage: UnmarshalJSON on nil pointer")
	}
	m.RawMessage = data
	return nil
}

func (k *RawJson) DeepCopyInto(out *RawJson) {
	k = k.EnsureJson()
	*out = *k
}

func (k *RawJson) DeepCopy() *RawJson {
	k = k.EnsureJson()
	if k == nil {
		return nil
	}
	out := new(RawJson)
	k.DeepCopyInto(out)
	return out
}

func (s *RawJson) EnsureJson() *RawJson {
	if s == nil {
		return &RawJson{}
	}

	return s
}

// old set
// type RawJson[K ~string, V any] struct {
// 	json.RawMessage `json:",inline"`
// }

// suppressing error
func (s *RawJson) fillMap() {
	if s == nil {
		*s = RawJson{}
	}
	if s.RawMessage != nil {
		s.items = map[string]any{}
		m, err := s.RawMessage.MarshalJSON()
		if err != nil {
			panic(err)
		}
		if err := json.Unmarshal(m, &s.items); err != nil {
			panic(err)
		}
	}

	if s.items == nil {
		s.items = map[string]any{}
	}
}

func (k *RawJson) Reset() {
	k = k.EnsureJson()
	k.items = nil
	k.RawMessage = nil
}

func (s *RawJson) complete() error {
	s = s.EnsureJson()
	b, err := json.Marshal(s.items)
	if err != nil {
		return err
	}
	s.RawMessage = b
	return nil
}

func (s *RawJson) Len() int {
	s = s.EnsureJson()
	s.fillMap()
	return len(s.items)
}

func (s *RawJson) Set(key string, value any) error {
	s = s.EnsureJson()
	s.fillMap()
	s.items[key] = value
	return s.complete()
}

func (s *RawJson) SetFromMap(m map[string]any) error {
	s = s.EnsureJson()
	s.fillMap()
	for k, v := range m {
		s.items[k] = v
	}
	return s.complete()
}

func (s *RawJson) Exists(keys ...string) bool {
	s = s.EnsureJson()
	s.fillMap()
	for i := range keys {
		if _, ok := s.items[keys[i]]; ok {
			return true
		}
	}
	return false
}

func (s *RawJson) Delete(key string) error {
	s = s.EnsureJson()
	s.fillMap()
	c := len(s.items)
	delete(s.items, key)
	if c != len(s.items) {
		return s.complete()
	}
	return nil
}

func (s *RawJson) Get(key string, fillInto any) error {
	s = s.EnsureJson()
	s.fillMap()
	value, ok := s.items[key]
	if !ok {
		fillInto = nil
		// return nil
		return errors.Newf("key %s does not exist", key)
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, fillInto); err != nil {
		return err
	}
	return nil
}

func (s *RawJson) GetString(key string) (string, bool) {
	s = s.EnsureJson()
	s.fillMap()
	x, ok := s.items[key]
	if !ok {
		return "", false
	}
	s2, ok := (x).(string)
	if !ok {
		return "", false
	}
	return s2, true
}
