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

func LoadConfigFromDynamoDB(table string, context *api.Config) (*api.Config, error) {
	db, err := newDynamoDBClient()
	if err != nil {
		return nil, err
	}
	if table != "" {
		return nil, fmt.Errorf(`not implemented error: table "%s" is specified, but it is unsupported`, table)
	}

	dynamicRDs := []api.ResourceDefinition{}
	for {
		err := db.Table(globalTableName(context.Metadata.Name, "resourcedefinition")).Scan().All(&dynamicRDs)
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

	rdOfDynamicRDs := api.ResourceDefinition{
		Kind: "ResourceDefinition",
		Metadata: api.Metadata{
			Name: "resourcedefinition",
		},
		Spec: api.ResourceDefinitionSpec{
			Names: api.ResourceDefinitionNames{
				Kind: "ResourceDefinition",
			},
		},
	}
	rds := []api.ResourceDefinition{
		rdOfDynamicRDs,
	}
	rds = append(rds, dynamicRDs...)
	return &api.Config{
		Spec: api.ConfigSpec{
			ResourceDefinitions: rds,
		},
	}, nil
}

func init() {
	framework.ConfigLoaders.Register("dynamodb", LoadConfigFromDynamoDB)
}
