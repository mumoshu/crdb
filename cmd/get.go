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
)

type GetOptions struct {
	Selectors []string
	Watch     bool
}

var getOpts GetOptions

func init() {
	getOpts = GetOptions{
		Selectors: []string{},
	}
}

func NewCmdGet() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get RESOURCE [NAME]",
		Short: "Displays one or more resources",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			db, err := dynamodb.NewDB(globalOpts.Config, globalOpts.Namespace)
			if err != nil {
				return err
			}
			var name string
			if len(args) > 1 {
				name = args[1]
			} else {
				name = ""
			}
			err = db.GetPrint(args[0], name, getOpts.Selectors, globalOpts.Output, getOpts.Watch)
			if err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringSliceVarP(&getOpts.Selectors, "selector", "l", []string{}, "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")
	flags.BoolVarP(&getOpts.Watch, "watch", "w", false, "After listing/getting the requested object, watch for changes. Uninitialized objects are excluded if no object name is provided.")

	return cmd

}
