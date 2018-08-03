package dynamodb

import (
	"fmt"
	"github.com/elgs/jsonql"
	"github.com/mumoshu/crdb/api"
	"github.com/mumoshu/crdb/framework"
)

func (p *dynamoResourceDB) Wait(resource, name string, query string, output string) error {
	if name == "" {
		return fmt.Errorf("missing resource name")
	}
	r, err := p.wait(resource, name, query)
	if err != nil {
		return err
	}
	framework.WriteToStdout(r.Format(output))
	return nil
}

func (p *dynamoResourceDB) wait(resource, name string, query string) (*api.Resource, error) {
	resources, err := p.get(resource, name, []string{})
	if err != nil {
		return nil, err
	}

	for _, r := range resources {
		matched, err := match(r, query)
		if err != nil {
			return nil, err
		}
		if matched {
			return &r, nil
		}
	}

	rs, es := p.streamedResources(resource, name, []string{})
	for {
		select {
		case err := <-es:
			return nil, fmt.Errorf("failed streaming: %v", err)
		case res := <-rs:
			matched, e := match(*res, query)
			if err != nil {
				return nil, e
			}
			if matched {
				return res, nil
			}
		default:
		}
	}

	return nil, fmt.Errorf("stream stopped unexpectedly: please rerun the crdb command")
}

func match(resource api.Resource, query string) (bool, error) {
	r := resource.Format("json")

	stringQuery, err := jsonql.NewStringQuery(r)
	if err != nil {
		return false, err
	}

	ret, err := stringQuery.Query(query)
	if err != nil {
		return false, err
	}

	switch ret.(type) {
	case map[string]interface{}:
		return true, nil
	case nil:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected type of jsonql query result")
	}
}
