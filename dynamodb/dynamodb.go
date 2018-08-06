package dynamodb

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodbstreams"
	"github.com/guregu/dynamo"
	"github.com/mumoshu/crdb/api"
	"github.com/mumoshu/crdb/dynamodb/awssession"
	"github.com/mumoshu/crdb/dynamodb/stream"
	"github.com/mumoshu/crdb/framework"
	"os"
	"time"
)

const HashKeyName = "name_hash_key"

type SingleResourceDB interface {
	Get(resource, name string, selectors []string, output string, watch bool) (api.Resources, error)
	Wait(resource, name, query, output string, timeout time.Duration, logs bool) error
	Apply(file string) error
	Delete(resource, name string) error
}

type dynamoResourceDB struct {
	databaseName string
	db           *dynamo.DB
	logs         *cwlogs
	session      *session.Session
	namespace    string
	resourceDefs []api.CustomResourceDefinition
}

func (p *dynamoResourceDB) tablePrefix() string {
	return fmt.Sprintf("%s%s", databasePrefix, p.databaseName)
}

func (p *dynamoResourceDB) tableNameForResourceNamed(resource string) string {
	if resource == crdName {
		return p.globalTableName(resource)
	}
	return p.namespacedTableName(resource)
}

func (p *dynamoResourceDB) namespacedTableName(resource string) string {
	return fmt.Sprintf("%s-%s-%s", p.tablePrefix(), p.namespace, resource)
}

func (p *dynamoResourceDB) tableForResourceNamed(resourceName string) dynamo.Table {
	return p.db.Table(p.tableNameForResourceNamed(resourceName))
}

func (p *dynamoResourceDB) namespacedTable(resource *api.CustomResourceDefinition) dynamo.Table {
	return p.tableForResourceNamed(resource.Metadata.Name)
}

func (p *dynamoResourceDB) globalTableName(resource string) string {
	return globalTableName(p.databaseName, resource)
}

func globalTableName(database, resource string) string {
	return fmt.Sprintf("%s%s-%s", databasePrefix, database, resource)
}

func partitionKey(name string) string {
	// We split tables by namespaces and resources rather than partitioning,
	// so that the cost of listing all the resources within the ns is lowest,, and the isolation level is maximum.
	// Also, we aren't write-heavy so not adding random suffixes.
	// See https://aws.amazon.com/jp/blogs/database/choosing-the-right-dynamodb-partition-key/
	return name
}

func newDefaultDynamoDBClient() (*dynamo.DB, error) {
	sess, err := awssession.New(os.Getenv("AWSDEBUG") != "")
	if err != nil {
		return nil, err
	}
	return dynamo.New(sess), nil
}

func newDynamoDBClient() (*dynamo.DB, error) {
	sess, err := awssession.New(os.Getenv("AWSDEBUG") != "")
	if err != nil {
		return nil, err
	}
	return dynamo.New(sess), nil
}

func (p *dynamoResourceDB) streamSubscriberForTable(table string) (*stream.StreamSubscriber, error) {
	cfg := aws.NewConfig()
	streamSvc := dynamodbstreams.New(p.session, cfg)
	dynamoSvc := dynamodb.New(p.session, cfg)
	return stream.NewStreamSubscriber(dynamoSvc, streamSvc, table), nil
}

func (p *dynamoResourceDB) streamForTable(table string) (<-chan *dynamodbstreams.Record, <-chan error, error) {
	subscriber, err := p.streamSubscriberForTable(table)
	if err != nil {
		return nil, nil, err
	}
	ch, errch := subscriber.GetStreamDataAsync()
	return ch, errch, nil
}

func (p *dynamoResourceDB) streamForResourceNamed(resourceName string) (<-chan *dynamodbstreams.Record, <-chan error, error) {
	return p.streamForTable(p.tableNameForResourceNamed(resourceName))
}

func NewDB(configFile string, namespace string) (SingleResourceDB, error) {
	config, err := framework.LoadConfigFromYamlFile(configFile)
	if err != nil {
		return nil, err
	}
	sess, err := awssession.New(os.Getenv("AWSDEBUG") != "")
	if err != nil {
		return nil, err
	}
	db := dynamo.New(sess)

	logs, err := newLogs(config, namespace, sess)
	if err != nil {
		return nil, err
	}
	//fmt.Fprintf(os.Stderr, "%+v\n", config)
	return &dynamoResourceDB{
		databaseName: config.Metadata.Name,
		db:           db,
		logs:         logs,
		session:      sess,
		namespace:    namespace,
		resourceDefs: config.Spec.CustomResourceDefinitions,
	}, nil
}
