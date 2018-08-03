crdb
=======

A Custom Resources DataBase that provides an equivalent to Kubernetes custom resources, without Kubernetes.

As of today, it supports `DynamoDB` as a backing datastore.
That is, you can use `crdb` as a kubectl-like interface to manage your custom resources persisted in DynamoDB tables.

## Use cases

### Centralize shared configuration of your CI pipelines

`crdb` is intended to compliment both Pipeline-based CI systems and your workflows, so that you can use
`crdb` as a source-of-truth across all your automations.

### Kubernetes Cluster Discovery

For example, you may use `crdb` to discover your clusters from the CI/CD pipeline, without maintaining
a list of known clusters within your application repositories.

### Implementing Event Hub With Minimum Moving Parts

In near future, `crdb wait` allows you to build an event hub for your system.

For example, a `wait until human approval` workflow that is useful in your CI/CD pipeline can be implemented simply like:

The requester would run `myjob1`:

```console
echo waiting for approval of job: myjob1

crdb wait approval myjob1approval --until jsonql="status.phase = 'Approved'"

echo myjob1 has been approved. continuing the process...
```

Whereas the approver approves the job:

```console
$ crdb get myjob1approval -o json > myjob1approval.unapproved.json
$ ./json-set "status.phase=Approved" myjob1approval.unapproved.json > myjob1approval.approved.json 
$ crdb apply -f myjob1approval.approved.json 
``` 

## Installation

```
$ go get github.com/mumoshu/crdb
```

## Getting started

1. Provide a proper AWS credentials to `crdb` via envvars(`AWS_PROFILE` is supported, too) or an instance profile.

2. Create `crdb.yaml`:

```yaml
metadata:
  name: static
spec:
  - kind: CustomResourceDefinition
    metadata:
      name: cluster
    spec:
      names:
        kind: Cluster
```

Now, you are ready to CRUD your resources by running `crdb`.

```
$ crdb get cluster
Error: no cluster found: dynamodb table named "crdb-static-default-cluster" does not exist

$ crdb apply -f example/foo.cluster.yaml
cluster "foo" created

$ ./crdb get cluster
{
  "items": [
    {
      "kind": "Cluster",
      "metadata": {
        "name": "foo",
        "namespace": "default",
        "creationTimestamp": "2018-08-01T17:03:11.591439013+09:00",
        "updateTimestamp": "2018-08-01T17:03:11.591442026+09:00"
      },
      "spec": {
        "kubeconfig": "mykubeconfigcontent\n"
      }
    }
  ],
  "kind": "list"
}

$ crdb apply -f example/foo.cluster.2.yaml
cluster "foo" updated

{
  "items": [
    {
      "kind": "Cluster",
      "metadata": {
        "name": "foo",
        "namespace": "default",
        "creationTimestamp": "2018-08-01T17:03:11.591439013+09:00",
        "updateTimestamp": "2018-08-01T17:03:58.741607915+09:00"
      },
      "spec": {
        "kubeconfig": "mykubeconfigcontent-updated\n"
      }
    }
  ],
  "kind": "list"
}

$ crdb delete cluster foo
cluster "foo" deleted

$ crdb delete cluster foo
Error: cluster "foo" not found

$ crdb get cluster foo
Error: cluster "foo" not found: dynamodb table named "crdb-test1-default-cluster" exists, but no item named "foo" found
```

## Usage

### Get

```
Displays one or more resources

Examples:
  # list all myresources
  crdb get myresources 

  # list a single myresource with specified name 
  crdb get myresource foo

  # list a myresource identified by name in JSON output format
  crdb get myresource foo -o json

  # list myresources whose labels match the specified selector
  crdb get myresources -l foo=bar 
```

### Apply

```
Apply a configuration to a resource by filename. The resource name must be specified in the file.
This resource will be created if it doesn't exist yet.

Examples:
  crdb apply [-f|--file] <FilePath>
```

### Delete

```
Delete resources by resources and names.

Examples:
  # Delete a myresource with specified name
  crdb delete myresource foo
```

## Configuration

Provide `crdb` your resource definitions via either `static` or `dynamic`(recommended) config.

### Static configuration

In static configuration, you provide one or more resource definitions in `crdb.yaml`:

```yaml
metadata:
  name: static
spec:
  customResourceDefinitions:
  - kind: CustomResourceDefinition
    metadata:
      name: cluster
    spec:
      names:
        kind: Cluster
```

Now you can CRUD the `cluster` resources by running `crdb` commands:

More concretely:

- `crdb apply -f yourcluster.yaml` to create a `cluster` resource. See `example/foo.cluster.yaml` for details on the yaml file.
- `crdb [get|delete] cluster foo` to get or delete a `cluster` named `foo`, respectively.

### Dynamic configuration(recommended)

In `dynamic` configuration, you just tell `crdb` to read resource definitions from the specified source.

An example `crdb.yaml` that loads resource definitions from a DynamoDB table named `crdb-dynamic-customresourcedefinitions` would look like:

```yaml
metadata:
  name: dynamic
spec:
  source: "dynamodb://"
```

Next, create your resource definition on DynamoDB:

```console
$ crdb apply -f example/cluster.crd.yaml
```

Assuming the `cluster.crd.yaml` looked like:

```yaml
kind: CustomResourceDefinition
metadata:
  name: cluster
spec:
  names:
    kind: Cluster
```

You can CRUD the `cluster` resources by running `crdb` commands:

More concretely:

- `crdb apply -f yourcluster.yaml` to create a `cluster` resource. See `example/foo.cluster.yaml` for details on the yaml file.
- `crdb [get|delete] cluster foo` to get or delete a `cluster` named `foo`, respectively.

## Roadmap

### List-Watch

- `crdb get myresource --watch` to stream resource changes, including creations, updates, and deletions.

### Wait for condition

- `crdb wait myresource foo --until jsonql="status.phase = 'Done'"` to wait until `myresource` named `foo` matches the [jsonql](https://github.com/elgs/jsonql) expression.

### Resource Logs

- `crdb writelogs myresource foo -f logs.txt` to write logs. logs can be streamed from another clients.
- `crdb readlogs myresource foo -f` to stream logs associated to the resource

### Usability Improvements

- `crdb wait myresource foo --logs --until jsonql="status.phase = 'Done'"` to wait until `myresource` named `foo` matches the [jsonql](https://github.com/elgs/jsonql) expression, while streaming all the logs associated to the resource until the end
- `crdb gen iampolicy [readonly|writeonly|readwrite] myresource` to generate cloud IAM policy like AWS IAM policy to ease setting up least privilege for your developers
- `crdb template -f myresoruce.yaml.tpl --pipe-to "crdb apply -f -"` to consume ndjson input to apply execute the specified command with the input generated from the template

## Contributing

Contributions to add another datastores from various clouds and OSSes are more than welcome!
See the `api` for the API which your additional datastore should support, whereas the `framework` pkg is to help implementing it.
Also, the `dynamodb` pkg is the example implementation of the `api`. You can even start by copying the whole `dynamodb` pkg into your own datastore pkg. 

Please feel free to ask @mumoshu if you had technical difficulty to do so. I'm here to help!

## Acknowledgement

This project is highly inspired by a DynamoDB CLI named [watarukura/gody](https://github.com/watarukura/gody).
A lot of thanks to @watarukura for sharing the awesome project!
