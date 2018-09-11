package dynamodb

import (
	"strings"

	"fmt"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodbstreams"
	"github.com/guregu/dynamo"
	"github.com/mumoshu/crdb/api"
	"github.com/mumoshu/crdb/framework"
	"os"
	"os/signal"
	"time"
)

func (p *dynamoResourceDB) GetPrint(resource, name string, selectors []string, output string, watch bool) error {
	var resCh <-chan *api.Resource
	var errCh <-chan error

	resCh, errCh = p.GetAsync(resource, name, selectors, watch)

	return p.printStreamedResourcesSync(resCh, errCh, output, watch)
}

func (p *dynamoResourceDB) get(resource, name string, selectors []string) (api.Resources, error) {
	var err error
	resources := api.Resources{}
	if name == "" {
		if len(selectors) > 0 {
			expr, args := exprAndArgs(selectors)
			err = p.tableForResourceNamed(resource).Get(HashKeyName, name).Filter(expr, args...).All(&resources)
		} else {
			// Otherwise we getWatch this:
			//   Error: ValidationException: Invalid FilterExpression: The expression can not be empty;
			//   status code: 400, request id: VMUUJ9O65UABHUM12TQNFVH2SBVV4KQNSO5AEMVJF66Q9ASUAAJG
			err = p.tableForResourceNamed(resource).Get(HashKeyName, name).All(&resources)
		}
	} else {
		if len(selectors) > 0 {
			expr, args := exprAndArgs(selectors)
			err = p.tableForResourceNamed(resource).Scan().Filter(expr, args...).All(&resources)
		} else {
			err = p.tableForResourceNamed(resource).Scan().All(&resources)
		}
	}
	if err == nil && len(resources) == 0 {
		return nil, fmt.Errorf(`%s "%s" not found: dynamodb table named "%s" exists, but no item named "%s" found`, resource, name, p.tableNameForResourceNamed(resource), name)
	} else if aerr, ok := err.(awserr.Error); ok && aerr.Code() == dynamodb.ErrCodeResourceNotFoundException {
		return nil, fmt.Errorf(`%s "%s" not found: no dynamodb table named "%s" does not exist`, resource, name, p.tableNameForResourceNamed(resource))
	}
	return resources, nil
}

func (p *dynamoResourceDB) GetSync(resource, name string, selectors []string) ([]*api.Resource, error) {
	rs := []*api.Resource{}
	resources, errs := p.GetAsync(resource, name, selectors, false)
	for {
		select {
		case r := <-resources:
			rs = append(rs, r)
		case e := <-errs:
			return nil, e
		}
	}
	return rs, nil
}

func (p *dynamoResourceDB) GetAsync(resource, name string, selectors []string, watch bool) (<-chan *api.Resource, <-chan error) {
	resCh := make(chan *api.Resource, 1)
	aggErrCh := make(chan error, 1)

	resources, err := p.get(resource, name, selectors)
	if err != nil {
		aggErrCh <- err

		close(resCh)
		close(aggErrCh)
		return resCh, aggErrCh
	}

	if resources != nil {
		for _, r := range resources {
			resCh <- &r
		}
	}

	if watch {
		ch, errCh := p.streamedResources(resource, name, selectors)
		go func(ch <-chan *api.Resource) {
			defer close(resCh)
			for resource := range ch {
				resCh <- resource
			}
		}(ch)
		go func(ch <-chan error) {
			defer close(aggErrCh)
			for err := range ch {
				aggErrCh <- err
			}
		}(errCh)
	} else {
		close(resCh)
		close(aggErrCh)
	}

	return resCh, aggErrCh
}

func (p *dynamoResourceDB) printStreamedResourcesSync(ch <-chan *api.Resource, errCh <-chan error, output string, wait bool) error {
	go func(ch <-chan *api.Resource) {
		rs := []api.Resource{}
		if wait {
			for resource := range ch {
				rs = append(rs, *resource)
			}
			framework.WriteToStdout(api.Resources(rs).Format(output))
		} else {
			for resource := range ch {
				framework.WriteToStdout(resource.Format(output))
			}
		}
	}(ch)

	return waitForInterruptionOrError(errCh)
}

func (p *dynamoResourceDB) streamedResources(resource, name string, selectors []string) (<-chan *api.Resource, <-chan error) {
	resCh := make(chan *api.Resource, 1)
	aggErrCh := make(chan error, 1)

	fmt.Fprintf(os.Stderr, "starting to stream %s changes\n", resource)
	ch, errCh, err := p.streamForResourceNamed(resource)
	if err != nil {
		aggErrCh <- err
		return resCh, aggErrCh
	}
	fmt.Fprintf(os.Stderr, "started streaming %s changes\n", resource)

	go func(ch <-chan *dynamodbstreams.Record) {
		for record := range ch {
			resource := &api.Resource{}
			if err := dynamo.UnmarshalItem(record.Dynamodb.NewImage, &resource); err != nil {
				aggErrCh <- err
			}
			if name == "" || name == resource.NameHashKey {
				resCh <- resource
			}
		}
	}(ch)

	go func(errCh <-chan error) {
		for err := range errCh {
			aggErrCh <- err
		}
	}(errCh)

	return resCh, aggErrCh
}

func waitForInterruptionOrError(errCh <-chan error) error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	for {
		select {
		case <-c:
			fmt.Fprintln(os.Stderr, "interrupted")
			return nil
		case err := <-errCh:
			return fmt.Errorf("stream error: %v", err)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
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
