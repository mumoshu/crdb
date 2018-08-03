package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGPIPE)
	go func() {
		for range c {
			fmt.Fprintln(os.Stderr, "pipe failed")
			os.Exit(2)
		}
	}()

	cmd := NewCmdRoot()
	cmd.SetOutput(os.Stdout)
	if err := cmd.Execute(); err != nil {
		//cmd.SetOutput(os.Stderr)
		//cmd.Println(err)
		os.Exit(1)
	}
}
