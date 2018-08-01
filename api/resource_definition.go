package api

type ResourceDefinition struct {
	// ResourceDefinition
	Kind     string                 `dynamo:"kind" json:"kind"`
	Metadata Metadata               `dynamo:"metadata" json:"metadata"`
	Spec     ResourceDefinitionSpec `dynamo:"spec" json:"spec"`
}

func (d ResourceDefinition) ResourceKind() string {
	return d.Spec.Names.Kind
}

type ResourceDefinitionSpec struct {
	Names ResourceDefinitionNames `dynamo:"names" json:"names"`
}

type ResourceDefinitionNames struct {
	Singular string `dynamo:"singular" json:"singular"`
	Kind     string `dynamo:"kind" json:"kind"`
}
