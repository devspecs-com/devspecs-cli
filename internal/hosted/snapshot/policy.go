package snapshot

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

const MaxPolicyBytes = 64 << 10

func DecodePolicy(raw []byte) (Policy, error) {
	var policy Policy
	if len(raw) > MaxPolicyBytes || !utf8.Valid(raw) || CheckJSON(raw) != nil {
		return policy, ErrInvalid
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || len(object) != 4 || object["schema"] == nil || object["schema_version"] == nil || object["policy_id"] == nil || object["roots"] == nil {
		return policy, ErrInvalid
	}
	var roots []map[string]json.RawMessage
	if json.Unmarshal(object["roots"], &roots) != nil {
		return policy, ErrInvalid
	}
	for _, root := range roots {
		if (len(root) != 3 && (len(root) != 4 || root["artifact_root"] == nil)) || root["label"] == nil || root["kind"] == nil || root["path"] == nil {
			return policy, ErrInvalid
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&policy) != nil {
		return policy, ErrInvalid
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}
