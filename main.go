package main

import (
	"log"
	"tdrive/cmd"
)

func main() {

	var err error

	err = cmd.Execute(&cmd.VersionInfo{
		Version: Version,
	})

	if nil != err {
		log.Print(err)
	}
}
