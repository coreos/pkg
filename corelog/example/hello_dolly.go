package main

import (
	"github.com/coreos/pkg/corelog"
	"os"
)

var log = corelog.NewPackageLogger("github.com/coreos/pkg/corelog/cmd", "main")
var dlog = corelog.NewPackageLogger("github.com/coreos/pkg/corelog/cmd", "dolly")

func main() {
	rl := corelog.MustRepoLogger("github.com/coreos/pkg/corelog/cmd")
	rl.SetLogLevel(corelog.INFO)
	corelog.SetOutput(os.Stderr)
	corelog.SetFormatter(&corelog.GlogFormatter{})

	if len(os.Args) > 1 {
		rl.ConfigLogLevel(os.Args[1])
		log.Infoln("Setting output to", os.Args[1])
	}

	dlog.Infoln("Hello Dolly")
	dlog.Warningln("Well hello, Dolly")
	log.Errorln("It's so nice to have you back where you belong")
	dlog.Debugln("You're looking swell, Dolly")
	dlog.Verboseln("I can tell, Dolly")
	log.Panicln("You're still glowin', you're still crowin', you're still lookin' strong")
}
