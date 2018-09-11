// Copyright © 2018 Yusuke KUOKA
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"github.com/mumoshu/crdb/dynamodb"
	"github.com/spf13/cobra"
	"github.com/Azure/brigade/pkg/script"
	"os"
	"fmt"
	"github.com/mumoshu/crdb/api"
	"io"
)

type GatewayOptions struct {
	Cluster string
	Project string
}

var gatewayOpts GatewayOptions

func NewCmdGateway() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "brigade gateway that exec command according to crdb resource changes like new deployment",
		Args:  cobra.RangeArgs(0, 0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Namespace corresponds to environment, that differentiates e.g. production vs staging vs development
			c, err := kubeClient()
			if err != nil {
				return err
			}

			env := globalOpts.Namespace
			db, err := dynamodb.NewDB(globalOpts.Config, env)
			if err != nil {
				return err
			}

			logs, err := dynamodb.NewLogs(globalOpts.Config, env)
			if err != nil {
				return err
			}

			clusterName := gatewayOpts.Cluster
			projectName := gatewayOpts.Project

			allProjects, err := db.GetSync("project", projectName, []string{})
			if err != nil {
				return err
			}

			if projectName != "" {
				noProject := true
				for _, p := range allProjects {
					noProject = noProject && p.NameHashKey != projectName
				}
				if noProject {
					return fmt.Errorf("no project named \"%s\" found", projectName)
				}
			}

			allApps, err := db.GetSync("application", "", []string{})
			if err != nil {
				return err
			}
			apps := map[string]*api.Resource{}
			for _, app := range allApps {
				appName := app.NameHashKey
				if deployOpts.App != "" && appName != deployOpts.App {
					continue
				}
				appProject := app.Spec["project"].(string)
				if projectName != "" && appProject != projectName {
					continue
				}
				apps[appName] = app
			}

			deploys, deployErrs := db.GetAsync("deployment", "", []string{}, true)
			releases, releaseErrs := db.GetAsync("release", "", []string{}, true)
			installs, installErrs := db.GetAsync("installs", "", []string{}, true)
			for {
				select {
				case d := <-deploys:
					if projectName == "" || projectName != "" && d.Spec["project"] == projectName {
						releaseName := fmt.Sprintf("%s-%s", d.NameHashKey, clusterName)
						sha1 := d.Spec["sha1"]

						rs, e := db.GetSync("release", releaseName, []string{})
						if e != nil {
							panic(e)
						}
						if len(rs) == 0 || rs[0].Spec["sha1"] != sha1 {
							newRelease := &api.Resource{
								NameHashKey: releaseName,
								Metadata: api.Metadata{
									Name: releaseName,
								},
								Spec: map[string]interface{}{
									"project": d.Spec["project"],
									"app":     d.Spec["app"],
									"sha1":    sha1,
									"cluster": clusterName,
								},
							}
							err := db.Apply(newRelease)
							if err != nil {
								panic(err)
							}
						}
					}
				case r := <-releases:
					if (projectName == "" || projectName != "" && r.Spec["project"] == projectName) && r.Spec["cluster"] == clusterName {
						sha1 := r.Spec["sha1"]
						installName := fmt.Sprintf("%s-%s", r.NameHashKey, sha1)

						is, e := db.GetSync("install", installName, []string{})
						if e != nil {
							panic(e)
						}
						if len(is) == 0 {
							newRelease := &api.Resource{
								NameHashKey: installName,
								Metadata: api.Metadata{
									Name: installName,
								},
								Spec: map[string]interface{}{
									"project": r.Spec["project"],
									"app":     r.Spec["app"],
									"sha1":    sha1,
									"cluster": clusterName,
									"phase":   "pending",
								},
							}
							err := db.Apply(newRelease)
							if err != nil {
								panic(err)
							}
						}
					}
				case i := <-installs:
					if (projectName == "" || projectName != "" && i.Spec["project"] == projectName) && i.Spec["cluster"] == clusterName {
						phase := i.Spec["phase"]
						switch phase {
						case "pending":
							app := i.Spec["app"].(string)
							set := i.Spec["set"].(string)
							sha1 := i.Spec["sha1"].(string)
							// dedup deployment to deployment_status by `app` and `cluster`
							//statusKey := fmt.Sprintf("%s-%s-%s", env, cluster, app)

							label := "-l=name=" + app
							envFlag := "--environment=" + env
							setFlag := "--set=ref=" + sha1
							if set != "" {
								setFlag += "," + set
							}
							payload := []byte(fmt.Sprintf(`
{"command": ["echo", "helmfile", "--log-level=debug", "-f=helmfile.yaml", ""%s", "%s", "%s", "apply", "--auto-approve"]}
`, envFlag, label, setFlag))
							s := []byte(`
const { events, Job } = require("brigadier")

events.on("exec", (e, p) => {
  console.log({"event": e, "payload": p})

  var job = new Job("helmfile-apply", "alpine:3.4")
  job.tasks = [
    "echo Hello",
    "echo World",
    p.command.join(",")
  ]

  job.run()
})
`)
							persistentLogsWriter, err := logs.Writer("install", i.NameHashKey)
							if err != nil {
								panic(err)
							}

							mul := io.MultiWriter(persistentLogsWriter, os.Stderr)

							r, err := script.NewDelegatedRunner(c, mul, "", false, false, false)
							if err != nil {
								panic(err)
							}

							err = r.SendScript("mumoshu/uuid-generator", s, "apply", "", sha1, payload, "")
							if err != nil {
								panic(err)
							}
						case "failed":
							// TODO retry
							panic(fmt.Errorf("failed install: %s", i.NameHashKey))
						case "running":
						default:
							panic(fmt.Errorf("unexpected phase for \"%s\": %s", i.NameHashKey, phase))
						}
				}
				case e := <-installErrs:
					panic(e)
				case e := <-deployErrs:
					panic(e)
				case e := <-releaseErrs:
					panic(e)
				}
			}

			// <namespace=production>
			// + (state) myproj-app1 (application w/ id=myproj-app1 name=app1 project=myproj, app1 in project myproj)
			// | + upsert + version : deploy myproj/app1 --ref v1.0.0
			// + (state) myproj-app1 (deployment w/ id=app1 version=v1.0.0, upsert a deployment identified by id=app1)
			//    | + upsert + cluster by in-cluster server
			//    +--+ (state) myproj-app1-prod1 (release w/ id=app1-prod1 version=v1.0.0, list-watch all production deployments like app1, and release if missing)
			//    |     | created
			//    |     +--+ (history) myproj-app1-prod1-v1.0.0 (install w/ id=app1-prod1-v1.0.0, list-watch all prod1 releases like app1-prod1, and install if missing)
			//    |
			//    +--+ myproj-app1-prod2 (release w/ id=app1-prod2 version=v1.0.0)
			//          |
			//          +--+ myproj-app1-prod2-v1.0.0 (install w/ id=app1-prod2-v1.0.0, list-watch all prod2 releases like app1-prod2, and install if missing)
			//
			// + production-last-releases(environment-state)     <- updated by release
			//
			// + production-prod1-last-installs(cluster-state)   <- created by environment-last-installs, updated by installs, holds all the list of installs for prod1, used for `sync --prune`.
			return nil
		},
	}

	options := cmd.Flags()
	options.StringVar(&gatewayOpts.Cluster, "cluster", "", "Unique name of the cluster on which this gateway is running")
	options.StringVar(&gatewayOpts.Project, "project", "", "Unique name of the project which this gateway watches")
	cmd.MarkFlagRequired("cluster")

	return cmd

}
