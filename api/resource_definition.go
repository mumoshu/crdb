package api

type CustomResourceDefinition struct {
	// CustomResourceDefinition
	Kind     string                 `dynamo:"kind" json:"kind"`
	Metadata Metadata               `dynamo:"metadata" json:"metadata"`
	Spec     CustomResourceDefinitionSpec `dynamo:"spec" json:"spec"`
}

func (d CustomResourceDefinition) ResourceKind() string {
	return d.Spec.Names.Kind
}

type CustomResourceDefinitionSpec struct {
	Names CustomResourceDefinitionNames `dynamo:"names" json:"names"`
}

type CustomResourceDefinitionNames struct {
	Singular string `dynamo:"singular" json:"singular"`
	Kind     string `dynamo:"kind" json:"kind"`
}
