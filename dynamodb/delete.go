package dynamodb

import (
	"fmt"
	"github.com/guregu/dynamo"
	"github.com/mumoshu/crdb/api"
)

func (p *dynamoResourceDB) tablePrefix() string {
	return fmt.Sprintf("%s%s", databasePrefix, p.databaseName)
}

func (p *dynamoResourceDB) tableNameForResourceNamed(resource string) string {
	if resource == "resourcedefinition" {
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

func (p *dynamoResourceDB) namespacedTable(resource *api.ResourceDefinition) dynamo.Table {
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

func (p *dynamoResourceDB) Delete(resource string, name string) error {
	err := p.tableForResourceNamed(resource).Delete(HashKeyName, partitionKey(name)).OldValue(&api.Resource{})
	if err != nil {
		// Small trick to make the error message a bit nicer
		//
		// BEFORE:
		//   Error: dynamo: no item found
		// AFTER:
		//   Error: cluster "foo" not found
		if err == dynamo.ErrNotFound {
			return fmt.Errorf(`%s "%s" not found`, resource, name)
		}
	} else {
		fmt.Printf("%s \"%s\" deleted\n", resource, name)
	}
	return err
}
