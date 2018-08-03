package dynamodb

import (
	"strings"

	"fmt"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodbstreams"
	"github.com/guregu/dynamo"
	"github.com/mumoshu/crdb/api"
	"os"
	"os/signal"
	"github.com/mumoshu/crdb/framework"
)

func (p *dynamoResourceDB) Get(resource, name string, selectors []string, output string, watch bool) (api.Resources, error) {
	var err error
	var resources api.Resources
	if name != "" {
		resources, err = p.get(resource, name, selectors, output, watch)
	} else {
		resources, err = p.query(resource, selectors, output, watch)
	}
	return resources, err
}

func (p *dynamoResourceDB) get(resource, name string, selectors []string, output string, watch bool) (api.Resources, error) {
	var err error
	resources := api.Resources{}
	if len(selectors) > 0 {
		expr, args := exprAndArgs(selectors)
		err = p.tableForResourceNamed(resource).Get(HashKeyName, name).Filter(expr, args...).All(&resources)
	} else {
		// Otherwise we get this:
		//   Error: ValidationException: Invalid FilterExpression: The expression can not be empty;
		//   status code: 400, request id: VMUUJ9O65UABHUM12TQNFVH2SBVV4KQNSO5AEMVJF66Q9ASUAAJG
		err = p.tableForResourceNamed(resource).Get(HashKeyName, name).All(&resources)
	}
	if err == nil && len(resources) == 0 {
		return nil, fmt.Errorf(`%s "%s" not found: dynamodb table named "%s" exists, but no item named "%s" found`, resource, name, p.tableNameForResourceNamed(resource), name)
	} else if aerr, ok := err.(awserr.Error); ok && aerr.Code() == dynamodb.ErrCodeResourceNotFoundException {
		return nil, fmt.Errorf(`%s "%s" not found: no dynamodb table named "%s" does not exist`, resource, name, p.tableNameForResourceNamed(resource))
	}

	if watch {
		for _, r := range resources {
			framework.WriteToStdout(r.Format(output))
		}
		err := p.streamSync(resource, selectors, output)
		if err != nil {
			return nil, err
		}
	} else {
		framework.WriteToStdout(resources.Format(output))
	}

	return resources, err
}

func (p *dynamoResourceDB) streamSync(resource string, selectors []string, output string) error {
	fmt.Fprintf(os.Stderr, "starting to stream %s changes\n", resource)
	ch, errCh, err := p.streamForResourceNamed(resource)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "started streaming %s changes\n", resource)

	go func(ch <-chan *dynamodbstreams.Record) {
		for record := range ch {
			resource := &api.Resource{}
			if err := dynamo.UnmarshalItem(record.Dynamodb.NewImage, &resource); err != nil {
				panic(err)
			}
			framework.WriteToStdout(resource.Format(output))
		}
	}(ch)

	go func(errCh <-chan error) {
		for err := range errCh {
			fmt.Fprintf(os.Stderr, "stream error: %v\n", err)
		}
	}(errCh)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	for range c {
		fmt.Fprintln(os.Stderr, "interrupted")
		return nil
	}
	return nil
}

func (p *dynamoResourceDB) query(resource string, selectors []string, output string, watch bool) (api.Resources, error) {
	var err error
	resources := api.Resources{}
	if len(selectors) > 0 {
		expr, args := exprAndArgs(selectors)
		err = p.tableForResourceNamed(resource).Scan().Filter(expr, args...).All(&resources)
	} else {
		err = p.tableForResourceNamed(resource).Scan().All(&resources)
	}
	if err == nil && len(resources) == 0 {
		return nil, fmt.Errorf(`no %s found: dynamodb table named "%s" exists, but no item found in it`, resource, p.tableNameForResourceNamed(resource))
	} else if aerr, ok := err.(awserr.Error); ok && aerr.Code() == dynamodb.ErrCodeResourceNotFoundException {
		return nil, fmt.Errorf(`no %s found: dynamodb table named "%s" does not exist`, resource, p.tableNameForResourceNamed(resource))
	}

	if watch {
		for _, r := range resources {
			framework.WriteToStdout(r.Format(output))
		}
		err := p.streamSync(resource, selectors, output)
		if err != nil {
			return nil, err
		}
	} else {
		framework.WriteToStdout(resources.Format(output))
	}

	return resources, err
}

func exprAndArgs(selectors []string) (string, []interface{}) {
	conds := []string{}
	args := []interface{}{}
	for _, selector := range selectors {
		kv := strings.Split(selector, "=")
		var op string
		k := kv[0]
		v := kv[1]
		if strings.HasSuffix(k, "!") {
			k = k[:len(k)-1]
			op = "!="
		} else {
			op = "="
		}
		args = append(args, k, v)
		conds = append(conds, fmt.Sprintf("'metadata'.'labels'.$ %s ?", op))
	}
	expr := strings.Join(conds, "AND")
	return expr, args
}
