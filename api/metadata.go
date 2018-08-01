package api

import "time"

type Metadata struct {
	Name              string            `dynamo:"name" json:"name"`
	Namespace         string            `dynamo:"namespace" json:"namespace"`
	Labels            map[string]string `dynamo:"labels" json:"labels,omitempty"`
	CreationTimestamp time.Time         `dynamo:"creationTimestamp" json:"creationTimestamp"`
	UpdateTimestamp   time.Time         `dynamo:"updateTimestamp" json:"updateTimestamp"`
}
