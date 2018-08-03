package dynamodb

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/mumoshu/crdb/api"
	"github.com/mumoshu/crdb/framework"
	"os"
	"time"
)

const databasePrefix = "crdb-"

const (
	crdName = "customresourcedefinition"
	crdKind = "CustomResourceDefinition"
)

func LoadConfigFromDynamoDB(table string, context *api.Config) (*api.Config, error) {
	db, err := newDefaultDynamoDBClient()
	if err != nil {
		return nil, err
	}
	if table != "" {
		return nil, fmt.Errorf(`not implemented error: table "%s" is specified, but it is unsupported`, table)
	}

	dynamicRDs := []api.CustomResourceDefinition{}
	for {
		err := db.Table(globalTableName(context.Metadata.Name, crdName)).Scan().All(&dynamicRDs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "err: %v\n", err.Error())
			if aerr, ok := err.(awserr.Error); ok {
				fmt.Fprintf(os.Stderr, "aerr.Code: %v\n", aerr.Code())
				switch aerr.Code() {
				case dynamodb.ErrCodeResourceNotFoundException:
				case dynamodb.ErrCodeProvisionedThroughputExceededException, dynamodb.ErrCodeLimitExceededException:
					fmt.Fprintf(os.Stderr, "retrying in 3 secounds: %v\n", err)
					time.Sleep(3 * time.Second)
					continue
				}
			} else {
				return nil, fmt.Errorf("unexpected error: %v", err)
			}
		}
		break
	}

	rdOfDynamicRDs := api.CustomResourceDefinition{
		Kind: crdKind,
		Metadata: api.Metadata{
			Name: crdName,
		},
		Spec: api.CustomResourceDefinitionSpec{
			Names: api.CustomResourceDefinitionNames{
				Kind: crdKind,
			},
		},
	}
	rds := []api.CustomResourceDefinition{
		rdOfDynamicRDs,
	}
	rds = append(rds, dynamicRDs...)
	return &api.Config{
		Spec: api.ConfigSpec{
			CustomResourceDefinitions: rds,
		},
	}, nil
}

func init() {
	framework.ConfigLoaders.Register("dynamodb", LoadConfigFromDynamoDB)
}
