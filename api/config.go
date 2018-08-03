package api

type Config struct {
	// Kind is "Config"
	Kind     string     `json:"kind"`
	Metadata Metadata   `json:"metadata"`
	Spec     ConfigSpec `json:"spec"`
}

type ConfigSpec struct {
	// CustomResourceDefinition is an inline-version of resource definitions
	// Either specify resource definitions in this section. Otherwise point `source` to the uri of the config store like a DynamoDB table prefix
	CustomResourceDefinitions []CustomResourceDefinition `json:"customResourceDefinitions"`
	// Source is the source of the remote config that is fetched and merged into this config
	Source string `json:"source"`
}
