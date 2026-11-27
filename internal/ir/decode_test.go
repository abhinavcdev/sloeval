package ir

import (
	"encoding/json"

	"github.com/abhinavcdev/sloeval/pkg/api/v1alpha1"
)

func decodeForTest(v any) (*v1alpha1.AISLO, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var obj v1alpha1.AISLO
	return &obj, json.Unmarshal(b, &obj)
}
