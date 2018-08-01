package dynamodb

import (
	"github.com/guregu/dynamo"
	"github.com/mumoshu/crdb/api"
	"github.com/mumoshu/crdb/dynamodb/awssession"
	"github.com/mumoshu/crdb/framework"
	"os"
)

const HashKeyName = "name_hash_key"

type SingleResourceDB interface {
	Get(resource, name string, selectors []string) (api.Resources, error)
	Apply(file string) error
	Delete(resource, name string) error
}

type dynamoResourceDB struct {
	databaseName string
	db           *dynamo.DB
	namespace    string
	resourceDefs []api.ResourceDefinition
}

func newDynamoDBClient() (*dynamo.DB, error) {
	sess, err := awssession.New(os.Getenv("AWSDEBUG") != "")
	if err != nil {
		return nil, err
	}
	return dynamo.New(sess), nil
}

func NewDB(configFile string, namespace string) (SingleResourceDB, error) {
	config, err := framework.LoadConfigFromYamlFile(configFile)
	if err != nil {
		return nil, err
	}
	db, err := newDynamoDBClient()
	if err != nil {
		return nil, err
	}
	//fmt.Fprintf(os.Stderr, "%+v\n", config)
	return &dynamoResourceDB{
		databaseName: config.Metadata.Name,
		db:           db,
		namespace:    namespace,
		resourceDefs: config.Spec.ResourceDefinitions,
	}, nil
}
