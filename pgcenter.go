package main

import (
	"fmt"
	"github.com/lesovsky/pgcenter/cmd"
)

func main() {
	if err := cmd.Root.Execute(); err != nil {
		fmt.Println(err)
	}
}
