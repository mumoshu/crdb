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

func NewCmdDel() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete single source by specified name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			db, err := dynamodb.NewDB(globalOpts.Config, globalOpts.Namespace)
			if err != nil {
				return err
			}
			return db.Delete(args[0], args[1])
		},
	}
	return cmd
}
