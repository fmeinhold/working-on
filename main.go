package main

import (
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/cmd"
)

func main() {
	err := cfg.InitGlobalConfig()
	if err != nil {
		panic(err)
	}
	cmd.Execute()
}
