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
	"fmt"
	"github.com/mumoshu/crdb/api"
)

type DeployOptions struct {
	Ref string
	App string
	Watch     bool
}

var deployOpts DeployOptions

func init() {
	deployOpts = DeployOptions{}
}

func NewCmdDeploy() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [PROJECT]",
		Short: "Deploys application(s) defined in PROJECT",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			db, err := dynamodb.NewDB(globalOpts.Config, globalOpts.Namespace)
			if err != nil {
				return err
			}

			logs, err := dynamodb.NewLogs(globalOpts.Config, globalOpts.Namespace)
			if err != nil {
				return err
			}

			var projectName string
			if len(args) > 1 {
				projectName = args[1]
			} else {
				projectName = ""
			}

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

			for _, app := range apps {
				//sha1, err := fetchSha1ForRef(deployOpts.Ref)
				//if err != nil {
				//	return err
				//}
				sha1 := deployOpts.Ref

				// e.g. "myproj_myapp1"
				deployId := app.NameHashKey
				deploys, err := db.GetSync("deployment", deployId, []string{})
				if err != nil {
					return err
				}
				if len(deploys) == 0 {
					newDeploy := &api.Resource{
						NameHashKey: deployId,
						Metadata: api.Metadata{
							Name: deployId,
						},
						Spec: map[string]interface{}{
							"project": projectName,
							"app": deployId,
							"sha1": sha1,
						},
					}
					err := db.Apply(newDeploy)
					if err != nil {
						return err
					}
					continue
				}
				deploy := deploys[0]

				if deploy.Spec["sha1"].(string) == sha1 {
					// already deployed
					break
				}

				deploy.Spec["sha1"] = sha1
				err = db.Apply(deploy)
				if err != nil {
					return err
				}
			}

			clusters, err := db.GetSync("cluster", "", []string{})
			if err != nil {
				return err
			}

			// wait for completion
			for _, app := range apps {
				for _, cluster := range clusters {
					// TODO record start time
					for {
						// TODO timeout
						deployId := app.NameHashKey
						deploys, err := db.GetSync("deployment", deployId, []string{})
						if err != nil {
							return err
						}
						if len(deploys) == 0 {
							continue
						}
						deploy := deploys[0]

						//sha1, err := fetchSha1ForRef(deployOpts.Ref)
						//if err != nil {
						//	return err
						//}
						sha1 := deploy.Spec["sha1"]

						installId := fmt.Sprintf("%s-%s-%s", app.NameHashKey, cluster.NameHashKey, sha1)
						installs, err := db.GetSync("install", installId, []string{})
						if err != nil {
							return err
						}
						if len(installs) == 0 {
							continue
						}
						// stream logs until end
						install := installs[0]

						err = logs.Read("install", install.NameHashKey, 0, true)
						if err != nil {
							return err
						}
						break
					}
				}
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&deployOpts.App, "app", "", "Name of the app to be deployed. Defaults to all")
	flags.StringVarP(&deployOpts.Ref, "ref", "", "master", "Commit SHA1 or branch name to be deployed. Defaults to master")
	flags.BoolVar(&deployOpts.Watch, "wait", false, "After listing/getting the requested object, watch for changes. Uninitialized objects are excluded if no object name is provided.")

	return cmd

}
