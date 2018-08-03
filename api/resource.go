package api

import (
	"encoding/json"
	"fmt"
	"github.com/ghodss/yaml"
	"log"
)

type Resources []Resource

type Resource struct {
	NameHashKey string `dynamo:"name_hash_key,hash" json:"-"`

	Kind     string                 `dynamo:"kind" json:"kind"`
	Metadata Metadata               `dynamo:"metadata" json:"metadata"`
	Spec     map[string]interface{} `dynamo:"spec" json:"spec"`
}

type List struct {
	Items Resources `json:"items"`
	Kind  string    `json:"kind"`
}

func (r Resources) Format(tpe string) string {
	data := List{
		Items: r,
		Kind:  "list",
	}
	switch tpe {
	case "json":
		raw, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			log.Panicf("unexpected error: %v", err)
		}
		return string(raw)
	case "yaml":
		raw, err := yaml.Marshal(data)
		if err != nil {
			log.Panicf("unepected error: %v", err)
		}
		return string(raw)
	default:
		panic(fmt.Sprintf("unexpected output format: %s", tpe))
	}
}

func (r Resource) Format(tpe string) string {
	switch tpe {
	case "json":
		raw, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			log.Panicf("unexpected error: %v", err)
		}
		return string(raw)
	case "yaml":
		raw, err := yaml.Marshal(r)
		if err != nil {
			log.Panicf("unepected error: %v", err)
		}
		return string(raw)
	default:
		panic(fmt.Sprintf("unexpected output format: %s", tpe))
	}
}
