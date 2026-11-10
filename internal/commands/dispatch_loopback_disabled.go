//go:build !devspecs_http_loopback

package commands

import "github.com/spf13/cobra"

func configureLoopbackDispatch(*cobra.Command) {}

func configureLoopbackCheckpoint(*cobra.Command) {}
